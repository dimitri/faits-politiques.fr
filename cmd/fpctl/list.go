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

// commandeList : fpctl list sources, fpctl list connectors, fpctl list
// stats, fpctl list deps — quatre vues différentes sur la même question,
// « qu'est-ce qui est chargé et par quoi » : les sources déclarées
// (raw.source), le code qui les charge (les fonctions Ingest* d'internal/),
// ce que ça pèse une fois en base, et — pour le seul socle parlementaire
// audité (voir fpctl-ingest(1), LE SOCLE PARLEMENTAIRE) — dans quel ordre.
func commandeList() *cobra.Command {
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
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), sources.Run(cmd.Context(), args))
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
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), listerConnecteurs())
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
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), afficherStats(cmd.Context()))
			},
		},
		commandeDeps(),
		&cobra.Command{
			Use:   "sections [groupe]",
			Short: "Sections et sujets que fpctl build sait reconstruire un par un",
			Long: "internal/sitegen construit désormais chaque section (et chaque sujet\n" +
				"de campagne) comme un nœud nommé d'un graphe de dépendances\n" +
				"(internal/pipeline.Registre, voir internal/sitegen/graphe.go) : « fpctl\n" +
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
				var groupe string
				if len(args) > 0 {
					groupe = args[0]
				}
				return executerInterne(cmd.Context(), afficherSections(groupe))
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
				if estDemandeAide(args) {
					return afficherManuel("fpctl-list")
				}
				return executerInterne(cmd.Context(), afficherMatviews(cmd.Context()))
			},
		},
	)
	return cmd
}

func commandeDeps() *cobra.Command {
	var enJSON, enPages bool
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
			"--json écrit la liste des nœuds concernés à plat (un objet par\n" +
			"nœud, depend_de nommant les autres par leur nom, commande portant\n" +
			"l'invocation exacte) plutôt que l'arbre déroulé — la forme qu'un\n" +
			"outil reconstruit plus facilement que des lignes indentées.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nom string
			if len(args) > 0 {
				nom = args[0]
			}
			return executerInterne(cmd.Context(), afficherDeps(cmd.Context(), nom, enJSON, enPages))
		},
	}
	cmd.Flags().BoolVar(&enJSON, "json", false, "écrit les nœuds à plat, en JSON, plutôt que l'arbre")
	cmd.Flags().BoolVar(&enPages, "pages", false, "n'affiche que les pages du site (fpctl build) et leurs préalables d'ingestion")
	return cmd
}

// composantesEtape : les tables que chaque étape du socle parlementaire
// écrit, pour afficher combien ça pèse en base à côté du graphe — tenu à la
// main, comme ingestPrerequisites (cmd/fpctl/build.go) : sept étapes assez
// rares et assez stables pour que ça reste à jour sans registre séparé.
// Contrairement aux octets archivés (voir tailleEtape), il n'existe pas de
// reflet générique pour la base : une table peut apparaître sous plusieurs
// étapes (carto.IngestPresidents écrit dans core.person/core.mandate, que
// normalize remplit aussi) — la taille affichée est alors celle de TOUTE la
// table, pas la part de cette seule étape, une approximation assumée pour
// un résumé de terminal, pas une comptabilité exacte.
var composantesEtape = map[string]struct {
	// SlugsSources : repli pour des octets archivés avant que
	// raw.source.etape existe, ou ingérés par RunTout (la chaîne
	// historique, qui n'étiquette pas ses sources — voir
	// internal/ingest.RunTout) ; ignoré dès que la colonne renvoie un total
	// non nul pour l'étape.
	SlugsSources []string
	Tables       []string
}{
	"download":  {SlugsSources: []string{"an-amo", "an-amo-15", "an-amo-16", "an-dossiers", "an-scrutins"}},
	"partis":    {SlugsSources: []string{"ches-2024", "cnccfp-comptes", "populist-v4"}},
	"senat":     {SlugsSources: []string{"senat-dosleg", "senat-senateurs"}},
	"europe":    {SlugsSources: []string{"howtheyvote"}},
	"normalize": {Tables: []string{"core.person", "core.organization", "core.mandate", "core.affiliation", "core.scrutin", "core.ballot", "core.dossier", "core.texte"}},
	"carto":     {Tables: []string{"core.party_group_link", "core.party_referential_link", "core.mapping_lineage", "core.mapping_revision", "core.gouvernement"}},
	"themes":    {Tables: []string{"derived.scrutin_topic", "derived.coverage"}},
}

