package communes

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// La population par département et grande tranche d'âge — le dénominateur
// qui manquait à toute carte de la vieillesse ou de la dépendance rapportée
// à la population totale plutôt qu'à la population concernée. Voir le
// commentaire de la migration 0115.
var SourcePopulationAgeDepartement = archive.Source{
	Slug: "insee-population-age-departement", Label: "INSEE — population par département, sexe et grande classe d'âge",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INSEE, estimations de population",
	Cadence:     "annuelle",
	Notes: "Une feuille par année, 1975 à l'édition la plus récente. Trois blocs de colonnes " +
		"(Ensemble, Hommes, Femmes) ; seul « Ensemble » est chargé. Les lignes d'agrégat " +
		"(« France métropolitaine », « DOM », « France métropolitaine et DOM ») sont écartées.",
}

const populationAgeDepartementURL = "https://www.insee.fr/fr/statistiques/fichier/8331297/estim-pop-dep-sexe-gca-1975-2025.xlsx"

// reCodeDepartement : un département (deux chiffres, ou 2A/2B pour la Corse)
// ou un DROM (trois chiffres) — jamais une ligne d'agrégat, dont la première
// colonne commence par un nom en toutes lettres.
var reCodeDepartement = regexp.MustCompile(`^(2[AB]|\d{2}|9\d{2})$`)

// trancheEnsemble : les colonnes 2 à 6 (0-indexées) du bloc « Ensemble » de
// chaque feuille — la colonne 0 est le code du département, la colonne 1
// son nom, la colonne 7 le total (reconstruit par somme, jamais stocké),
// les colonnes 8+ les blocs Hommes et Femmes, non chargés.
var trancheEnsemble = []struct {
	col  int
	code string
}{
	{2, "00_19"}, {3, "20_39"}, {4, "40_59"}, {5, "60_74"}, {6, "75_PLUS"},
}

func IngestPopulationAgeDepartement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePopulationAgeDepartement)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "population-age-departement-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, populationAgeDepartementURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur Insee illisible : %w", err))
	}
	defer wb.Close()

	var rows [][]any
	var nAnnees int
	for _, feuille := range wb.GetSheetList() {
		annee, err := strconv.Atoi(feuille)
		if err != nil {
			continue // "À savoir" et toute feuille qui n'est pas un millésime
		}
		lignes, err := wb.GetRows(feuille)
		if err != nil {
			return fail(fmt.Errorf("feuille %q : %w", feuille, err))
		}
		if len(lignes) < 6 {
			return fail(fmt.Errorf("feuille %q trop courte (%d lignes) — format changé", feuille, len(lignes)))
		}
		nAnnees++
		for _, l := range lignes[5:] {
			if len(l) < 7 {
				continue
			}
			champs := strings.Fields(l[0])
			if len(champs) == 0 || !reCodeDepartement.MatchString(champs[0]) {
				continue // ligne vide, ou agrégat France/DOM
			}
			dep := champs[0]
			for _, t := range trancheEnsemble {
				brut := strings.ReplaceAll(strings.TrimSpace(l[t.col]), ",", "")
				pop, err := strconv.Atoi(brut)
				if err != nil {
					return fail(fmt.Errorf("%s %d, tranche %s : %q illisible : %w", dep, annee, t.code, l[t.col], err))
				}
				rows = append(rows, []any{dep, annee, t.code, pop, srcID})
			}
		}
	}
	if nAnnees == 0 {
		return fail(fmt.Errorf("aucune feuille de millésime trouvée — format du classeur changé"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE core.population_age_departement`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "population_age_departement"},
		[]string{"code_departement", "annee", "tranche", "population", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("population_age_departement : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n, "millesimes": nAnnees}, "")
	fmt.Printf("  population par département et âge (Insee) : %d lignes, %d millésimes\n", n, nAnnees)
	return nil
}
