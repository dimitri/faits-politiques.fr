package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceLigneDemarcation = archive.Source{
	Slug: "ligne-demarcation-ain", Label: "Ligne de démarcation, 1940-1942",
	Publisher: "Département de l'Ain", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte 2.0", ReuseClass: "OPEN",
	Attribution: "Source : Département de l'Ain, data.gouv.fr",
	Cadence:     "ponctuelle",
	Notes: "Le seul tracé géographique vérifié de l'Occupation identifié à ce jour : ni " +
		"l'annexion de fait de l'Alsace-Moselle ni la zone d'occupation italienne (à partir " +
		"de novembre 1942) n'ont de géométrie ouverte trouvée — citées en prose seulement, " +
		"voir docs/seconde-guerre-mondiale-donnees.md.",
}

const urlLigneDemarcation = "https://departement-ain.opendata.arcgis.com/api/download/v1/items/73760b9eff0849f6afedc3d8b68b98ce/geojson?layers=5"

// IngestLigneDemarcation charge le tracé de la ligne de démarcation
// 1940-1942 (Département de l'Ain). Voir
// docs/seconde-guerre-mondiale-donnees.md.
func IngestLigneDemarcation(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceLigneDemarcation)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "ligne-demarcation-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlLigneDemarcation, ".geojson")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var doc struct {
		Features []struct {
			Properties struct {
				Longueur float64 `json:"SHAPE__Length"`
			} `json:"properties"`
			Geometry json.RawMessage `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fail(fmt.Errorf("GeoJSON illisible : %w", err))
	}
	if len(doc.Features) != 1 {
		return fail(fmt.Errorf("attendu un unique tracé, trouvé %d", len(doc.Features)))
	}
	ft := doc.Features[0]
	if len(ft.Geometry) == 0 || string(ft.Geometry) == "null" {
		return fail(fmt.Errorf("géométrie absente"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.ligne_demarcation`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.ligne_demarcation (longueur_m, geom, source_id)
		VALUES ($1, ST_SetSRID(ST_GeomFromGeoJSON($2), 4326), $3)`,
		ft.Properties.Longueur, string(ft.Geometry), srcID); err != nil {
		return fail(fmt.Errorf("insertion : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"longueur_m": ft.Properties.Longueur}, "")
	fmt.Printf("  Ligne de démarcation : tracé chargé (%.0f km)\n", ft.Properties.Longueur/1000)
	return nil
}
