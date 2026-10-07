package sitegen

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SourceDetail struct {
	Slug, Label, Publisher, Tier, License, ReuseClass string
	Attribution, Cadence, Notes                       string
	Redistributable                                   bool
	LastIngestion, Status                             string
	Documents, Records, Bytes                         int64
	Footprint                                         string
	URLs                                              []string
}

var tierFr = map[string]string{
	"PRIMARY_OFFICIAL": "Niveau 1 — source primaire officielle",
	"SECONDARY_PRESS":  "Niveau 2 — source secondaire",
	"DECLARATIVE":      "Niveau 3 — source déclarative",
}

var reuseFr = map[string]string{
	"OPEN":           "Domaine public ou équivalent",
	"ATTRIBUTION":    "Réutilisation libre avec attribution",
	"NON_COMMERCIAL": "Réutilisation non commerciale seulement",
	"RESTRICTED":     "Usage restreint, ou licence non explicitée",
}

// loadSources décrit chaque flux ingéré : sa licence, sa classe de
// réutilisation, sa dernière récupération et son volume.
//
// Une source absente de cette page n'alimente rien : c'est la liste
// exhaustive de ce sur quoi le site repose.
func loadSources(ctx context.Context, pool *pgxpool.Pool) ([]SourceDetail, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.slug, s.label, s.publisher, s.tier::text, s.licence,
		       s.reuse_class::text, s.attribution_text,
		       coalesce(s.expected_cadence,''), coalesce(s.notes,''),
		       s.commercial_use,
		       coalesce(to_char(max(r.fetched_at),'DD/MM/YYYY à HH24:MI'),''),
		       coalesce((SELECT fr.status FROM raw.fetch_run fr
		                  WHERE fr.source_id = s.id AND fr.finished_at IS NOT NULL
		                  ORDER BY fr.finished_at DESC LIMIT 1),''),
		       count(DISTINCT d.id), coalesce(sum(DISTINCT d.byte_size),0)
		FROM raw.source s
		LEFT JOIN raw.retrieval r ON r.source_id = s.id
		LEFT JOIN raw.document d ON d.id = r.document_id
		GROUP BY s.id, s.slug, s.label, s.publisher, s.tier, s.licence,
		         s.reuse_class, s.attribution_text, s.expected_cadence, s.notes,
		         s.commercial_use
		ORDER BY s.tier, s.label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SourceDetail
	for rows.Next() {
		var d SourceDetail
		if err := rows.Scan(&d.Slug, &d.Label, &d.Publisher, &d.Tier, &d.License,
			&d.ReuseClass, &d.Attribution, &d.Cadence, &d.Notes, &d.Redistributable,
			&d.LastIngestion, &d.Status, &d.Documents, &d.Bytes); err != nil {
			return nil, err
		}
		d.Tier = tierFr[d.Tier]
		d.ReuseClass = reuseFr[d.ReuseClass]
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// URL réellement récupérées, empreinte du dernier document, et nombre
	// d'enregistrements : trois requêtes AU TOTAL plutôt que trois PAR
	// SOURCE (166 sources, jusqu'à 498 aller-retours) — chacune couvre
	// toutes les sources d'un coup et se répartit par slug ensuite en Go.
	urls, err := pool.Query(ctx, `
		WITH distinctes AS (
			SELECT DISTINCT s.slug, r.url
			  FROM raw.retrieval r
			  JOIN raw.source s ON s.id = r.source_id
			 WHERE r.document_id IS NOT NULL
		)
		SELECT slug, url FROM (
			SELECT slug, url, row_number() OVER (PARTITION BY slug ORDER BY url) AS rn
			  FROM distinctes
		) x WHERE rn <= 6`)
	if err != nil {
		return nil, err
	}
	urlsPerSlug := map[string][]string{}
	for urls.Next() {
		var slug, u string
		if err := urls.Scan(&slug, &u); err != nil {
			urls.Close()
			return nil, err
		}
		urlsPerSlug[slug] = append(urlsPerSlug[slug], u)
	}
	urls.Close()
	if err := urls.Err(); err != nil {
		return nil, err
	}

	footprints, err := pool.Query(ctx, `
		SELECT DISTINCT ON (s.slug) s.slug, encode(d.sha256,'hex')
		  FROM raw.retrieval r
		  JOIN raw.document d ON d.id = r.document_id
		  JOIN raw.source s ON s.id = r.source_id
		 ORDER BY s.slug, r.fetched_at DESC`)
	if err != nil {
		return nil, err
	}
	footprintPerSlug := map[string]string{}
	for footprints.Next() {
		var slug, emp string
		if err := footprints.Scan(&slug, &emp); err != nil {
			footprints.Close()
			return nil, err
		}
		footprintPerSlug[slug] = emp
	}
	footprints.Close()
	if err := footprints.Err(); err != nil {
		return nil, err
	}

	// mv.source_enregistrements (internal/matview) — plus le JOIN à quatre
	// tables sur la totalité de raw.record (489 Mo) rejoué une fois par
	// source.
	registeredRows, err := pool.Query(ctx, `SELECT source_slug, nombre_enregistrements FROM mv.source_enregistrements`)
	if err != nil {
		return nil, err
	}
	registeredPerSlug := map[string]int64{}
	for registeredRows.Next() {
		var slug string
		var n int64
		if err := registeredRows.Scan(&slug, &n); err != nil {
			registeredRows.Close()
			return nil, err
		}
		registeredPerSlug[slug] = n
	}
	registeredRows.Close()
	if err := registeredRows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		out[i].URLs = urlsPerSlug[out[i].Slug]
		out[i].Footprint = footprintPerSlug[out[i].Slug]
		if len(out[i].Footprint) > 20 {
			out[i].Footprint = out[i].Footprint[:20] + "…"
		}
		out[i].Records = registeredPerSlug[out[i].Slug]
	}
	return out, nil
}

