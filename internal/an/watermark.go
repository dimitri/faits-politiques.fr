package an

import (
	"context"
	"fmt"

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

// watermarkDiff dit si scope a déjà traité avec succès exactement cet état de
// raw.record (core.ingest_watermark, migration 0172) — auquel cas une étape
// de normalisation peut sauter sa reconstruction : elle referait bit à bit ce
// qu'elle a déjà produit. Quand ce n'est PAS le cas, raison explique
// pourquoi, en clair : jamais un « rebuilding 1 270 476 ballots » sans dire
// si c'est parce que rien n'avait encore tourné, ou parce que l'Assemblée a
// publié une mise à jour depuis la dernière fois.
func watermarkDiff(ctx context.Context, pool *pgxpool.Pool, scope string, count, highWater int64) (unchanged bool, raison string, err error) {
	var seenCount, seenHigh int64
	err = pool.QueryRow(ctx,
		`SELECT record_count, high_water_id FROM core.ingest_watermark WHERE scope = $1`,
		scope).Scan(&seenCount, &seenHigh)
	if err == pgx.ErrNoRows {
		return false, fmt.Sprintf("%s: no prior watermark, this is the first run", scope), nil
	}
	if err != nil {
		return false, "", err
	}
	if seenCount == count && seenHigh == highWater {
		return true, "", nil
	}
	return false, fmt.Sprintf("%s: %d new raw.record row(s) since last run (%d -> %d rows, high-water %d -> %d)",
		scope, count-seenCount, seenCount, count, seenHigh, highWater), nil
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