// tailleEtape additionne les octets archivés et occupés en base d'une étape
// — 0, sans erreur, quand aucune base n'est joignable (pool nil : voir
// ouvrirBaseBrievement).
//
// Les octets archivés viennent de raw.source.etape (internal/archive.
// Archive.Etape, écrit par EnsureSource à chaque ingestion via RunSource ou
// RunCategorie) : un reflet générique, qui couvre N'IMPORTE QUELLE étape du
// catalogue, pas seulement les sept du socle — c'est ce qui permet de
// chiffrer une page entière (fpctl list deps --pages) sans registre tenu à
// la main pour chacune des dizaines de sources qu'elle peut requérir. Le
// repli sur composantesEtape[nom].SlugsSources ne joue que si cette requête
// renvoie 0 : données jamais réingérées depuis la colonne, ou chargées par
// RunTout (qui ne l'écrit pas).
func tailleEtape(ctx context.Context, pool *pgxpool.Pool, nom string) (archive, base int64, err error) {
	if pool == nil {
		return 0, 0, nil
	}
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(sum(d.byte_size), 0) FROM raw.document d WHERE d.id IN (
			SELECT DISTINCT r.document_id FROM raw.retrieval r JOIN raw.source s ON s.id = r.source_id
			WHERE s.etape = $1 AND r.document_id IS NOT NULL)`,
		nom).Scan(&archive); err != nil {
		return 0, 0, err
	}
	c := composantesEtape[nom]
	if archive == 0 && len(c.SlugsSources) > 0 {
		if err := pool.QueryRow(ctx, `
			SELECT coalesce(sum(d.byte_size), 0) FROM raw.document d WHERE d.id IN (
				SELECT DISTINCT r.document_id FROM raw.retrieval r JOIN raw.source s ON s.id = r.source_id
				WHERE s.slug = ANY($1) AND r.document_id IS NOT NULL)`,
			c.SlugsSources).Scan(&archive); err != nil {
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
			base += *o
		}
	}
	return archive, base, nil
}

// ouvrirBaseBrievement tente une connexion courte (2s, pas les 30s de
// nouvelles tentatives de store.Open — une commande d'AFFICHAGE n'a aucune
// raison de faire attendre qui la lance pour une base qui met du temps à
// démarrer) pour enrichir le graphe statique d'informations qu'IL n'a pas :
// dernière exécution, taille. nil sans erreur si la base est injoignable ou
// pas encore migrée par cette version de fpctl — un avertissement le dit,
// le graphe déclaré dans le code reste affichable quand même.
func ouvrirBaseBrievement(ctx context.Context) *pgxpool.Pool {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	pool, err := store.Open(cctx)
	if err != nil {
		slog.Warn("base injoignable : affichage du graphe déclaré dans le code seul, sans dernière exécution ni taille",
			"erreur", err)
		return nil
	}
	var existe bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('core.pipeline_etape') IS NOT NULL`).Scan(&existe); err != nil || !existe {
		slog.Warn("core.pipeline_etape n'existe pas encore sur cette base : dernière exécution et taille non affichées — lancez « fpctl ingest migrate » pour les avoir")
		pool.Close()
		return nil
	}
	return pool
}

// noeud : une étape du graphe, telle qu'affichée ou exportée — le code pour
// la structure (Nom, Description, DependDe), la base pour l'enrichir quand
// elle est joignable (DerniereExecutionReussie, ArchiveOctets, BaseOctets ;
// tous à zéro/nil sinon, jamais une raison de refuser l'affichage). Les
// champs JSON nomment les autres étapes par leur Nom (depend_de) plutôt que
// de les imbriquer : la forme qu'un outil reconstruit le plus facilement,
// et celle qui n'a pas à choisir quelle branche dupliquer un nœud partagé.
// noeud.Type : "etape" (une étape d'ingestion, internal/ingest.Source) ou
// "page" (une section de fpctl build, cmd/fpctl/build.go — une page ou un
// groupe de pages du site). Une page ne dépend jamais d'une autre page :
// seulement d'étapes, jamais l'inverse — le graphe reste un DAG à deux
// niveaux, données puis pages, pas un DAG général entre les deux.
type noeud struct {
	Nom                      string     `json:"nom"`
	Type                     string     `json:"type"`
	Commande                 string     `json:"commande"`
	Description              string     `json:"description"`
	DerniereExecutionReussie *time.Time `json:"derniere_execution_reussie"`
	ArchiveOctets            int64      `json:"archive_octets"`
	BaseOctets               int64      `json:"base_octets"`
	DependDe                 []string   `json:"depend_de"`
	// ArchiveOctetsTransitif/BaseOctetsTransitif : le total, DÉPENDANCES
	// COMPRISES (une fois chacune, quel que soit le nombre de chemins qui y
	// mènent), à ingérer pour amener ce nœud à jour depuis rien — ce qu'une
	// décision « cette page rentre-t-elle dans le budget de la CI ? » lit
	// directement, sans redérouler l'arbre. Toujours 0 pour une étape
	// (Type == "etape") : ArchiveOctets/BaseOctets seuls suffisent, une
	// étape n'a jamais qu'un parent direct à elle-même. Calculé seulement
	// pour une page (Type == "page", voir ajouterNoeudPage) — c'est là que
	// la question se pose.
	ArchiveOctetsTransitif int64 `json:"archive_octets_transitif,omitempty"`
	BaseOctetsTransitif    int64 `json:"base_octets_transitif,omitempty"`
}

