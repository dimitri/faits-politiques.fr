package main

import (
	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/spf13/cobra"
)

// commandeIngest : fpctl ingest data. L'option -only reste la référence
// unique pour la liste des sources (une soixantaine) : la dupliquer ici en
// sous-commandes cobra créerait deux endroits à tenir synchronisés, l'un
// d'eux finirait par mentir.
func commandeIngest() *cobra.Command {
	cmd := &cobra.Command{Use: "ingest", Short: "Charge une ressource dans la base"}
	cmd.AddCommand(&cobra.Command{
		Use:   "data [options]",
		Short: "Télécharge, archive et charge les jeux de données sources",
		Long: "Sans -only, recharge tout, dans l'ordre attendu par les dépendances\n" +
			"entre sources. Avec -only=<source> (migrate, checksums, ou l'un des\n" +
			"connecteurs), ne recharge que celle-là — voir « fpctl help ingest ».",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-ingest")
			}
			return executerInterne(cmd.Context(), ingest.Run(cmd.Context(), args))
		},
	})
	return cmd
}
