// Package sources écrit le catalogue JSON de toutes les sources ingérées.
// Appelé par fpctl (voir cmd/fpctl) : fpctl list sources.
//
// Ne déclare rien qui ne soit mesuré : chaque champ vient de raw.source,
// raw.fetch_run, raw.retrieval, raw.document, ou d'une introspection des
// tables core/ref qui portent un source_id — jamais d'une valeur recopiée à
// la main, pour ne jamais dériver de la réalité de la base au fil des
// chantiers (docs/perimetre.md §2).
package sources

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Table struct {
	Schema            string `json:"schema"`
	Table             string `json:"table"`
	Lignes            int64  `json:"lignes_totales_estimees"`
	LignesSource      int64  `json:"lignes_pour_cette_source"`
	TailleOctets      int64  `json:"taille_octets"`
	AnneeMin          *int   `json:"annee_min"`
	AnneeMax          *int   `json:"annee_max"`
	ColonneHistorique string `json:"colonne_historique,omitempty"`
}

type Source struct {
	Slug       string `json:"slug"`
	Label      string `json:"label"`
	Publisher  string `json:"publisher"`
	Tier       string `json:"tier"`
	Licence    string `json:"licence"`
	ReuseClass string `json:"reuse_class"`
	Cadence    string `json:"cadence_attendue"`
	Notes      string `json:"notes,omitempty"`

	DerniereIngestionOK string `json:"derniere_ingestion_reussie"`
	DernierStatut       string `json:"dernier_statut_run"`
	NombreRuns          int64  `json:"nombre_runs"`
	ConnecteurVersion   string `json:"connecteur_derniere_version"`

	FormatDetecte string   `json:"format_detecte"`
	URLsOrigine   []string `json:"urls_origine"`

	NombreDocuments    int64    `json:"nombre_documents"`
	TailleLocaleOctets int64    `json:"taille_locale_octets"`
	CheminsLocaux      []string `json:"chemins_locaux"`
	CleS3              *string  `json:"cle_s3"` // toujours null tant que le bucket n'existe pas — voir docs/ci-pipeline.md

	// StockageVerifie : "disque", "s3", ou "" si -verify-store=none (défaut) —
	// contre quel support DocumentsManquants a été compté. Vide ne veut pas
	// dire « tout est là », ça veut dire « pas vérifié ».
	StockageVerifie    string `json:"stockage_verifie"`
	DocumentsManquants int    `json:"documents_manquants"`

	Tables []Table `json:"tables"`

	IngestionIncrementale string `json:"ingestion_incrementale"`
}

type Catalogue struct {
	GenereLe      string   `json:"genere_le"`
	Note          string   `json:"note"`
	RacineLocale  string   `json:"racine_locale"`
	NombreSources int      `json:"nombre_sources"`
	Sources       []Source `json:"sources"`
}

// Run exécute la commande sources. Appelée par fpctl, qui route vers ce
// paquet plutôt que de dupliquer son analyse d'options. ctx est celui de
// fpctl (cmd.Context()), déjà annulé au premier signal.
func Run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sources", flag.ContinueOnError)
	out := fs.String("out", "docs/catalogue-sources.json", "fichier JSON à écrire")
	racine := fs.String("raw-root", "raw", "racine locale de l'archive scellée")
	verifierStockage := fs.String("verify-store", "none",
		"vérifie la présence des documents : none (défaut, pas de vérification) | disk | s3")
	bucket := fs.String("bucket", "fp-archive", "bucket à interroger si -verify-store=s3")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	var empreintes EmpreintesStockage
	switch *verifierStockage {
	case "none":
	case "disk":
		if empreintes, err = StockageDisque(*racine); err != nil {
			return fmt.Errorf("vérification disque : %w", err)
		}
	case "s3":
		if empreintes, err = StockageS3(ctx, *bucket); err != nil {
			return fmt.Errorf("vérification s3 : %w", err)
		}
	default:
		return fmt.Errorf("-verify-store=%s inconnu (attendu : none, disk, s3)", *verifierStockage)
	}

	cat, err := construire(ctx, pool, *racine, *verifierStockage, empreintes)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("catalogue écrit : %s (%d sources)\n", *out, cat.NombreSources)
	return nil
}

