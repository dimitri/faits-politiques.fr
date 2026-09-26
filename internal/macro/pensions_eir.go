package macro

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La distribution — pas seulement la moyenne — de la pension des retraités.
// Voir le commentaire de core.pension_tranche_eir (0067_socle_universel.sql) :
// une fonction non linéaire appliquée à une moyenne ne donne pas la moyenne de
// la fonction, et docs/revenu-universel-microsimulation.md en chiffre le biais.
var SourcePensionsEIR = archive.Source{
	Slug: "drees-eir-distribution-pensions", Label: "DREES — distribution des pensions (EIR)",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees, Échantillon interrégimes de retraités 2020",
	Cadence:     "quadriennale",
	Notes: "Champ : bénéficiaires d'un avantage principal de droit direct d'un régime " +
		"de base, nés en France ou à l'étranger, résidant en France ou à l'étranger, " +
		"vivants au 31 décembre 2020.",
}

const eirXLSXURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"4178_distribution-des-pensions-mensuelles/attachments/eir2020_distribution_des_pensions_mensuelles_xlsx"

var reTranche = regexp.MustCompile(`^De (\d+) à (\d+) euros`)
var reTrancheOuverte = regexp.MustCompile(`^Supérieur à (\d+) euros`)

func IngestPensionsEIR(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePensionsEIR)
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

	f, err := arch.Fetch(ctx, srcID, runID, eirXLSXURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	lignes, err := x.rows("pension brute de droit direct")
	if err != nil {
		return fail(err)
	}

	const annee = 2020
	var rows [][]any
	var total float64
	for _, l := range lignes {
		lib := l["A"]
		femmes, ok1 := valeurNumerique(l, "B")
		hommes, ok2 := valeurNumerique(l, "C")
		ens, ok3 := valeurNumerique(l, "D")
		if !ok1 || !ok2 || !ok3 {
			continue
		}
		if m := reTranche.FindStringSubmatch(lib); m != nil {
			min, _ := strconv.Atoi(m[1])
			max, _ := strconv.Atoi(m[2])
			rows = append(rows, []any{annee, min, max, femmes, hommes, ens, srcID})
			total += ens
		} else if m := reTrancheOuverte.FindStringSubmatch(lib); m != nil {
			min, _ := strconv.Atoi(m[1])
			rows = append(rows, []any{annee, min, nil, femmes, hommes, ens, srcID})
			total += ens
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune tranche extraite"))
	}
	// Les pourcentages doivent reconstituer 100 % — sinon une tranche a été
	// mal reconnue ou double comptée, silencieusement.
	if total < 99.5 || total > 100.5 {
		return fail(fmt.Errorf("les tranches totalisent %.2f %%, pas 100 %%", total))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_pension_tranche_eir (
			annee int, tranche_min int, tranche_max int,
			pct_femmes numeric, pct_hommes numeric, pct_ensemble numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_pension_tranche_eir"},
		[]string{"annee", "tranche_min", "tranche_max", "pct_femmes", "pct_hommes", "pct_ensemble", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, qui ne porte jamais qu'un seul millésime à la fois (celui de la
	// constante annee) — l'ancien DELETE scopé par année payait le prix des
	// triggers RI pour l'intégralité des tranches à chaque republication.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.pension_tranche_eir AS tgt
		USING tmp_pension_tranche_eir AS src
		ON tgt.annee = src.annee AND tgt.tranche_min = src.tranche_min
		WHEN MATCHED AND (tgt.tranche_max, tgt.pct_femmes, tgt.pct_hommes, tgt.pct_ensemble, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.tranche_max, src.pct_femmes, src.pct_hommes, src.pct_ensemble, src.source_id) THEN
		    UPDATE SET tranche_max = src.tranche_max, pct_femmes = src.pct_femmes,
		               pct_hommes = src.pct_hommes, pct_ensemble = src.pct_ensemble, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, tranche_min, tranche_max, pct_femmes, pct_hommes, pct_ensemble, source_id)
		    VALUES (src.annee, src.tranche_min, src.tranche_max, src.pct_femmes, src.pct_hommes,
		            src.pct_ensemble, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"tranches": len(rows), "total_pct": total, "touchees": touchees}, "")
	fmt.Printf("  distribution des pensions (EIR %d) : %d tranches, total %.1f %%, %d touchées par la fusion\n",
		annee, len(rows), total, touchees)
	return nil
}
