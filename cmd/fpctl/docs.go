package main

import "github.com/spf13/cobra"

// commandeDocs regroupe les deux générateurs qui régénèrent, entre des
// marqueurs, une section précise d'un document docs/*.md à partir de la
// base — jamais le document en entier, jamais une valeur recopiée à la main.
func commandeDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Régénère les sections chiffrées des dossiers documentaires",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "sections-dossiers",
			Short: "Régénère contexte/enjeux/cadre/contrôles/situation de chaque dossier",
			Long: "Écrit, dans chaque docs/<dossier>.md, les sections tirées de\n" +
				"ref.fait_dossier — le texte entre deux marqueurs est régénéré à\n" +
				"chaque exécution, le reste du document n'est jamais touché.",
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-docs")
				}
				return execBinaire("fpsections-dossiers", "cmd/sections-dossiers", args)
			},
		},
		&cobra.Command{
			Use:   "figure-bulletin",
			Short: "Régénère la figure « D'une fiche de paie aux caisses »",
			Long: "Réécrit, dans docs/cotisations-et-droits.md, la figure tirée des\n" +
				"vues derived.bulletin_* — même convention de marqueurs.",
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-docs")
				}
				return execBinaire("fpfigure-bulletin", "cmd/figure-bulletin", args)
			},
		},
	)
	return cmd
}