// tailleTransitiveMulti additionne ArchiveOctets/BaseOctets de chaque racine
// et de tout ce dont elles dépendent, transitivement — une seule fois par
// nœud même si plusieurs racines ou plusieurs chemins y mènent (normalize,
// sous senat ET europe ; ou requis directement par une page ET par l'une de
// ses autres dépendances), pour ne jamais compter un même jeu de données
// deux fois dans le total.
func tailleTransitiveMulti(noeuds map[string]noeud, racines []string) (archive, base int64) {
	vus := map[string]bool{}
	var visiter func(nom string)
	visiter = func(nom string) {
		if vus[nom] {
			return
		}
		vus[nom] = true
		n, ok := noeuds[nom]
		if !ok {
			return
		}
		archive += n.ArchiveOctets
		base += n.BaseOctets
		for _, d := range n.DependDe {
			visiter(d)
		}
	}
	for _, r := range racines {
		visiter(r)
	}
	return
}

// clePage préfixe la clé interne d'un nœud « page » : « communes » nomme à
// la fois une section de fpctl build et une source de la catégorie
// collectivites (fpctl ingest collectivites communes) — deux choses
// différentes qui doivent pouvoir coexister dans la même carte sans que
// l'une écrase l'autre. Nom et Commande, eux, restent la forme lisible
// (« communes », « fpctl build communes ») : seule cette clé de carte est
// préfixée.
func clePage(nomSection string) string { return "page:" + nomSection }

// ajouterNoeudEtape ajoute, si elle n'y est pas déjà, l'étape d'ingestion n
// à noeuds — connue du catalogue (internal/ingest) ou non (un préalable
// hors du socle audité, comme « exposes » : on affiche alors le nom seul,
// sans description ni dépendance plus profonde, plutôt que de refuser).
func ajouterNoeudEtape(ctx context.Context, pool *pgxpool.Pool, noeuds map[string]noeud, n string) error {
	if _, ok := noeuds[n]; ok {
		return nil
	}
	description, commande := n, n
	var dependDe []string
	if s, ok := ingest.SourceParNom(n); ok {
		description = s.Description
		commande = fmt.Sprintf("fpctl ingest %s %s", s.Categorie, s.Nom)
		dependDe = s.Dependances
	}
	archive, base, err := tailleEtape(ctx, pool, n)
	if err != nil {
		return fmt.Errorf("%s : %w", n, err)
	}
	noeuds[n] = noeud{Nom: n, Type: "etape", Commande: commande, Description: description, DependDe: dependDe, ArchiveOctets: archive, BaseOctets: base}
	return nil
}

// ajouterNoeudPage ajoute la section de fpctl build nomSection à noeuds,
// avec pour dépendances ses préalables d'ingestion (ingestPrerequisites,
// cmd/fpctl/build.go) — ajoutés eux aussi si besoin, pour que le nœud page
// pointe vers des étapes qui existent vraiment dans la carte.
func ajouterNoeudPage(ctx context.Context, pool *pgxpool.Pool, noeuds map[string]noeud, nomSection, description string) error {
	prealables := ingestPrerequisites[nomSection]
	for _, p := range prealables {
		if err := ajouterNoeudEtape(ctx, pool, noeuds, p); err != nil {
			return err
		}
	}
	// Le total se calcule sur les préalables (DependDe), pas sur la page
	// elle-même : une page n'a pas d'ArchiveOctets/BaseOctets en propre, ce
	// serait compter à vide. Une seule marche (tailleTransitiveMulti,
	// vus partagé) sur TOUS les préalables à la fois — pas un total par
	// préalable additionné ensuite, qui recompterait une dépendance
	// partagée entre deux d'entre eux.
	archiveTotal, baseTotal := tailleTransitiveMulti(noeuds, prealables)
	noeuds[clePage(nomSection)] = noeud{
		Nom: nomSection, Type: "page",
		Commande: "fpctl build " + nomSection, Description: description,
		DependDe:               prealables,
		ArchiveOctetsTransitif: archiveTotal,
		BaseOctetsTransitif:    baseTotal,
	}
	return nil
}

