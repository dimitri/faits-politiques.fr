package main

import (
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/spf13/cobra"
)

// commandeIngest : fpctl ingest all | <catégorie> [all|<source>]. L'arbre de
// sous-commandes est construit en ITÉRANT internal/ingest.Categories() et
// SourcesDeCategorie plutôt que recopié à la main — le catalogue
// (internal/ingest/catalogue.go) reste la référence unique des noms, comme
// l'ancien -only l'était, mais nommé et rangé par thème au lieu d'un flag
// plat à deviner.
//
// -dry-run et -j s'appliquent à tout le catalogue : le socle parlementaire
// audité (voir internal/ingest.socleParlementaire) passe par son registre
// publié, le reste d'une catégorie par un registre générique construit à
// la volée (internal/ingest.registreDe) — la quasi-totalité de ces sources
// n'ont aucune dépendance déclarée entre elles, donc partagent une seule
// vague et tournent de front jusqu'à -j, là où elles s'exécutaient
// jusqu'ici une par une dans l'ordre du catalogue.
func commandeIngest() *cobra.Command {
	var rawDir, migDir string
	var dryRun bool
	var concurrence int
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Charge une ressource dans la base",
		Long: "Sans sous-commande, liste les catégories de sources. « fpctl ingest\n" +
			"all » recharge la chaîne complète (le socle habituel). Pour une\n" +
			"catégorie : « fpctl ingest <catégorie> » liste ses sources,\n" +
			"« fpctl ingest <catégorie> all » les recharge toutes (y compris ce\n" +
			"qu'elle a de plus coûteux, hors chaîne par défaut), « fpctl ingest\n" +
			"<catégorie> <source> » ne recharge que celle-là — voir « fpctl help\n" +
			"ingest ». « fpctl list deps » affiche le graphe de dépendances du\n" +
			"socle parlementaire, tel que publié en base.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&rawDir, "raw", "raw", "répertoire de l'archive scellée")
	cmd.PersistentFlags().StringVar(&migDir, "migrations", "db/migrations", "répertoire des migrations")
	cmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false,
		"affiche l'ordre d'exécution par vagues sans rien exécuter")
	cmd.PersistentFlags().IntVarP(&concurrence, "concurrence", "j", 1,
		"étapes indépendantes exécutées de front, par vague")
	opts := func() pipeline.Options { return pipeline.Options{DryRun: dryRun, Concurrence: concurrence} }

	cmd.AddCommand(&cobra.Command{
		Use:   "all",
		Short: "Recharge la chaîne complète (le socle habituel, pas tout le catalogue)",
		Long: "Recharge le socle que fpctl ingest a toujours rechargé sans -only —\n" +
			"pas littéralement chaque source du catalogue : plusieurs sont\n" +
			"délibérément hors chaîne par défaut (coûteuses, ponctuelles, ou\n" +
			"exigeant une clé ou un binaire particulier). Pour recharger une\n" +
			"catégorie entière, y compris ce qu'elle a de plus coûteux, voir\n" +
			"« fpctl ingest <catégorie> all ». Passe par le même graphe de\n" +
			"dépendances que les autres commandes (internal/ingest.\n" +
			"registreComplet) : -dry-run et -j s'y appliquent aussi.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executerInterne(cmd.Context(), ingest.RunTout(cmd.Context(), rawDir, migDir, opts()))
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Applique les migrations de schéma en attente",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executerInterne(cmd.Context(), ingest.RunSource(cmd.Context(), rawDir, migDir, "migrate"))
		},
	})

	for _, categorie := range ingest.Categories() {
		cmd.AddCommand(commandeIngestCategorie(categorie, &rawDir, &migDir, opts))
	}

	return cmd
}

func commandeIngestCategorie(categorie string, rawDir, migDir *string, opts func() pipeline.Options) *cobra.Command {
	sources := ingest.SourcesDeCategorie(categorie)

	catCmd := &cobra.Command{
		Use:   categorie,
		Short: fmt.Sprintf("Sources de la catégorie %s (%d)", categorie, len(sources)),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	catCmd.AddCommand(&cobra.Command{
		Use:   "all",
		Short: fmt.Sprintf("Recharge toutes les sources de %s", categorie),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executerInterne(cmd.Context(), ingest.RunCategorie(cmd.Context(), *rawDir, *migDir, categorie, opts()))
		},
	})

	for _, s := range sources {
		s := s
		catCmd.AddCommand(&cobra.Command{
			Use:   s.Nom,
			Short: s.Description,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return executerInterne(cmd.Context(), ingest.RunSource(cmd.Context(), *rawDir, *migDir, s.Nom, opts()))
			},
		})
	}

	return catCmd
}
