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

var SourceContourPays = archive.Source{
	Slug: "natural-earth-admin0", Label: "Natural Earth — pays du monde (admin-0), 1:50m",
	Publisher: "Natural Earth", Tier: "PRIMARY_OFFICIAL",
	Licence: "Domaine public (Natural Earth)", ReuseClass: "OPEN",
	Attribution: "Source : Natural Earth, naturalearthdata.com",
	Cadence:     "ponctuelle",
	Notes: "Échelle 1:50 000 000, un repère mondial, pas un cadastre. Les départements " +
		"d'outre-mer français (Guadeloupe, Martinique, Réunion, Mayotte, Guyane) " +
		"n'existent pas comme entités séparées à cette échelle.",
}

const urlContourPays = "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_50m_admin_0_countries.geojson"

type paysFeature struct {
	Properties struct {
		NameFR     string `json:"NAME_FR"`
		Name       string `json:"NAME"`
		Sovereignt string `json:"SOVEREIGNT"`
		ISOA3      string `json:"ISO_A3"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// IngestContourPays charge le fond de carte mondial par pays (Natural
// Earth, admin-0, 1:50m) — une infrastructure géographique partagée, pas
// propre à un seul dossier.
func IngestContourPays(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceContourPays)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "contour-pays-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlContourPays, ".geojson")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var doc struct {
		Features []paysFeature `json:"features"`
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
		if p.NameFR == "" {
			return fail(fmt.Errorf("entité sans NAME_FR (ISO %s)", p.ISOA3))
		}
		if len(ft.Geometry) == 0 || string(ft.Geometry) == "null" {
			return fail(fmt.Errorf("%s : géométrie absente", p.NameFR))
		}
		rows = append(rows, []any{p.NameFR, p.Name, p.Sovereignt, p.ISOA3, string(ft.Geometry), srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_pays`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE contour_pays_in (nom_fr text, nom_en text, souverain text, iso_a3 text, gj text, source_id bigint)
		ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"contour_pays_in"},
		[]string{"nom_fr", "nom_en", "souverain", "iso_a3", "gj", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.contour_pays (nom_fr, nom_en, souverain, iso_a3, geom, source_id)
		SELECT nom_fr, nom_en, souverain, nullif(iso_a3, '-99'),
		       ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(gj), 4326)), source_id
		FROM contour_pays_in`); err != nil {
		return fail(fmt.Errorf("insertion : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"pays": len(rows)}, "")
	fmt.Printf("  Fond de carte mondial (Natural Earth) : %d pays/territoires\n", len(rows))
	return nil
}
