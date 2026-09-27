package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceEmploiSecteurNACE = archive.Source{
	Slug: "eurostat-emploi-secteur-nace", Label: "Eurostat — emploi intérieur total par branche d'activité (NACE Rév. 2)",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat, nama_10_a10_e",
	Cadence:     "annuelle",
	Notes: "Nomenclature A10 : la branche C (industrie manufacturière) est une SOUS-catégorie " +
		"de B-E (industrie y compris énergie), publiée en plus, pas en complément — ne jamais " +
		"additionner B-E et C. Série continue 1975-2025 selon Eurostat, aucune rupture documentée.",
}

// libelleNACE : les libellés officiels de la nomenclature A10 (NAF Rév. 2),
// tels qu'utilisés par l'Insee et Eurostat.
var libelleNACE = map[string]string{
	"TOTAL": "Ensemble",
	"A":     "Agriculture, sylviculture et pêche",
	"B-E":   "Industrie (y compris énergie)",
	"C":     "dont industrie manufacturière",
	"F":     "Construction",
	"G-I":   "Commerce, transports, hébergement et restauration",
	"J":     "Information et communication",
	"K":     "Activités financières et d'assurance",
	"L":     "Activités immobilières",
	"M_N":   "Activités spécialisées, scientifiques et techniques ; activités de services administratifs et de soutien",
	"O-Q":   "Administration publique, enseignement, santé humaine et action sociale",
	"R-U":   "Arts, spectacles et activités récréatives ; autres activités de services",
}

const urlEmploiSecteurNACE = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/nama_10_a10_e?" +
	"format=JSON&lang=EN&geo=FR&na_item=EMP_DC&unit=THS_PER"

// IngestEmploiSecteurNACE charge l'emploi intérieur total par branche
// (niveau A10), France, 1975 à aujourd'hui.
func IngestEmploiSecteurNACE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEmploiSecteurNACE)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "emploi-secteur-nace-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlEmploiSecteurNACE, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var doc struct {
		Error     []struct{ Label string } `json:"error"`
		Value     map[string]float64       `json:"value"`
		Dimension map[string]struct {
			Category struct {
				Index map[string]int `json:"index"`
			} `json:"category"`
		} `json:"dimension"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fail(fmt.Errorf("JSON-stat illisible : %w", err))
	}
	if len(doc.Error) > 0 {
		return fail(fmt.Errorf("Eurostat : %s", doc.Error[0].Label))
	}
	naceIdx := doc.Dimension["nace_r2"].Category.Index
	timeIdx := doc.Dimension["time"].Category.Index
	nTemps := len(timeIdx)
	if nTemps == 0 || len(naceIdx) == 0 {
		return fail(fmt.Errorf("dimensions nace_r2 ou time absentes"))
	}

	var rows [][]any
	for code, ci := range naceIdx {
		lib, ok := libelleNACE[code]
		if !ok {
			continue // branches non retenues pour ce dossier (agrégats intermédiaires non listés ci-dessus)
		}
		for anneeStr, ti := range timeIdx {
			v, ok := doc.Value[strconv.Itoa(ci*nTemps+ti)]
			if !ok {
				continue
			}
			annee, err := strconv.Atoi(anneeStr)
			if err != nil {
				continue
			}
			rows = append(rows, []any{annee, code, lib, v, srcID})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune valeur décodée"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité de la série 1975-2025 à chaque republication
	// annuelle, changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_emploi_secteur_nace (
			annee int, code_nace text, libelle_nace text, emploi_milliers numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_emploi_secteur_nace"},
		[]string{"annee", "code_nace", "libelle_nace", "emploi_milliers", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("core.emploi_secteur_nace : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.emploi_secteur_nace AS tgt
		USING tmp_emploi_secteur_nace AS src
		ON tgt.annee = src.annee AND tgt.code_nace = src.code_nace
		WHEN MATCHED AND (tgt.libelle_nace, tgt.emploi_milliers, tgt.source_id)
		                  IS DISTINCT FROM (src.libelle_nace, src.emploi_milliers, src.source_id) THEN
		    UPDATE SET libelle_nace = src.libelle_nace, emploi_milliers = src.emploi_milliers,
		               source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, code_nace, libelle_nace, emploi_milliers, source_id)
		    VALUES (src.annee, src.code_nace, src.libelle_nace, src.emploi_milliers, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": len(rows), "touchees": n}, "")
	fmt.Printf("  Emploi par secteur (NACE A10) : %d lignes (%d touchées par la fusion)\n", len(rows), n)
	return nil
}
