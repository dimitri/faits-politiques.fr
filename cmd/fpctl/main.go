// Commande fpctl : point d'entrée unique de la chaîne de construction de
// faits-politiques.fr.
//
// L'arbre de commandes suit un seul principe : fpctl <verbe> [<nom>], comme
// git — jamais un nom seul (git n'a pas de commande « branch-name »), jamais
// un verbe qui déguise un nom (« ingest » n'est pas « ingest-data », c'est
// ingest appliqué à data). La plupart des paquets qu'il route sont importés
// directement (build, verify, list, generate) : leur logique vit dans
// internal/, fpctl n'en est que la façade. Seul « build site » reste à part,
// compilé et exécuté comme un binaire séparé plutôt qu'importé — voir le
// commentaire de commandeBuild pour pourquoi.
package main

import (
	"log/slog"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/spf13/cobra"
)

func main() {
	logs.Setup()

	// Un seul contexte pour toute l'exécution, annulé par le premier
	// SIGINT, SIGTERM ou SIGHUP — repris par cmd.Context() dans chaque
	// commande, jusqu'aux pools pgx et aux binaires exécutés (fpbuild,
	// docker), plutôt qu'un context.Background() propre à chacun qui
	// laisserait un Ctrl-C sans effet sur l'ingestion ou la construction
	// en cours. Voir internal/logs.Context pour ce qu'un signal doit
	// nettoyer ici, et pourquoi la réponse est « presque rien ».
	ctx, stop := logs.Context()
	defer stop()

	racine, err := racineDepot()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	// Comme git : les commandes se comportent pareil qu'on les lance depuis
	// la racine du dépôt ou depuis un sous-répertoire, parce que fpctl s'y
	// place lui-même avant de faire quoi que ce soit — les chemins par défaut
	// des paquets routés (web/templates, docs, data, raw...) restent alors
	// relatifs à la racine, jamais au répertoire d'appel.
	if err := os.Chdir(racine); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	racineCmd := &cobra.Command{
		Use:   "fpctl",
		Short: "Chaîne de construction de faits-politiques.fr",
		Long: "fpctl assemble en une seule commande l'ingestion, la vérification et\n" +
			"la construction du site. Chaque verbe s'applique à un nom :\n" +
			"fpctl <verbe> <nom> [options], comme « git <verbe> <nom> ».\n\n" +
			"« fpctl help » affiche le manuel complet ; « fpctl help <verbe> »\n" +
			"affiche celui d'un verbe précis.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return afficherManuel("fpctl")
		},
	}
	racineCmd.CompletionOptions.DisableDefaultCmd = true

	racineCmd.AddCommand(
		commandeBuild(),
		commandeIngest(),
		commandeVerify(),
		commandeList(),
		commandeGenerate(),
		commandeProvision(),
		commandeSync(),
		commandeDump(),
	)
	// Remplace l'aide générée par cobra (une liste d'options) par la vraie
	// page de manuel : « fpctl help » et « fpctl help <verbe> » doivent se
	// comporter comme « git help », pas comme --help.
	racineCmd.SetHelpCommand(commandeHelp())

	if err := racineCmd.ExecuteContext(ctx); err != nil {
		// Un contexte déjà annulé (Ctrl-C pendant l'exécution) a sa
		// propre convention de sortie — l'erreur qu'il a fait remonter
		// est déjà celle du signal, pas la peine de la répéter.
		if ctx.Err() != nil {
			os.Exit(logs.ExitCode)
		}
		slog.Error(err.Error())
		os.Exit(1)
	}
}
