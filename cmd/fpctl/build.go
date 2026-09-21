package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/sitegen"
	"github.com/spf13/cobra"
)

// commandeBuild : fpctl build site | scrutin | communes | <groupe> | section
// <nom> | topic <id>. internal/sitegen (l'ancien binaire séparé fpbuild) est
// importé directement — un seul binaire, plus de recompilation à la volée
// ni de second exécutable dans bin/.
//
// site est la seule construction complète, jamais filtrée : c'est ce que
// mettreEnPlace (internal/sitegen) doit voir pour ne jamais publier un site
// incomplet. Chaque groupe, section ou topic ingère d'abord ce qu'il déclare
// nécessiter (ingestPrerequisites, idempotent — via internal/ingest.RunSources,
// qui les ingère TOUS ENSEMBLE plutôt qu'un par un) puis appelle
// sitegen.RunSections avec les noms de section voulus — jamais un -only à
// composer soi-même : ce drapeau n'existe plus sur la ligne de commande,
// remplacé par ces sous-commandes (le graphe de dépendances qui décide de ce
// qu'il faut vraiment charger, lui, n'a pas changé — voir internal/sitegen/
// graphe.go). Ne met JAMAIS en place : réservé à l'itération locale.
//
// « fpctl build reste » n'existe plus : ce fourre-tout couvrait environ
// cinquante pages indépendantes, toujours reconstruites ensemble, cache
// invalidé dès qu'UNE SEULE ingestion avait tourné n'importe où (voir
// l'ancien resteInchange, internal/sitegen/cache.go). Chacune de ces pages a
// désormais son propre nom de section (voir sitegen.Sections()) : les
// groupes ci-dessous en couvrent les plus utiles pour l'itération locale,
// « section <nom> » atteint n'importe laquelle des autres, « topic <id> »
// un sujet de campagne en particulier (voir internal/sitegen/sujets.go).
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
			return executerInterne(cmd.Context(), sitegen.Run(cmd.Context(), args))
		},
	})
	for _, group := range buildGroups {
		group := group
		cmd.AddCommand(&cobra.Command{
			Use:   group.name + " [options]",
			Short: group.description,
			Long: group.description + ", en ingérant d'abord ce qu'elle déclare\n" +
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
				return executerInterne(cmd.Context(), buildSection(cmd, group.name, group.sections, args, concurrence))
			},
		})
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "section <nom> [options]",
		Short: "Reconstruit une seule section, par son nom",
		Long: "Comme les groupes ci-dessus, mais pour une seule section parmi\n" +
			"celles qu'internal/sitegen connaît (dette, chomage, collectivites... —\n" +
			"voir sitegen.Sections(), ou « fpctl build section » sans argument\n" +
			"pour la liste). Pour un sujet de campagne, voir « fpctl build topic »\n" +
			"à la place. Sans préalable déclaré pour ce nom précis, ingère le\n" +
			"socle parlementaire complet par défaut : presque toutes les pages\n" +
			"du site en dépendent au moins pour les fiches qu'elles citent.",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-build")
			}
			if len(args) == 0 {
				fmt.Println("sections connues :", strings.Join(sitegen.Sections(), ", "))
				return nil
			}
			nom := args[0]
			if !slices.Contains(sitegen.Sections(), nom) {
				return fmt.Errorf("section inconnue : %s (connues : %s)", nom, strings.Join(sitegen.Sections(), ", "))
			}
			return executerInterne(cmd.Context(), buildSection(cmd, nom, []string{nom}, args[1:], concurrence))
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "topic <id> [options]",
		Short: "Reconstruit un seul sujet de campagne, par son identifiant",
		Long: "Comme « fpctl build section », mais pour un sujet de campagne\n" +
			"individuel (fraude-fiscale, appareil-productif... — voir\n" +
			"sitegen.Topics(), internal/sitegen/sujets.go, ou « fpctl build\n" +
			"topic » sans argument pour la liste). Ingère le socle parlementaire\n" +
			"complet par défaut, comme « section » sans préalable déclaré.",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-build")
			}
			if len(args) == 0 {
				fmt.Println("sujets connus :", strings.Join(sitegen.Topics(), ", "))
				return nil
			}
			id := args[0]
			if !slices.Contains(sitegen.Topics(), id) {
				return fmt.Errorf("sujet inconnu : %s (connus : %s)", id, strings.Join(sitegen.Topics(), ", "))
			}
			return executerInterne(cmd.Context(), buildSection(cmd, id, []string{id}, args[1:], concurrence))
		},
	})
	return cmd
}

