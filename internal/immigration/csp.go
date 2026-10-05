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
	melodiImmiCSPURL = "https://api.insee.fr/melodi/data/DS_RP_TD_IMMI_AGESEXPCS_COMP" +
		"?GEO=FRANCE-FM&maxResult=10000"
	melodiNatCSPURL = "https://api.insee.fr/melodi/data/DS_RP_TD_NAT_AGESEXPCS_COMP" +
		"?GEO=FRANCE-FM&maxResult=10000"
)

// pcsLib : la nomenclature à un chiffre que ces deux jeux Melodi utilisent —
// plus grossière que les vingt-huit postes de la PCS complète, mais c'est le
// seul niveau que l'Insee croise avec le statut migratoire en open data.
var pcsLib = map[string]string{
	"1": "Agriculteurs", "2": "Artisans, commerçants et chefs d'entreprise",
	"3": "Cadres et professions intellectuelles supérieures", "4": "Professions intermédiaires",
	"5": "Employés", "6": "Ouvriers", "7": "Retraités", "9": "Autres inactifs", "_T": "Total",
}

func IngestCSP(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
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

	immi, err := melodiLire(ctx, arch, srcID, runID, melodiImmiCSPURL)
	if err != nil {
		return fail(fmt.Errorf("immigration × CSP : %w", err))
	}
	nat, err := melodiLire(ctx, arch, srcID, runID, melodiNatCSPURL)
	if err != nil {
		return fail(fmt.Errorf("nationalité × CSP : %w", err))
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
		pcs := o.Dimensions["PCS"]
		lib, okP := pcsLib[pcs]
		if !ok || !okS || !okP {
			rejets++
			continue
		}
		rows = append(rows, []any{"IMMIGRATION", cat, annee, sexe, pcs, lib,
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
		pcs := o.Dimensions["PCS"]
		lib, okP := pcsLib[pcs]
		if !ok || !okS || !okP {
			rejets++
			continue
		}
		rows = append(rows, []any{"NATIONALITE", cat, annee, sexe, pcs, lib,
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
		CREATE TEMP TABLE tmp_population_statut_migratoire_csp (
			classification text, categorie text, annee smallint, sexe text,
			csp_code text, csp_libelle text, population numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_population_statut_migratoire_csp"},
		[]string{"classification", "categorie", "annee", "sexe", "csp_code", "csp_libelle",
			"population", "source_id"},
		pgx.CopyFromRows(dedupe(rows))); err != nil {
		return fail(fmt.Errorf("population_statut_migratoire_csp : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table ; l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la table à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.population_statut_migratoire_csp AS tgt
		USING tmp_population_statut_migratoire_csp AS src
		ON tgt.classification = src.classification AND tgt.categorie = src.categorie
		   AND tgt.annee = src.annee AND tgt.sexe = src.sexe AND tgt.csp_code = src.csp_code
		WHEN MATCHED AND (tgt.csp_libelle, tgt.population, tgt.source_id)
		                  IS DISTINCT FROM (src.csp_libelle, src.population, src.source_id) THEN
		    UPDATE SET csp_libelle = src.csp_libelle, population = src.population, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (classification, categorie, annee, sexe, csp_code, csp_libelle, population, source_id)
		    VALUES (src.classification, src.categorie, src.annee, src.sexe, src.csp_code, src.csp_libelle,
		            src.population, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion population_statut_migratoire_csp : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": touchees, "rejet_dimension_inconnue": rejets}, "")
	fmt.Printf("  population par catégorie socioprofessionnelle : %d lignes touchées (%d rejetées)\n",
		touchees, rejets)
	return nil
}
