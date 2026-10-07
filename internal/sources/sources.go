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
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Table struct {
	Schema        string `json:"schema"`
	Table         string `json:"table"`
	Rows          int64  `json:"lignes_totales_estimees"`
	SourceRows    int64  `json:"lignes_pour_cette_source"`
	SizeBytes     int64  `json:"taille_octets"`
	YearMin       *int   `json:"annee_min"`
	YearMax       *int   `json:"annee_max"`
	HistoryColumn string `json:"colonne_historique,omitempty"`
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

	LastIngestionOK  string `json:"derniere_ingestion_reussie"`
	LastRunStatus    string `json:"dernier_statut_run"`
	RunCount         int64  `json:"nombre_runs"`
	ConnectorVersion string `json:"connecteur_derniere_version"`

	DetectedFormat string   `json:"format_detecte"`
	OriginURLs     []string `json:"urls_origine"`

	DocumentCount  int64    `json:"nombre_documents"`
	LocalSizeBytes int64    `json:"taille_locale_octets"`
	LocalPaths     []string `json:"chemins_locaux"`
	S3Key          *string  `json:"cle_s3"` // toujours null tant que le bucket n'existe pas — voir docs/ci-pipeline.md

	// StorageChecked : "disque", "s3", ou "" si -verify-store=none (défaut) —
	// contre quel support MissingDocuments a été compté. Vide ne veut pas
	// dire « tout est là », ça veut dire « pas vérifié ».
	StorageChecked   string `json:"stockage_verifie"`
	MissingDocuments int    `json:"documents_manquants"`

	Tables []Table `json:"tables"`

	IncrementalIngestion string `json:"ingestion_incrementale"`
}

type Catalog struct {
	GeneratedAt string   `json:"genere_le"`
	Note        string   `json:"note"`
	LocalRoot   string   `json:"racine_locale"`
	SourceCount int      `json:"nombre_sources"`
	Sources     []Source `json:"sources"`
}

// Run exécute la commande sources. Appelée par fpctl, qui route vers ce
// paquet plutôt que de dupliquer son analyse d'options. ctx est celui de
// fpctl (cmd.Context()), déjà annulé au premier signal.
func Run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sources", flag.ContinueOnError)
	out := fs.String("out", "docs/catalogue-sources.json", "fichier JSON à écrire")
	root := fs.String("raw-root", "raw", "racine locale de l'archive scellée")
	verifyStore := fs.String("verify-store", "none",
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

	var footprint StorageFootprint
	switch *verifyStore {
	case "none":
	case "disk":
		if footprint, err = DiskFootprint(*root); err != nil {
			return fmt.Errorf("vérification disque : %w", err)
		}
	case "s3":
		if footprint, err = S3Footprint(ctx, *bucket); err != nil {
			return fmt.Errorf("vérification s3 : %w", err)
		}
	default:
		return fmt.Errorf("-verify-store=%s inconnu (attendu : none, disk, s3)", *verifyStore)
	}

	cat, err := build(ctx, pool, *root, *verifyStore, footprint)
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
	fmt.Printf("catalogue écrit : %s (%d sources)\n", *out, cat.SourceCount)
	showSizes(cat.Sources)
	return nil
}

