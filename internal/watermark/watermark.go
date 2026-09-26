// Package watermark porte le pendant, pour un connecteur sans étage
// raw.record (internal/europe, internal/senat, internal/campagne,
// internal/partis : ils parsent un fichier téléchargé directement), du
// mécanisme record_count/high_water_id qu'internal/an tient pour lui-même
// contre raw.record (internal/an/watermark.go) — même table
// (core.ingest_watermark, migration 0172 puis 0175 pour content_hash),
// même question (« faut-il refaire ce travail ? »), une empreinte
// différente : le sha256 qu'internal/archive.Fetch calcule déjà pour
// chaque fichier téléchargé (Fetched.SHA256), plutôt qu'un comptage sur
// raw.record que ces connecteurs n'alimentent pas.
package watermark

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Executeur : ce que *pgxpool.Pool et pgx.Tx offrent tous deux — Record peut
// donc s'appeler seul ou dans la transaction du rebuild qu'il atteste,
// exactement comme internal/an/watermark.go l'utilise pour la même raison.
type Executeur interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// forceKey — voir WithForce.
type forceKey struct{}

// WithForce marque ctx pour que tout watermark rencontré en aval (FileDiff
// ici, watermarkDiff dans internal/an/watermark.go) se comporte comme si
// rien n'avait jamais été vu — sans supprimer la ligne core.ingest_watermark
// elle-même, qui sera simplement réécrite par le Record qui suit un
// rebuild forcé. Sert « fpctl ingest --force » / « fpctl build --force » :
// jamais un état par défaut, un appelant qui n'y passe pas garde le
// comportement normal.
func WithForce(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceKey{}, true)
}

// Forced dit si ctx porte WithForce — internal/an/watermark.go s'en sert
// aussi, pour son propre mécanisme (raw.record), sans dépendre du reste de
// ce paquet.
func Forced(ctx context.Context) bool {
	v, _ := ctx.Value(forceKey{}).(bool)
	return v
}

// FileDiff dit si scope a déjà traité avec succès exactement ce sha256 —
// auquel cas l'appelant peut sauter sa reconstruction. Le style du message
// suit celui d'internal/an/watermark.go (watermarkDiff) à dessein : les deux
// mécanismes répondent à la même question, un lecteur de logs ne doit pas
// avoir à apprendre deux vocabulaires.
func FileDiff(ctx context.Context, pool *pgxpool.Pool, scope, hash string) (unchanged bool, raison string, err error) {
	if Forced(ctx) {
		return false, fmt.Sprintf("%s: rebuild forced (--force)", scope), nil
	}
	var seen *string
	err = pool.QueryRow(ctx,
		`SELECT content_hash FROM core.ingest_watermark WHERE scope = $1`, scope).Scan(&seen)
	if err == pgx.ErrNoRows {
		return false, fmt.Sprintf("%s: no prior watermark, this is the first run", scope), nil
	}
	if err != nil {
		return false, "", err
	}
	if seen != nil && *seen == hash {
		return true, "", nil
	}
	ancien := "—"
	if seen != nil {
		ancien = court(*seen)
	}
	return false, fmt.Sprintf("%s: source file changed since last run (sha256 %s -> %s)",
		scope, ancien, court(hash)), nil
}

// Record note le sha256 que scope vient de traiter avec succès, pour que le
// prochain appel puisse s'y comparer.
func Record(ctx context.Context, exec Executeur, scope, hash string) error {
	_, err := exec.Exec(ctx, `
		INSERT INTO core.ingest_watermark (scope, content_hash, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (scope) DO UPDATE SET
		  content_hash = EXCLUDED.content_hash, updated_at = EXCLUDED.updated_at`,
		scope, hash)
	return err
}

func court(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
