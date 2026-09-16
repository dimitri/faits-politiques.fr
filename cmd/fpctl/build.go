package main

import "github.com/spf13/cobra"

// commandeBuild enveloppe cmd/build tel quel : mêmes options (-out,
// -templates, -data, -root, -max-scrutins, -only, -cpuprofile), même
// comportement (construction dans <out>.construction/ puis mise en place
// atomique) — fpctl.go ne réimplémente rien, il compile et lance le même
// binaire qu'un « go run ./cmd/build ». Voir cmd/fpctl/man/fpctl-build.md.
func commandeBuild() *cobra.Command {
	return &cobra.Command{
		Use:   "build [options]",
		Short: "Génère le site statique et le met en place",
		Long: "Construit le site dans <out>.construction/ puis le met en place d'un\n" +
			"coup (le domaine réel n'est jamais interrompu). Recopie depuis la\n" +
			"construction précédente les sections dont ni les données ni le code\n" +
			"n'ont changé — voir « fpctl help build » pour le détail des options.",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-build")
			}
			return execBinaire("fpbuild", "cmd/build", args)
		},
	}
}