func afficherDeps(ctx context.Context, nom string, enJSON, enPages bool) error {
	// Le graphe lui-même vient du code, jamais de la base — internal/
	// pipeline le republie à chaque exécution, mais un REFLET ne se lit pas
	// à la place de la référence.
	pool := ouvrirBaseBrievement(ctx)
	if pool != nil {
		defer pool.Close()
	}
	noeuds := map[string]noeud{}
	for _, n := range ingest.SocleParlementaire() {
		if err := ajouterNoeudEtape(ctx, pool, noeuds, n); err != nil {
			return err
		}
	}
	if pool != nil {
		// Seule la dernière exécution vient de la base ; la structure
		// (Description, DependDe) reste celle du code, au cas où le reflet
		// publié daterait d'une version antérieure du catalogue.
		etapes, err := pipeline.LireTopologie(ctx, pool)
		if err != nil {
			return err
		}
		for _, e := range etapes {
			if n, ok := noeuds[e.Nom]; ok {
				n.DerniereExecutionReussie = e.DerniereExecutionReussie
				noeuds[e.Nom] = n
			}
		}
	}

	// racines : dans la portée demandée, ce dont rien d'autre dans cette
	// même portée ne dépend — le sommet de chaque arbre qu'on va dérouler.
	// « fpctl list deps » sans argument affiche le socle complet : deux
	// racines (carto, themes), pas une seule, parce que ce n'est justement
	// pas un arbre avant qu'on le déroule.
	var racines []string
	switch {
	case nom == "" && !enPages:
		racines = calculerRacines(noeuds, ingest.SocleParlementaire())
	case nom == "" && enPages:
		for _, section := range buildGroups {
			if err := ajouterNoeudPage(ctx, pool, noeuds, section.name, section.description); err != nil {
				return err
			}
			racines = append(racines, clePage(section.name))
		}
	case ingest.EstSurLeSocle(nom):
		racines = []string{nom}
	default:
		trouve := false
		for _, section := range buildGroups {
			if section.name == nom {
				trouve = true
				if err := ajouterNoeudPage(ctx, pool, noeuds, nom, section.description); err != nil {
					return err
				}
				racines = []string{clePage(nom)}
			}
		}
		if !trouve {
			connus := ingest.SocleParlementaire()
			for _, section := range buildGroups {
				connus = append(connus, section.name)
			}
			sort.Strings(connus)
			return fmt.Errorf("%s inconnu (attendu : %s)", nom, strings.Join(connus, ", "))
		}
	}

	if !enJSON && nom == "" && !enPages {
		if err := afficherDepsArbre(pool, noeuds, racines); err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("pages du site (fpctl build) :")
		var racinesPages []string
		for _, section := range buildGroups {
			if err := ajouterNoeudPage(ctx, pool, noeuds, section.name, section.description); err != nil {
				return err
			}
			racinesPages = append(racinesPages, clePage(section.name))
		}
		return afficherDepsArbre(pool, noeuds, racinesPages)
	}

	if enJSON {
		return afficherDepsJSON(noeuds, racines)
	}
	return afficherDepsArbre(pool, noeuds, racines)
}

// calculerRacines : parmi noms, ceux qu'aucun autre (dans ce même ensemble)
// ne cite dans son DependDe.
func calculerRacines(noeuds map[string]noeud, noms []string) []string {
	dependant := map[string]bool{}
	for _, n := range noms {
		for _, d := range noeuds[n].DependDe {
			dependant[d] = true
		}
	}
	var racines []string
	for _, n := range noms {
		if !dependant[n] {
			racines = append(racines, n)
		}
	}
	return racines
}

