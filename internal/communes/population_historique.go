package communes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// La population par commune sur longue période — vérifiée directement
// (fichier téléchargé, feuille "pop_1876_2023" inspectée) : en-tête sur
// deux lignes (libellés puis codes courts), 19 colonnes historiques
// (PTOT1876 à PSDC1999), puis les millésimes récents (PMUN2006-2023), non
// repris ici (voir la migration 0140). Le fichier ne couvre pas Mayotte.
var SourcePopulationHistorique = archive.Source{
	Slug: "insee-population-historique-communes", Label: "Séries historiques de population par commune, 1876-2023",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, recensements de la population",
	Cadence:     "irrégulière (recensements)",
	Notes: "France hors Mayotte. Millésimes historiques seulement (1876-1999) ; les millésimes " +
		"2006-2023 du même fichier ne sont pas chargés ici. La géographie est celle du " +
		"01/01/2025 : une commune fusionnée porte le code de la commune nouvelle sur toute la " +
		"série. Aucune valeur pour 1946 dans la source (la série saute de 1936 à 1954).",
}

const urlPopulationHistorique = "https://www.insee.fr/fr/statistiques/fichier/3698339/base-pop-historiques-1876-2023.xlsx"

// colonneAnneeHistorique : nom de colonne -> année, dans l'ordre où elles
// apparaissent dans le fichier (colonnes 22 à 40 de la feuille). Le
// préfixe (PSDC = population sans doubles comptes, PTOT = population
// totale) change selon la nomenclature Insee de l'époque, l'année en fin
// de nom suffit pour l'extraire.
var colonnesHistoriques = []struct {
	col   int
	annee int
}{
	{22, 1999}, {23, 1990}, {24, 1982}, {25, 1975}, {26, 1968}, {27, 1962},
	{28, 1954}, {29, 1936}, {30, 1931}, {31, 1926}, {32, 1921}, {33, 1911},
	{34, 1906}, {35, 1901}, {36, 1896}, {37, 1891}, {38, 1886}, {39, 1881}, {40, 1876},
}

func parserPopulation(s string) (int64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// IngestPopulationHistorique charge la population communale 1876-1999.
// Voir docs/seconde-guerre-mondiale-donnees.md et
// docs/premiere-guerre-mondiale-donnees.md.
func IngestPopulationHistorique(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePopulationHistorique)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "population-historique-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlPopulationHistorique, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur illisible : %w", err))
	}
	defer wb.Close()

	rows, err := wb.GetRows("pop_1876_2023")
	if err != nil {
		return fail(fmt.Errorf("feuille illisible : %w", err))
	}
	if len(rows) < 7 {
		return fail(fmt.Errorf("seulement %d lignes lues, en-tête attendu avant la ligne 7", len(rows)))
	}

	var lignes [][]any
	for _, r := range rows[6:] {
		if len(r) < 5 {
			continue
		}
		codeInsee := strings.TrimSpace(r[0])
		if codeInsee == "" {
			continue
		}
		for _, c := range colonnesHistoriques {
			if c.col >= len(r) {
				continue
			}
			v, ok := parserPopulation(r[c.col])
			if !ok {
				continue
			}
			lignes = append(lignes, []any{codeInsee, c.annee, v, srcID})
		}
	}
	if len(lignes) < 100_000 {
		return fail(fmt.Errorf("seulement %d lignes à insérer, attendu plusieurs centaines de milliers", len(lignes)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_population_historique_commune (
			code_insee text NOT NULL,
			annee integer NOT NULL,
			population bigint NOT NULL,
			source_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_population_historique_commune"},
		[]string{"code_insee", "annee", "population", "source_id"},
		pgx.CopyFromRows(lignes)); err != nil {
		return fail(fmt.Errorf("population_historique_commune : %w", err))
	}
	var n int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.population_historique_commune", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.population_historique_commune AS tgt
			USING tmp_population_historique_commune AS src
			ON tgt.code_insee = src.code_insee AND tgt.annee = src.annee
			WHEN MATCHED AND (tgt.population, tgt.source_id)
				IS DISTINCT FROM (src.population, src.source_id) THEN
				UPDATE SET population = src.population, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
				INSERT (code_insee, annee, population, source_id)
				VALUES (src.code_insee, src.annee, src.population, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return fmt.Errorf("population_historique_commune, fusion : %w", err)
		}
		n = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  population historique par commune : %d lignes touchées par la fusion\n", n)
	return nil
}
