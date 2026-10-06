package main

import (
	"github.com/faits-politiques/faits-politiques/internal/verify"
	"github.com/spf13/cobra"
)

// commandeVerify : fpctl verify data. Les contrôles de cohérence des
// données chargées, à rejouer avant toute publication.
func commandeVerify() *cobra.Command {
	cmd := &cobra.Command{Use: "verify", Short: "Contrôle la cohérence d'une ressource"}
	cmd.AddCommand(&cobra.Command{
		Use:   "data",
		Short: "Contrôles de cohérence des données chargées",
		Long: "Compte les anomalies entre les données chargées et les relevés\n" +
			"officiels — une porte de publication qui passe à zéro, jamais un\n" +
			"contrôle de schéma (voir db/tests/ pour ceux-là).\n\n" +
			"Sans option : seuls les contrôles dont la source est chargée par\n" +
			"« fpctl ingest default » (le socle parlementaire et runAllSupplement).\n" +
			"« fpctl verify data full » : tous les contrôles connus, y compris ceux\n" +
			"qui portent sur une source hors chaîne par défaut (empire colonial,\n" +
			"SIRENE, ports...) — à réserver à un suivi d'un ingest plus large\n" +
			"(fpctl ingest <catégorie> all, catégorie par catégorie).",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-verify")
			}
			return executerInterne(cmd.Context(), verify.Run(cmd.Context(), args))
		},
	})
	return cmd
}
