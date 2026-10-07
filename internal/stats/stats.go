// Package stats résume le contenu de la base : nombre de tables, lignes,
// taille sur disque, par schéma — pour répondre vite à « qu'est-ce qu'il y a
// là-dedans » sans écrire une requête à chaque fois. Appelé par fpctl (voir
// cmd/fpctl) : fpctl list stats.
package stats

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema : un schéma applicatif du projet (jamais pg_catalog ni les
// schémas d'extension) et ce qu'il pèse.
type Schema struct {
	Name          string
	Tables        int64
	EstimatedRows int64
	Bytes         int64
}

// Summary lit pg_stat_user_tables, groupé par schéma — n_live_tup est une
// estimée (mise à jour par autovacuum/analyze, pas un COUNT(*) exact), assez
// bonne pour une vue d'ensemble et infiniment moins coûteuse qu'un comptage
// réel sur des tables de plusieurs millions de lignes.
func Summary(ctx context.Context, pool *pgxpool.Pool) ([]Schema, error) {
	rows, err := pool.Query(ctx, `
		SELECT schemaname,
		       count(*),
		       coalesce(sum(n_live_tup), 0),
		       coalesce(sum(pg_total_relation_size(relid)), 0)
		FROM pg_stat_user_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		GROUP BY schemaname
		ORDER BY sum(pg_total_relation_size(relid)) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schema
	for rows.Next() {
		var s Schema
		if err := rows.Scan(&s.Name, &s.Tables, &s.EstimatedRows, &s.Bytes); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// LargestTables : les n tables les plus lourdes, tous schémas confondus
// — utile pour repérer d'un coup d'œil ce qui domine le volume total.
type Table struct {
	Schema        string
	Name          string
	EstimatedRows int64
	Bytes         int64
}

func LargestTables(ctx context.Context, pool *pgxpool.Pool, n int) ([]Table, error) {
	rows, err := pool.Query(ctx, `
		SELECT schemaname, relname, n_live_tup, pg_total_relation_size(relid)
		FROM pg_stat_user_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY pg_total_relation_size(relid) DESC
		LIMIT $1`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Table
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Schema, &t.Name, &t.EstimatedRows, &t.Bytes); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
