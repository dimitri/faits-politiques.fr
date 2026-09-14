package macro

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

var SourceAgeDepartRetraite = archive.Source{
	Slug: "drees-age-depart-retraite", Label: "Drees — âge conjoncturel moyen de départ à la retraite",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees",
	Cadence:     "annuelle",
	Notes: "Indicateur CONJONCTUREL, calculé sur les départs d'une seule année (comme un " +
		"indice conjoncturel de fécondité) : pas l'âge moyen réel auquel une génération " +
		"donnée est partie, qui ne se connaît qu'une fois cette génération entièrement " +
		"retraitée.",
}

const ageDepartURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"retraite_graphique-1-age-conjoncturel-moyen-de-depart-a-la-retraite-selon-le-se0/exports/json"

type ligneAgeDepart struct {
	Annee    string  `json:"annee"`
	Femmes   float64 `json:"femmes"`
	Hommes   float64 `json:"hommes"`
	Ensemble float64 `json:"ensemble"`
}

func IngestAgeDepartRetraite(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAgeDepartRetraite)
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

	f, err := arch.Fetch(ctx, srcID, runID, ageDepartURL, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var lignes []ligneAgeDepart
	if err := json.Unmarshal(raw, &lignes); err != nil {
		return fail(fmt.Errorf("export illisible : %w", err))
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("export vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.age_depart_retraite`); err != nil {
		return fail(err)
	}
	var rows [][]any
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee)
		if err != nil {
			continue
		}
		rows = append(rows, []any{annee, l.Femmes, l.Hommes, l.Ensemble, srcID})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "age_depart_retraite"},
		[]string{"annee", "age_femmes", "age_hommes", "age_ensemble", "source_id"}, pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("age_depart_retraite : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees_chargees": n}, "")
	fmt.Printf("  âge de départ à la retraite : %d millésimes\n", n)
	return nil
}
