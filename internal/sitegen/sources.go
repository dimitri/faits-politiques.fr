package sitegen

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SourceDetail struct {
	Slug, Label, Publisher, Tier, Licence, ReuseClass string
	Attribution, Cadence, Notes                       string
	Redistribuable                                    bool
	DerniereIngestion, Statut                         string
	Documents, Enregistrements, Octets                int64
	Empreinte                                         string
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
		if err := rows.Scan(&d.Slug, &d.Label, &d.Publisher, &d.Tier, &d.Licence,
			&d.ReuseClass, &d.Attribution, &d.Cadence, &d.Notes, &d.Redistribuable,
			&d.DerniereIngestion, &d.Statut, &d.Documents, &d.Octets); err != nil {
			return nil, err
		}
		d.Tier = tierFr[d.Tier]
		d.ReuseClass = reuseFr[d.ReuseClass]
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// URL réellement récupérées et empreinte du dernier document : c'est ce qui
	// rend une ingestion vérifiable par un tiers.
	for i := range out {
		urls, err := pool.Query(ctx, `
			SELECT DISTINCT r.url FROM raw.retrieval r
			JOIN raw.source s ON s.id = r.source_id
			WHERE s.slug = $1 AND r.document_id IS NOT NULL
			ORDER BY r.url LIMIT 6`, out[i].Slug)
		if err != nil {
			return nil, err
		}
		for urls.Next() {
			var u string
			if err := urls.Scan(&u); err != nil {
				return nil, err
			}
			out[i].URLs = append(out[i].URLs, u)
		}
		urls.Close()

		_ = pool.QueryRow(ctx, `
			SELECT encode(d.sha256,'hex') FROM raw.retrieval r
			JOIN raw.document d ON d.id = r.document_id
			JOIN raw.source s ON s.id = r.source_id
			WHERE s.slug = $1 ORDER BY r.fetched_at DESC LIMIT 1`, out[i].Slug).
			Scan(&out[i].Empreinte)
		if len(out[i].Empreinte) > 20 {
			out[i].Empreinte = out[i].Empreinte[:20] + "…"
		}

		_ = pool.QueryRow(ctx, `
			SELECT count(*) FROM raw.record rec
			JOIN raw.document d ON d.id = rec.document_id
			JOIN raw.retrieval r ON r.document_id = d.id
			JOIN raw.source s ON s.id = r.source_id
			WHERE s.slug = $1`, out[i].Slug).Scan(&out[i].Enregistrements)
	}
	return out, nil
}

type TypeFichier struct {
	Type   string
	Nombre int64
	Octets int64
}

type TableVolumineuse struct {
	Nom    string
	Lignes int64
	Octets int64
}

// StatsGlobalesSources : de quoi répondre, avant la liste des flux, à
// « combien de sources, combien de fichiers, combien pèse tout ça » — un
// chiffre vérifié par introspection PostgreSQL directe, pas une estimation.
type StatsGlobalesSources struct {
	NbSources         int64
	NbFichiers        int64
	OctetsFichiers    int64
	TypesFichiers     []TypeFichier
	NbTables          int64
	NbLignes          int64
	OctetsBase        int64
	PlusGrossesTables []TableVolumineuse
}

// chargerStatsGlobalesSources : deux mesures de taille distinctes, jamais
// fusionnées — celle de l'archive scellée (raw.document, les fichiers sources
// tels que récupérés) et celle de la base (pg_database_size, les données une
// fois extraites et normalisées). Qu'elles se ressemblent en ordre de
// grandeur est une coïncidence, pas la même chose : l'une mesure ce qui a été
// téléchargé, l'autre ce que Postgres stocke une fois structuré.
func chargerStatsGlobalesSources(ctx context.Context, pool *pgxpool.Pool) (*StatsGlobalesSources, error) {
	var st StatsGlobalesSources

	if err := pool.QueryRow(ctx, `SELECT count(*) FROM raw.source`).Scan(&st.NbSources); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*), coalesce(sum(byte_size),0) FROM raw.document`).
		Scan(&st.NbFichiers, &st.OctetsFichiers); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT split_part(lower(content_type),';',1) AS type, count(*), sum(byte_size)
		FROM raw.document GROUP BY 1 ORDER BY count(*) DESC LIMIT 4`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t TypeFichier
		if err := rows.Scan(&t.Type, &t.Nombre, &t.Octets); err != nil {
			rows.Close()
			return nil, err
		}
		st.TypesFichiers = append(st.TypesFichiers, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables WHERE schemaname IN ('core','ref','geo','derived','raw')`).
		Scan(&st.NbTables); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(sum(n_live_tup),0) FROM pg_stat_user_tables
		WHERE schemaname IN ('core','ref','geo','derived','raw')`).
		Scan(&st.NbLignes); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).
		Scan(&st.OctetsBase); err != nil {
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
		var t TableVolumineuse
		if err := rowsT.Scan(&t.Nom, &t.Lignes, &t.Octets); err != nil {
			rowsT.Close()
			return nil, err
		}
		st.PlusGrossesTables = append(st.PlusGrossesTables, t)
	}
	rowsT.Close()
	if err := rowsT.Err(); err != nil {
		return nil, err
	}

	return &st, nil
}
