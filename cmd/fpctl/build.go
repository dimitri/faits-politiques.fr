package main

import (
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/spf13/cobra"
)

// commandeBuild : fpctl build site | scrutin | communes | <groupe> | page
// <nom>. Reste un binaire séparé (compilé et exécuté par execBinaire), pas
// un import direct comme les autres verbes : cmd/build fait 16 000 lignes
// activement modifiées par d'autres sessions en parallèle de celle-ci —
// l'importer forcerait fpctl à recompiler l'un dans l'autre, couplage
// qu'aucune des deux commandes ne demande, pour un paquet dont la seule
// interface utile est déjà sa ligne de commande.
//
// site est la seule construction complète, jamais filtrée : c'est ce que
// mettreEnPlace (cmd/build) doit voir pour ne jamais publier un site
// incomplet. Chaque groupe et chaque page ingèrent d'abord ce qu'ils
// déclarent nécessiter (ingestPrealables, idempotent — via
// internal/ingest.RunSources, qui les ingère TOUS ENSEMBLE plutôt qu'un
// par un : la plupart des sources n'ont aucune dépendance déclarée entre
// elles, donc tournent de front) puis passent -only=<nom> au binaire — et
// ne mettent JAMAIS en place : réservés à l'itération locale.
//
// « fpctl build reste » n'existe plus : ce fourre-tout couvrait environ
// cinquante pages indépendantes, toujours reconstruites ensemble, cache
// invalidé dès qu'UNE SEULE ingestion avait tourné n'importe où (voir
// l'ancien resteInchange, cmd/build/cache.go). cmd/build/main.go donne
// maintenant son propre nom -only à chacune de ces pages (voir la
// fonction ecrire(), qui remplace write() partout sauf scrutin/communes
// et les quelques pages toujours écrites — 404, sitemap...) : les groupes
// ci-dessous en couvrent les plus utiles pour l'itération locale, « page
// <nom> » atteint n'importe laquelle des autres (un sujet en particulier,
// par exemple — voir cmd/build/sujets.go pour les identifiants).
func commandeBuild() *cobra.Command {
	var concurrence int
	cmd := &cobra.Command{Use: "build", Short: "Construit le site, ou une section limitée pour itérer localement"}
	cmd.PersistentFlags().IntVarP(&concurrence, "concurrence", "j", 4,
		"préalables d'ingestion indépendants exécutés de front")
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
			Short: section.description,
			Long: section.description + ", en ingérant d'abord ce qu'elle déclare\n" +
				"nécessiter (idempotent — relancer ne refait pas ce qui est déjà à\n" +
				"jour), sans reconstruire le reste du site — voir « fpctl help\n" +
				"build » pour le détail des options (-out, -max-scrutins...).\n" +
				"-dry-run affiche le plan d'ingestion (vagues, concurrence) sans\n" +
				"rien ingérer ni construire.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if estDemandeAide(args) {
					return afficherManuel("fpctl-build")
				}
				return construireSection(cmd, section.nom, section.only, args, concurrence)
			},
		})
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "page <nom> [options]",
		Short: "Reconstruit une seule page ou un seul sujet, par son nom -only",
		Long: "Comme les autres sections (voir SECTIONS), mais pour n'importe quel\n" +
			"nom -only que cmd/build/main.go connaît et qu'aucun groupe ci-dessus\n" +
			"ne couvre déjà — une page d'indicateur (dette, chomage, eau...) ou\n" +
			"un sujet de campagne par son identifiant (fraude-fiscale,\n" +
			"appareil-productif... voir cmd/build/sujets.go). Sans préalable\n" +
			"déclaré pour ce nom précis (voir ingestPrealables), ingère le socle\n" +
			"parlementaire complet par défaut : presque toutes les pages du site\n" +
			"en dépendent au moins pour les fiches de personnes qu'elles citent.",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if estDemandeAide(args) || len(args) == 0 {
				return afficherManuel("fpctl-build")
			}
			return construireSection(cmd, args[0], args[0], args[1:], concurrence)
		},
	})
	return cmd
}