// showSizes résume, sur le terminal, ce que le catalogue JSON détaille
// déjà par source (Source.LocalSizeBytes, Source.Tables[].SizeBytes)
// — le fichier reste la référence, ceci n'en est qu'une lecture rapide : les
// dix sources qui pèsent le plus, archive scellée et tables core/ref
// confondues, plus le total sur l'ensemble du catalogue.
func showSizes(src []Source) {
	type row struct {
		slug          string
		archive, base int64
		documents     int64
	}
	rows := make([]row, len(src))
	var totalArchive, totalBase int64
	for i, s := range src {
		var base int64
		for _, t := range s.Tables {
			base += t.SizeBytes
		}
		rows[i] = row{slug: s.Slug, archive: s.LocalSizeBytes, base: base, documents: s.DocumentCount}
		totalArchive += s.LocalSizeBytes
		totalBase += base
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].archive+rows[i].base > rows[j].archive+rows[j].base
	})

	fmt.Printf("\n%-28s %10s %14s %14s\n", "source", "documents", "archive (raw)", "base (core/ref)")
	n := len(rows)
	if n > 10 {
		n = 10
	}
	for _, r := range rows[:n] {
		fmt.Printf("%-28s %10d %14s %14s\n", r.slug, r.documents, humanSize(r.archive), humanSize(r.base))
	}
	if len(rows) > n {
		fmt.Printf("... et %d autres sources (voir le catalogue JSON pour le détail)\n", len(rows)-n)
	}
	fmt.Printf("%-28s %10s %14s %14s\n", "total", "", humanSize(totalArchive), humanSize(totalBase))
}

// humanSize : un nombre d'octets en unité lisible, la même échelle que
// fpctl list stats (cmd/fpctl/list.go) — dupliquée plutôt que partagée : un
// paquet interne n'a pas à dépendre de cmd/fpctl pour cinq lignes.
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

func build(ctx context.Context, pool *pgxpool.Pool, root, storageCheckName string, footprint StorageFootprint) (*Catalog, error) {
	sources, err := loadSources(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("sources : %w", err)
	}

	tablesBySource, err := tablesWithSourceID(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("introspection tables : %w", err)
	}

	// Le connecteur qui déclare un slug donné : cherché dans le code plutôt que
	// documenté à la main, pour ne jamais devenir faux au premier connecteur
	// ajouté après coup.
	filesBySlug, err := connectorBySlug(".")
	if err != nil {
		return nil, fmt.Errorf("recherche des connecteurs : %w", err)
	}

	// Une passe PAR TABLE, pas par (source × table) : sur ~80 sources et ~40
	// tables, compter par source dans une boucle imbriquée revenait à 3 000+
	// balayages de table, dont plusieurs sur des tables à plusieurs millions de
	// lignes (délinquance, corpus JO). Un seul GROUP BY source_id par table
	// donne le même résultat en une passe.
	idBySlug := map[string]int64{}
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
		idBySlug[slug] = id
	}
	rowsID.Close()
	if err := rowsID.Err(); err != nil {
		return nil, err
	}

	type enrichedTable struct {
		Table
		bySource map[int64]int64
	}
	// Le seul endroit lent de cette commande : un GROUP BY, un COUNT/taille
	// et une emprise historique PAR TABLE — jusqu'à une minute, sur des
	// tables à plusieurs millions de lignes (délinquance, corpus JO), et
	// rien à montrer avant la toute fin sans ce log : un terminal silencieux
	// dix secondes ne se distingue pas d'un outil planté.
	logs.Notice("measuring " + logs.Plural(len(tablesBySource), "table"))
	var enrichedTables []enrichedTable
	for i, t := range tablesBySource {
		logs.Notice(fmt.Sprintf("[%d/%d] %s.%s", i+1, len(tablesBySource), t.Schema, t.Table))
		bySource, err := countBySourceGrouped(ctx, pool, t.Schema, t.Table)
		if err != nil {
			return nil, fmt.Errorf("%s.%s : %w", t.Schema, t.Table, err)
		}
		rows, size, err := tableSize(ctx, pool, t.Schema, t.Table)
		if err != nil {
			return nil, fmt.Errorf("taille %s.%s : %w", t.Schema, t.Table, err)
		}
		ymin, ymax, col, err := yearRange(ctx, pool, t.Schema, t.Table)
		if err != nil {
			return nil, fmt.Errorf("emprise %s.%s : %w", t.Schema, t.Table, err)
		}
		t.Rows, t.SizeBytes = rows, size
		t.YearMin, t.YearMax, t.HistoryColumn = ymin, ymax, col
		enrichedTables = append(enrichedTables, enrichedTable{Table: t, bySource: bySource})
	}

	for i := range sources {
		s := &sources[i]
		// Tableaux vides sérialisés en [] plutôt qu'en null : une source sans
		// document ou sans table rattachée est un fait à afficher, pas une
		// absence de champ à deviner côté lecteur du JSON.
		s.OriginURLs = []string{}
		s.LocalPaths = []string{}
		s.Tables = []Table{}

		urls, docs, bytes, paths, sizes, err := sourceDocuments(ctx, pool, s.Slug)
		if err != nil {
			return nil, fmt.Errorf("%s : %w", s.Slug, err)
		}
		if urls != nil {
			s.OriginURLs = urls
		}
		s.DocumentCount = docs
		s.LocalSizeBytes = bytes
		if paths != nil {
			s.LocalPaths = paths
		}
		s.DetectedFormat = detectFormat(urls, paths)
		if footprint != nil {
			s.StorageChecked = storageCheckName
			s.MissingDocuments = footprint.Missing(paths, sizes)
		}

		srcID, ok := idBySlug[s.Slug]
		if !ok {
			return nil, fmt.Errorf("%s : id introuvable", s.Slug)
		}
		for _, te := range enrichedTables {
			n, ok := te.bySource[srcID]
			if !ok || n == 0 {
				continue // cette table n'appartient pas à cette source
			}
			tt := te.Table
			tt.SourceRows = n
			s.Tables = append(s.Tables, tt)
		}

		s.IncrementalIngestion = classifyIncremental(filesBySlug[s.Slug])
	}

	return &Catalog{
		GeneratedAt: nowRFC3339(),
		Note: "Généré par fpctl list sources — chaque champ vient d'une requête sur raw.*/core.*, " +
			"aucun n'est recopié à la main. cle_s3 reste null tant que le bucket fp-archive " +
			"(docs/ci-pipeline.md) n'existe pas ; à remplir le jour où l'archive y est synchronisée.",
		LocalRoot:   root,
		SourceCount: len(sources),
		Sources:     sources,
	}, nil
}