func afficherDepsJSON(noeuds map[string]noeud, racines []string) error {
	// Chaque racine ET tout ce dont elle dépend, transitivement — une seule
	// fois par nom même si plusieurs racines ou plusieurs chemins y mènent,
	// puisqu'ici (contrairement à l'arbre) rien n'a besoin d'être dupliqué.
	vus := map[string]bool{}
	var portee []noeud
	var visiter func(nom string)
	visiter = func(nom string) {
		if vus[nom] {
			return
		}
		vus[nom] = true
		n, ok := noeuds[nom]
		if !ok {
			return
		}
		portee = append(portee, n)
		for _, d := range n.DependDe {
			visiter(d)
		}
	}
	for _, r := range racines {
		visiter(r)
	}
	sort.Slice(portee, func(i, j int) bool { return portee[i].Nom < portee[j].Nom })
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(portee)
}

func afficherDepsArbre(pool *pgxpool.Pool, noeuds map[string]noeud, racines []string) error {
	for i, r := range racines {
		imprimerArbre(pool, noeuds, r, "", i == len(racines)-1, true)
	}
	return nil
}

// imprimerArbre dessine nom et ce dont il dépend, récursivement, avec les
// caractères de branche habituels (├──, └──, │) — une étape partagée par
// plusieurs branches y réapparaît sous chacune, dépliant le graphe en arbre
// plutôt que de fusionner ses nœuds partagés (voir le commentaire de
// afficherDeps).
func imprimerArbre(pool *pgxpool.Pool, noeuds map[string]noeud, nom, prefixe string, dernier, racine bool) {
	var ligne strings.Builder
	switch {
	case racine:
	case dernier:
		ligne.WriteString(prefixe + "└── ")
	default:
		ligne.WriteString(prefixe + "├── ")
	}
	n, connu := noeuds[nom]
	if !connu {
		ligne.WriteString(nom + " — inconnu")
		fmt.Println(ligne.String())
		return
	}
	if n.Commande != "" {
		ligne.WriteString(n.Commande)
	} else {
		ligne.WriteString(n.Nom)
	}
	switch {
	case n.Type == "page":
		// Une section de fpctl build n'a pas, pour l'instant, de dernière
		// exécution suivie pour elle seule (contrairement aux étapes
		// d'ingestion, voir internal/pipeline) — rien à ajouter ici.
	case pool == nil && n.DependDe == nil && n.Description == "":
		// Un préalable hors du socle (ex. "exposes") : pas de dernière
		// exécution suivie pour lui, jamais une raison d'écrire « aucune
		// base joignable » pour une donnée qu'on ne calcule de toute façon
		// pas.
	case pool == nil:
		ligne.WriteString(" — dernière exécution inconnue (aucune base joignable)")
	case n.DerniereExecutionReussie != nil:
		ligne.WriteString(" — " + n.DerniereExecutionReussie.Local().Format("2006-01-02 15:04"))
	default:
		ligne.WriteString(" — jamais exécutée avec succès")
	}
	if n.ArchiveOctets > 0 || n.BaseOctets > 0 {
		fmt.Fprintf(&ligne, "  (archive %s, base %s)", tailleLisible(n.ArchiveOctets), tailleLisible(n.BaseOctets))
	}
	fmt.Println(ligne.String())

	var suite string
	if !racine {
		if dernier {
			suite = prefixe + "    "
		} else {
			suite = prefixe + "│   "
		}
	}
	if n.Description != "" {
		fmt.Println(suite + "    " + n.Description)
	}
	if n.Type == "page" && (n.ArchiveOctetsTransitif > 0 || n.BaseOctetsTransitif > 0) {
		fmt.Printf(suite+"    ≈ %s à télécharger, %s en base (préalables compris)\n",
			tailleLisible(n.ArchiveOctetsTransitif), tailleLisible(n.BaseOctetsTransitif))
	}
	for i, d := range n.DependDe {
		imprimerArbre(pool, noeuds, d, suite, i == len(n.DependDe)-1, false)
	}
}

func listerConnecteurs() error {
	cs, err := sources.ListerConnecteurs(".")
	if err != nil {
		return err
	}
	paquet := ""
	for _, c := range cs {
		if c.Paquet != paquet {
			paquet = c.Paquet
			fmt.Printf("%s\n", paquet)
		}
		fmt.Printf("  %s\n", c.Fonction)
	}
	fmt.Printf("\n%d connecteurs, %d paquets\n", len(cs), nombrePaquets(cs))
	return nil
}

func nombrePaquets(cs []sources.Connecteur) int {
	vus := map[string]bool{}
	for _, c := range cs {
		vus[c.Paquet] = true
	}
	return len(vus)
}

