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
	Notes: "Millésime 2025. France métropolitaine (suffixe FXX, Lambert-93/EPSG:2154) plus " +
		"deux bassins d'outre-mer disponibles à ce thème du catalogue : Martinique (MTQ, " +
		"RGAF09/UTM20N — EPSG:5490) et Mayotte (MYT, RGM04/UTM38S — EPSG:4471), chacun " +
		"transformé depuis son propre SRID natif, jamais supposé être en Lambert-93. " +
		"Guadeloupe, Guyane et Réunion n'ont pas d'extrait à ce thème (vérifié par requête " +
		"directe sur le catalogue, 404 pour les trois) — absents, pas oubliés.",
}

type territoireBassin struct {
	Code, SuffixeURL string
	SRIDSource       int
}

var territoiresBassins = []territoireBassin{
	{Code: "metropole", SuffixeURL: "FXX", SRIDSource: 2154},
	{Code: "outremer", SuffixeURL: "MTQ", SRIDSource: 5490},
	{Code: "outremer", SuffixeURL: "MYT", SRIDSource: 4471},
}

const bassinsURLBase = "https://services.sandre.eaufrance.fr/telechargement/geo/ETH/BDTopage/2025/" +
	"BassinHydrographique/BassinHydrographique_"

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

	total := map[string]int{}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_bassin`); err != nil {
		return fail(err)
	}
	for _, terr := range territoiresBassins {
		url := bassinsURLBase + terr.SuffixeURL + "-geojson.zip"
		f, err := arch.Fetch(ctx, srcID, runID, url, ".zip")
		if err != nil {
			return fail(fmt.Errorf("%s : %w", terr.SuffixeURL, err))
		}
		gj, err := lireGeoJSONDuZip(f.Path)
		if err != nil {
			return fail(fmt.Errorf("%s : %w", terr.SuffixeURL, err))
		}
		var fc featureCollection
		if err := json.Unmarshal(gj, &fc); err != nil {
			return fail(fmt.Errorf("%s : %w", terr.SuffixeURL, err))
		}
		if len(fc.Features) == 0 {
			return fail(fmt.Errorf("%s : aucune entité dans le GeoJSON", terr.SuffixeURL))
		}
		for _, feat := range fc.Features {
			if _, err := tx.Exec(ctx, `
				INSERT INTO geo.contour_bassin (code, nom, geom, srid_source, territoire, source_id)
				VALUES ($1, $2, ST_Multi(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($3), $4), 4326)), $4, $5, $6)`,
				feat.Properties.CdBH, feat.Properties.LbBH, string(feat.Geometry),
				terr.SRIDSource, terr.Code, srcID); err != nil {
				return fail(fmt.Errorf("%s, bassin %s (%s) : %w", terr.SuffixeURL, feat.Properties.CdBH, feat.Properties.LbBH, err))
			}
		}
		total[terr.Code] += len(fc.Features)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"metropole": total["metropole"], "outremer": total["outremer"]}, "")
	fmt.Printf("  bassins hydrographiques : %d métropole, %d outre-mer\n", total["metropole"], total["outremer"])
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
