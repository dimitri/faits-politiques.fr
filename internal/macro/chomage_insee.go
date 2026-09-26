package macro

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le taux de chômage au sens du BIT, publié directement par l'INSEE au pas
// trimestriel (série BDM 001688527) — pas la republication annuelle
// d'Eurostat (chomage.taux dans core.macro_value), plus lente et moins
// fine. C'est la mesure de référence du débat public français : celle que
// citent les gouvernements et les médias à chaque publication trimestrielle.
var SourceInseeChomageTrimestriel = archive.Source{
	Slug: "insee-chomage-trimestriel", Label: "INSEE — taux de chômage trimestriel au sens du BIT",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INSEE, taux de chômage au sens du BIT, série 001688527",
	Cadence:     "trimestrielle",
	Notes: "France hors Mayotte, données CVS. API SDMX de la Banque de données " +
		"macro-économiques (BDM), pas de fichier téléchargé : la réponse EST la donnée.",
}

const inseeChomageURL = "https://www.bdm.insee.fr/series/sdmx/data/SERIES_BDM/001688527"

type sdmxDataSet struct {
	Series struct {
		IDBank string    `xml:"IDBANK,attr"`
		Title  string    `xml:"TITLE_FR,attr"`
		Obs    []sdmxObs `xml:"Obs"`
	} `xml:"DataSet>Series"`
}

type sdmxObs struct {
	Periode string `xml:"TIME_PERIOD,attr"`
	Valeur  string `xml:"OBS_VALUE,attr"`
}

func IngestChomageINSEE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceInseeChomageTrimestriel)
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

	f, err := arch.Fetch(ctx, srcID, runID, inseeChomageURL, ".xml")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var ds sdmxDataSet
	if err := xml.Unmarshal(raw, &ds); err != nil {
		return fail(fmt.Errorf("réponse SDMX illisible : %w", err))
	}
	if len(ds.Series.Obs) == 0 {
		return fail(fmt.Errorf("aucune observation dans la série %s", ds.Series.IDBank))
	}

	var rows [][]any
	for _, o := range ds.Series.Obs {
		annee, trim, err := trimestreDe(o.Periode)
		if err != nil {
			continue // une ligne mal formée n'invalide pas les 200 autres
		}
		v, err := strconv.ParseFloat(o.Valeur, 64)
		if err != nil {
			continue
		}
		rows = append(rows, []any{o.Periode, annee, trim, v, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("%d observations lues, aucune exploitable", len(ds.Series.Obs)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour tous les trimestres à chaque republication trimestrielle, changement
	// ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_chomage_taux_trimestriel (
			trimestre text, annee smallint, trimestre_num smallint, taux numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_chomage_taux_trimestriel"},
		[]string{"trimestre", "annee", "trimestre_num", "taux", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.chomage_taux_trimestriel AS tgt
		USING tmp_chomage_taux_trimestriel AS src
		ON tgt.trimestre = src.trimestre
		WHEN MATCHED AND (tgt.annee, tgt.trimestre_num, tgt.taux, tgt.source_id)
		                  IS DISTINCT FROM (src.annee, src.trimestre_num, src.taux, src.source_id) THEN
		    UPDATE SET annee = src.annee, trimestre_num = src.trimestre_num,
		               taux = src.taux, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (trimestre, annee, trimestre_num, taux, source_id)
		    VALUES (src.trimestre, src.annee, src.trimestre_num, src.taux, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"trimestres_charges": len(rows), "touchees": touchees}, "")
	fmt.Printf("  taux de chômage trimestriel (INSEE, série %s) : %d trimestres (%d touchés par la fusion)\n",
		ds.Series.IDBank, len(rows), touchees)
	return nil
}

// trimestreDe lit "2026-Q2" en (2026, 2). Le format SDMX de l'INSEE ne varie
// pas d'une observation à l'autre — pas besoin d'un regexp pour ça.
func trimestreDe(periode string) (annee, trimestre int, err error) {
	parts := strings.SplitN(periode, "-Q", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("période inattendue : %q", periode)
	}
	a, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	q, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return a, q, nil
}