func afficherStats(ctx context.Context) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	schemas, err := stats.Resume(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Printf("%-12s %8s %14s %10s\n", "schéma", "tables", "lignes (est.)", "taille")
	var totalOctets, totalLignes int64
	for _, s := range schemas {
		fmt.Printf("%-12s %8d %14d %10s\n", s.Nom, s.Tables, s.LignesEstimee, tailleLisible(s.Octets))
		totalOctets += s.Octets
		totalLignes += s.LignesEstimee
	}
	fmt.Printf("%-12s %8s %14d %10s\n", "total", "", totalLignes, tailleLisible(totalOctets))

	grosses, err := stats.PlusGrossesTables(ctx, pool, 10)
	if err != nil {
		return err
	}
	fmt.Printf("\nplus grosses tables :\n")
	for _, t := range grosses {
		fmt.Printf("  %-10s %-40s %12d lignes  %10s\n", t.Schema, t.Nom, t.LignesEstimee, tailleLisible(t.Octets))
	}
	return nil
}

// afficherSections liste les deux catalogues que internal/sitegen.Sections()/
// Topics() exposent — jamais recopiés à la main ici, la même source que
// cmd/fpctl/build.go valide contre pour "section <nom>"/"topic <id>".
// afficherSections liste les sections connues de sitegen.Sections()/
// Topics() — sans argument, la vue complète (chaque section avec son
// groupe, chaque groupe avec ses sections, chaque sujet) ; avec le nom
// d'un groupe (« dossiers », « indicateurs »...), seulement ses sections.
func afficherSections(groupe string) error {
	groupeDe := map[string]string{}
	for _, g := range buildGroups {
		for _, s := range g.sections {
			groupeDe[s] = g.name
		}
	}

	if groupe != "" {
		for _, g := range buildGroups {
			if g.name == groupe {
				fmt.Printf("fpctl build %s : %s\n", g.name, g.description)
				for _, s := range g.sections {
					fmt.Printf("  %s\n", s)
				}
				return nil
			}
		}
		var connus []string
		for _, g := range buildGroups {
			connus = append(connus, g.name)
		}
		return fmt.Errorf("groupe inconnu : %s (connus : %s)", groupe, strings.Join(connus, ", "))
	}

	fmt.Println("sections (fpctl build section <nom>) :")
	for _, s := range sitegen.Sections() {
		if g, ok := groupeDe[s]; ok {
			fmt.Printf("  %-20s dans le groupe « fpctl build %s »\n", s, g)
		} else {
			fmt.Printf("  %-20s\n", s)
		}
	}

	fmt.Println("\ngroupes (fpctl build <groupe>, ou fpctl list sections <groupe> pour le détail) :")
	for _, g := range buildGroups {
		fmt.Printf("  %-14s %s — %s\n", g.name, g.description, strings.Join(g.sections, ", "))
	}

	sujets := sitegen.Topics()
	fmt.Printf("\nsujets de campagne (fpctl build topic <id>) : %d — %s\n",
		len(sujets), strings.Join(sujets, ", "))
	return nil
}

func afficherMatviews(ctx context.Context) error {
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	etats, err := matview.Lister(ctx, pool)
	if err != nil {
		return err
	}
	// Largeur calculée sur le plus long nom réel plutôt qu'une constante :
	// une constante trop courte (mv.commune_association_count dépassait déjà
	// %-24s) désaligne toutes les colonnes qui suivent dès qu'un nom la
	// dépasse, pas seulement sa propre ligne.
	largeur := len("matvue")
	for _, e := range etats {
		if n := len("mv." + e.Nom); n > largeur {
			largeur = n
		}
	}
	fmt.Printf("%-*s %10s %20s  %s\n", largeur, "matvue", "lignes", "actualisée le", "tables source")
	for _, e := range etats {
		nom := "mv." + e.Nom
		if !e.Connue {
			fmt.Printf("%-*s %10s %20s  %s\n", largeur, nom, "—", "jamais", strings.Join(e.Tables, ", "))
			continue
		}
		fmt.Printf("%-*s %10d %20s  %s\n", largeur, nom, e.Lignes,
			e.ActualiseeLe.Local().Format("2006-01-02 15:04"), strings.Join(e.Tables, ", "))
	}
	return nil
}

func tailleLisible(octets int64) string {
	const unite = 1024.0
	v := float64(octets)
	for _, suffixe := range []string{"o", "Ko", "Mo", "Go", "To"} {
		if v < unite {
			return fmt.Sprintf("%.1f %s", v, suffixe)
		}
		v /= unite
	}
	return fmt.Sprintf("%.1f Po", v)
}
