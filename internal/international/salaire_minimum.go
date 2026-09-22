// Package international charge les comparaisons internationales du chantier
// 11 (docs/international-donnees.md) : la France resituée dans son contexte
// européen, G8, mondial — pas un jugement sur qui fait mieux, une mesure côte
// à côte, avec les réserves de comparabilité que chaque source impose.
package international

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

const ConnectorVersion = "international-v1"

// Le seul jeu identifié qui réunit l'Europe et un pays du G8 hors Union dans
// la même définition (salaire minimum légal mensuel brut) : les États-Unis
// y figurent, ni le Japon ni le Canada (minimums fixés par État/province,
// pas de minimum fédéral unique — un fait à documenter, pas une lacune de
// chargement). Voir docs/international-donnees.md § 11.10.
var SourceSalaireMinimum = archive.Source{
	Slug: "eurostat-salaire-minimum", Label: "Eurostat — salaire minimum légal mensuel (earn_mw_cur)",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat (earn_mw_cur)",
	Cadence:     "semestrielle",
	Notes: "Couvre les pays européens et les États-Unis, mais ni le Japon ni le Canada (minimum " +
		"fixé par État ou province, pas de minimum fédéral unique dans les deux cas). L'absence " +
		"d'un pays pour un semestre signale l'absence de salaire minimum légal (Allemagne avant " +
		"2015, pays nordiques à négociation collective) — jamais une valeur nulle.",
}

const salaireMinimumURL = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/" +
	"earn_mw_cur?format=JSON&lang=FR"

func IngestSalaireMinimum(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSalaireMinimum)
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

	f, err := arch.Fetch(ctx, srcID, runID, salaireMinimumURL, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var js jsonStatSMIC
	if err := json.Unmarshal(b, &js); err != nil {
		return fail(err)
	}
	if len(js.Error) > 0 {
		return fail(fmt.Errorf("Eurostat : %s", js.Error[0].Label))
	}
	cells, err := js.cellules()
	if err != nil {
		return fail(err)
	}
	if len(cells) == 0 {
		return fail(fmt.Errorf("salaire minimum : aucune cellule décodée"))
	}
	libGeo := js.Dimension["geo"].Category.Label

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_salaire_minimum (
			geo_code text, geo_label text, semestre text, unite text, valeur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, c := range cells {
		rows = append(rows, []any{c.dims["geo"], libGeo[c.dims["geo"]], c.dims["time"], c.dims["currency"], c.valeur, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_salaire_minimum"},
		[]string{"geo_code", "geo_label", "semestre", "unite", "valeur", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("salaire_minimum : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table ; l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la table à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.salaire_minimum AS tgt
		USING tmp_salaire_minimum AS src
		ON tgt.geo_code = src.geo_code AND tgt.semestre = src.semestre AND tgt.unite = src.unite
		WHEN MATCHED AND (tgt.geo_label, tgt.valeur, tgt.source_id)
		                  IS DISTINCT FROM (src.geo_label, src.valeur, src.source_id) THEN
		    UPDATE SET geo_label = src.geo_label, valeur = src.valeur, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (geo_code, geo_label, semestre, unite, valeur, source_id)
		    VALUES (src.geo_code, src.geo_label, src.semestre, src.unite, src.valeur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion salaire_minimum : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": touchees}, "")
	fmt.Printf("  salaire minimum (Eurostat) : %d lignes touchées\n", touchees)
	return nil
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestSalaireMinimum(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestPIBEpargneNette(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestCommerceExtraUE(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestSanteOCDE(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestDepenseSanteOCDE(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestSIPRIMilex(ctx, pool, arch); err != nil {
		return err
	}
	return IngestIDEA(ctx, pool, arch)
}

type jsonStatSMIC struct {
	Error []struct {
		Label string `json:"label"`
	} `json:"error"`
	ID        []string                         `json:"id"`
	Size      []int                            `json:"size"`
	Value     map[string]*float64              `json:"value"`
	Dimension map[string]jsonStatSMICDimension `json:"dimension"`
}

type jsonStatSMICDimension struct {
	Category struct {
		Index map[string]int    `json:"index"`
		Label map[string]string `json:"label"`
	} `json:"category"`
}

type celluleSMIC struct {
	dims   map[string]string
	valeur float64
}

// cellules décode un cube JSON-stat générique — même algorithme que
// internal/dette/eurostat.go, internal/ecologie et internal/macro/pauvrete_eu.go,
// dupliqué ici plutôt que partagé (convention déjà établie dans ce dépôt).
func (js *jsonStatSMIC) cellules() ([]celluleSMIC, error) {
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
	out := make([]celluleSMIC, 0, len(js.Value))
	for cle, v := range js.Value {
		if v == nil {
			continue
		}
		n, err := strconv.Atoi(cle)
		if err != nil {
			return nil, fmt.Errorf("JSON-stat : index %q", cle)
		}
		dims := make(map[string]string, len(js.ID))
		reste := n
		for i := len(js.ID) - 1; i >= 0; i-- {
			dims[js.ID[i]] = codes[i][reste%js.Size[i]]
			reste /= js.Size[i]
		}
		if reste != 0 {
			return nil, fmt.Errorf("JSON-stat : index %d hors du cube", n)
		}
		out = append(out, celluleSMIC{dims: dims, valeur: *v})
	}
	return out, nil
}
