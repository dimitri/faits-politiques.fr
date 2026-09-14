// Package hydro charge la géographie de la gestion de l'eau — la couche
// cartographique du dossier « pour aller plus loin »
// (docs/bassins-versants-donnees.md), distincte des découpages administratifs
// classiques déjà chargés par internal/geo et internal/carto.
package hydro

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "hydro-v1"

var SourceBassins = archive.Source{
	Slug: "sandre-bassins-hydrographiques", Label: "Sandre/IGN — bassins hydrographiques (BD Topage)",
	Publisher: "Service d'administration nationale des données et référentiels sur l'eau (Sandre)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Sandre, BD Topage (IGN/OFB)",
	Cadence:     "irrégulière (révision du référentiel hydrographique)",
	Notes: "Millésime 2025, France métropolitaine uniquement (suffixe FXX du fichier source) — " +
		"pas de bassin d'outre-mer dans ce chargement. Coordonnées natives en Lambert-93 " +
		"(EPSG:2154), transformées en WGS84 (4326) pour rejoindre la convention de geo.contour.",
}

const bassinsURL = "https://services.sandre.eaufrance.fr/telechargement/geo/ETH/BDTopage/2025/" +
	"BassinHydrographique/BassinHydrographique_FXX-geojson.zip"

type featureCollection struct {
	Features []struct {
		Properties struct {
			CdBH string `json:"CdBH"`
			LbBH string `json:"LbBH"`
		} `json:"properties"`
		Geometry json.RawMessage `json:"geometry"`
	} `json:"features"`
}

func IngestBassins(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceBassins)
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

	f, err := arch.Fetch(ctx, srcID, runID, bassinsURL, ".zip")
	if err != nil {
		return fail(err)
	}

	gj, err := lireGeoJSONDuZip(f.Path)
	if err != nil {
		return fail(err)
	}

	var fc featureCollection
	if err := json.Unmarshal(gj, &fc); err != nil {
		return fail(err)
	}
	if len(fc.Features) == 0 {
		return fail(fmt.Errorf("bassins hydrographiques : aucune entité dans le GeoJSON"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_bassin`); err != nil {
		return fail(err)
	}
	for _, feat := range fc.Features {
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.contour_bassin (code, nom, geom, source_id)
			VALUES ($1, $2, ST_Multi(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($3), 2154), 4326)), $4)`,
			feat.Properties.CdBH, feat.Properties.LbBH, string(feat.Geometry), srcID); err != nil {
			return fail(fmt.Errorf("bassin %s (%s) : %w", feat.Properties.CdBH, feat.Properties.LbBH, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"bassins": len(fc.Features)}, "")
	fmt.Printf("  bassins hydrographiques : %d bassins\n", len(fc.Features))
	return nil
}

// lireGeoJSONDuZip : le fichier Sandre est distribué en .zip contenant un
// unique .geojson — extrait en mémoire, jamais désarchivé sur disque, pour ne
// pas laisser un second exemplaire non scellé à côté de l'archive.
func lireGeoJSONDuZip(cheminZip string) ([]byte, error) {
	b, err := os.ReadFile(cheminZip)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	for _, zf := range zr.File {
		if len(zf.Name) > 8 && zf.Name[len(zf.Name)-8:] == ".geojson" {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("aucun .geojson trouvé dans %s", cheminZip)
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return IngestBassins(ctx, pool, arch)
}
