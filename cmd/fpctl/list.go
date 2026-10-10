package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/ingest"
	"github.com/faits-politiques/faits-politiques/internal/matview"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/sitegen"
	"github.com/faits-politiques/faits-politiques/internal/sources"
	"github.com/faits-politiques/faits-politiques/internal/stats"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// commandList : fpctl list sources, fpctl list connectors, fpctl list
// stats, fpctl list deps — quatre vues différentes sur la même question,
// « qu'est-ce qui est chargé et par quoi » : les sources déclarées
// (raw.source), le code qui les charge (les fonctions Ingest* d'internal/),
// ce que ça pèse une fois en base, et — pour le seul socle parlementaire
// audité (voir fpctl-ingest(1), LE SOCLE PARLEMENTAIRE) — dans quel ordre.
func commandList() *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Liste une collection"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "sources [options]",
			Short: "Catalogue des sources de données ingérées",
			Long: "Écrit le catalogue JSON des sources (raw.source), avec leur dernière\n" +
				"exécution d'ingestion (raw.fetch_run) — licence, éditeur, cadence,\n" +
				"date de la dernière collecte réussie. Par défaut dans\n" +
				"docs/catalogue-sources.json (voir -out).",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if isHelpRequested(args) {
					return showManual("fpctl-list")
				}
				return runInternal(cmd.Context(), sources.Run(cmd.Context(), args))
			},
		},
		&cobra.Command{
			Use:   "connectors",
			Short: "Liste les fonctions Ingest* trouvées dans internal/",
			Long: "Le type de connecteur n'est pas déclaré dans un registre à part :\n" +
				"cette commande cherche directement, dans internal/, toute fonction\n" +
				"exportée dont le nom commence par Ingest, et les groupe par paquet.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if isHelpRequested(args) {
					return showManual("fpctl-list")
				}
				return runInternal(cmd.Context(), listConnectors())
			},
		},
		&cobra.Command{
			Use:   "stats",
			Short: "Résumé du contenu de la base : tables, lignes, taille par schéma",
			Long: "Lit pg_stat_user_tables, groupé par schéma applicatif (core, ref,\n" +
				"geo, raw, derived...) : nombre de tables, lignes estimées (n_live_tup,\n" +
				"pas un COUNT(*) exact), taille sur disque. Puis les dix tables les\n" +
				"plus lourdes, tous schémas confondus.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if isHelpRequested(args) {
					return showManual("fpctl-list")
				}
				return runInternal(cmd.Context(), showStats(cmd.Context()))
			},
		},
		commandDeps(),
		&cobra.Command{
			Use:   "sections [groupe]",
			Short: "Sections et sujets que fpctl build sait reconstruire un par un",
			Long: "internal/sitegen construit désormais chaque section (et chaque sujet\n" +
				"de campagne) comme un nœud nommé d'un graphe de dépendances\n" +
				"(internal/pipeline.Registry, voir internal/sitegen/graphe.go) : « fpctl\n" +
				"build section <nom> » ou « fpctl build topic <id> » ne charge plus\n" +
				"que la fermeture transitive de ce nœud, jamais la totalité du site.\n" +
				"Cette commande liste les deux catalogues (sitegen.Sections(),\n" +
				"sitegen.Topics()) et, pour chaque section, le groupe de « fpctl\n" +
				"build » qui la couvre déjà, s'il y en a un.\n\n" +
				"Sans argument, affiche aussi les groupes eux-mêmes (« fpctl build\n" +
				"dossiers », « fpctl build indicateurs »...), chacun avec ses\n" +
				"sections. Avec le nom d'un groupe, n'affiche que ses sections.",
			Args: cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				var group string
				if len(args) > 0 {
					group = args[0]
				}
				return runInternal(cmd.Context(), showSections(group))
			},
		},
		&cobra.Command{
			Use:   "matviews",
			Short: "État des matvues du schéma mv (internal/matview)",
			Long: "Le catalogue des matérialisations Postgres (voir internal/matview) —\n" +
				"pour chacune, quand elle a été actualisée pour la dernière fois et\n" +
				"sur combien de lignes. Lit mv.etat tel quel : ne recalcule PAS\n" +
				"l'empreinte des tables source (plusieurs secondes sur core.ballot),\n" +
				"donc ne dit pas si une matvue est périmée — seulement son dernier\n" +
				"état connu. « fpctl ingest systeme matviews » l'actualise pour de\n" +
				"vrai, en sautant tout REFRESH inutile.",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if isHelpRequested(args) {
					return showManual("fpctl-list")
				}
				return runInternal(cmd.Context(), showMatviews(cmd.Context()))
			},
		},
	)
	return cmd
}

