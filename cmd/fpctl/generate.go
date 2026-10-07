package main

import (
	"github.com/faits-politiques/faits-politiques/internal/bulletin"
	"github.com/faits-politiques/faits-politiques/internal/dossiersgen"
	"github.com/spf13/cobra"
)

// commandGenerate : fpctl generate dossiers, fpctl generate bulletin. Les
// deux régénèrent, entre des marqueurs, une section précise d'un document
// docs/*.md à partir de la base — jamais le document en entier, jamais une
// valeur recopiée à la main.
func commandGenerate() *cobra.Command {
	cmd := &cobra.Command{Use: "generate", Short: "Régénère une section chiffrée d'un document"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "dossiers",
			Short: "Régénère contexte/enjeux/cadre/contrôles/situation de chaque dossier",
			Long: "Écrit, dans chaque docs/<dossier>.md, les sections tirées de\n" +
				"ref.fait_dossier — le texte entre deux marqueurs est régénéré à\n" +
				"chaque exécution, le reste du document n'est jamais touché.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if isHelpRequested(args) {
					return showManual("fpctl-generate")
				}
				return runInternal(cmd.Context(), dossiersgen.Run(cmd.Context(), args))
			},
		},
		&cobra.Command{
			Use:   "bulletin",
			Short: "Régénère la figure « D'une fiche de paie aux caisses »",
			Long: "Réécrit, dans docs/cotisations-et-droits.md, la figure tirée des\n" +
				"vues derived.bulletin_* — même convention de marqueurs.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if isHelpRequested(args) {
					return showManual("fpctl-generate")
				}
				return runInternal(cmd.Context(), bulletin.Run(cmd.Context(), args))
			},
		},
	)
	return cmd
}
