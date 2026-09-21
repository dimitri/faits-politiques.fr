package an

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rawWatermark renvoie l'état courant de raw.record pour un record_type : son
// nombre de lignes et son plus grand id. raw.record n'est jamais réécrit en
// place — une fiche mise à jour arrive comme une NOUVELLE ligne, jamais comme
// une modification d'une ligne existante (voir internal/archive) — donc ce
// couple suffit à détecter tout ajout depuis un état antérieur, pour le coût
// d'un balayage d'index plutôt qu'un hachage de chaque ligne.
func rawWatermark(ctx context.Context, pool *pgxpool.Pool, recordType string) (count, highWater int64, err error) {
	err = pool.QueryRow(ctx,
		`SELECT count(*), coalesce(max(id), 0) FROM raw.record WHERE record_type = $1`,
		recordType).Scan(&count, &highWater)
	return count, highWater, err
}

// watermarkUnchanged dit si scope a déjà traité avec succès exactement cet
// état de raw.record (core.ingest_watermark, migration 0172) — auquel cas
// une étape de normalisation peut sauter sa reconstruction : elle referait
// bit à bit ce qu'elle a déjà produit.
func watermarkUnchanged(ctx context.Context, pool *pgxpool.Pool, scope string, count, highWater int64) (bool, error) {
	var seenCount, seenHigh int64
	err := pool.QueryRow(ctx,
		`SELECT record_count, high_water_id FROM core.ingest_watermark WHERE scope = $1`,
		scope).Scan(&seenCount, &seenHigh)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return seenCount == count && seenHigh == highWater, nil
}

// recordWatermark note l'état de raw.record que scope vient de traiter avec
// succès, pour que le prochain appel puisse s'y comparer.
func recordWatermark(ctx context.Context, pool *pgxpool.Pool, scope string, count, highWater int64) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO core.ingest_watermark (scope, record_count, high_water_id, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (scope) DO UPDATE SET
		  record_count = EXCLUDED.record_count, high_water_id = EXCLUDED.high_water_id,
		  updated_at = EXCLUDED.updated_at`,
		scope, count, highWater)
	return err
}
