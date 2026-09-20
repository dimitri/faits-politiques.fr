// Package store ouvre la connexion Postgres.
package store

import (
	"context"
	"fmt"
	"os"
	"time"

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
