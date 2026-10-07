package hydro

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersionWatercourses = "cours-eau-v1"

var SourceWatercourses = archive.Source{
	Slug: "sandre-cours-eau", Label: "Sandre/IGN — cours d'eau (BD Topage)",
	Publisher: "Service d'administration nationale des données et référentiels sur l'eau (Sandre)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Sandre, BD Topage (IGN/OFB)",
	Cadence:     "irrégulière (révision du référentiel hydrographique)",
	Notes: "Millésime 2025, France métropolitaine uniquement (suffixe FXX). Fichier source : " +
		"134 739 tronçons, 1,4 Go décompressé — filtré à l'ingestion à une liste de grands " +
		"cours d'eau nommés (voir retainedWatercourses), jamais chargé en entier.",
}

const watercourseURL = "https://services.sandre.eaufrance.fr/telechargement/geo/ETH/BDTopage/2025/" +
	"CoursEau/CoursEau_FXX-geojson.zip"

// retainedWatercourses : les cours d'eau assez grands et assez connus pour
// servir de repère visuel sur une petite carte — pas un critère de débit ou
// d'ordre de Strahler (absent du fichier source), une liste choisie. Le nom
// doit correspondre exactement à la valeur TopoOH publiée par Sandre
// (article inclus : "la Seine", "le Rhône", "l'Adour").
var retainedWatercourses = map[string]bool{
	"la Seine": true, "la Loire": true, "le Rhône": true, "la Garonne": true,
	"le Rhin": true, "la Moselle": true, "la Saône": true, "la Dordogne": true,
	"l'Adour": true, "la Charente": true, "la Marne": true, "l'Oise": true,
	"l'Yonne": true, "la Vienne": true, "la Sarthe": true, "la Mayenne": true,
	"l'Ain": true, "l'Isère": true, "la Durance": true,
}

type watercourseFeature struct {
	Properties struct {
		TopoOH string `json:"TopoOH"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// IngestWatercourses filtre le fichier Sandre (134 739 tronçons) à la liste
// retainedWatercourses en le décodant en flux : le fichier décompressé fait
// 1,4 Go, bien trop volumineux pour le désérialiser d'un bloc comme
// IngestBasins le fait pour le fichier des bassins (quelques centaines de
// Ko).
func IngestWatercourses(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceWatercourses)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionWatercourses)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, watercourseURL, ".zip")
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.cours_eau`); err != nil {
		return fail(err)
	}

	counters := map[string]int{}
	total := 0
	err = walkWatercourses(f.Path, func(feat watercourseFeature) error {
		if !retainedWatercourses[feat.Properties.TopoOH] {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.cours_eau (nom, geom, source_id)
			VALUES ($1, ST_Multi(ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($2), 2154), 4326)), $3)`,
			feat.Properties.TopoOH, string(feat.Geometry), srcID); err != nil {
			return fmt.Errorf("%s : insertion : %w", feat.Properties.TopoOH, err)
		}
		counters[feat.Properties.TopoOH]++
		total++
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if total == 0 {
		return fail(fmt.Errorf("cours d'eau : aucun tronçon retenu — vérifier retainedWatercourses contre TopoOH"))
	}
	missing := 0
	for nom := range retainedWatercourses {
		if counters[nom] == 0 {
			missing++
			fmt.Printf("  cours d'eau : %q absent du fichier source\n", nom)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"troncons": total, "cours_deau_manquants": missing}, "")
	fmt.Printf("  cours d'eau : %d tronçons retenus sur %d cours d'eau\n", total, len(counters))
	return nil
}

// walkWatercourses décode le GeoJSON en flux, sans jamais garder plus d'une
// entité en mémoire : ouvrir le tronçon zip via son propre io.Reader plutôt
// que d'appeler os.ReadFile sur l'archive (211 Mo) ou sur son contenu
// décompressé (1,4 Go), comme le fait readGeoJSONFromZip pour le fichier
// bassins, bien plus petit.
func walkWatercourses(zipPath string, fn func(watercourseFeature) error) error {
	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return err
	}
	var zf *zip.File
	for _, cand := range zr.File {
		if len(cand.Name) > 8 && cand.Name[len(cand.Name)-8:] == ".geojson" {
			zf = cand
			break
		}
	}
	if zf == nil {
		return fmt.Errorf("aucun .geojson trouvé dans %s", zipPath)
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	dec := json.NewDecoder(rc)
	if err := advanceTo(dec, "features"); err != nil {
		return err
	}
	if _, err := dec.Token(); err != nil { // '['
		return err
	}
	for dec.More() {
		var feat watercourseFeature
		if err := dec.Decode(&feat); err != nil {
			return err
		}
		if err := fn(feat); err != nil {
			return err
		}
	}
	return nil
}

// advanceTo consomme les tokens du flux jusqu'à trouver la clé donnée à la
// racine de l'objet JSON, en s'arrêtant juste après elle.
func advanceTo(dec *json.Decoder, key string) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return fmt.Errorf("clé %q non trouvée avant la fin du flux", key)
		}
		if err != nil {
			return err
		}
		if s, ok := tok.(string); ok && s == key {
			return nil
		}
	}
}
