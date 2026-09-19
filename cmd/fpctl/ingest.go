package main

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/spf13/cobra"
)

// commandeIngest : fpctl ingest all | <catégorie> [all|<source>]. L'arbre de
// sous-commandes est construit en ITÉRANT internal/ingest.Categories() et
// SourcesDeCategorie plutôt que recopié à la main — le catalogue
// (internal/ingest/catalogue.go) reste la référence unique des noms, comme
// l'ancien -only l'était, mais nommé et rangé par thème au lieu d'un flag
// plat à deviner.
func commandeIngest() *cobra.Command {
	var rawDir, migDir string
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Charge une ressource dans la base",
		Long: "Sans sous-commande, liste les catégories de sources. « fpctl ingest\n" +
			"all » recharge la chaîne complète (le socle habituel). Pour une\n" +
			"catégorie : « fpctl ingest <catégorie> » liste ses sources,\n" +
			"« fpctl ingest <catégorie> all » les recharge toutes (y compris ce\n" +
			"qu'elle a de plus coûteux, hors chaîne par défaut), « fpctl ingest\n" +
			"<catégorie> <source> » ne recharge que celle-là — voir « fpctl help\n" +
			"ingest ».",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&rawDir, "raw", "raw", "répertoire de l'archive scellée")
	cmd.PersistentFlags().StringVar(&migDir, "migrations", "db/migrations", "répertoire des migrations")

	cmd.AddCommand(&cobra.Command{
		Use:   "all",
		Short: "Recharge la chaîne complète (le socle habituel, pas tout le catalogue)",
		Long: "Recharge le socle que fpctl ingest a toujours rechargé sans -only —\n" +
			"pas littéralement chaque source du catalogue : plusieurs sont\n" +
			"délibérément hors chaîne par défaut (coûteuses, ponctuelles, ou\n" +
			"exigeant une clé ou un binaire particulier). Pour recharger une\n" +
			"catégorie entière, y compris ce qu'elle a de plus coûteux, voir\n" +
			"« fpctl ingest <catégorie> all ».",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return executerInterne(ingest.RunTout(context.Background(), rawDir, migDir))
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Applique les migrations de schéma en attente",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return executerInterne(ingest.RunSource(context.Background(), rawDir, migDir, "migrate"))
		},
	})

	for _, categorie := range ingest.Categories() {
		cmd.AddCommand(commandeIngestCategorie(categorie, &rawDir, &migDir))
	}

	return cmd
}

func commandeIngestCategorie(categorie string, rawDir, migDir *string) *cobra.Command {
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return executerInterne(ingest.RunCategorie(context.Background(), *rawDir, *migDir, categorie))
		},
	})

	for _, s := range sources {
		s := s
		catCmd.AddCommand(&cobra.Command{
			Use:   s.Nom,
			Short: s.Description,
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return executerInterne(ingest.RunSource(context.Background(), *rawDir, *migDir, s.Nom))
			},
		})
	}

	return catCmd
}
