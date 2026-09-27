// Package store ouvre la connexion Postgres.
package store

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DSN retourne l'URL de connexion, avec un défaut de développement local.
func DSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://fp:fp@localhost:55432/fp?sslmode=disable"
}

// Open ouvre un pool à 4 connexions au plus — le défaut partagé par les
// commandes qui parlent à la base ponctuellement (ingest, verify). Il est
// délibérément bas dans l'hypothèse d'un hébergement serverless, où chaque
// instance aurait son propre pool et un scale-up saturerait une base managée
// partagée (cf. docs/decisions.md D-012). Cette hypothèse ne tient plus pour
// le déploiement réel (une seule machine, sa propre Postgres) mais rien ne
// prouve encore qu'elle ne tiendra jamais : OpenWithMaxConns existe pour les
// commandes qui, comme internal/sitegen, ont un besoin ponctuel et mesuré de plus de
// parallélisme, sans changer ce défaut pour tout le monde.
func Open(ctx context.Context) (*pgxpool.Pool, error) {
	return OpenWithMaxConns(ctx, 4)
}

// OpenWithMaxConns : comme Open, avec un nombre de connexions choisi par
// l'appelant. internal/sitegen s'en sert pour paralléliser plusieurs requêtes
// indépendantes (internal/sitegen/lieux_pages.go) — un connecteur d'ingestion
// ordinaire n'a pas cette raison de s'écarter du défaut.
func OpenWithMaxConns(ctx context.Context, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(DSN())
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = time.Hour
	// 1 Go sur CHAQUE connexion, pas au coup par coup : le défaut serveur
	// (4 Mo de work_mem, 64 Mo de maintenance_work_mem) a fait déborder sur
	// disque plusieurs tris/jointures de ce projet une fois la volumétrie
	// dépassée (mesuré : ssmsi.go, ~5,2M lignes ; le MERGE de core.ballot
	// pour l'Europe, ~1,97M ; RE-ADD CONSTRAINT après bulkload.
	// SansContraintesFK, qui puise dans maintenance_work_mem). Plutôt que
	// d'ajouter un SET LOCAL à chaque nouvel appelant qui découvre le
	// problème à son tour — déjà fait six fois séparément dans ce
	// projet — un réglage à la connexion couvre tout le monde une bonne
	// fois, y compris REFRESH MATERIALIZED VIEW (internal/matview), qui
	// n'avait jamais eu cette couverture. Toujours annulable localement par
	// un SET LOCAL plus bas dans une transaction si un appelant a besoin
	// d'un budget différent.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET work_mem = '1GB'; SET maintenance_work_mem = '1GB'`)
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 30; i++ {
		if err = pool.Ping(ctx); err == nil {
			return pool, nil
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("base injoignable : %w", err)
}
