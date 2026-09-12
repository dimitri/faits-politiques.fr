package presidentielle

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

var SourcePopulation = archive.Source{
	Slug: "insee-population-age", Label: "INSEE — population au 1er janvier par âge détaillé",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : INSEE, estimations de population, séries longues",
	Cadence:     "annuelle",
	Notes: "Une feuille par année, de 1901 à aujourd'hui. Deux champs : France " +
		"métropolitaine (colonne C) et France entière (colonne F), cette dernière " +
		"absente des feuilles anciennes. La dernière ligne de chaque feuille est " +
		"une tranche ouverte (« 100 ou plus »), pas un âge.",
}

const PopulationURL = "https://www.insee.fr/fr/statistiques/fichier/8560651/3_Pop1janv_age.xlsx"

// Les colonnes du fichier. Elles sont stables sur toute la série ; on les fige
// ici plutôt que de deviner d'après l'en-tête, qui est sur plusieurs lignes
// fusionnées et n'est pas lisible mécaniquement.
const (
	colAge    = "B"
	colMetro  = "C" // France métropolitaine, ensemble
	colFrance = "F" // France entière, ensemble — absente avant 1991
)

// Une feuille est nommée par son millésime, rien d'autre.
var reAnnee = regexp.MustCompile(`^\d{4}$`)

// « 100 ou plus », « 105 ou plus » : tranche ouverte, à charger avec son âge
// plancher et le drapeau qui dit qu'elle agrège la suite.
var reAgeOuvert = regexp.MustCompile(`^(\d+)\s*(?:ou|et)\s+plus$`)

func IngestPopulation(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePopulation)
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

	f, err := arch.Fetch(ctx, srcID, runID, PopulationURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	var rows [][]any
	annees := 0
	for _, sheet := range x.sheetNames() {
		if !reAnnee.MatchString(sheet) {
			continue
		}
		annee, _ := strconv.Atoi(sheet)
		lignes, err := x.rows(sheet)
		if err != nil {
			return fail(err)
		}
		vu := map[string]bool{}
		for _, l := range lignes {
			age, ouvert, ok := lireAge(l[colAge])
			if !ok {
				continue
			}
			for _, c := range []struct{ champ, col string }{{"METRO", colMetro}, {"FRANCE", colFrance}} {
				v, ok := l[c.col]
				if !ok {
					continue
				}
				n, err := strconv.ParseInt(strings.ReplaceAll(v, " ", ""), 10, 64)
				if err != nil {
					continue
				}
				// Un âge présent deux fois dans une feuille fausserait toute somme
				// sans que rien ne le signale. On refuse plutôt que de dédupliquer.
				k := c.champ + ":" + strconv.Itoa(age)
				if vu[k] {
					return fail(fmt.Errorf("feuille %s : âge %d présent deux fois pour le champ %s", sheet, age, c.champ))
				}
				vu[k] = true
				rows = append(rows, []any{c.champ, annee, age, ouvert, n, srcID})
			}
		}
		annees++
	}
	if annees == 0 {
		return fail(fmt.Errorf("aucune feuille annuelle dans %s", PopulationURL))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Reconstruction complète : une révision INSEE change les valeurs des années
	// passées, et compléter laisserait cohabiter deux millésimes d'estimation.
	if _, err := tx.Exec(ctx, `TRUNCATE core.population_age`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "population_age"},
		[]string{"champ", "annee", "age", "age_ouvert", "population", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"annees": annees, "lignes": len(rows)}, "")
	fmt.Printf("  population par âge : %d années, %d lignes\n", annees, len(rows))
	return nil
}

func lireAge(s string) (age int, ouvert bool, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, false, true
	}
	if m := reAgeOuvert.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, true, true
	}
	return 0, false, false
}
