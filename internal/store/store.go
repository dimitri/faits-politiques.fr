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

// Open ouvre un pool. MaxConns est délibérément bas : en conteneur serverless,
// chaque instance a son propre pool et un scale-up saturerait la base managée
// (cf. docs/decisions.md D-012).
func Open(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(DSN())
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 4
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
