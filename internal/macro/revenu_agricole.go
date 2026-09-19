package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceRevenuAgricole = archive.Source{
	Slug: "eurostat-aact-eaa06", Label: "Eurostat, aact_eaa06 — revenu agricole réel par unité de travail",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Eurostat (réutilisation libre avec attribution)", ReuseClass: "OPEN",
	Attribution: "Source : Eurostat, aact_eaa06",
	Cadence:     "annuelle",
	Notes: "RFI_AWU_CLV, unité CLV15_EUR_AWU : le revenu réel des facteurs de production en agriculture " +
		"par unité de travail annuel, en euros constants 2015 — un agrégat macroéconomique par actif, " +
		"pas un revenu personnel observé par enquête.",
}

const urlRevenuAgricole = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/aact_eaa06?format=JSON&lang=EN&geo=FR&geo=EU27_2020&indic_agr=RFI_AWU_CLV&unit=CLV15_EUR_AWU"

var geoLabelsAgricole = map[string]string{"FR": "France", "EU27_2020": "Union européenne (27)"}

// IngestRevenuAgricole charge le revenu agricole réel par UTA, France et
// Union européenne. Voir docs/agriculture-donnees.md.
func IngestRevenuAgricole(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceRevenuAgricole)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "revenu-agricole-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlRevenuAgricole, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var geoDim struct {
		Category struct {
			Index map[string]int `json:"index"`
		} `json:"category"`
	}
	var full struct {
		Value     map[string]float64 `json:"value"`
		Dimension struct {
			Geo  json.RawMessage `json:"geo"`
			Time struct {
				Category struct {
					Index map[string]int `json:"index"`
				} `json:"category"`
			} `json:"time"`
		} `json:"dimension"`
		Size []int `json:"size"`
	}
	if err := json.Unmarshal(raw, &full); err != nil {
		return fail(fmt.Errorf("JSON-stat illisible : %w", err))
	}
	if err := json.Unmarshal(full.Dimension.Geo, &geoDim); err != nil {
		return fail(fmt.Errorf("dimension geo illisible : %w", err))
	}
	if len(full.Size) != 5 {
		return fail(fmt.Errorf("forme inattendue (%d dimensions, 5 attendues)", len(full.Size)))
	}
	nTime := full.Size[4]

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.revenu_agricole_reel`); err != nil {
		return fail(err)
	}

	var n int
	for geoCode, iGeo := range geoDim.Category.Index {
		label, ok := geoLabelsAgricole[geoCode]
		if !ok {
			return fail(fmt.Errorf("%s : libellé géographique inconnu", geoCode))
		}
		for anneeStr, iTime := range full.Dimension.Time.Category.Index {
			idx := iGeo*nTime + iTime
			v, ok := full.Value[fmt.Sprint(idx)]
			if !ok {
				continue
			}
			var annee int
			if _, err := fmt.Sscanf(anneeStr, "%d", &annee); err != nil {
				return fail(fmt.Errorf("année illisible : %q", anneeStr))
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.revenu_agricole_reel (geo_code, geo_label, annee, euro_par_uta, source_id)
				VALUES ($1,$2,$3,$4,$5)`, geoCode, label, annee, v, srcID); err != nil {
				return fail(fmt.Errorf("%s %d : insertion : %w", geoCode, annee, err))
			}
			n++
		}
	}
	if n == 0 {
		return fail(fmt.Errorf("aucune valeur chargée"))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  Revenu agricole réel par UTA (Eurostat) : %d lignes\n", n)
	return nil
}
