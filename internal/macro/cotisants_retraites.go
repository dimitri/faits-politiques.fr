package macro

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le rapport démographique cotisants/retraités — la mesure la plus directe de
// la pression sur un système de retraite par répartition. Voir
// docs/retraite-donnees.md.
var SourceCotisantsRetraites = archive.Source{
	Slug: "insee-cotisants-retraites-ratio", Label: "Insee — cotisants, retraités et rapport démographique",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees (EACR, EIR, modèle ANCETRE) ; Insee, comptes nationaux",
	Cadence:     "annuelle",
	Notes: "Rupture de série en 2020 : les effectifs de retraités résidant à l'étranger " +
		"ont été revus à la baisse. Champ : retraités ayant perçu un droit direct au " +
		"cours de l'année, résidant en France ou à l'étranger, vivants au 31 décembre.",
}

const cotisantsRetraitesURL = "https://www.insee.fr/fr/statistiques/fichier/2415121/reve-protec-cotisant-retraite.xlsx"

func IngestCotisantsRetraites(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCotisantsRetraites)
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

	f, err := arch.Fetch(ctx, srcID, runID, cotisantsRetraitesURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()
	lignes, err := x.rows("Données")
	if err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, l := range lignes {
		lib, ok := l["A"]
		if !ok {
			continue
		}
		// L'année de rupture de série porte une note en exposant collée au
		// nombre : "20203" pour 2020, note 3. Les quatre premiers caractères
		// sont toujours l'année ; le reste, s'il y en a, est le numéro de note.
		if len(lib) < 4 {
			continue
		}
		annee, err := strconv.Atoi(lib[:4])
		if err != nil || annee < 1990 || annee > 2100 {
			continue
		}
		cot, e1 := strconv.ParseFloat(l["B"], 64)
		ret, e2 := strconv.ParseFloat(l["C"], 64)
		rap, e3 := strconv.ParseFloat(l["D"], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		rows = append(rows, []any{annee, cot, ret, rap, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne reconnue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.cotisants_retraites_ratio`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "cotisants_retraites_ratio"},
		[]string{"annee", "cotisants_millions", "retraites_millions", "ratio_demographique", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("cotisants_retraites_ratio : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees_chargees": n}, "")
	fmt.Printf("  ratio cotisants/retraités : %d millésimes\n", n)
	return nil
}
