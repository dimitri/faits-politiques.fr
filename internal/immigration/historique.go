package immigration

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceHistoriqueINSEE = archive.Source{
	Slug: "insee-population-immigree-etrangere-historique", Label: "Insee — population immigrée et étrangère depuis 1921",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, recensements de la population et estimations",
	Cadence:     "annuelle (recensements espacés avant 2006)",
	Notes: "Rupture de série à partir de 2024 (protocole de collecte revu), et changement " +
		"de champ géographique en 1990 (métropole -> hors Mayotte) puis 2014 (Mayotte " +
		"incluse) : la source elle-même avertit de ne pas comparer ces bornes sans réserve.",
}

const historiqueURL = "https://www.insee.fr/fr/statistiques/fichier/2381757/demo-etran-part-pop-etran-immig.xlsx"

var reAnneeHisto = regexp.MustCompile(`^(\d{4})`)

func IngestHistorique(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceHistoriqueINSEE)
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

	f, err := arch.Fetch(ctx, srcID, runID, historiqueURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSXImmigration(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	fig1, err := x.rows("Figure 1")
	if err != nil {
		return fail(err)
	}
	fig2, err := x.rows("Figure 2")
	if err != nil {
		return fail(err)
	}

	// Figure 1 : A=année, B=immigrés (milliers), C=part (%), D=population totale.
	// Figure 2 : A=année, B=Français de naissance, D=Français par acquisition,
	// F=étrangers (milliers), G=part (%), H=population totale.
	type ligne1 struct{ immigres, immigresPct, popTotale float64 }
	m1 := map[int]ligne1{}
	for _, l := range fig1 {
		annee, champ, ok := anneeChampHisto(l["A"])
		if !ok {
			continue
		}
		imm, e1 := strconv.ParseFloat(l["B"], 64)
		pct, e2 := strconv.ParseFloat(l["C"], 64)
		pop, e3 := strconv.ParseFloat(l["D"], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		m1[annee] = ligne1{imm, pct, pop}
		_ = champ
	}

	var rows [][]any
	for _, l := range fig2 {
		annee, champ, ok := anneeChampHisto(l["A"])
		if !ok {
			continue
		}
		v1, ok1 := m1[annee]
		if !ok1 {
			continue
		}
		fn, e1 := strconv.ParseFloat(l["B"], 64)
		fa, e2 := strconv.ParseFloat(l["D"], 64)
		etr, e3 := strconv.ParseFloat(l["F"], 64)
		etrPct, e4 := strconv.ParseFloat(l["G"], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			continue
		}
		rows = append(rows, []any{annee, v1.popTotale, v1.immigres, v1.immigresPct, fn, fa, etr, etrPct, champ, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune année reconnue (figure 1 : %d lignes, figure 2 : %d lignes)", len(fig1), len(fig2)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.population_historique_nationalite`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "population_historique_nationalite"},
		[]string{"annee", "population_totale_milliers", "immigres_milliers", "immigres_pct",
			"francais_naissance_milliers", "francais_acquisition_milliers", "etrangers_milliers",
			"etrangers_pct", "champ", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("population_historique_nationalite : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees_chargees": n}, "")
	fmt.Printf("  population immigrée et étrangère, 1921-2025 : %d millésimes\n", n)
	return nil
}

// anneeChampHisto lit un libellé d'année tel que "2024 (p) (*)" ou "1921", et
// déduit le champ géographique de la même règle que la source documente en
// note : métropole jusqu'en 1982, France (hors puis avec Mayotte) ensuite.
func anneeChampHisto(lib string) (int, string, bool) {
	m := reAnneeHisto.FindStringSubmatch(strings.TrimSpace(lib))
	if m == nil {
		return 0, "", false
	}
	annee, _ := strconv.Atoi(m[1])
	if annee < 1900 || annee > 2100 {
		return 0, "", false
	}
	champ := "FRANCE"
	if annee <= 1982 {
		champ = "METROPOLE"
	}
	return annee, champ, true
}