func commandDeps() *cobra.Command {
	var jsonOutput, pagesOnly bool
	cmd := &cobra.Command{
		Use:   "deps [nom]",
		Short: "Graphe de dépendances du socle parlementaire, et des pages du site qui en dépendent",
		Long: "Le graphe lui-même (download, partis, normalize, carto, senat,\n" +
			"europe, themes, et ce que chacune exige) vient du code\n" +
			"(internal/ingest, Source.Dependances) : cette commande n'a besoin\n" +
			"d'aucune base pour l'afficher — voir fpctl-ingest(1), LE SOCLE\n" +
			"PARLEMENTAIRE. Si une base est joignable et migrée, elle enrichit\n" +
			"chaque étape de sa dernière exécution réussie et de ce qu'elle pèse\n" +
			"(la base n'est jamais qu'un REFLET républié par internal/pipeline,\n" +
			"jamais la référence) ; sinon un avertissement le dit (voir\n" +
			"internal/logs) et le graphe s'affiche quand même, sans ces deux\n" +
			"colonnes.\n\n" +
			"Chaque nœud s'affiche par sa vraie commande (« fpctl ingest\n" +
			"parlement download », « fpctl build communes »), jamais un nom nu\n" +
			"— deux commandes différentes peuvent partager le même nom (la\n" +
			"section communes de fpctl build et la source collectivites\n" +
			"communes de fpctl ingest, par exemple) : la carte ne les confond\n" +
			"pas, l'affichage ne les distingue pas moins.\n\n" +
			"Affiché en arbre : c'est un graphe orienté acyclique, pas un arbre\n" +
			"(normalize a deux « parents », senat et europe, tous deux exigés par\n" +
			"themes), à plusieurs racines (ce dont rien ne dépend — carto et\n" +
			"themes, dans le socle complet). Rendu quand même comme un arbre, ce\n" +
			"qu'il redevient une fois déroulé : une étape partagée réapparaît sous\n" +
			"chacun de ses parents plutôt que d'être fusionnée en un seul nœud.\n\n" +
			"Sans nom, affiche ensuite un second arbre : les pages du site que\n" +
			"fpctl build sait reconstruire seules (scrutin, communes, reste),\n" +
			"chacune comme racine de ses préalables d'ingestion (ingestPrerequisites,\n" +
			"cmd/fpctl/build.go) — --pages n'affiche que celui-là. Avec un nom,\n" +
			"limite l'affichage à une seule chose : l'une des sept étapes du\n" +
			"socle, ou une page (--pages ignoré, déjà implicite).\n\n" +
			"Chaque page affiche aussi « tables : » — les tables core/ref/geo\n" +
			"qu'elle lit directement, dépendances comprises, jamais mv.* (voir\n" +
			"internal/matview.Definition.Tables pour ça). Mesuré à l'exécution\n" +
			"par internal/sitegen (core.sitegen_table_usage), pas deviné par\n" +
			"relecture du code : absent tant qu'un « fpctl build site » n'a pas\n" +
			"encore tourné depuis la migration qui introduit cette table.\n\n" +
			"--json écrit la liste des nœuds concernés à plat (un objet par\n" +
			"nœud, depend_de nommant les autres par leur nom, commande portant\n" +
			"l'invocation exacte) plutôt que l'arbre déroulé — la forme qu'un\n" +
			"outil reconstruit plus facilement que des lignes indentées.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			return runInternal(cmd.Context(), showDeps(cmd.Context(), name, jsonOutput, pagesOnly))
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "écrit les nœuds à plat, en JSON, plutôt que l'arbre")
	cmd.Flags().BoolVar(&pagesOnly, "pages", false, "n'affiche que les pages du site (fpctl build) et leurs préalables d'ingestion")
	return cmd
}

