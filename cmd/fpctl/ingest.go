package main

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/watermark"
	"github.com/spf13/cobra"
)

// commandeIngest : fpctl ingest default | complet | <catégorie> [all|<source>].
// L'arbre de sous-commandes est construit en ITÉRANT internal/ingest.
// Categories() et SourcesDeCategorie plutôt que recopié à la main — le
// catalogue (internal/ingest/catalogue.go) reste la référence unique des
// noms, comme l'ancien -only l'était, mais nommé et rangé par thème au lieu
// d'un flag plat à deviner.
//
// Deux PORTÉES possibles, nommées pareil des deux côtés de fpctl (voir
// « fpctl verify data », qui reprend exactement ces deux noms) :
//   - « default » (internal/ingest.ChaineParDefaut) : le socle parlementaire
//     plus runAllSupplement — ce que ce dépôt a toujours rechargé sans
//     -only, jamais littéralement tout le catalogue. C'était l'ancien
//     « fpctl ingest all », renommé pour ne plus dire « tout » quand il ne
//     recharge qu'une partie.
//   - « full » : littéralement tout le catalogue (internal/ingest.
//     TousLesNoms) — l'équivalent de « fpctl ingest <catégorie> all » rejoué
//     pour les neuf catégories à la fois.
//
// « fpctl ingest <catégorie> all » garde son sens propre, inchangé : toutes
// les sources d'UNE catégorie, un troisième usage de « all » sans rapport
// avec les deux portées ci-dessus.
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
	var dryRun, force bool
	var concurrence int
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Charge une ressource dans la base",
		Long: "Sans sous-commande, liste les catégories de sources. « fpctl ingest\n" +
			"default » recharge la chaîne complète (le socle habituel) ;\n" +
			"« fpctl ingest full », littéralement tout le catalogue. Pour une\n" +
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
	cmd.PersistentFlags().BoolVar(&force, "force", false,
		"ignore tous les watermarks (internal/watermark, internal/an/watermark.go) : "+
			"reconstruit comme si rien n'avait jamais tourné")
	opts := func() pipeline.Options { return pipeline.Options{DryRun: dryRun, Concurrency: concurrence} }
	// ctxForce applique --force à cmd.Context(), jamais l'inverse : un appel
	// qui l'oublierait garderait le comportement normal (les watermarks
	// jouent), plutôt que de forcer par défaut sans qu'on l'ait demandé.
	ctxForce := func(cmd *cobra.Command) context.Context {
		if force {
			return watermark.WithForce(cmd.Context())
		}
		return cmd.Context()
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "default",
		Short: "Recharge la chaîne par défaut (le socle habituel, pas tout le catalogue)",
		Long: "Recharge le socle que fpctl ingest a toujours rechargé sans -only —\n" +
			"pas littéralement chaque source du catalogue : plusieurs sont\n" +
			"délibérément hors chaîne par défaut (coûteuses, ponctuelles, ou\n" +
			"exigeant une clé ou un binaire particulier) — voir « fpctl ingest\n" +
			"full » pour les recharger aussi. Pour recharger une catégorie\n" +
			"entière, y compris ce qu'elle a de plus coûteux, voir « fpctl\n" +
			"ingest <catégorie> all ». Passe par le même graphe de dépendances\n" +
			"que les autres commandes (internal/ingest.registreComplet) :\n" +
			"-dry-run et -j s'y appliquent aussi.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := ctxForce(cmd)
			return executerInterne(ctx, ingest.RunAll(ctx, rawDir, migDir, opts()))
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "full",
		Short: "Recharge littéralement tout le catalogue (coûteux, ponctuel)",
		Long: "Littéralement tout le catalogue (les ~120 sources, pas seulement\n" +
			"les ~23 de « fpctl ingest default ») — l'ÉQUIVALENT de rejouer\n" +
			"« fpctl ingest <catégorie> all » pour chacune des neuf catégories,\n" +
			"en une seule commande qui passe par le même graphe de dépendances\n" +
			"(internal/ingest.registreDe) plutôt que neuf appels séparés.\n" +
			"C'est le pendant exact de « fpctl verify data full » : la seule\n" +
			"façon pour ce contrôle élargi de ne plus rien trouver à redire,\n" +
			"puisqu'il porte alors sur des données réellement chargées.\n" +
			"Ponctuel et coûteux par construction (plusieurs sources de\n" +
			"plusieurs centaines de Mo chacune) — jamais la commande qu'un\n" +
			"ingest nocturne ou qu'une CI légère doit rejouer.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := ctxForce(cmd)
			return executerInterne(ctx, ingest.RunSources(ctx, rawDir, migDir, ingest.TousLesNoms(), opts()))
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
		cmd.AddCommand(commandeIngestCategorie(categorie, &rawDir, &migDir, opts, ctxForce))
	}

	return cmd
}

func commandeIngestCategorie(categorie string, rawDir, migDir *string, opts func() pipeline.Options,
	ctxForce func(cmd *cobra.Command) context.Context) *cobra.Command {
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
			ctx := ctxForce(cmd)
			return executerInterne(ctx, ingest.RunCategorie(ctx, *rawDir, *migDir, categorie, opts()))
		},
	})

	for _, s := range sources {
		s := s
		catCmd.AddCommand(&cobra.Command{
			Use:   s.Nom,
			Short: s.Description,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				ctx := ctxForce(cmd)
				return executerInterne(ctx, ingest.RunSource(ctx, *rawDir, *migDir, s.Nom, opts()))
			},
		})
	}

	return catCmd
}
