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
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
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
	)
	return cmd
}

func commandeDeps() *cobra.Command {
	var enJSON bool
	cmd := &cobra.Command{
		Use:   "deps [nom]",
		Short: "Graphe de dépendances du socle parlementaire (download, partis, normalize, carto, senat, europe, themes)",
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
			"Affiché en arbre : c'est un graphe orienté acyclique, pas un arbre\n" +
			"(normalize a deux « parents », senat et europe, tous deux exigés par\n" +
			"themes), à plusieurs racines (ce dont rien ne dépend — carto et\n" +
			"themes, dans le socle complet). Rendu quand même comme un arbre, ce\n" +
			"qu'il redevient une fois déroulé : une étape partagée réapparaît sous\n" +
			"chacun de ses parents plutôt que d'être fusionnée en un seul nœud.\n\n" +
			"Avec un nom, limite l'affichage à une seule chose : l'une des sept\n" +
			"étapes (racine de son propre arbre), ou une section de fpctl build\n" +
			"(scrutin, communes, reste — une racine par préalable d'ingestion).\n\n" +
			"--json écrit la liste des étapes concernées à plat (un objet par\n" +
			"étape, depend_de nommant les autres par leur nom) plutôt que\n" +
			"l'arbre déroulé — la forme qu'un outil reconstruit plus facilement\n" +
			"que des lignes indentées.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nom string
			if len(args) > 0 {
				nom = args[0]
			}
			return executerInterne(cmd.Context(), afficherDeps(cmd.Context(), nom, enJSON))
		},
	}
	cmd.Flags().BoolVar(&enJSON, "json", false, "écrit la liste des étapes à plat, en JSON, plutôt que l'arbre")
	return cmd
}

// composantesEtape : ce que chaque étape du socle parlementaire remplit,
// pour afficher combien ça pèse à côté du graphe — tenu à la main, comme
// ingestPrealables (cmd/fpctl/build.go) : ces sept étapes sont assez rares
// et assez stables pour que ça reste à jour sans registre séparé.
//
// SlugsSources : les documents archivés (raw.document, via raw.retrieval)
// que cette étape télécharge — vide pour une étape qui n'en télécharge
// aucun. Tables : ce qu'elle écrit dans core/derived, mesuré par
// pg_total_relation_size — une table peut apparaître sous plusieurs étapes
// (carto.IngestPresidents écrit dans core.person/core.mandate, que
// normalize remplit aussi) : la taille affichée est alors celle de TOUTE
// la table, pas la part de cette seule étape — une approximation
// assumée pour un résumé de terminal, pas une comptabilité exacte.
var composantesEtape = map[string]struct {
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

// tailleEtape additionne les octets archivés (SlugsSources) et occupés en
// base (Tables) d'une étape — 0, sans erreur, pour une étape non déclarée
// dans composantesEtape (rien à afficher plutôt qu'un blocage), ou quand
// aucune base n'est joignable (pool nil : voir ouvrirBaseBrievement).
func tailleEtape(ctx context.Context, pool *pgxpool.Pool, nom string) (archive, base int64, err error) {
	if pool == nil {
		return 0, 0, nil
	}
	c, ok := composantesEtape[nom]
	if !ok {
		return 0, 0, nil
	}
	if len(c.SlugsSources) > 0 {
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
type noeud struct {
	Nom                      string     `json:"nom"`
	Description              string     `json:"description"`
	DerniereExecutionReussie *time.Time `json:"derniere_execution_reussie"`
	ArchiveOctets            int64      `json:"archive_octets"`
	BaseOctets               int64      `json:"base_octets"`
	DependDe                 []string   `json:"depend_de"`
}

func afficherDeps(ctx context.Context, nom string, enJSON bool) error {
	// Le graphe lui-même vient du code, jamais de la base — internal/
	// pipeline le republie à chaque exécution, mais un REFLET ne se lit pas
	// à la place de la référence.
	pool := ouvrirBaseBrievement(ctx)
	if pool != nil {
		defer pool.Close()
	}
	noeuds := map[string]noeud{}
	for _, n := range ingest.SocleParlementaire() {
		s, ok := ingest.SourceParNom(n)
		if !ok {
			return fmt.Errorf("%s déclaré dans le socle parlementaire mais absent du catalogue", n)
		}
		archive, base, err := tailleEtape(ctx, pool, n)
		if err != nil {
			return fmt.Errorf("%s : %w", n, err)
		}
		noeuds[n] = noeud{Nom: n, Description: s.Description, DependDe: s.Dependances, ArchiveOctets: archive, BaseOctets: base}
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
	case nom == "":
		racines = calculerRacines(noeuds, ingest.SocleParlementaire())
	case ingest.EstSurLeSocle(nom):
		racines = []string{nom}
	default:
		prealables, ok := ingestPrealables[nom]
		if !ok {
			connus := ingest.SocleParlementaire()
			for s := range ingestPrealables {
				connus = append(connus, s)
			}
			sort.Strings(connus)
			return fmt.Errorf("%s inconnu (attendu : %s)", nom, strings.Join(connus, ", "))
		}
		for _, p := range prealables {
			if _, ok := noeuds[p]; !ok {
				description := p
				if s, ok := ingest.SourceParNom(p); ok {
					description = s.Description
				}
				noeuds[p] = noeud{Nom: p, Description: description}
			}
		}
		racines = prealables
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
	ligne.WriteString(nom)

	n, connu := noeuds[nom]
	if !connu {
		fmt.Println(ligne.String() + " — inconnu")
		return
	}
	ligne.WriteString(" — ")
	switch {
	case pool == nil && n.DependDe == nil && n.Description == "":
		// Un préalable hors du socle (ex. "exposes") : pas de dernière
		// exécution suivie pour lui, jamais une raison d'écrire « aucune
		// base joignable » pour une donnée qu'on ne calcule de toute façon
		// pas.
	case pool == nil:
		ligne.WriteString("dernière exécution inconnue (aucune base joignable)")
	case n.DerniereExecutionReussie != nil:
		ligne.WriteString(n.DerniereExecutionReussie.Local().Format("2006-01-02 15:04"))
	default:
		ligne.WriteString("jamais exécutée avec succès")
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