// buildSections : les groupes de pages que fpbuild sait reconstruire
// ensemble (-only=<liste>), les plus utiles à nommer pour l'itération
// locale — partagé avec « fpctl list deps », qui les affiche comme autant
// de racines dépendant de ce qu'ingestPrealables déclare pour chacun (voir
// fpctl-list(1)). Une page qui n'appartient à aucun groupe reste
// atteignable par son propre nom via « fpctl build page <nom> ».
var buildSections = []struct{ nom, description, only string }{
	{"scrutin", "reconstruit seulement les pages de scrutin", "scrutin"},
	{"communes", "reconstruit seulement les pages communes/EPCI", "communes"},
	{"identite", "candidats, partis, Assemblée, sources, Europe, thèmes, Sénat",
		"candidats,partis,assemblee,sources,europe,themes,senat"},
	{"indicateurs", "frise, dette, chômage, vieillesse, jeunesse, richesse, dividendes, sécurité, agriculture, protection sociale",
		"frise,dette,chomage,vieillesse,jeunesse,richesse,dividendes,securite,agriculture,social"},
	{"gouvernance", "qui décide, présidentielle 2027, gouvernement, collectivités, circonscriptions, argent public",
		"qui-decide,election2027,gouvernement,collectivites,circonscriptions,argent-public"},
	{"fiches", "fiches personnes, candidats, organisations, référentiels, groupes parlementaires", "fiches"},
	{"dossiers", "accueil, index des sujets de campagne, documents de méthode (pas chaque sujet — voir « fpctl build page »)",
		"accueil,sujets,comprendre"},
}

// ingestPrealables : ce qu'un groupe ou une page a besoin de trouver déjà
// ingéré avant que la construire ait un sens. Tenue à la main, comme
// internal/checksum.Sections dont elle prolonge l'esprit — sous-couvrir
// cette liste ne fait jamais servir une page fausse (le cache par
// empreinte de cmd/build s'en assure déjà), au pire une construction sans
// les toutes dernières données. Nommer une seule source suffit pour toute
// sa chaîne : internal/ingest.RunSources résout les dépendances déclarées
// de proche en proche (« themes » entraîne senat, europe, normalize,
// download et partis) et ingère de front tout ce qui peut l'être. Pas
// d'entrée pour « page <nom> » : voir son défaut dans construireSection.
var ingestPrealables = map[string][]string{
	"communes":    {"normalize", "communes", "associations"},
	"scrutin":     {"normalize", "exposes"},
	"identite":    {"themes", "carto"},
	"indicateurs": {"normalize"},
	"gouvernance": {"themes", "carto"},
	"fiches":      {"themes", "carto"},
	"dossiers":    {"themes", "carto"},
}

// construireSection ingère les préalables de nom (déclarés dans
// ingestPrealables, ou le socle complet par défaut) puis construit
// -only=only — nom identifie la section pour l'utilisateur et pour
// ingestPrealables ; only est ce que cmd/build/main.go reconnaît vraiment
// (un nom seul, ou la liste que couvre un groupe de buildSections).
func construireSection(cmd *cobra.Command, nom, only string, args []string, concurrence int) error {
	prealables, ok := ingestPrealables[nom]
	if !ok {
		// Défaut pour « page <nom> » sans préalable déclaré : le socle
		// parlementaire complet, dont presque toute page cite au moins une
		// fiche de personne ou d'organisation.
		prealables = []string{"themes", "carto"}
	}
	reste := args

	// Le bandeau « simulation » et le plan par vagues viennent de
	// pipeline.Registre.afficherPlan (appelé par RunSources ci-dessous) —
	// pas d'en-tête à nous ici : en écrire un avant RunSources affichait
	// la « réponse » AVANT le NOTICE migrations que RunSources émet en
	// premier (stderr, narration), à l'envers de la règle du projet
	// (stderr d'abord, la réponse ensuite, sur stdout). Notre seul ajout,
	// la ligne « puis : » ci-dessous, vient donc après coup, dans le même
	// ordre.
	dryRun := len(reste) > 0 && (reste[0] == "-dry-run" || reste[0] == "--dry-run")
	opts := pipeline.Options{DryRun: dryRun, Concurrence: concurrence}
	if err := ingest.RunSources(cmd.Context(), "raw", "db/migrations", prealables, opts); err != nil {
		return fmt.Errorf("préalables (%s) : %w", strings.Join(prealables, ", "), err)
	}
	if dryRun {
		fmt.Printf("  puis : fpctl build site -only=%s\n", only)
		return nil
	}
	return execBinaire(cmd.Context(), "fpbuild", "cmd/build", append([]string{"-only=" + only}, reste...))
}
