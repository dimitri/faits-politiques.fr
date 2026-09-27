package immigration

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const melodiOrigineURL = "https://api.insee.fr/melodi/data/DS_RP_TD_IMMI_AGESEX_PAYSNAISS_R_PRINC" +
	"?GEO=FRANCE-FM&maxResult=10000"

// paysLib : le regroupement Insee des pays de naissance — les pays qui pèsent
// individuellement (Algérie, Maroc, Tunisie, Italie, Espagne, Portugal,
// Turquie) et cinq agrégats pour le reste. Ce n'est pas la liste des 195 pays
// du monde : c'est celle que l'Insee choisit de publier à ce niveau de détail.
var paysLib = map[string]string{
	"12": "Algérie", "504": "Maroc", "788": "Tunisie", "380": "Italie",
	"620": "Portugal", "724": "Espagne", "792": "Turquie",
	"EUR_OTH": "Autres pays d'Europe", "UE27_OTH": "Autres pays de l'Union européenne",
	"AFR_OTH": "Autres pays d'Afrique", "ROW": "Reste du monde", "_T": "Total",
}

func IngestOrigine(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
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

	obs, err := melodiLire(ctx, arch, srcID, runID, melodiOrigineURL)
	if err != nil {
		return fail(err)
	}

	var rows [][]any
	var rejets int
	for _, o := range obs {
		annee, err := strconv.Atoi(o.Dimensions["TIME_PERIOD"])
		if err != nil {
			rejets++
			continue
		}
		sexe, okS := sexeLib[o.Dimensions["SEX"]]
		age, okA := ageLib[o.Dimensions["AGE"]]
		pays := o.Dimensions["AREA_COUNTRY"]
		lib, okP := paysLib[pays]
		if !okS || !okA || !okP {
			rejets++
			continue
		}
		rows = append(rows, []any{annee, sexe, age, pays, lib, o.Measures.OBSVALUENIVEAU.Value, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne reconnue sur %d observations", len(obs)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_population_immigree_origine (
			annee smallint, sexe text, age_tranche text, pays_code text, pays_libelle text,
			population numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_population_immigree_origine"},
		[]string{"annee", "sexe", "age_tranche", "pays_code", "pays_libelle", "population", "source_id"},
		pgx.CopyFromRows(dedupe(rows))); err != nil {
		return fail(fmt.Errorf("population_immigree_origine : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table ; l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la table à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.population_immigree_origine AS tgt
		USING tmp_population_immigree_origine AS src
		ON tgt.annee = src.annee AND tgt.sexe = src.sexe
		   AND tgt.age_tranche = src.age_tranche AND tgt.pays_code = src.pays_code
		WHEN MATCHED AND (tgt.pays_libelle, tgt.population, tgt.source_id)
		                  IS DISTINCT FROM (src.pays_libelle, src.population, src.source_id) THEN
		    UPDATE SET pays_libelle = src.pays_libelle, population = src.population, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, sexe, age_tranche, pays_code, pays_libelle, population, source_id)
		    VALUES (src.annee, src.sexe, src.age_tranche, src.pays_code, src.pays_libelle,
		            src.population, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion population_immigree_origine : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": touchees, "rejet_dimension_inconnue": rejets}, "")
	fmt.Printf("  population immigrée par pays de naissance : %d lignes touchées (%d rejetées)\n",
		touchees, rejets)
	return nil
}
