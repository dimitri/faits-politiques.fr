package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/logs"
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

// executerInterne traduit l'erreur d'un paquet interne (déjà importé, déjà
// routé par une commande fpctl) en code de sortie du process. Toujours
// affichée : certains paquets détaillent déjà l'échec sur stderr avant de
// remonter une erreur volontairement sobre (verify.ErrAnomalies, par
// exemple), d'autres remontent l'erreur brute — dans les deux cas, ne rien
// afficher ici laisserait le second cas totalement muet.
//
// ctx est celui que la commande a reçu de cobra (cmd.Context()) : un
// contexte déjà annulé quand err remonte veut dire que l'interruption
// vient d'un signal, déjà annoncé par internal/logs.Context — le code de
// sortie conventionnel (130) le dit à son tour, plutôt que de remonter en
// échec générique une exécution qui s'est arrêtée proprement parce qu'on
// le lui a demandé.
func executerInterne(ctx context.Context, err error) error {
	if err != nil {
		if ctx.Err() != nil {
			os.Exit(logs.ExitCode)
		}
		slog.Error(err.Error())
		os.Exit(1)
	}
	return nil
}
