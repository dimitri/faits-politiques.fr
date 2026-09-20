package main

import "github.com/spf13/cobra"

// commandeBuild : fpctl build site | scrutin | communes | reste. Reste un
// binaire séparé (compilé et exécuté par execBinaire), pas un import direct
// comme les autres verbes : cmd/build fait 16 000 lignes activement
// modifiées par d'autres sessions en parallèle de celle-ci — l'importer
// forcerait fpctl à recompiler l'un dans l'autre, couplage qu'aucune des
// deux commandes ne demande, pour un paquet dont la seule interface utile
// est déjà sa ligne de commande.
//
// site est la seule construction complète, jamais filtrée : c'est ce que
// mettreEnPlace (cmd/build) doit voir pour ne jamais publier un site
// incomplet. scrutin/communes/reste passent -only=<section> au binaire —
// même mécanisme qu'avant, mais nommé plutôt que deviné derrière un flag —
// et ne mettent JAMAIS en place : réservées à l'itération locale.
func commandeBuild() *cobra.Command {
	cmd := &cobra.Command{Use: "build", Short: "Construit le site, ou une section limitée pour itérer localement"}
	cmd.AddCommand(&cobra.Command{
		Use:   "site [options]",
		Short: "Génère le site complet et le met en place",
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
	for _, section := range []struct{ nom, seul, description string }{
		{"scrutin", "scrutin", "reconstruit seulement les pages de scrutin"},
		{"communes", "communes", "reconstruit seulement les pages communes/EPCI"},
		{"reste", "reste", "reconstruit tout SAUF scrutin et communes (accueil, dossiers, thèmes, gouvernement, budget...)"},
	} {
		section := section
		cmd.AddCommand(&cobra.Command{
			Use:   section.nom + " [options]",
			Short: section.description + " — jamais mis en place, réservé à l'itération locale",
			Long: section.description + ", sans reconstruire le reste du site (plus\n" +
				"rapide pour itérer sur cette seule section). Le site produit est\n" +
				"DÉLIBÉRÉMENT INCOMPLET : jamais mis en place automatiquement, jamais\n" +
				"ce que doit servir le domaine réel — voir « fpctl help build » pour\n" +
				"le détail des options (-out, -max-scrutins...).",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-build")
				}
				return execBinaire(cmd.Context(), "fpbuild", "cmd/build", append([]string{"-only=" + section.seul}, args...))
			},
		})
	}
	return cmd
}