// buildGroups : les groupes de sections que fpbuild sait reconstruire
// ensemble, les plus utiles à nommer pour l'itération locale — partagé avec
// « fpctl list deps », qui les affiche comme autant de racines dépendant de
// ce qu'ingestPrerequisites déclare pour chacun (voir fpctl-list(1)). Une
// section qui n'appartient à aucun groupe reste atteignable par son propre
// nom via « fpctl build section <nom> ».
var buildGroups = []struct {
	name, description string
	sections          []string
}{
	{"scrutin", "reconstruit seulement les pages de scrutin", []string{"scrutin"}},
	{"communes", "reconstruit seulement les pages communes/EPCI", []string{"communes"}},
	{"identite", "candidats, partis, Assemblée, sources, Europe, thèmes, Sénat",
		[]string{"candidats", "partis", "assemblee", "sources", "europe", "themes", "senat"}},
	{"indicateurs", "frise, dette, chômage, vieillesse, jeunesse, richesse, dividendes, sécurité, agriculture, protection sociale",
		[]string{"frise", "dette", "chomage", "vieillesse", "jeunesse", "richesse", "dividendes", "securite", "agriculture", "social"}},
	{"gouvernance", "qui décide, présidentielle 2027, gouvernement, collectivités, circonscriptions, argent public",
		[]string{"qui-decide", "election2027", "gouvernement", "collectivites", "circonscriptions", "argent-public"}},
	{"fiches", "fiches personnes, candidats, organisations, référentiels, groupes parlementaires", []string{"fiches"}},
	{"dossiers", "accueil, index des sujets de campagne, documents de méthode, index de recherche (pas chaque sujet — voir « fpctl build topic »)",
		[]string{"accueil", "sujets", "comprendre", "recherche"}},
}

// ingestPrerequisites : ce qu'un groupe, une section ou un sujet a besoin de
// trouver déjà ingéré avant que le construire ait un sens. Tenue à la main,
// comme internal/checksum.Sections dont elle prolonge l'esprit — sous-couvrir
// cette liste ne fait jamais servir une page fausse (le cache par empreinte
// de internal/sitegen s'en assure déjà), au pire une construction sans les
// toutes dernières données. Nommer une seule source suffit pour toute sa
// chaîne : internal/ingest.RunSources résout les dépendances déclarées de
// proche en proche (« themes » entraîne senat, europe, normalize, download
// et partis) et ingère de front tout ce qui peut l'être. Pas d'entrée pour
// une section ou un sujet isolé : voir leur défaut dans buildSection.
var ingestPrerequisites = map[string][]string{
	"communes":    {"normalize", "communes", "associations"},
	"scrutin":     {"normalize", "exposes"},
	"identite":    {"themes", "carto"},
	"indicateurs": {"normalize"},
	"gouvernance": {"themes", "carto"},
	"fiches":      {"themes", "carto"},
	"dossiers":    {"themes", "carto"},
}

// buildSection ingère les préalables de name (déclarés dans
// ingestPrerequisites, ou le socle complet par défaut) puis construit
// sections — name identifie le groupe/section/sujet pour l'utilisateur et
// pour ingestPrerequisites ; sections est ce qu'internal/sitegen.RunSections
// reçoit vraiment (un nom seul, ou la liste que couvre un groupe de
// buildGroups).
func buildSection(cmd *cobra.Command, name string, sections []string, args []string, concurrence int) error {
	prerequisites, ok := ingestPrerequisites[name]
	if !ok {
		// Défaut pour une section/un sujet isolé sans préalable déclaré : le
		// socle parlementaire complet, dont presque toute page cite au moins
		// une fiche de personne ou d'organisation.
		prerequisites = []string{"themes", "carto"}
	}
	remaining := args

	// Le bandeau « simulation » et le plan par vagues viennent de
	// pipeline.Registre.afficherPlan (appelé par RunSources ci-dessous) —
	// pas d'en-tête à nous ici : en écrire un avant RunSources affichait
	// la « réponse » AVANT le NOTICE migrations que RunSources émet en
	// premier (stderr, narration), à l'envers de la règle du projet
	// (stderr d'abord, la réponse ensuite, sur stdout). Notre seul ajout,
	// la ligne « puis : » ci-dessous, vient donc après coup, dans le même
	// ordre.
	dryRun := len(remaining) > 0 && (remaining[0] == "-dry-run" || remaining[0] == "--dry-run")
	opts := pipeline.Options{DryRun: dryRun, Concurrence: concurrence}
	if err := ingest.RunSources(cmd.Context(), "raw", "db/migrations", prerequisites, opts); err != nil {
		return fmt.Errorf("préalables (%s) : %w", strings.Join(prerequisites, ", "), err)
	}
	if dryRun {
		fmt.Printf("  puis : fpctl build %s\n", name)
		return nil
	}
	return sitegen.RunSections(cmd.Context(), remaining, sections)
}
