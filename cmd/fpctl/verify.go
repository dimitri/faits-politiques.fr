package main

import "github.com/spf13/cobra"

// commandeVerify enveloppe cmd/verify : les contrôles de cohérence des
// données chargées, à rejouer avant toute publication.
func commandeVerify() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Contrôles de cohérence des données chargées",
		Long: "Compte les anomalies entre les données chargées et les relevés\n" +
			"officiels — une porte de publication qui passe à zéro, jamais un\n" +
			"contrôle de schéma (voir db/tests/ pour ceux-là).",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-verify")
			}
			return execBinaire("fpverify", "cmd/verify", args)
		},
	}
}