func construire(ctx context.Context, pool *pgxpool.Pool, racine, nomStockageVerifie string, empreintes EmpreintesStockage) (*Catalogue, error) {
	sources, err := chargerSources(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("sources : %w", err)
	}

	tablesParSource, err := tablesSourceID(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("introspection tables : %w", err)
	}

	// Le connecteur qui déclare un slug donné : cherché dans le code plutôt que
	// documenté à la main, pour ne jamais devenir faux au premier connecteur
	// ajouté après coup.
	fichiersParSlug, err := connecteurParSlug(".")
	if err != nil {
		return nil, fmt.Errorf("recherche des connecteurs : %w", err)
	}

	// Une passe PAR TABLE, pas par (source × table) : sur ~80 sources et ~40
	// tables, compter par source dans une boucle imbriquée revenait à 3 000+
	// balayages de table, dont plusieurs sur des tables à plusieurs millions de
	// lignes (délinquance, corpus JO). Un seul GROUP BY source_id par table
	// donne le même résultat en une passe.
	idParSlug := map[string]int64{}
	rowsID, err := pool.Query(ctx, `SELECT id, slug FROM raw.source`)
	if err != nil {
		return nil, fmt.Errorf("id des sources : %w", err)
	}
	for rowsID.Next() {
		var id int64
		var slug string
		if err := rowsID.Scan(&id, &slug); err != nil {
			rowsID.Close()
			return nil, err
		}
		idParSlug[slug] = id
	}
	rowsID.Close()
	if err := rowsID.Err(); err != nil {
		return nil, err
	}

	type tableEnrichie struct {
		Table
		parSource map[int64]int64
	}
	var tablesEnrichies []tableEnrichie
	for _, t := range tablesParSource {
		parSource, err := comptageParSourceGroupe(ctx, pool, t.Schema, t.Table)
		if err != nil {
			return nil, fmt.Errorf("%s.%s : %w", t.Schema, t.Table, err)
		}
		lignes, taille, err := tailleTable(ctx, pool, t.Schema, t.Table)
		if err != nil {
			return nil, fmt.Errorf("taille %s.%s : %w", t.Schema, t.Table, err)
		}
		amin, amax, col, err := empriseHistorique(ctx, pool, t.Schema, t.Table)
		if err != nil {
			return nil, fmt.Errorf("emprise %s.%s : %w", t.Schema, t.Table, err)
		}
		t.Lignes, t.TailleOctets = lignes, taille
		t.AnneeMin, t.AnneeMax, t.ColonneHistorique = amin, amax, col
		tablesEnrichies = append(tablesEnrichies, tableEnrichie{Table: t, parSource: parSource})
	}

	for i := range sources {
		s := &sources[i]
		// Tableaux vides sérialisés en [] plutôt qu'en null : une source sans
		// document ou sans table rattachée est un fait à afficher, pas une
		// absence de champ à deviner côté lecteur du JSON.
		s.URLsOrigine = []string{}
		s.CheminsLocaux = []string{}
		s.Tables = []Table{}

		urls, docs, octets, chemins, tailles, err := documentsDeSource(ctx, pool, s.Slug)
		if err != nil {
			return nil, fmt.Errorf("%s : %w", s.Slug, err)
		}
		if urls != nil {
			s.URLsOrigine = urls
		}
		s.NombreDocuments = docs
		s.TailleLocaleOctets = octets
		if chemins != nil {
			s.CheminsLocaux = chemins
		}
		s.FormatDetecte = detecterFormat(urls, chemins)
		if empreintes != nil {
			s.StockageVerifie = nomStockageVerifie
			s.DocumentsManquants = empreintes.Manquants(chemins, tailles)
		}

		srcID, ok := idParSlug[s.Slug]
		if !ok {
			return nil, fmt.Errorf("%s : id introuvable", s.Slug)
		}
		for _, te := range tablesEnrichies {
			n, ok := te.parSource[srcID]
			if !ok || n == 0 {
				continue // cette table n'appartient pas à cette source
			}
			tt := te.Table
			tt.LignesSource = n
			s.Tables = append(s.Tables, tt)
		}

		s.IngestionIncrementale = classerIncremental(fichiersParSlug[s.Slug])
	}

	return &Catalogue{
		GenereLe: nowRFC3339(),
		Note: "Généré par fpctl list sources — chaque champ vient d'une requête sur raw.*/core.*, " +
			"aucun n'est recopié à la main. cle_s3 reste null tant que le bucket fp-archive " +
			"(docs/ci-pipeline.md) n'existe pas ; à remplir le jour où l'archive y est synchronisée.",
		RacineLocale:  racine,
		NombreSources: len(sources),
		Sources:       sources,
	}, nil
}

