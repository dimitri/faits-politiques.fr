package macro

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La population du Viêt Nam, du Cambodge et du Laos depuis 1500 — CLIO-INFRA
// (IISH Amsterdam), pour le dossier Indochine (guerres-decolonisation-donnees.md)
// et le dossier empire colonial. Licence CC0-1.0, vérifiée sur la fiche
// Dataverse (hdl.handle.net/10622/SNETZV) avant tout chargement. Un fichier
// mondial (191 pays) : on ne charge que les trois pays utiles ici, jamais le
// reste tant qu'aucun autre dossier n'en a besoin (voir le commentaire de la
// migration 0153 sur le périmètre — frontières actuelles, pas coloniales).
var SourceCLIOInfraPopulation = archive.Source{
	Slug: "clio-infra-population", Label: "CLIO-INFRA — Total Population",
	Publisher: "IISH Amsterdam (CLIO-INFRA)", Tier: "SECONDARY_PRESS",
	Licence: "CC0-1.0", ReuseClass: "OPEN",
	Attribution: "Source : CLIO-INFRA, Total Population (clio-infra.eu)",
	Cadence:     "ponctuelle (série historique figée, non révisée)",
	Notes: "Séries par pays aux frontières ACTUELLES, pas aux frontières coloniales : la " +
		"population du « Viêt Nam » couvre tout le territoire réunifié à chaque date, y compris " +
		"avant l'indépendance de 1945 et avant la partition de 1954. Jamais le même périmètre que " +
		"geo.territoire_colonial (Indochine française).",
}

const clioInfraPopulationURL = "https://clio-infra.eu/data/TotalPopulation_Compact.xlsx"

var clioInfraPaysIndochine = map[string]string{
	"Vietnam":  "Vietnam",
	"Cambodia": "Cambodge",
	"Laos":     "Laos",
}

func IngestCLIOInfraPopulation(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCLIOInfraPopulation)
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

	f, err := arch.Fetch(ctx, srcID, runID, clioInfraPopulationURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	// « Data Long Format » : une ligne par (pays, année), pas un classeur
	// large avec une colonne par année — jamais besoin de repérer un en-tête
	// d'années comme pour internal/macro/pauvrete.go.
	lignes, err := x.rows("Data Long Format")
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_population_indochine_historique (
			pays text, annee smallint, population_milliers numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for i, l := range lignes {
		if i == 0 {
			continue // en-tête : ccode, country.name, year, value
		}
		nomSource := strings.TrimSpace(l["B"])
		pays, ok := clioInfraPaysIndochine[nomSource]
		if !ok {
			continue
		}
		anneeF, err := strconv.ParseFloat(l["C"], 64)
		if err != nil {
			continue
		}
		valeur, err := strconv.ParseFloat(l["D"], 64)
		if err != nil || valeur <= 0 {
			continue
		}
		rows = append(rows, []any{pays, int(anneeF), valeur, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne extraite pour le Viêt Nam, le Cambodge ou le Laos"))
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_population_indochine_historique"},
		[]string{"pays", "annee", "population_milliers", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("insertion population_indochine_historique : %w", err))
	}

	if _, err := tx.Exec(ctx, `
		MERGE INTO core.population_indochine_historique AS tgt
		USING tmp_population_indochine_historique AS src
		ON tgt.pays = src.pays AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.population_milliers, tgt.source_id)
		                  IS DISTINCT FROM (src.population_milliers, src.source_id) THEN
		    UPDATE SET population_milliers = src.population_milliers, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (pays, annee, population_milliers, source_id)
		    VALUES (src.pays, src.annee, src.population_milliers, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("fusion population_indochine_historique : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": len(rows)}, "")
	fmt.Printf("  population Indochine (CLIO-INFRA) : %d lignes (Viêt Nam, Cambodge, Laos, 1500-2000)\n", len(rows))
	return nil
}
