package main

import "github.com/spf13/cobra"

// commandeSources enveloppe cmd/inventaire : le catalogue des sources
// ingérées (raw.source, raw.fetch_run...), jamais une valeur recopiée à la
// main — voir la documentation de cmd/inventaire.
func commandeSources() *cobra.Command {
	return &cobra.Command{
		Use:   "sources [options]",
		Short: "Catalogue des sources de données ingérées",
		Long: "Écrit le catalogue JSON des sources (raw.source), avec leur dernière\n" +
			"exécution d'ingestion (raw.fetch_run) — licence, éditeur, cadence,\n" +
			"date de la dernière collecte réussie. Par défaut dans\n" +
			"docs/catalogue-sources.json (voir -out).",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-sources")
			}
			return execBinaire("fpinventaire", "cmd/inventaire", args)
		},
	}
}