func chargerSources(ctx context.Context, pool *pgxpool.Pool) ([]Source, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.slug, s.label, s.publisher, s.tier::text, s.licence, s.reuse_class::text,
		       coalesce(s.expected_cadence,''), coalesce(s.notes,''),
		       coalesce(to_char(max(fr.finished_at) FILTER (WHERE fr.status='SUCCESS'),'YYYY-MM-DD"T"HH24:MI:SS'),''),
		       coalesce((SELECT fr2.status FROM raw.fetch_run fr2
		                  WHERE fr2.source_id = s.id AND fr2.finished_at IS NOT NULL
		                  ORDER BY fr2.finished_at DESC LIMIT 1), ''),
		       count(fr.id),
		       coalesce((SELECT fr3.connector_version FROM raw.fetch_run fr3
		                  WHERE fr3.source_id = s.id
		                  ORDER BY fr3.started_at DESC LIMIT 1), '')
		FROM raw.source s
		LEFT JOIN raw.fetch_run fr ON fr.source_id = s.id
		GROUP BY s.id, s.slug, s.label, s.publisher, s.tier, s.licence, s.reuse_class,
		         s.expected_cadence, s.notes
		ORDER BY s.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.Slug, &s.Label, &s.Publisher, &s.Tier, &s.Licence, &s.ReuseClass,
			&s.Cadence, &s.Notes, &s.DerniereIngestionOK, &s.DernierStatut, &s.NombreRuns,
			&s.ConnecteurVersion); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func documentsDeSource(ctx context.Context, pool *pgxpool.Pool, slug string) (urls []string, docs, octets int64, chemins []string, tailles map[string]int64, err error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT r.url FROM raw.retrieval r
		JOIN raw.source s ON s.id = r.source_id
		WHERE s.slug = $1 AND r.document_id IS NOT NULL
		ORDER BY r.url`, slug)
	if err != nil {
		return nil, 0, 0, nil, nil, err
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return nil, 0, 0, nil, nil, err
		}
		urls = append(urls, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, 0, nil, nil, err
	}

	drows, err := pool.Query(ctx, `
		SELECT DISTINCT d.storage_key, d.byte_size FROM raw.retrieval r
		JOIN raw.source s ON s.id = r.source_id
		JOIN raw.document d ON d.id = r.document_id
		WHERE s.slug = $1
		ORDER BY d.storage_key`, slug)
	if err != nil {
		return nil, 0, 0, nil, nil, err
	}
	defer drows.Close()
	tailles = map[string]int64{}
	for drows.Next() {
		var key string
		var size int64
		if err := drows.Scan(&key, &size); err != nil {
			return nil, 0, 0, nil, nil, err
		}
		chemins = append(chemins, key)
		tailles[key] = size
		octets += size
		docs++
	}
	return urls, docs, octets, chemins, tailles, drows.Err()
}

// tablesSourceID introspecte le schéma : toute table core.*/ref.*/derived.*
// avec une colonne source_id est une table de fait alimentée par une source
// scellée — la même convention appliquée sans exception depuis le début du
// projet (voir par ex. core.prime_activite_effectif.source_id).
func tablesSourceID(ctx context.Context, pool *pgxpool.Pool) ([]Table, error) {
	rows, err := pool.Query(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.columns
		WHERE column_name = 'source_id'
		  AND table_schema IN ('core','ref','derived')
		ORDER BY table_schema, table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Table
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Schema, &t.Table); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// comptageParSourceGroupe fait un seul passage sur la table (GROUP BY
// source_id) plutôt qu'un COUNT(*) filtré par source répété pour chacune des
// dizaines de sources possibles.
func comptageParSourceGroupe(ctx context.Context, pool *pgxpool.Pool, schema, table string) (map[int64]int64, error) {
	ident := pgx.Identifier{schema, table}.Sanitize()
	rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT source_id, count(*) FROM %s GROUP BY source_id`, ident))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id *int64
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		if id == nil {
			continue // source_id NULL : ligne non attribuable à une source précise
		}
		out[*id] = n
	}
	return out, rows.Err()
}

// tailleTable donne le total de lignes (estimation du planificateur — un
// signal de complexité, pas un compte exact ; certaines tables dépassent le
// million de lignes et un COUNT(*) exact les rendrait coûteuses à répéter à
// chaque génération) et la taille sur disque de la table — partagés par
// toutes les sources qui l'alimentent, à ne pas confondre avec LignesSource
// (le compte exact, filtré sur une seule source).
func tailleTable(ctx context.Context, pool *pgxpool.Pool, schema, table string) (lignes, octets int64, err error) {
	ident := pgx.Identifier{schema, table}.Sanitize()
	if err = pool.QueryRow(ctx, `SELECT reltuples::bigint FROM pg_class WHERE oid = $1::regclass`, ident).Scan(&lignes); err != nil {
		return 0, 0, err
	}
	err = pool.QueryRow(ctx, `SELECT pg_total_relation_size($1::regclass)`, ident).Scan(&octets)
	return lignes, octets, err
}

