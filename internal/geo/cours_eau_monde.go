package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceCoursEauMonde = archive.Source{
	Slug: "natural-earth-rivers", Label: "Natural Earth — cours d'eau et lacs du monde, 1:50m",
	Publisher: "Natural Earth", Tier: "PRIMARY_OFFICIAL",
	Licence: "Domaine public (Natural Earth)", ReuseClass: "OPEN",
	Attribution: "Source : Natural Earth, naturalearthdata.com",
	Cadence:     "ponctuelle",
	Notes: "Échelle 1:50 000 000, un repère mondial de reconnaissance, pas une couche " +
		"hydrographique fine — voir geo.cours_eau (IGN, France seule) pour un tracé détaillé.",
}

const urlCoursEauMonde = "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_50m_rivers_lake_centerlines.geojson"

type riverFeature struct {
	Properties struct {
		Name   string `json:"name"`
		NameEn string `json:"name_en"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// IngestCoursEauMonde charge le fond mondial des grands cours d'eau (Natural
// Earth, 1:50m) — une infrastructure géographique partagée, même source et
// même échelle que geo.contour_pays (internal/geo/pays.go), pour que les deux
// calques se superposent sans décalage sur une carte d'Europe ou d'Asie.
func IngestCoursEauMonde(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCoursEauMonde)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "cours-eau-monde-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlCoursEauMonde, ".geojson")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var doc struct {
		Features []riverFeature `json:"features"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fail(fmt.Errorf("GeoJSON illisible : %w", err))
	}
	if len(doc.Features) == 0 {
		return fail(fmt.Errorf("aucune entité lue"))
	}

	var rows [][]any
	for _, ft := range doc.Features {
		p := ft.Properties
		if len(ft.Geometry) == 0 || string(ft.Geometry) == "null" {
			continue // quelques lacs sans tracé (centerline non calculée) : ignorés, pas une erreur
		}
		var name, nameEn any
		if p.Name != "" {
			name = p.Name
		}
		if p.NameEn != "" {
			nameEn = p.NameEn
		}
		rows = append(rows, []any{name, nameEn, string(ft.Geometry), srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune géométrie exploitable"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.cours_eau_monde`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE cours_eau_monde_in (nom text, nom_en text, gj text, source_id bigint)
		ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"cours_eau_monde_in"},
		[]string{"nom", "nom_en", "gj", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.cours_eau_monde (nom, nom_en, geom, source_id)
		SELECT nom, nom_en, ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(gj), 4326)), source_id
		FROM cours_eau_monde_in`); err != nil {
		return fail(fmt.Errorf("insertion : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"cours_eau": len(rows)}, "")
	fmt.Printf("  Fond mondial des cours d'eau (Natural Earth) : %d tracés\n", len(rows))
	return nil
}
