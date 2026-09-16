package main

import (
	"github.com/faits-politiques/faits-politiques/internal/sources"
	"github.com/spf13/cobra"
)

// commandeList : fpctl list sources. Un seul nom pour l'instant ; le verbe
// « list » est le bon endroit pour une future fpctl list connectors ou
// fpctl list stats le jour où ce code existera — jamais avant.
func commandeList() *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Liste une collection"}
	cmd.AddCommand(&cobra.Command{
		Use:   "sources [options]",
		Short: "Catalogue des sources de données ingérées",
		Long: "Écrit le catalogue JSON des sources (raw.source), avec leur dernière\n" +
			"exécution d'ingestion (raw.fetch_run) — licence, éditeur, cadence,\n" +
			"date de la dernière collecte réussie. Par défaut dans\n" +
			"docs/catalogue-sources.json (voir -out).",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-list")
			}
			return executerInterne(sources.Run(args))
		},
	})
	return cmd
}
