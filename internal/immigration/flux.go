package immigration

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceFluxMigratoire = archive.Source{
	Slug: "eurostat-flux-migratoire", Label: "Eurostat — flux annuels d'immigration et de naturalisation",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat, migr_imm1ctz et migr_acq",
	Cadence:     "annuelle",
	Notes: "Flux, pas des stocks : combien de personnes immigrent ou acquièrent la " +
		"nationalité française CHAQUE ANNÉE — à ne pas confondre avec core." +
		"population_historique_nationalite, qui donne un stock à un instant donné.",
}

func IngestFlux(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceFluxMigratoire)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	imm, err := fetchEurostatMigr(ctx, arch, srcID, runID,
		eurostatMigrBase+"migr_imm1ctz?geo=FR&agedef=COMPLET&age=TOTAL&sex=T&citizen=TOTAL&format=JSON&lang=EN",
		"citizen")
	if err != nil {
		return fail(fmt.Errorf("flux d'immigration : %w", err))
	}
	acq, err := fetchEurostatMigr(ctx, arch, srcID, runID,
		eurostatMigrBase+"migr_acq?geo=FR&agedef=COMPLET&age=TOTAL&sex=T&citizen=TOTAL&format=JSON&lang=EN",
		"citizen")
	if err != nil {
		return fail(fmt.Errorf("naturalisations : %w", err))
	}

	var rows [][]any
	for _, t := range imm {
		rows = append(rows, []any{"IMMIGRATION", "FR", t.Annee, t.Valeur, srcID})
	}
	for _, t := range acq {
		rows = append(rows, []any{"NATURALISATION", "FR", t.Annee, t.Valeur, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune valeur décodée"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.flux_migratoire`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "flux_migratoire"},
		[]string{"type_flux", "pays", "annee", "effectif", "source_id"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("flux_migratoire : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": n}, "")
	fmt.Printf("  flux migratoires (immigration %d ans, naturalisation %d ans)\n", len(imm), len(acq))
	return nil
}
