package education

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le taux de réussite au baccalauréat, demandé nommément depuis l'INSEE —
// pas RERS, qui publie pourtant le même indicateur (tableau 8.02) sur une
// période plus longue et sans trou (voir docs/education-donnees.md).
// L'INSEE republie ici la série Depp dans sa banque de données
// macro-économiques (BDM), même mécanique que
// internal/macro/chomage_insee.go (API SDMX, pas de fichier téléchargé : la
// réponse EST la donnée).
var SourceInseeBacReussite = archive.Source{
	Slug: "insee-bac-reussite", Label: "INSEE — taux de réussite au baccalauréat, France",
	Publisher: "INSEE (série Depp republiée en BDM)",
	Tier:      "PRIMARY_OFFICIAL",
	License:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : INSEE, taux de réussite au baccalauréat, série 001769473",
	Cadence:     "annuelle",
	Notes: "Tous baccalauréats confondus (général, technologique, professionnel) — l'INSEE publie " +
		"aussi une série par série de bac (001769271 pour le seul bac général, etc.), non chargées " +
		"ici, seule la série agrégée demandée. La série ne commence qu'en 2011 (RERS, source " +
		"première de cette donnée, remonte à 1980) et n'a AUCUNE observation pour 2022 et 2023 — " +
		"un trou réel de cette republication INSEE au moment du chargement, vérifié en relisant la " +
		"réponse SDMX brute, pas un filtrage de ce connecteur.",
}

const inseeBacReussiteURL = "https://www.bdm.insee.fr/series/sdmx/data/SERIES_BDM/001769473"

type sdmxDataSetBac struct {
	Series struct {
		IDBank string       `xml:"IDBANK,attr"`
		Obs    []sdmxObsBac `xml:"Obs"`
	} `xml:"DataSet>Series"`
}

type sdmxObsBac struct {
	Periode string `xml:"TIME_PERIOD,attr"`
	Valeur  string `xml:"OBS_VALUE,attr"`
}

func IngestBacReussite(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceInseeBacReussite)
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

	f, err := arch.Fetch(ctx, srcID, runID, inseeBacReussiteURL, ".xml")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var ds sdmxDataSetBac
	if err := xml.Unmarshal(raw, &ds); err != nil {
		return fail(fmt.Errorf("réponse SDMX illisible : %w", err))
	}
	if len(ds.Series.Obs) == 0 {
		return fail(fmt.Errorf("aucune observation dans la série %s", ds.Series.IDBank))
	}

	var rows [][]any
	for _, o := range ds.Series.Obs {
		annee, err := strconv.Atoi(o.Periode)
		if err != nil {
			continue // une ligne mal formée n'invalide pas les autres
		}
		v, err := strconv.ParseFloat(o.Valeur, 64)
		if err != nil {
			continue
		}
		rows = append(rows, []any{annee, v, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("%d observations lues, aucune exploitable", len(ds.Series.Obs)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_education_bac_reussite (
			annee integer, taux_pct numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_education_bac_reussite"},
		[]string{"annee", "taux_pct", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO core.education_bac_reussite AS tgt
		USING tmp_education_bac_reussite AS src
		ON tgt.annee = src.annee
		WHEN MATCHED AND (tgt.taux_pct, tgt.source_id) IS DISTINCT FROM (src.taux_pct, src.source_id) THEN
		    UPDATE SET taux_pct = src.taux_pct, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, taux_pct, source_id) VALUES (src.annee, src.taux_pct, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("fusion education_bac_reussite : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees": len(rows)}, "")
	fmt.Printf("  taux de réussite au baccalauréat (INSEE, série %s) : %d années\n", ds.Series.IDBank, len(rows))
	return nil
}
