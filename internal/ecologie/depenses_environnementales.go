// Package ecologie charge les données ouvertes de dépense environnementale.
// Voir docs/ecologie-donnees.md.
package ecologie

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

const ConnectorVersion = "ecologie-v1"

// La dépense de protection de l'environnement au sens large (CEPA — pas le
// budget vert, qui cote des dépenses existantes, ni la fiscalité écologique
// stricte, qui ne compte qu'un impôt étroit). Voir docs/ecologie-donnees.md § 4.
var SourceDepenseEnvironnementale = archive.Source{
	Slug: "eurostat-depense-environnementale", Label: "Eurostat — dépense de protection de l'environnement (CEP)",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat (env_epea_neep)",
	Cadence:     "annuelle",
	Notes: "Classification CEP (anciennement CEPA/CReMA jusqu'en 2024, transition Eurostat vers " +
		"CEP à partir de la collecte 2025). Le total (purpose_code=TOT_CEP_EP) et le total " +
		"économie (secteur_code=S1) sont déjà des sommes de leurs sous-catégories — voir le " +
		"commentaire de core.depense_environnementale.",
}

const depenseEnvURL = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/" +
	"env_epea_neep?geo=FR&format=JSON&lang=en"

type jsonStat struct {
	Error []struct {
		Label string `json:"label"`
	} `json:"error"`
	ID        []string                     `json:"id"`
	Size      []int                        `json:"size"`
	Value     map[string]*float64          `json:"value"`
	Dimension map[string]jsonStatDimension `json:"dimension"`
}

type jsonStatDimension struct {
	Category struct {
		Index map[string]int    `json:"index"`
		Label map[string]string `json:"label"`
	} `json:"category"`
}

type cellule struct {
	dims   map[string]string
	valeur float64
}

// cellules décode un cube JSON-stat générique — même algorithme que
// internal/dette/eurostat.go's jsonStat.cellules, dupliqué ici plutôt que
// partagé : les deux jeux n'ont rien d'autre en commun, et un import croisé
// entre paquets thématiques ajouterait un couplage pour quinze lignes.
func (js *jsonStat) cellules() ([]cellule, error) {
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
	out := make([]cellule, 0, len(js.Value))
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
		out = append(out, cellule{dims: dims, valeur: *v})
	}
	return out, nil
}

func IngestDepensesEnvironnementales(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDepenseEnvironnementale)
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

	f, err := arch.Fetch(ctx, srcID, runID, depenseEnvURL, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var js jsonStat
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
		return fail(fmt.Errorf("dépense environnementale : aucune cellule décodée"))
	}

	libPurpose := js.Dimension["env_pa"].Category.Label
	libSecteur := js.Dimension["sector"].Category.Label

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	var rows [][]any
	for _, c := range cells {
		annee, err := strconv.Atoi(c.dims["time"])
		if err != nil {
			continue
		}
		rows = append(rows, []any{
			annee, c.dims["env_pa"], libPurpose[c.dims["env_pa"]],
			c.dims["sector"], libSecteur[c.dims["sector"]],
			c.dims["unit"], c.valeur, srcID,
		})
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_depense_environnementale (
			annee smallint, purpose_code text, purpose_libelle text,
			secteur_code text, secteur_libelle text, unite text,
			valeur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_depense_environnementale"},
		[]string{"annee", "purpose_code", "purpose_libelle", "secteur_code", "secteur_libelle",
			"unite", "valeur", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("depense_environnementale : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.depense_environnementale AS tgt
		USING tmp_depense_environnementale AS src
		     ON tgt.annee = src.annee AND tgt.purpose_code = src.purpose_code
		    AND tgt.secteur_code = src.secteur_code AND tgt.unite = src.unite
		WHEN MATCHED AND (tgt.purpose_libelle, tgt.secteur_libelle, tgt.valeur, tgt.source_id)
		     IS DISTINCT FROM (src.purpose_libelle, src.secteur_libelle, src.valeur, src.source_id)
		THEN UPDATE SET purpose_libelle = src.purpose_libelle, secteur_libelle = src.secteur_libelle,
		     valeur = src.valeur, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		     INSERT (annee, purpose_code, purpose_libelle, secteur_code, secteur_libelle, unite, valeur, source_id)
		     VALUES (src.annee, src.purpose_code, src.purpose_libelle, src.secteur_code,
		             src.secteur_libelle, src.unite, src.valeur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion depense_environnementale : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	n := ct.RowsAffected()
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows)}, "")
	fmt.Printf("  dépense environnementale (Eurostat) : %d lignes reçues, %d touchées par la fusion\n", len(rows), n)
	return nil
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return IngestDepensesEnvironnementales(ctx, pool, arch)
}
