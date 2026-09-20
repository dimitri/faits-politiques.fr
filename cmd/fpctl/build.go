package main

import (
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/spf13/cobra"
)

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
// incomplet. scrutin/communes/reste ingèrent d'abord ce qu'elles déclarent
// nécessiter (ingestPrealables, idempotent) puis passent -only=<section> au
// binaire — même mécanisme qu'avant, mais nommé plutôt que deviné derrière
// un flag — et ne mettent JAMAIS en place : réservées à l'itération locale.
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
	for _, section := range buildSections {
		section := section
		cmd.AddCommand(&cobra.Command{
			Use:   section.nom + " [options]",
			Short: section.description + " — jamais mis en place, réservé à l'itération locale",
			Long: section.description + ", en ingérant d'abord ce qu'elle déclare\n" +
				"nécessiter (idempotent — relancer ne refait pas ce qui est déjà à\n" +
				"jour), sans reconstruire le reste du site. Le site produit est\n" +
				"DÉLIBÉRÉMENT INCOMPLET : jamais mis en place automatiquement, jamais\n" +
				"ce que doit servir le domaine réel — voir « fpctl help build » pour\n" +
				"le détail des options (-out, -max-scrutins...). -dry-run affiche les\n" +
				"préalables sans rien ingérer ni construire.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-build")
				}
				return construireSection(cmd, section.nom, args)
			},
		})
	}
	return cmd
}

// buildSections : les pages du site que fpbuild sait reconstruire seules
// (-only=<nom>) — partagé avec « fpctl list deps », qui les affiche comme
// autant de racines dépendant de ce qu'ingestPrealables déclare pour
// chacune (voir fpctl-list(1)).
var buildSections = []struct{ nom, description string }{
	{"scrutin", "reconstruit seulement les pages de scrutin"},
	{"communes", "reconstruit seulement les pages communes/EPCI"},
	{"reste", "reconstruit tout SAUF scrutin et communes (accueil, dossiers, thèmes, gouvernement, budget...)"},
}

// ingestPrealables : ce qu'une section du site a besoin de trouver déjà
// ingéré avant que la construire ait un sens. Tenue à la main, comme
// internal/checksum.Sections dont elle prolonge l'esprit — sous-couvrir
// cette liste ne fait jamais servir une page fausse (le cache par
// empreinte de cmd/build s'en assure déjà), au pire une construction sans
// les toutes dernières données. « normalize » résout et ingère lui-même ses
// propres préalables (download, partis — voir internal/ingest et
// internal/pipeline) : le déclarer ici suffit, la cascade est automatique.
var ingestPrealables = map[string][]string{
	"communes": {"normalize", "communes", "associations"},
	"scrutin":  {"normalize", "exposes"},
	"reste":    {"normalize"},
}

func construireSection(cmd *cobra.Command, nom string, args []string) error {
	prealables := ingestPrealables[nom]
	reste := args

	dryRun := len(reste) > 0 && (reste[0] == "-dry-run" || reste[0] == "--dry-run")
	if dryRun {
		fmt.Printf("simulation (rien n'est ingéré ni construit) :\n")
		fmt.Printf("  préalables d'ingestion : %s\n", strings.Join(prealables, ", "))
		fmt.Printf("  puis : fpctl build site -only=%s\n", nom)
		return nil
	}

	for _, source := range prealables {
		if err := ingest.RunSource(cmd.Context(), "raw", "db/migrations", source); err != nil {
			return fmt.Errorf("préalable %s : %w", source, err)
		}
	}
	return execBinaire(cmd.Context(), "fpbuild", "cmd/build", append([]string{"-only=" + nom}, reste...))
}
