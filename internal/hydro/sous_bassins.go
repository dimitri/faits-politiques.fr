package hydro

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersionSousBassins = "sous-bassins-v1"

var SourceSousBassins = archive.Source{
	Slug: "sandre-sous-bassins-versants", Label: "Sandre/IGN — sous-bassins versants topographiques (BD Topage)",
	Publisher: "Service d'administration nationale des données et référentiels sur l'eau (Sandre)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Sandre, BD Topage (IGN/OFB)",
	Cadence:     "irrégulière (révision du référentiel hydrographique)",
	Notes: "Millésime 2025, France métropolitaine uniquement (suffixe FXX). Résolution bien " +
		"plus fine que geo.contour_bassin (7 grands bassins) : 6 190 polygones, chacun rattaché à " +
		"son grand bassin par CdBH. Le nom (TopoOH) est souvent celui du tronçon de cours d'eau " +
		"associé, pas un nom de sous-bassin standardisé — laissé tel quel, jamais reconstruit.",
}

const urlSousBassinsFXX = "https://services.sandre.eaufrance.fr/telechargement/geo/ETH/BDTopage/2025/" +
	"BassinVersantTopographique/BassinVersantTopographique_FXX-geojson.zip"

type featureSousBassin struct {
	Properties struct {
		CdBH   string `json:"CdBH"`
		TopoOH string `json:"TopoOH"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// IngestSousBassins charge les sous-bassins versants topographiques
// (BD Topage). Voir docs/bassins-versants-donnees.md.
func IngestSousBassins(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSousBassins)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionSousBassins)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlSousBassinsFXX, ".zip")
	if err != nil {
		return fail(err)
	}
	gj, err := lireGeoJSONDuZip(f.Path)
	if err != nil {
		return fail(err)
	}
	var fc struct {
		Features []featureSousBassin `json:"features"`
	}
	if err := json.Unmarshal(gj, &fc); err != nil {
		return fail(err)
	}
	if len(fc.Features) == 0 {
		return fail(fmt.Errorf("sous-bassins versants : aucune entité dans le GeoJSON"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_sous_bassin`); err != nil {
		return fail(err)
	}

	// ST_GeomFromGeoJSON n'a pas d'équivalent en forme "COPY" : un INSERT
	// classique par ligne plutôt qu'un CopyFrom — 6 190 lignes reste rapide,
	// pas besoin d'optimiser davantage.
	for i, feat := range fc.Features {
		var nom *string
		if feat.Properties.TopoOH != "" {
			v := feat.Properties.TopoOH
			nom = &v
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.contour_sous_bassin (code_bassin, nom, geom, source_id)
			VALUES ($1, $2, ST_Multi(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($3), 2154), 4326)), $4)`,
			feat.Properties.CdBH, nom, string(feat.Geometry), srcID); err != nil {
			return fail(fmt.Errorf("entité %d (CdBH=%s) : insertion : %w", i, feat.Properties.CdBH, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"sous_bassins": len(fc.Features)}, "")
	fmt.Printf("  sous-bassins versants topographiques : %d polygones\n", len(fc.Features))
	return nil
}
