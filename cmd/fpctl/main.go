// Commande fpctl : point d'entrée unique de la chaîne de construction de
// faits-politiques.fr.
//
// N'exécute aucune logique propre au-delà du routage : chaque sous-commande
// compile puis lance le binaire cmd/... correspondant, options transmises
// telles quelles. Un seul exécutable, un seul arbre d'aide (fpctl help),
// sans réécrire une seule ligne des commandes existantes.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	racine := &cobra.Command{
		Use:   "fpctl",
		Short: "Chaîne de construction de faits-politiques.fr",
		Long: "fpctl assemble en une seule commande l'ingestion, la vérification et\n" +
			"la construction du site — chacune reste le même binaire qu'avant,\n" +
			"compilé à la volée et lancé avec les mêmes options.\n\n" +
			"« fpctl help » affiche le manuel complet ; « fpctl help <commande> »\n" +
			"affiche celui d'une commande précise.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return afficherManuel("fpctl")
		},
	}
	racine.CompletionOptions.DisableDefaultCmd = true

	racine.AddCommand(
		commandeBuild(),
		commandeIngest(),
		commandeVerify(),
		commandeSources(),
		commandeDocs(),
	)
	// Remplace l'aide générée par cobra (une liste d'options) par la vraie
	// page de manuel : « fpctl help » et « fpctl help <commande> » doivent se
	// comporter comme « git help », pas comme --help.
	racine.SetHelpCommand(commandeHelp())

	if err := racine.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "fpctl : %v\n", err)
		os.Exit(1)
	}
}
