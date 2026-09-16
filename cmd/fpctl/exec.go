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
// Interceptée ici plutôt que laissée filer, pour qu'elle ouvre la page de
// manuel dédiée — plus complète que l'usage généré par flag — exactement
// comme « git commit --help » ouvre git-commit(1) plutôt que d'imprimer un
// résumé d'options.
func estDemandeAide(args []string) bool {
	return len(args) > 0 && (args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help")
}

// racineDepot trouve la racine du dépôt (là où vit go.mod), appelée une
// seule fois par main() avant tout le reste — fpctl doit se comporter comme
// git, qu'on le lance depuis la racine ou depuis n'importe quel
// sous-répertoire.
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
// avec args, terminaux et code de sortie transmis tels quels. Le répertoire
// de travail est déjà la racine du dépôt (main a fait le chdir), donc "./"+pkg
// et bin/nom y résolvent correctement quel que soit le répertoire d'où fpctl
// a été appelé.
//
// Toujours recompiler, jamais un test de fraîcheur maison (mtime du binaire
// contre celui des sources) : le cache de compilation de Go rend une
// recompilation à l'identique quasi instantanée, alors qu'un test de
// fraîcheur qui oublierait un paquet interne modifié exécuterait un binaire
// périmé sans le dire — le genre d'erreur qu'aucune vitesse gagnée ne vaut.
func execBinaire(nom, pkg string, args []string) error {
	chemin := filepath.Join("bin", nom)

	compiler := exec.Command("go", "build", "-o", chemin, "./"+pkg)
	compiler.Stdout, compiler.Stderr = os.Stderr, os.Stderr
	if err := compiler.Run(); err != nil {
		return fmt.Errorf("compilation de %s : %w", pkg, err)
	}

	cmd := exec.Command(chemin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// executerInterne traduit l'erreur d'un paquet interne (déjà importé, déjà
// routé par une commande fpctl) en code de sortie du process. Toujours
// affichée : certains paquets détaillent déjà l'échec sur stderr avant de
// remonter une erreur volontairement sobre (verify.ErrAnomalies, par
// exemple), d'autres remontent l'erreur brute — dans les deux cas, ne rien
// afficher ici laisserait le second cas totalement muet.
func executerInterne(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fpctl : %v\n", err)
		os.Exit(1)
	}
	return nil
}