func loadSources(ctx context.Context, pool *pgxpool.Pool) ([]Source, error) {
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
			&s.Cadence, &s.Notes, &s.LastIngestionOK, &s.LastRunStatus, &s.RunCount,
			&s.ConnectorVersion); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func sourceDocuments(ctx context.Context, pool *pgxpool.Pool, slug string) (urls []string, docs, bytes int64, paths []string, sizes map[string]int64, err error) {
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
	sizes = map[string]int64{}
	for drows.Next() {
		var key string
		var size int64
		if err := drows.Scan(&key, &size); err != nil {
			return nil, 0, 0, nil, nil, err
		}
		paths = append(paths, key)
		sizes[key] = size
		bytes += size
		docs++
	}
	return urls, docs, bytes, paths, sizes, drows.Err()
}

// tablesWithSourceID introspecte le schéma : toute table core.*/ref.*/derived.*
// avec une colonne source_id est une table de fait alimentée par une source
// scellée — la même convention appliquée sans exception depuis le début du
// projet (voir par ex. core.prime_activite_effectif.source_id).
func tablesWithSourceID(ctx context.Context, pool *pgxpool.Pool) ([]Table, error) {
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

// countBySourceGrouped fait un seul passage sur la table (GROUP BY
// source_id) plutôt qu'un COUNT(*) filtré par source répété pour chacune des
// dizaines de sources possibles.
func countBySourceGrouped(ctx context.Context, pool *pgxpool.Pool, schema, table string) (map[int64]int64, error) {
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

// tableSize donne le total de lignes (estimation du planificateur — un
// signal de complexité, pas un compte exact ; certaines tables dépassent le
// million de lignes et un COUNT(*) exact les rendrait coûteuses à répéter à
// chaque génération) et la taille sur disque de la table — partagés par
// toutes les sources qui l'alimentent, à ne pas confondre avec SourceRows
// (le compte exact, filtré sur une seule source).
func tableSize(ctx context.Context, pool *pgxpool.Pool, schema, table string) (rows, bytes int64, err error) {
	ident := pgx.Identifier{schema, table}.Sanitize()
	if err = pool.QueryRow(ctx, `SELECT reltuples::bigint FROM pg_class WHERE oid = $1::regclass`, ident).Scan(&rows); err != nil {
		return 0, 0, err
	}
	err = pool.QueryRow(ctx, `SELECT pg_total_relation_size($1::regclass)`, ident).Scan(&bytes)
	return rows, bytes, err
}

var reYear = regexp.MustCompile(`(?i)^(annee|année)$|annee$|^annee_`)

// yearRange cherche une colonne « annee » (la convention quasi
// systématique de ce dépôt) et en donne le min/max. Aucune colonne trouvée :
// renvoie des bornes nulles plutôt qu'une supposition — le non-détecté est
// une réponse valide (docs/decisions.md D-003), pas une erreur à masquer.
func yearRange(ctx context.Context, pool *pgxpool.Pool, schema, table string) (*int, *int, string, error) {
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
		if reYear.MatchString(c) {
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

// connectorBySlug associe un slug de source au contenu du fichier Go qui le
// déclare (var Source... = archive.Source{Slug: "...", ...}), pour décider si
// l'ingestion est une reconstruction complète ou un upsert sans le documenter
// à la main dans un registre qui dériverait du code.
func connectorBySlug(root string) (map[string]string, error) {
	out := map[string]string{}
	reSlug := regexp.MustCompile(`Slug:\s*"([^"]+)"`)
	err := walkGo(root, func(path, content string) error {
		for _, m := range reSlug.FindAllStringSubmatch(content, -1) {
			// Le même fichier héberge parfois plusieurs sources ; chacune reçoit
			// le contenu entier du fichier, suffisant pour repérer DELETE/ON CONFLICT.
			if _, exists := out[m[1]]; !exists {
				out[m[1]] = content
			}
		}
		return nil
	})
	return out, err
}

func walkGo(root string, fn func(path, content string) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "site" || name == "site.construction" || name == "site.precedent" ||
				name == "raw" || name == ".git" || strings.HasPrefix(name, "site-v1-") {
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

func classifyIncremental(content string) string {
	if content == "" {
		return "connecteur non retrouvé automatiquement — à vérifier manuellement"
	}
	hasDelete := strings.Contains(content, "DELETE FROM")
	hasConflict := strings.Contains(content, "ON CONFLICT")
	switch {
	case hasDelete && !hasConflict:
		return "non — reconstruction complète (DELETE puis COPY) à chaque exécution"
	case hasConflict && !hasDelete:
		return "oui — upsert (ON CONFLICT) sans purge préalable"
	case hasDelete && hasConflict:
		return "mixte — DELETE et ON CONFLICT présents dans le même fichier, à vérifier lequel s'applique à cette table"
	default:
		return "non déterminé automatiquement (ni DELETE FROM ni ON CONFLICT trouvé) — à vérifier manuellement"
	}
}

func detectFormat(urls, paths []string) string {
	all := strings.ToLower(strings.Join(append(append([]string{}, urls...), paths...), " "))
	switch {
	case strings.Contains(all, "opendatasoft") || strings.Contains(all, "/exports/json"):
		return "API Opendatasoft (export JSON)"
	case strings.Contains(all, "eurostat"):
		return "API Eurostat (JSON-stat)"
	case strings.Contains(all, "melodi"):
		return "API Insee Melodi (JSON)"
	case strings.HasSuffix(all, ".xlsx") || strings.Contains(all, ".xlsx"):
		return "Fichier XLSX"
	case strings.HasSuffix(all, ".csv") || strings.Contains(all, ".csv"):
		return "Fichier CSV"
	case strings.Contains(all, ".json"):
		return "Fichier JSON"
	case strings.Contains(all, ".xml"):
		return "Fichier XML"
	case all == "":
		return "aucun document récupéré à ce jour"
	default:
		return "format non reconnu automatiquement"
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
