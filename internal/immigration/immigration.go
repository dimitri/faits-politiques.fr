package immigration

import (
	"context"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ingest charge, dans l'ordre, la population par statut migratoire et
// nationalité, sa catégorie socioprofessionnelle, les origines géographiques,
// la comparaison européenne et le stock de titres de séjour. Voir
// docs/immigration-donnees.md.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestStatutMigratoire(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestCSP(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestOrigine(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestEurostatMigration(ctx, pool, arch); err != nil {
		return err
	}
	return IngestTitresSejour(ctx, pool, arch)
}
