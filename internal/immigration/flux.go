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

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_flux_migratoire (
			type_flux text, pays text, annee smallint, effectif numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_flux_migratoire"},
		[]string{"type_flux", "pays", "annee", "effectif", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("flux_migratoire : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table ; l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la table à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.flux_migratoire AS tgt
		USING tmp_flux_migratoire AS src
		ON tgt.type_flux = src.type_flux AND tgt.pays = src.pays AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.effectif, tgt.source_id) IS DISTINCT FROM (src.effectif, src.source_id) THEN
		    UPDATE SET effectif = src.effectif, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (type_flux, pays, annee, effectif, source_id)
		    VALUES (src.type_flux, src.pays, src.annee, src.effectif, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion flux_migratoire : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": touchees}, "")
	fmt.Printf("  flux migratoires (immigration %d ans, naturalisation %d ans, %d touchées)\n",
		len(imm), len(acq), touchees)
	return nil
}
