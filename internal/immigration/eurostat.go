package immigration

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

var SourceEurostatMigration = archive.Source{
	Slug: "eurostat-migration", Label: "Eurostat — population par citoyenneté et par pays de naissance",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat, migr_pop1ctz et migr_pop3ctb",
	Cadence:     "annuelle",
	Notes: "Comparaison harmonisée européenne : la même distinction que core." +
		"population_statut_migratoire (immigré ~ né à l'étranger, étranger ~ citoyenneté " +
		"étrangère), mesurée par une source et une méthode différentes de l'Insee — les " +
		"deux ne sont pas censées coïncider au chiffre près. La décomposition national / " +
		"UE27 / hors UE27 n'est complète pour la France qu'à partir de 2015 ; avant, seul " +
		"le total est fiable (constaté, pas documenté par Eurostat).",
}

// Quatre catégories seulement, sur les ~290 que chaque dataflow publie par
// pays : le national (NAT), le reste de l'Union à 27 (EU27_2020_FOR), le hors
// UE27 (NEU27_2020_FOR), et leur somme (TOTAL) — assez pour situer la France,
// pas un inventaire pays par pays que ce projet ne synthétiserait pas.
var eurostatCategorieLib = map[string]string{
	"TOTAL": "TOTAL", "NAT": "NATIONAL", "EU27_2020_FOR": "UE27_AUTRE", "NEU27_2020_FOR": "HORS_UE27",
}

const eurostatMigrBase = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/"

func IngestEurostatMigration(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEurostatMigration)
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

	citoyennete, err := fetchEurostatMigr(ctx, arch, srcID, runID,
		eurostatMigrBase+"migr_pop1ctz?geo=FR&age=TOTAL&sex=T&citizen=TOTAL&citizen=NAT&"+
			"citizen=EU27_2020_FOR&citizen=NEU27_2020_FOR&format=JSON&lang=EN", "citizen")
	if err != nil {
		return fail(fmt.Errorf("citoyenneté : %w", err))
	}
	naissance, err := fetchEurostatMigr(ctx, arch, srcID, runID,
		eurostatMigrBase+"migr_pop3ctb?geo=FR&age=TOTAL&sex=T&c_birth=TOTAL&c_birth=NAT&"+
			"c_birth=EU27_2020_FOR&c_birth=NEU27_2020_FOR&format=JSON&lang=EN", "c_birth")
	if err != nil {
		return fail(fmt.Errorf("pays de naissance : %w", err))
	}

	var rows [][]any
	for _, t := range citoyennete {
		rows = append(rows, []any{"CITOYENNETE", t.Categorie, "FR", t.Annee, t.Valeur, srcID})
	}
	for _, t := range naissance {
		rows = append(rows, []any{"PAYS_NAISSANCE", t.Categorie, "FR", t.Annee, t.Valeur, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune valeur décodée"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.eurostat_population_migratoire`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "eurostat_population_migratoire"},
		[]string{"dimension", "categorie", "pays", "annee", "population", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("eurostat_population_migratoire : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": n}, "")
	fmt.Printf("  Eurostat, population par citoyenneté et pays de naissance : %d lignes\n", n)
	return nil
}

// triplet : catégorie, année, population — le type de retour de
// fetchEurostatMigr.
type triplet struct {
	Categorie string
	Annee     int
	Valeur    float64
}

// fetchEurostatMigr décode un JSON-stat à DEUX dimensions variables (la
// catégorie nommée par catDim, et le temps) — toutes les autres (freq, age,
// unit, sex, geo) sont fixées par la requête à une seule valeur, donc de
// taille 1 dans le tableau `size` et sans effet sur l'index linéaire, qui se
// réduit à catégorie_idx * n_temps + temps_idx.
func fetchEurostatMigr(ctx context.Context, arch *archive.Archive, srcID, runID int64, url, catDim string,
) ([]triplet, error) {
	f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("JSON-stat illisible : %w", err)
	}
	if len(doc.Error) > 0 {
		return nil, fmt.Errorf("Eurostat : %s", doc.Error[0].Label)
	}
	catIdx := doc.Dimension[catDim].Category.Index
	timeIdx := doc.Dimension["time"].Category.Index
	nTemps := len(timeIdx)
	if nTemps == 0 || len(catIdx) == 0 {
		return nil, fmt.Errorf("dimensions %s ou time absentes", catDim)
	}
	var out []triplet
	for code, ci := range catIdx {
		lib, ok := eurostatCategorieLib[code]
		if !ok {
			continue
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
			out = append(out, triplet{lib, annee, v})
		}
	}
	return out, nil
}
