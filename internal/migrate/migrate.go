// Package migrate applique les migrations goose sans dépendre de l'outil goose :
// le format est simple, et une dépendance de moins vaut mieux qu'une de plus.
package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const marker = "-- +goose Up"
const downMarker = "-- +goose Down"

// Up applique, dans l'ordre, les migrations non encore enregistrées.
func Up(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("table schema_migrations : %w", err)
	}

	applied := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	rows.Close()

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, f := range files {
		version := strings.TrimSuffix(filepath.Base(f), ".sql")
		if applied[version] {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		up, err := upSection(string(body))
		if err != nil {
			return fmt.Errorf("%s : %w", version, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, up); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s : %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Printf("  migration %s appliquée\n", version)
	}
	return nil
}

func upSection(body string) (string, error) {
	i := strings.Index(body, marker)
	if i < 0 {
		return "", fmt.Errorf("marqueur %q absent", marker)
	}
	s := body[i+len(marker):]
	if j := strings.Index(s, downMarker); j >= 0 {
		s = s[:j]
	}
	s = strings.ReplaceAll(s, "-- +goose StatementBegin", "")
	s = strings.ReplaceAll(s, "-- +goose StatementEnd", "")
	return s, nil
}
