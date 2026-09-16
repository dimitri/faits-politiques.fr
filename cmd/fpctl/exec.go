package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// estDemandeAide : le premier argument brut d'une commande à options
// transmises telles quelles (DisableFlagParsing) demande-t-il de l'aide ?
// Interceptée ici plutôt que laissée filer vers le binaire enveloppé, pour
// qu'elle ouvre la page de manuel dédiée — plus complète que l'usage
// généré par flag — exactement comme « git commit --help » ouvre
// git-commit(1) plutôt que d'imprimer un résumé d'options.
func estDemandeAide(args []string) bool {
	return len(args) > 0 && (args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help")
}

// racineDepot trouve la racine du dépôt (là où vit go.mod) sans supposer un
// répertoire de travail précis : fpctl doit se comporter comme git, qu'on le
// lance depuis la racine ou depuis n'importe quel sous-répertoire.
func racineDepot() (string, error) {
	sortie, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD : %w", err)
	}
	gomod := strings.TrimSpace(string(sortie))
	if gomod == "" || gomod == os.DevNull {
		return "", fmt.Errorf("fpctl doit être lancé depuis le dépôt faits-politiques.fr (aucun go.mod trouvé)")
	}
	return filepath.Dir(gomod), nil
}

// execBinaire compile pkg (un chemin cmd/... du même module) puis l'exécute
// avec args, terminaux et code de sortie transmis tels quels.
//
// Toujours recompiler, jamais un test de fraîcheur maison (mtime du binaire
// contre celui des sources) : le cache de compilation de Go rend une
// recompilation à l'identique quasi instantanée, alors qu'un test de
// fraîcheur qui oublierait un paquet interne modifié (internal/store, par
// exemple, pas seulement cmd/build) exécuterait un binaire périmé sans le
// dire — le genre d'erreur qu'aucune vitesse gagnée ne vaut.
//
// Le binaire compilé va dans bin/ (déjà ignoré par git) et cmd.Dir est fixé
// à la racine du dépôt : les chemins par défaut des commandes enveloppées
// (web/templates, docs, data...) restent relatifs à la racine, pas au
// répertoire depuis lequel fpctl a été lancé.
func execBinaire(nom, pkg string, args []string) error {
	racine, err := racineDepot()
	if err != nil {
		return err
	}
	chemin := filepath.Join(racine, "bin", nom)

	compiler := exec.Command("go", "build", "-o", chemin, "./"+pkg)
	compiler.Dir = racine
	compiler.Stdout, compiler.Stderr = os.Stderr, os.Stderr
	if err := compiler.Run(); err != nil {
		return fmt.Errorf("compilation de %s : %w", pkg, err)
	}

	cmd := exec.Command(chemin, args...)
	cmd.Dir = racine
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}
