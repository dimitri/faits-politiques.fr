package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/spf13/cobra"
)

// commandeIngest : fpctl ingest all | <catégorie> [all|<source>]. L'arbre de
// sous-commandes est construit en ITÉRANT internal/ingest.Categories() et
// SourcesDeCategorie plutôt que recopié à la main — le catalogue
// (internal/ingest/catalogue.go) reste la référence unique des noms, comme
// l'ancien -only l'était, mais nommé et rangé par thème au lieu d'un flag
// plat à deviner.
//
// -dry-run et -j ne s'appliquent qu'au socle parlementaire audité (voir
// internal/ingest.socleParlementaire) : le reste du catalogue, non encore
// vérifié pour des dépendances implicites, continue de s'exécuter en
// séquence — demander -dry-run ou -j>1 en dehors du socle échoue plutôt
// que de faire silencieusement comme si de rien n'était.
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
			"ingest ». « fpctl ingest deps » affiche le graphe de dépendances du\n" +
			"socle parlementaire, tel que publié en base.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&rawDir, "raw", "raw", "répertoire de l'archive scellée")
	cmd.PersistentFlags().StringVar(&migDir, "migrations", "db/migrations", "répertoire des migrations")
	cmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false,
		"affiche l'ordre d'exécution par vagues sans rien exécuter (socle parlementaire seulement)")
	cmd.PersistentFlags().IntVar(&concurrence, "j", 1,
		"étapes indépendantes exécutées de front, par vague (socle parlementaire seulement)")
	opts := func() pipeline.Options { return pipeline.Options{DryRun: dryRun, Concurrence: concurrence} }

	cmd.AddCommand(&cobra.Command{
		Use:   "all",
		Short: "Recharge la chaîne complète (le socle habituel, pas tout le catalogue)",
		Long: "Recharge le socle que fpctl ingest a toujours rechargé sans -only —\n" +
			"pas littéralement chaque source du catalogue : plusieurs sont\n" +
			"délibérément hors chaîne par défaut (coûteuses, ponctuelles, ou\n" +
			"exigeant une clé ou un binaire particulier). Pour recharger une\n" +
			"catégorie entière, y compris ce qu'elle a de plus coûteux, voir\n" +
			"« fpctl ingest <catégorie> all ». Ne prend pas -dry-run/-j : c'est\n" +
			"la chaîne historique, hors du socle audité par internal/pipeline —\n" +
			"voir « fpctl ingest parlement all » pour ça.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executerInterne(cmd.Context(), ingest.RunTout(cmd.Context(), rawDir, migDir))
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

	cmd.AddCommand(&cobra.Command{
		Use:   "deps",
		Short: "Graphe de dépendances du socle parlementaire, tel que publié en base",
		Long: "Le socle parlementaire (download, partis, normalize, carto, senat,\n" +
			"europe, themes) est la seule partie du catalogue dont les\n" +
			"dépendances sont déclarées et vérifiées — voir internal/pipeline.\n" +
			"Republié à chaque exécution touchant ce socle (fpctl ingest\n" +
			"parlement ... ou l'une de ces sept sources) ; vide avant la\n" +
			"première.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executerInterne(cmd.Context(), afficherDeps(cmd.Context()))
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

func afficherDeps(ctx context.Context) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	etapes, err := pipeline.LireTopologie(ctx, pool)
	if err != nil {
		return err
	}
	if len(etapes) == 0 {
		fmt.Println("rien à afficher — lancez « fpctl ingest parlement all » (ou l'une de ses sources) au moins une fois")
		return nil
	}
	for _, e := range etapes {
		derniere := "jamais exécutée avec succès"
		if e.DerniereExecutionReussie != nil {
			derniere = e.DerniereExecutionReussie.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("%s — %s\n", e.Nom, derniere)
		if e.Description != "" {
			fmt.Printf("  %s\n", e.Description)
		}
		if len(e.DependDe) > 0 {
			fmt.Printf("  dépend de : %s\n", strings.Join(e.DependDe, ", "))
		}
	}
	return nil
}