var reAnnee = regexp.MustCompile(`(?i)^(annee|année)$|annee$|^annee_`)

// empriseHistorique cherche une colonne « annee » (la convention quasi
// systématique de ce dépôt) et en donne le min/max. Aucune colonne trouvée :
// renvoie des bornes nulles plutôt qu'une supposition — le non-détecté est
// une réponse valide (docs/decisions.md D-003), pas une erreur à masquer.
func empriseHistorique(ctx context.Context, pool *pgxpool.Pool, schema, table string) (*int, *int, string, error) {
	rows, err := pool.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema=$1 AND table_name=$2
		  AND data_type IN ('smallint','integer','bigint')`, schema, table)
	if err != nil {
		return nil, nil, "", err
	}
	var candidates []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return nil, nil, "", err
		}
		if reAnnee.MatchString(c) {
			candidates = append(candidates, c)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, "", err
	}
	if len(candidates) == 0 {
		return nil, nil, "", nil
	}
	col := candidates[0]
	// « annee » lui-même passe avant toute variante composée (premiere_annee_...).
	for _, c := range candidates {
		if strings.EqualFold(c, "annee") {
			col = c
			break
		}
	}
	ident := pgx.Identifier{schema, table}.Sanitize()
	colIdent := pgx.Identifier{col}.Sanitize()
	var min, max *int
	err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT min(%s), max(%s) FROM %s`, colIdent, colIdent, ident)).Scan(&min, &max)
	if err != nil {
		return nil, nil, "", err
	}
	return min, max, col, nil
}

// connecteurParSlug associe un slug de source au contenu du fichier Go qui le
// déclare (var Source... = archive.Source{Slug: "...", ...}), pour décider si
// l'ingestion est une reconstruction complète ou un upsert sans le documenter
// à la main dans un registre qui dériverait du code.
func connecteurParSlug(racine string) (map[string]string, error) {
	out := map[string]string{}
	reSlug := regexp.MustCompile(`Slug:\s*"([^"]+)"`)
	err := walkGo(racine, func(path, contenu string) error {
		for _, m := range reSlug.FindAllStringSubmatch(contenu, -1) {
			// Le même fichier héberge parfois plusieurs sources ; chacune reçoit
			// le contenu entier du fichier, suffisant pour repérer DELETE/ON CONFLICT.
			if _, exists := out[m[1]]; !exists {
				out[m[1]] = contenu
			}
		}
		return nil
	})
	return out, err
}

func walkGo(racine string, fn func(path, contenu string) error) error {
	return filepath.WalkDir(racine, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			nom := d.Name()
			if nom == "site" || nom == "site.construction" || nom == "site.precedent" ||
				nom == "raw" || nom == ".git" || strings.HasPrefix(nom, "site-v1-") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return fn(path, string(b))
	})
}

func classerIncremental(contenu string) string {
	if contenu == "" {
		return "connecteur non retrouvé automatiquement — à vérifier manuellement"
	}
	aDelete := strings.Contains(contenu, "DELETE FROM")
	aConflit := strings.Contains(contenu, "ON CONFLICT")
	switch {
	case aDelete && !aConflit:
		return "non — reconstruction complète (DELETE puis COPY) à chaque exécution"
	case aConflit && !aDelete:
		return "oui — upsert (ON CONFLICT) sans purge préalable"
	case aDelete && aConflit:
		return "mixte — DELETE et ON CONFLICT présents dans le même fichier, à vérifier lequel s'applique à cette table"
	default:
		return "non déterminé automatiquement (ni DELETE FROM ni ON CONFLICT trouvé) — à vérifier manuellement"
	}
}

func detecterFormat(urls, chemins []string) string {
	tout := strings.ToLower(strings.Join(append(append([]string{}, urls...), chemins...), " "))
	switch {
	case strings.Contains(tout, "opendatasoft") || strings.Contains(tout, "/exports/json"):
		return "API Opendatasoft (export JSON)"
	case strings.Contains(tout, "eurostat"):
		return "API Eurostat (JSON-stat)"
	case strings.Contains(tout, "melodi"):
		return "API Insee Melodi (JSON)"
	case strings.HasSuffix(tout, ".xlsx") || strings.Contains(tout, ".xlsx"):
		return "Fichier XLSX"
	case strings.HasSuffix(tout, ".csv") || strings.Contains(tout, ".csv"):
		return "Fichier CSV"
	case strings.Contains(tout, ".json"):
		return "Fichier JSON"
	case strings.Contains(tout, ".xml"):
		return "Fichier XML"
	case tout == "":
		return "aucun document récupéré à ce jour"
	default:
		return "format non reconnu automatiquement"
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
