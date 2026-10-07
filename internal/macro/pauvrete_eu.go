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

// Le taux de pauvreté français (core.pauvrete_seuil_annuel) n'a de sens en
// comparaison internationale que rapproché d'une mesure construite pareil
// ailleurs. Voir docs/pauvrete-donnees.md § 5.
var SourcePovertyRateEU = archive.Source{
	Slug: "eurostat-taux-pauvrete", Label: "Eurostat — taux de risque de pauvreté par pays (tps00184)",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	License: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat (tps00184)",
	Cadence:     "annuelle",
	Notes: "Seuil à 60 % du revenu médian équivalent, population totale (sexe=T). Ne couvre " +
		"que les pays européens (UE, AELE, candidats et Royaume-Uni) — ni les États-Unis, ni " +
		"le Japon, ni le Canada, absents de la nomenclature géographique Eurostat. Une " +
		"comparaison avec ces pays doit passer par l'OCDE (seuil à 50 %, non directement " +
		"comparable) — voir docs/pauvrete-donnees.md § 5.",
}

const povertyEuURL = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/" +
	"tps00184?format=JSON&lang=FR&sex=T"

func IngestPovertyRateEU(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePovertyRateEU)
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

	f, err := arch.Fetch(ctx, srcID, runID, povertyEuURL, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var js jsonStatPoverty
	if err := json.Unmarshal(b, &js); err != nil {
		return fail(err)
	}
	if len(js.Error) > 0 {
		return fail(fmt.Errorf("Eurostat : %s", js.Error[0].Label))
	}
	cells, err := js.cells()
	if err != nil {
		return fail(err)
	}
	if len(cells) == 0 {
		return fail(fmt.Errorf("taux de pauvreté UE : aucune cellule décodée"))
	}
	geoLabel := js.Dimension["geo"].Category.Label

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_pauvrete_taux_eu (
			geo_code text, geo_label text, annee int, taux_pct numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, c := range cells {
		year, err := strconv.Atoi(c.dims["time"])
		if err != nil {
			continue
		}
		rows = append(rows, []any{c.dims["geo"], geoLabel[c.dims["geo"]], year, c.value, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_pauvrete_taux_eu"},
		[]string{"geo_code", "geo_label", "annee", "taux_pct", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("pauvrete_taux_eu : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité des pays et années à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.pauvrete_taux_eu AS tgt
		USING tmp_pauvrete_taux_eu AS src
		ON tgt.geo_code = src.geo_code AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.geo_label, tgt.taux_pct, tgt.source_id)
		                  IS DISTINCT FROM (src.geo_label, src.taux_pct, src.source_id) THEN
		    UPDATE SET geo_label = src.geo_label, taux_pct = src.taux_pct, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (geo_code, geo_label, annee, taux_pct, source_id)
		    VALUES (src.geo_code, src.geo_label, src.annee, src.taux_pct, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion pauvrete_taux_eu : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  taux de pauvreté européen (Eurostat) : %d lignes touchées par la fusion\n", n)
	return nil
}

type jsonStatPoverty struct {
	Error []struct {
		Label string `json:"label"`
	} `json:"error"`
	ID        []string                            `json:"id"`
	Size      []int                               `json:"size"`
	Value     map[string]*float64                 `json:"value"`
	Dimension map[string]jsonStatPovertyDimension `json:"dimension"`
}

type jsonStatPovertyDimension struct {
	Category struct {
		Index map[string]int    `json:"index"`
		Label map[string]string `json:"label"`
	} `json:"category"`
}

type povertyCell struct {
	dims  map[string]string
	value float64
}

// cells décode un cube JSON-stat générique — même algorithme que
// internal/dette/eurostat.go et internal/ecologie/depenses_environnementales.go,
// dupliqué ici plutôt que partagé (convention déjà établie dans ce dépôt).
func (js *jsonStatPoverty) cells() ([]povertyCell, error) {
	if len(js.ID) != len(js.Size) {
		return nil, fmt.Errorf("JSON-stat : %d dimensions pour %d tailles", len(js.ID), len(js.Size))
	}
	codes := make([][]string, len(js.ID))
	for i, id := range js.ID {
		dim, ok := js.Dimension[id]
		if !ok {
			return nil, fmt.Errorf("JSON-stat : dimension %q non décrite", id)
		}
		codes[i] = make([]string, js.Size[i])
		for code, pos := range dim.Category.Index {
			if pos < 0 || pos >= js.Size[i] {
				return nil, fmt.Errorf("JSON-stat : %s=%s hors bornes", id, code)
			}
			codes[i][pos] = code
		}
	}
	out := make([]povertyCell, 0, len(js.Value))
	for key, v := range js.Value {
		if v == nil {
			continue
		}
		n, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("JSON-stat : index %q", key)
		}
		dims := make(map[string]string, len(js.ID))
		remainder := n
		for i := len(js.ID) - 1; i >= 0; i-- {
			dims[js.ID[i]] = codes[i][remainder%js.Size[i]]
			remainder /= js.Size[i]
		}
		if remainder != 0 {
			return nil, fmt.Errorf("JSON-stat : index %d hors du cube", n)
		}
		out = append(out, povertyCell{dims: dims, value: *v})
	}
	return out, nil
}