// stepComponents : les tables que chaque étape du socle parlementaire
// écrit, pour afficher combien ça pèse en base à côté du graphe — tenu à la
// main, comme ingestPrerequisites (cmd/fpctl/build.go) : sept étapes assez
// rares et assez stables pour que ça reste à jour sans registre séparé.
// Contrairement aux octets archivés (voir stepSize), il n'existe pas de
// reflet générique pour la base : une table peut apparaître sous plusieurs
// étapes (carto.IngestPresidents écrit dans core.person/core.mandate, que
// normalize remplit aussi) — la taille affichée est alors celle de TOUTE la
// table, pas la part de cette seule étape, une approximation assumée pour
// un résumé de terminal, pas une comptabilité exacte.
var stepComponents = map[string]struct {
	// SourceSlugs : repli pour des octets archivés avant que
	// raw.source.etape existe, ou ingérés par RunAll (la chaîne
	// historique, qui n'étiquette pas ses sources — voir
	// internal/ingest.RunAll) ; ignoré dès que la colonne renvoie un total
	// non nul pour l'étape.
	SourceSlugs []string
	Tables      []string
}{
	"download":  {SourceSlugs: []string{"an-amo", "an-amo-15", "an-amo-16", "an-dossiers", "an-scrutins"}},
	"partis":    {SourceSlugs: []string{"ches-2024", "cnccfp-comptes", "populist-v4"}},
	"senat":     {SourceSlugs: []string{"senat-dosleg", "senat-senateurs"}},
	"europe":    {SourceSlugs: []string{"howtheyvote"}},
	"normalize": {Tables: []string{"core.person", "core.organization", "core.mandate", "core.affiliation", "core.scrutin", "core.ballot", "core.dossier", "core.texte"}},
	"carto":     {Tables: []string{"core.party_group_link", "core.party_referential_link", "core.mapping_lineage", "core.mapping_revision", "core.gouvernement"}},
	"themes":    {Tables: []string{"derived.scrutin_topic", "derived.coverage"}},
}

