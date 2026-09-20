package main

import "github.com/spf13/cobra"

// commandeBuild : fpctl build site. Reste un binaire séparé (compilé et
// exécuté par execBinaire), pas un import direct comme les autres verbes :
// cmd/build fait 16 000 lignes activement modifiées par d'autres sessions en
// parallèle de celle-ci — l'importer forcerait fpctl à recompiler l'un dans
// l'autre, couplage qu'aucune des deux commandes ne demande, pour un paquet
// dont la seule interface utile est déjà sa ligne de commande.
func commandeBuild() *cobra.Command {
	cmd := &cobra.Command{Use: "build", Short: "Construit une ressource du site"}
	cmd.AddCommand(&cobra.Command{
		Use:   "site [options]",
		Short: "Génère le site statique et le met en place",
		Long: "Construit le site dans <out>.construction/ puis le met en place d'un\n" +
			"coup (le domaine réel n'est jamais interrompu). Recopie depuis la\n" +
			"construction précédente les sections dont ni les données ni le code\n" +
			"n'ont changé — voir « fpctl help build » pour le détail des options.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-build")
			}
			return execBinaire(cmd.Context(), "fpbuild", "cmd/build", args)
		},
	})
	return cmd
}