type TypeFile struct {
	Type  string
	Count int64
	Bytes int64
}

type TableLarge struct {
	Name  string
	Lines int64
	Bytes int64
}

// StatsGlobalSources : de quoi répondre, avant la liste des flux, à
// « combien de sources, combien de fichiers, combien pèse tout ça » — un
// chiffre vérifié par introspection PostgreSQL directe, pas une estimation.
type StatsGlobalSources struct {
	CountSources    int64
	CountFiles      int64
	BytesFiles      int64
	TypesFiles      []TypeFile
	CountTables     int64
	CountLines      int64
	BytesBase       int64
	PlusLargeTables []TableLarge
}

// loadStatsGlobalSources : deux mesures de taille distinctes, jamais
// fusionnées — celle de l'archive scellée (raw.document, les fichiers sources
// tels que récupérés) et celle de la base (pg_database_size, les données une
// fois extraites et normalisées). Qu'elles se ressemblent en ordre de
// grandeur est une coïncidence, pas la même chose : l'une mesure ce qui a été
// téléchargé, l'autre ce que Postgres stocke une fois structuré.
func loadStatsGlobalSources(ctx context.Context, pool *pgxpool.Pool) (*StatsGlobalSources, error) {
	var st StatsGlobalSources

	if err := pool.QueryRow(ctx, `SELECT count(*) FROM raw.source`).Scan(&st.CountSources); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*), coalesce(sum(byte_size),0) FROM raw.document`).
		Scan(&st.CountFiles, &st.BytesFiles); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT split_part(lower(content_type),';',1) AS type, count(*), sum(byte_size)
		FROM raw.document GROUP BY 1 ORDER BY count(*) DESC LIMIT 4`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t TypeFile
		if err := rows.Scan(&t.Type, &t.Count, &t.Bytes); err != nil {
			rows.Close()
			return nil, err
		}
		st.TypesFiles = append(st.TypesFiles, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables WHERE schemaname IN ('core','ref','geo','derived','raw')`).
		Scan(&st.CountTables); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(sum(n_live_tup),0) FROM pg_stat_user_tables
		WHERE schemaname IN ('core','ref','geo','derived','raw')`).
		Scan(&st.CountLines); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).
		Scan(&st.BytesBase); err != nil {
		return nil, err
	}

	rowsT, err := pool.Query(ctx, `
		SELECT schemaname||'.'||relname, n_live_tup, pg_total_relation_size(schemaname||'.'||relname) AS taille
		FROM pg_stat_user_tables WHERE schemaname IN ('core','ref','geo','derived','raw')
		ORDER BY taille DESC LIMIT 5`)
	if err != nil {
		return nil, err
	}
	for rowsT.Next() {
		var t TableLarge
		if err := rowsT.Scan(&t.Name, &t.Lines, &t.Bytes); err != nil {
			rowsT.Close()
			return nil, err
		}
		st.PlusLargeTables = append(st.PlusLargeTables, t)
	}
	rowsT.Close()
	if err := rowsT.Err(); err != nil {
		return nil, err
	}

	return &st, nil
}