// stepSize additionne les octets archivés et occupés en base d'une étape
// — 0, sans erreur, quand aucune base n'est joignable (pool nil : voir
// openDBBriefly).
//
// Les octets archivés viennent de raw.source.etape (internal/archive.
// Archive.Etape, écrit par EnsureSource à chaque ingestion via RunSource ou
// RunCategorie) : un reflet générique, qui couvre N'IMPORTE QUELLE étape du
// catalogue, pas seulement les sept du socle — c'est ce qui permet de
// chiffrer une page entière (fpctl list deps --pages) sans registre tenu à
// la main pour chacune des dizaines de sources qu'elle peut requérir. Le
// repli sur stepComponents[name].SourceSlugs ne joue que si cette requête
// renvoie 0 : données jamais réingérées depuis la colonne, ou chargées par
// RunAll (qui ne l'écrit pas).
func stepSize(ctx context.Context, pool *pgxpool.Pool, name string) (archive, db int64, err error) {
	if pool == nil {
		return 0, 0, nil
	}
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(sum(d.byte_size), 0) FROM raw.document d WHERE d.id IN (
			SELECT DISTINCT r.document_id FROM raw.retrieval r JOIN raw.source s ON s.id = r.source_id
			WHERE s.etape = $1 AND r.document_id IS NOT NULL)`,
		name).Scan(&archive); err != nil {
		return 0, 0, err
	}
	c := stepComponents[name]
	if archive == 0 && len(c.SourceSlugs) > 0 {
		if err := pool.QueryRow(ctx, `
			SELECT coalesce(sum(d.byte_size), 0) FROM raw.document d WHERE d.id IN (
				SELECT DISTINCT r.document_id FROM raw.retrieval r JOIN raw.source s ON s.id = r.source_id
				WHERE s.slug = ANY($1) AND r.document_id IS NOT NULL)`,
			c.SourceSlugs).Scan(&archive); err != nil {
			return 0, 0, err
		}
	}
	for _, t := range c.Tables {
		// to_regclass plutôt que caster directement en regclass : une table
		// qui n'existe pas encore (base pas à jour) ne doit pas faire
		// échouer tout l'affichage du graphe, juste compter pour 0 ici.
		var o *int64
		if err := pool.QueryRow(ctx, `SELECT pg_total_relation_size(to_regclass($1))`, t).Scan(&o); err != nil {
			return 0, 0, fmt.Errorf("taille de %s : %w", t, err)
		}
		if o != nil {
			db += *o
		}
	}
	return archive, db, nil
}

// openDBBriefly tente une connexion courte (2s, pas les 30s de
// nouvelles tentatives de store.Open — une commande d'AFFICHAGE n'a aucune
// raison de faire attendre qui la lance pour une base qui met du temps à
// démarrer) pour enrichir le graphe statique d'informations qu'IL n'a pas :
// dernière exécution, taille. nil sans erreur si la base est injoignable ou
// pas encore migrée par cette version de fpctl — un avertissement le dit,
// le graphe déclaré dans le code reste affichable quand même.
func openDBBriefly(ctx context.Context) *pgxpool.Pool {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	pool, err := store.Open(cctx)
	if err != nil {
		slog.Warn("base injoignable : affichage du graphe déclaré dans le code seul, sans dernière exécution ni taille",
			"erreur", err)
		return nil
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('core.pipeline_etape') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		slog.Warn("core.pipeline_etape n'existe pas encore sur cette base : dernière exécution et taille non affichées — lancez « fpctl ingest migrate » pour les avoir")
		pool.Close()
		return nil
	}
	return pool
}

// node : une étape du graphe, telle qu'affichée ou exportée — le code pour
// la structure (Name, Description, DependsOn), la base pour l'enrichir quand
// elle est joignable (LastSuccessfulRun, ArchiveBytes, DBBytes ; tous à
// zéro/nil sinon, jamais une raison de refuser l'affichage). Les champs
// JSON nomment les autres étapes par leur Nom (depend_de) plutôt que de les
// imbriquer : la forme qu'un outil reconstruit le plus facilement, et celle
// qui n'a pas à choisir quelle branche dupliquer un nœud partagé.
// node.Type : "etape" (une étape d'ingestion, internal/ingest.Source) ou
// "page" (une section de fpctl build, cmd/fpctl/build.go — une page ou un
// groupe de pages du site). Une page ne dépend jamais d'une autre page :
// seulement d'étapes, jamais l'inverse — le graphe reste un DAG à deux
// niveaux, données puis pages, pas un DAG général entre les deux.
type node struct {
	Name              string     `json:"nom"`
	Type              string     `json:"type"`
	Command           string     `json:"commande"`
	Description       string     `json:"description"`
	LastSuccessfulRun *time.Time `json:"derniere_execution_reussie"`
	ArchiveBytes      int64      `json:"archive_octets"`
	DBBytes           int64      `json:"base_octets"`
	DependsOn         []string   `json:"depend_de"`
	// TransitiveArchiveBytes/TransitiveDBBytes : le total, DÉPENDANCES
	// COMPRISES (une fois chacune, quel que soit le nombre de chemins qui y
	// mènent), à ingérer pour amener ce nœud à jour depuis rien — ce qu'une
	// décision « cette page rentre-t-elle dans le budget de la CI ? » lit
	// directement, sans redérouler l'arbre. Toujours 0 pour une étape
	// (Type == "etape") : ArchiveBytes/DBBytes seuls suffisent, une étape
	// n'a jamais qu'un parent direct à elle-même. Calculé seulement pour
	// une page (Type == "page", voir addPageNode) — c'est là que la
	// question se pose.
	TransitiveArchiveBytes int64 `json:"archive_octets_transitif,omitempty"`
	TransitiveDBBytes      int64 `json:"base_octets_transitif,omitempty"`
	// Tables : les tables core/ref/geo/derived/jo (jamais mv.*) nécessaires
	// à cette page, dépendances comprises — lu dans core.sitegen_table_usage
	// (internal/sitegen.TablesPubliees), un REFLET mesuré au dernier
	// « fpctl build site » réussi, jamais une liste tenue à la main. nil
	// tant qu'aucune construction n'a encore tourné depuis cette migration,
	// ou pour un nœud de type "etape" (cette question ne se pose que pour
	// une page).
	Tables []string `json:"tables,omitempty"`
}

// transitiveSizeMulti additionne ArchiveBytes/DBBytes de chaque racine et
// de tout ce dont elles dépendent, transitivement — une seule fois par
// nœud même si plusieurs racines ou plusieurs chemins y mènent (normalize,
// sous senat ET europe ; ou requis directement par une page ET par l'une de
// ses autres dépendances), pour ne jamais compter un même jeu de données
// deux fois dans le total.
func transitiveSizeMulti(nodes map[string]node, roots []string) (archive, db int64) {
	seen := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		n, ok := nodes[name]
		if !ok {
			return
		}
		archive += n.ArchiveBytes
		db += n.DBBytes
		for _, d := range n.DependsOn {
			visit(d)
		}
	}
	for _, r := range roots {
		visit(r)
	}
	return
}

// pageKey préfixe la clé interne d'un nœud « page » : « communes » nomme à
// la fois une section de fpctl build et une source de la catégorie
// collectivites (fpctl ingest collectivites communes) — deux choses
// différentes qui doivent pouvoir coexister dans la même carte sans que
// l'une écrase l'autre. Name et Command, eux, restent la forme lisible
// (« communes », « fpctl build communes ») : seule cette clé de carte est
// préfixée.
func pageKey(sectionName string) string { return "page:" + sectionName }

// addStepNode ajoute, si elle n'y est pas déjà, l'étape d'ingestion n à
// nodes — connue du catalogue (internal/ingest) ou non (un préalable hors
// du socle audité, comme « exposes » : on affiche alors le nom seul, sans
// description ni dépendance plus profonde, plutôt que de refuser).
func addStepNode(ctx context.Context, pool *pgxpool.Pool, nodes map[string]node, n string) error {
	if _, ok := nodes[n]; ok {
		return nil
	}
	description, command := n, n
	var dependsOn []string
	if s, ok := ingest.SourceParNom(n); ok {
		description = s.Description
		command = fmt.Sprintf("fpctl ingest %s %s", s.Categorie, s.Nom)
		dependsOn = s.Dependances
	}
	archive, db, err := stepSize(ctx, pool, n)
	if err != nil {
		return fmt.Errorf("%s : %w", n, err)
	}
	nodes[n] = node{Name: n, Type: "etape", Command: command, Description: description, DependsOn: dependsOn, ArchiveBytes: archive, DBBytes: db}
	return nil
}

// addPageNode ajoute la section ou le groupe de fpctl build sectionName
// à nodes, avec pour dépendances ses préalables d'ingestion
// (ingestPrerequisites, cmd/fpctl/build.go) — ajoutés eux aussi si besoin,
// pour que le nœud page pointe vers des étapes qui existent vraiment dans
// la carte. sitegenSections est la liste des VRAIS noms de nœuds internal/
// sitegen que sectionName recouvre (pour un groupe comme « indicateurs »,
// buildGroups[i].sections — plusieurs sections ; pour une section isolée
// comme « dette », []string{sectionName} suffit, sectionName EST déjà le
// nom sitegen) : sectionName lui-même ne l'est pas forcément, voir
// buildGroups. tablesByNode vient de sitegen.TablesPubliees (nil si aucune
// base n'était joignable, ou si aucune construction n'a encore tourné
// depuis la migration qui a introduit core.sitegen_table_usage) ; les
// tables de chaque section du groupe sont réunies ici en un seul ensemble
// dédupliqué.
func addPageNode(ctx context.Context, pool *pgxpool.Pool, nodes map[string]node, sectionName, description string, sitegenSections []string, tablesByNode map[string][]string) error {
	prerequisites := ingestPrerequisites[sectionName]
	for _, p := range prerequisites {
		if err := addStepNode(ctx, pool, nodes, p); err != nil {
			return err
		}
	}
	// Le total se calcule sur les préalables (DependsOn), pas sur la page
	// elle-même : une page n'a pas d'ArchiveBytes/DBBytes en propre, ce
	// serait compter à vide. Une seule marche (transitiveSizeMulti,
	// seen partagé) sur TOUS les préalables à la fois — pas un total par
	// préalable additionné ensuite, qui recompterait une dépendance
	// partagée entre deux d'entre eux.
	archiveTotal, dbTotal := transitiveSizeMulti(nodes, prerequisites)
	var tables []string
	if tablesByNode != nil {
		seen := map[string]bool{}
		for _, target := range sitegen.ResolveTargets(sitegenSections) {
			for _, t := range tablesByNode[target] {
				if !seen[t] {
					seen[t] = true
					tables = append(tables, t)
				}
			}
		}
		sort.Strings(tables)
	}
	nodes[pageKey(sectionName)] = node{
		Name: sectionName, Type: "page",
		Command: "fpctl build " + sectionName, Description: description,
		DependsOn:              prerequisites,
		TransitiveArchiveBytes: archiveTotal,
		TransitiveDBBytes:      dbTotal,
		Tables:                 tables,
	}
	return nil
}

func showDeps(ctx context.Context, name string, jsonOutput, pagesOnly bool) error {
	// Le graphe lui-même vient du code, jamais de la base — internal/
	// pipeline le republie à chaque exécution, mais un REFLET ne se lit pas
	// à la place de la référence.
	pool := openDBBriefly(ctx)
	if pool != nil {
		defer pool.Close()
	}
	nodes := map[string]node{}
	for _, n := range ingest.SocleParlementaire() {
		if err := addStepNode(ctx, pool, nodes, n); err != nil {
			return err
		}
	}
	var tablesByNode map[string][]string
	if pool != nil {
		// Seule la dernière exécution vient de la base ; la structure
		// (Description, DependsOn) reste celle du code, au cas où le reflet
		// publié daterait d'une version antérieure du catalogue.
		steps, err := pipeline.ReadTopology(ctx, pool)
		if err != nil {
			return err
		}
		for _, s := range steps {
			if n, ok := nodes[s.Name]; ok {
				n.LastSuccessfulRun = s.LastSuccessfulRun
				nodes[s.Name] = n
			}
		}
		// tablesByNode reste nil (pas une erreur fatale pour cette
		// commande d'affichage) si la migration qui introduit
		// core.sitegen_table_usage n'est pas encore passée — addPageNode
		// se contente alors de ne rien afficher pour Tables.
		if t, err := sitegen.TablesPublished(ctx, pool); err == nil {
			tablesByNode = t
		}
	}

	// roots : dans la portée demandée, ce dont rien d'autre dans cette
	// même portée ne dépend — le sommet de chaque arbre qu'on va dérouler.
	// « fpctl list deps » sans argument affiche le socle complet : deux
	// racines (carto, themes), pas une seule, parce que ce n'est justement
	// pas un arbre avant qu'on le déroule.
	var roots []string
	switch {
	case name == "" && !pagesOnly:
		roots = computeRoots(nodes, ingest.SocleParlementaire())
	case name == "" && pagesOnly:
		for _, section := range buildGroups {
			if err := addPageNode(ctx, pool, nodes, section.name, section.description, section.sections, tablesByNode); err != nil {
				return err
			}
			roots = append(roots, pageKey(section.name))
		}
	case ingest.EstSurLeSocle(name):
		roots = []string{name}
	default:
		found := false
		for _, section := range buildGroups {
			if section.name == name {
				found = true
				if err := addPageNode(ctx, pool, nodes, name, section.description, section.sections, tablesByNode); err != nil {
					return err
				}
				roots = []string{pageKey(name)}
			}
		}
		if !found {
			known := ingest.SocleParlementaire()
			for _, section := range buildGroups {
				known = append(known, section.name)
			}
			sort.Strings(known)
			return fmt.Errorf("%s inconnu (attendu : %s)", name, strings.Join(known, ", "))
		}
	}

	if !jsonOutput && name == "" && !pagesOnly {
		if err := showDepsTree(pool, nodes, roots); err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("pages du site (fpctl build) :")
		var pageRoots []string
		for _, section := range buildGroups {
			if err := addPageNode(ctx, pool, nodes, section.name, section.description, section.sections, tablesByNode); err != nil {
				return err
			}
			pageRoots = append(pageRoots, pageKey(section.name))
		}
		return showDepsTree(pool, nodes, pageRoots)
	}

	if jsonOutput {
		return showDepsJSON(nodes, roots)
	}
	return showDepsTree(pool, nodes, roots)
}

// computeRoots : parmi names, ceux qu'aucun autre (dans ce même ensemble)
// ne cite dans son DependsOn.
func computeRoots(nodes map[string]node, names []string) []string {
	dependent := map[string]bool{}
	for _, n := range names {
		for _, d := range nodes[n].DependsOn {
			dependent[d] = true
		}
	}
	var roots []string
	for _, n := range names {
		if !dependent[n] {
			roots = append(roots, n)
		}
	}
	return roots
}

func showDepsJSON(nodes map[string]node, roots []string) error {
	// Chaque racine ET tout ce dont elle dépend, transitivement — une seule
	// fois par nom même si plusieurs racines ou plusieurs chemins y mènent,
	// puisqu'ici (contrairement à l'arbre) rien n'a besoin d'être dupliqué.
	seen := map[string]bool{}
	var scope []node
	var visit func(name string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		n, ok := nodes[name]
		if !ok {
			return
		}
		scope = append(scope, n)
		for _, d := range n.DependsOn {
			visit(d)
		}
	}
	for _, r := range roots {
		visit(r)
	}
	sort.Slice(scope, func(i, j int) bool { return scope[i].Name < scope[j].Name })
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(scope)
}

func showDepsTree(pool *pgxpool.Pool, nodes map[string]node, roots []string) error {
	for i, r := range roots {
		printTree(pool, nodes, r, "", i == len(roots)-1, true)
	}
	return nil
}

// printTree dessine name et ce dont il dépend, récursivement, avec les
// caractères de branche habituels (├──, └──, │) — une étape partagée par
// plusieurs branches y réapparaît sous chacune, dépliant le graphe en arbre
// plutôt que de fusionner ses nœuds partagés (voir le commentaire de
// showDeps).
func printTree(pool *pgxpool.Pool, nodes map[string]node, name, prefix string, last, root bool) {
	var line strings.Builder
	switch {
	case root:
	case last:
		line.WriteString(prefix + "└── ")
	default:
		line.WriteString(prefix + "├── ")
	}
	n, known := nodes[name]
	if !known {
		line.WriteString(name + " — inconnu")
		fmt.Println(line.String())
		return
	}
	if n.Command != "" {
		line.WriteString(n.Command)
	} else {
		line.WriteString(n.Name)
	}
	switch {
	case n.Type == "page":
		// Une section de fpctl build n'a pas, pour l'instant, de dernière
		// exécution suivie pour elle seule (contrairement aux étapes
		// d'ingestion, voir internal/pipeline) — rien à ajouter ici.
	case pool == nil && n.DependsOn == nil && n.Description == "":
		// Un préalable hors du socle (ex. "exposes") : pas de dernière
		// exécution suivie pour lui, jamais une raison d'écrire « aucune
		// base joignable » pour une donnée qu'on ne calcule de toute façon
		// pas.
	case pool == nil:
		line.WriteString(" — dernière exécution inconnue (aucune base joignable)")
	case n.LastSuccessfulRun != nil:
		line.WriteString(" — " + n.LastSuccessfulRun.Local().Format("2006-01-02 15:04"))
	default:
		line.WriteString(" — jamais exécutée avec succès")
	}
	if n.ArchiveBytes > 0 || n.DBBytes > 0 {
		fmt.Fprintf(&line, "  (archive %s, base %s)", humanSize(n.ArchiveBytes), humanSize(n.DBBytes))
	}
	fmt.Println(line.String())

	var childPrefix string
	if !root {
		if last {
			childPrefix = prefix + "    "
		} else {
			childPrefix = prefix + "│   "
		}
	}
	if n.Description != "" {
		fmt.Println(childPrefix + "    " + n.Description)
	}
	if n.Type == "page" && (n.TransitiveArchiveBytes > 0 || n.TransitiveDBBytes > 0) {
		fmt.Printf(childPrefix+"    ≈ %s à télécharger, %s en base (préalables compris)\n",
			humanSize(n.TransitiveArchiveBytes), humanSize(n.TransitiveDBBytes))
	}
	if n.Type == "page" && len(n.Tables) > 0 {
		fmt.Println(childPrefix + "    tables : " + strings.Join(n.Tables, ", "))
	}
	for i, d := range n.DependsOn {
		printTree(pool, nodes, d, childPrefix, i == len(n.DependsOn)-1, false)
	}
}

func listConnectors() error {
	cs, err := sources.ListConnectors(".")
	if err != nil {
		return err
	}
	pkg := ""
	for _, c := range cs {
		if c.Package != pkg {
			pkg = c.Package
			fmt.Printf("%s\n", pkg)
		}
		fmt.Printf("  %s\n", c.Function)
	}
	fmt.Printf("\n%d connecteurs, %d paquets\n", len(cs), packageCount(cs))
	return nil
}

func packageCount(cs []sources.Connector) int {
	seen := map[string]bool{}
	for _, c := range cs {
		seen[c.Package] = true
	}
	return len(seen)
}

func showStats(ctx context.Context) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	schemas, err := stats.Summary(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Printf("%-12s %8s %14s %10s\n", "schéma", "tables", "lignes (est.)", "taille")
	var totalBytes, totalRows int64
	for _, s := range schemas {
		fmt.Printf("%-12s %8d %14d %10s\n", s.Name, s.Tables, s.EstimatedRows, humanSize(s.Bytes))
		totalBytes += s.Bytes
		totalRows += s.EstimatedRows
	}
	fmt.Printf("%-12s %8s %14d %10s\n", "total", "", totalRows, humanSize(totalBytes))

	largest, err := stats.LargestTables(ctx, pool, 10)
	if err != nil {
		return err
	}
	fmt.Printf("\nplus grosses tables :\n")
	for _, t := range largest {
		fmt.Printf("  %-10s %-40s %12d lignes  %10s\n", t.Schema, t.Name, t.EstimatedRows, humanSize(t.Bytes))
	}
	return nil
}

// showSections liste les deux catalogues que internal/sitegen.Sections()/
// Topics() exposent — jamais recopiés à la main ici, la même source que
// cmd/fpctl/build.go valide contre pour "section <nom>"/"topic <id>".
// showSections liste les sections connues de sitegen.Sections()/
// Topics() — sans argument, la vue complète (chaque section avec son
// groupe, chaque groupe avec ses sections, chaque sujet) ; avec le nom
// d'un groupe (« dossiers », « indicateurs »...), seulement ses sections.
func showSections(group string) error {
	groupOf := map[string]string{}
	for _, g := range buildGroups {
		for _, s := range g.sections {
			groupOf[s] = g.name
		}
	}

	if group != "" {
		for _, g := range buildGroups {
			if g.name == group {
				fmt.Printf("fpctl build %s : %s\n", g.name, g.description)
				for _, s := range g.sections {
					fmt.Printf("  %s\n", s)
				}
				return nil
			}
		}
		var known []string
		for _, g := range buildGroups {
			known = append(known, g.name)
		}
		return fmt.Errorf("groupe inconnu : %s (connus : %s)", group, strings.Join(known, ", "))
	}

	fmt.Println("sections (fpctl build section <nom>) :")
	for _, s := range sitegen.Sections() {
		if g, ok := groupOf[s]; ok {
			fmt.Printf("  %-20s dans le groupe « fpctl build %s »\n", s, g)
		} else {
			fmt.Printf("  %-20s\n", s)
		}
	}

	fmt.Println("\ngroupes (fpctl build <groupe>, ou fpctl list sections <groupe> pour le détail) :")
	for _, g := range buildGroups {
		fmt.Printf("  %-14s %s — %s\n", g.name, g.description, strings.Join(g.sections, ", "))
	}

	topics := sitegen.Topics()
	fmt.Printf("\nsujets de campagne (fpctl build topic <id>) : %d — %s\n",
		len(topics), strings.Join(topics, ", "))
	return nil
}

func showMatviews(ctx context.Context) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	states, err := matview.Lister(ctx, pool)
	if err != nil {
		return err
	}
	// Largeur calculée sur le plus long nom réel plutôt qu'une constante :
	// une constante trop courte (mv.commune_association_count dépassait déjà
	// %-24s) désaligne toutes les colonnes qui suivent dès qu'un nom la
	// dépasse, pas seulement sa propre ligne.
	width := len("matvue")
	for _, e := range states {
		if n := len("mv." + e.Nom); n > width {
			width = n
		}
	}
	fmt.Printf("%-*s %10s %20s  %s\n", width, "matvue", "lignes", "actualisée le", "tables source")
	for _, e := range states {
		name := "mv." + e.Nom
		if !e.Connue {
			fmt.Printf("%-*s %10s %20s  %s\n", width, name, "—", "jamais", strings.Join(e.Tables, ", "))
			continue
		}
		fmt.Printf("%-*s %10d %20s  %s\n", width, name, e.Lignes,
			e.ActualiseeLe.Local().Format("2006-01-02 15:04"), strings.Join(e.Tables, ", "))
	}
	return nil
}

func humanSize(bytes int64) string {
	const unit = 1024.0
	v := float64(bytes)
	for _, suffix := range []string{"o", "Ko", "Mo", "Go", "To"} {
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, suffix)
		}
		v /= unit
	}
	return fmt.Sprintf("%.1f Po", v)
}
