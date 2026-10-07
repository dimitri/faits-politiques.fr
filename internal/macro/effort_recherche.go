package macro

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

var SourceResearchEffort = archive.Source{
	Slug: "insee-effort-recherche", Label: "L'effort de recherche : DIRD/PIB, France et Union européenne",
	Publisher: "Insee (sources MESR-SIES, OCDE)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : Insee, données MESR-SIES (France) et OCDE (UE27)",
	Cadence:     "annuelle",
	Notes:       "UE27 sur toute la période (rétropolée par l'OCDE) ; le dernier point est une estimation.",
}

const researchEffortURL = "https://www.insee.fr/fr/statistiques/fichier/3281637/Effort-recherche_tableaux_2024.xlsx"

var reYearEstimate = regexp.MustCompile(`^(\d{4})\s*(\(estim\))?$`)

func parseFrenchFloat(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

// IngestResearchEffort charge la DIRD/PIB et la DIRDE/PIB, France et UE27.
// Voir docs/recherche-enseignement-superieur-donnees.md.
func IngestResearchEffort(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceResearchEffort)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "effort-recherche-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, researchEffortURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur illisible : %w", err))
	}
	defer wb.Close()

	sheetRows, err := wb.GetRows("Tableau 1")
	if err != nil {
		return fail(fmt.Errorf("feuille illisible : %w", err))
	}

	type row struct {
		year                             int
		estimated                        bool
		dirdFR, dirdUE, dirdeFR, dirdeUE *float64
	}
	var rows []row
	for _, r := range sheetRows {
		if len(r) == 0 {
			continue
		}
		m := reYearEstimate.FindStringSubmatch(strings.TrimSpace(r[0]))
		if m == nil {
			continue // titre, en-tête ou note de bas de tableau
		}
		year, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		row := row{year: year, estimated: m[2] != ""}
		get := func(i int) *float64 {
			if i >= len(r) {
				return nil
			}
			return parseFrenchFloat(r[i])
		}
		row.dirdFR, row.dirdUE, row.dirdeFR, row.dirdeUE = get(1), get(2), get(3), get(4)
		rows = append(rows, row)
	}
	if len(rows) < 25 {
		return fail(fmt.Errorf("seulement %d années lues, attendu au moins 25", len(rows)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.effort_recherche`); err != nil {
		return fail(err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.effort_recherche (annee, dird_pib_fr, dird_pib_ue27, dirde_pib_fr, dirde_pib_ue27, estimation, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			row.year, row.dirdFR, row.dirdUE, row.dirdeFR, row.dirdeUE, row.estimated, srcID); err != nil {
			return fail(fmt.Errorf("%d : insertion : %w", row.year, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees": len(rows)}, "")
	fmt.Printf("  Effort de recherche (DIRD/PIB) : %d années\n", len(rows))
	return nil
}
