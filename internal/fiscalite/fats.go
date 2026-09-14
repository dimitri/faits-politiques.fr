package fiscalite

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

const eurostatBase = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/"

// Décodeur JSON-stat : copie de internal/dette/eurostat.go. Les valeurs sont
// dans un tableau à plat, indexé en ordre ligne-majeur sur les dimensions
// listées dans id (la dernière varie le plus vite).
type jsonStat struct {
	Error []struct {
		Label string `json:"label"`
	} `json:"error"`
	ID        []string                     `json:"id"`
	Size      []int                        `json:"size"`
	Value     map[string]*float64          `json:"value"`
	Status    map[string]string            `json:"status"`
	Dimension map[string]jsonStatDimension `json:"dimension"`
}

type jsonStatDimension struct {
	Category struct {
		Index map[string]int `json:"index"`
	} `json:"category"`
}

type cellule struct {
	dims   map[string]string
	valeur float64
}

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
		// Valeur confidentielle ou absente : null, écartée (jamais 0).
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

// Deux jeux, une rupture : jusqu'en 2020 la nomenclature SBS « V » sur
// l'économie marchande hors finance (B-N_S95_X_K), depuis 2021 la nomenclature
// « indic_sbs » sur un champ un peu plus large (B-S_X_O_S94). Les deux
// séries sont gardées côte à côte, jamais raccordées.
var jeuxFATS = []struct {
	jeu, activite, indic string
}{
	{"fats_g1b_08", "B-N_S95_X_K", "indic_sb"},
	{"fats_ctrl", "B-S_X_O_S94", "indic_sbs"},
}

func IngestFATS(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceEurostatFATS, func(srcID, runID int64) (map[string]any, error) {
		var lignes [][]any
		stats := map[string]any{}
		for _, j := range jeuxFATS {
			u := eurostatBase + j.jeu + "?format=JSON&lang=EN&geo=FR&nace_r2=" + j.activite
			f, err := arch.Fetch(ctx, srcID, runID, u, ".json")
			if err != nil {
				return nil, err
			}
			b, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, err
			}
			var js jsonStat
			if err := json.Unmarshal(b, &js); err != nil {
				return nil, fmt.Errorf("%s : %w", j.jeu, err)
			}
			if len(js.Error) > 0 {
				return nil, fmt.Errorf("%s : Eurostat : %s", j.jeu, js.Error[0].Label)
			}
			cells, err := js.cellules()
			if err != nil {
				return nil, fmt.Errorf("%s : %w", j.jeu, err)
			}
			if len(cells) < 1000 {
				return nil, fmt.Errorf("%s : %d valeurs seulement", j.jeu, len(cells))
			}
			for _, c := range cells {
				annee, err := strconv.Atoi(c.dims["time"])
				if err != nil {
					return nil, fmt.Errorf("%s : période %q", j.jeu, c.dims["time"])
				}
				lignes = append(lignes, []any{"FR", annee, c.dims["c_ctrl"], j.activite, c.dims[j.indic], j.jeu, c.valeur, f.DocumentID})
			}
			stats[j.jeu] = len(cells)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.fats_controle WHERE pays_hote = 'FR'`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "fats_controle"},
			[]string{"pays_hote", "annee", "pays_controle", "activite", "indicateur", "serie", "valeur", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		return stats, tx.Commit(ctx)
	})
}
