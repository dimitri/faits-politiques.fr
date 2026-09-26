package immigration

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	melodiImmiEmploiURL = "https://api.insee.fr/melodi/data/DS_RP_TD_IMMI_AGESEXEMPSTA_PRINC" +
		"?GEO=FRANCE-FM&maxResult=10000"
	melodiNatEmploiURL = "https://api.insee.fr/melodi/data/DS_RP_TD_NAT_AGESEXEMPSTA_PRINC" +
		"?GEO=FRANCE-FM&maxResult=10000"
)

// IngestStatutMigratoire charge la population par statut migratoire
// (immigré/non-immigré) et par nationalité (étranger/français), croisée avec
// l'âge, le sexe et le statut d'emploi — les deux classifications de
// db/migrations/0071_immigration.sql, jamais fusionnées.
func IngestStatutMigratoire(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMelodiImmigration)
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

	immi, err := melodiLire(ctx, arch, srcID, runID, melodiImmiEmploiURL)
	if err != nil {
		return fail(fmt.Errorf("immigration × emploi : %w", err))
	}
	nat, err := melodiLire(ctx, arch, srcID, runID, melodiNatEmploiURL)
	if err != nil {
		return fail(fmt.Errorf("nationalité × emploi : %w", err))
	}

	var rows [][]any
	var rejets int
	for _, o := range immi {
		annee, err := strconv.Atoi(o.Dimensions["TIME_PERIOD"])
		if err != nil {
			rejets++
			continue
		}
		cat, ok := map[string]string{"0": "NON_IMMIGRE", "1": "IMMIGRE", "_T": "TOTAL"}[o.Dimensions["IMMI"]]
		sexe, okS := sexeLib[o.Dimensions["SEX"]]
		age, okA := ageLib[o.Dimensions["AGE"]]
		emp, okE := empstaLib[o.Dimensions["EMPSTA_ENQ"]]
		if !ok || !okS || !okA || !okE {
			rejets++
			continue
		}
		rows = append(rows, []any{"IMMIGRATION", cat, annee, sexe, age, emp,
			o.Measures.OBSVALUENIVEAU.Value, srcID})
	}
	for _, o := range nat {
		annee, err := strconv.Atoi(o.Dimensions["TIME_PERIOD"])
		if err != nil {
			rejets++
			continue
		}
		cat, ok := map[string]string{"100": "ETRANGER", "250": "FRANCAIS", "_T": "TOTAL"}[o.Dimensions["NATIONALITY_TYPE"]]
		sexe, okS := sexeLib[o.Dimensions["SEX"]]
		age, okA := ageLib[o.Dimensions["AGE"]]
		emp, okE := empstaLib[o.Dimensions["EMPSTA_ENQ"]]
		if !ok || !okS || !okA || !okE {
			rejets++
			continue
		}
		rows = append(rows, []any{"NATIONALITE", cat, annee, sexe, age, emp,
			o.Measures.OBSVALUENIVEAU.Value, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne reconnue sur %d+%d observations", len(immi), len(nat)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_population_statut_migratoire (
			classification text, categorie text, annee smallint, sexe text, age_tranche text,
			statut_emploi text, population numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_population_statut_migratoire"},
		[]string{"classification", "categorie", "annee", "sexe", "age_tranche", "statut_emploi",
			"population", "source_id"},
		pgx.CopyFromRows(dedupe(rows))); err != nil {
		return fail(fmt.Errorf("population_statut_migratoire : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table ; l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la table à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.population_statut_migratoire AS tgt
		USING tmp_population_statut_migratoire AS src
		ON tgt.classification = src.classification AND tgt.categorie = src.categorie
		   AND tgt.annee = src.annee AND tgt.sexe = src.sexe
		   AND tgt.age_tranche = src.age_tranche AND tgt.statut_emploi = src.statut_emploi
		WHEN MATCHED AND (tgt.population, tgt.source_id) IS DISTINCT FROM (src.population, src.source_id) THEN
		    UPDATE SET population = src.population, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (classification, categorie, annee, sexe, age_tranche, statut_emploi, population, source_id)
		    VALUES (src.classification, src.categorie, src.annee, src.sexe, src.age_tranche,
		            src.statut_emploi, src.population, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion population_statut_migratoire : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": touchees, "rejet_dimension_inconnue": rejets}, "")
	fmt.Printf("  population par statut migratoire et nationalité : %d lignes touchées (%d rejetées)\n",
		touchees, rejets)
	return nil
}

// dedupe : les deux appels Melodi peuvent, pour des raisons de pagination côté
// Insee, renvoyer deux fois une même combinaison de dimensions dans de rares
// cas ; la clé primaire de la table le refuserait au COPY entier. Un aller
// par une map élimine les doublons AVANT l'insertion plutôt que de faire
// échouer tout le chargement pour une poignée de lignes.
func dedupe(rows [][]any) [][]any {
	seen := map[string]bool{}
	out := make([][]any, 0, len(rows))
	for _, r := range rows {
		k := fmt.Sprint(r[:6])
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}
