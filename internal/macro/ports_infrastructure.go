package macro

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceAutoroutesPortuaires = archive.Source{
	Slug: "osm-autoroutes-france", Label: "Autoroutes de France (OpenStreetMap)",
	// Comme les autres calques géographiques de repère de ce dépôt (contours
	// administratifs, bassins hydrographiques), une couche de fond n'est pas
	// un fait à trois niveaux de preuve : classée au même niveau que les
	// autres sources géographiques republiées par un portail public, la
	// note ci-dessous garde l'origine communautaire explicite.
	Publisher: "OpenStreetMap, republié sur data.gouv.fr", Tier: "PRIMARY_OFFICIAL",
	Licence: "ODbL", ReuseClass: "OPEN",
	Attribution: "Source : contributeurs OpenStreetMap, via data.gouv.fr",
	Cadence:     "quotidienne (extraction figée à l'ingestion)",
	Notes: "Fichier national (56 938 tronçons) filtré à l'ingestion aux tronçons situés à moins " +
		"de 80 km de l'un des quatre grands ports du dossier ports-donnees.md — pas une couche " +
		"autoroutière nationale. Le champ the_geom du fichier source est en Web Mercator " +
		"(EPSG:3857, vérifié directement : les coordonnées brutes, prises pour du Lambert-93 " +
		"lors d'un premier chargement, plaçaient les tronçons à plusieurs dizaines de km de leur " +
		"position réelle) et non en Lambert-93 comme le laisserait croire l'ordre de grandeur des " +
		"coordonnées.",
}

const urlAutoroutesFrance = "https://www.data.gouv.fr/api/1/datasets/r/4780d322-97d8-4eb4-8d4b-7bf68ec16d75"

// ancragesPorts : un point WGS84 (lon, lat) par port suivi (ou par ville de
// l'axe HAROPA), vérifié via geo.contour_cog (centroïde de la commune, IGN
// COG 2026) plutôt que saisi à l'estime. Sert à filtrer géographiquement
// les couches autoroute/rail de ce dossier, pas à autre chose.
var ancragesPorts = map[string][2]float64{
	"Dunkerque":    {2.3374, 51.0304},
	"LeHavre":      {0.1412, 49.4983},
	"Rouen":        {1.0939, 49.4412},
	"FosSurMer":    {4.9041, 43.4559},
	"SaintNazaire": {-2.2510, 47.2799},
}

const rayonPortM = 80_000.0 // 80 km, distance réelle (grand cercle)

// webMercatorXY : projection directe lon/lat (WGS84) vers Web Mercator
// (EPSG:3857), en forme close — évite un aller-retour PostGIS pour chaque
// point testé pendant la lecture du fichier CSV national (56 938 lignes).
func webMercatorXY(lon, lat float64) (float64, float64) {
	const rayonTerre = 6378137.0
	x := lon * math.Pi / 180 * rayonTerre
	y := math.Log(math.Tan(math.Pi/4+lat*math.Pi/360)) * rayonTerre
	return x, y
}

// presLarge : un premier filtre approximatif en Web Mercator (marge large,
// 160 km, pour absorber la déformation du mode conforme aux latitudes
// françaises) — juste pour éviter d'insérer les 56 938 tronçons du fichier
// national avant le filtre précis (celui-là en distance réelle, exécuté en
// SQL après chargement, voir la clause DELETE plus bas).
const rayonPreFiltreM = 160_000.0

func presLarge(x1, y1 float64) bool {
	for _, a := range ancragesPorts {
		ax, ay := webMercatorXY(a[0], a[1])
		if math.Hypot(x1-ax, y1-ay) <= rayonPreFiltreM {
			return true
		}
	}
	return false
}

var reWKTPremierPoint = regexp.MustCompile(`\(\s*(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)`)

// IngestAutoroutesPortuaires télécharge le fichier national des
// autoroutes (OSM) et ne conserve que les tronçons proches d'un des
// quatre grands ports — filtrage sur le premier point du tracé, les
// tronçons de ce fichier étant courts (56 938 tronçons pour tout le
// réseau national, soit quelques centaines de mètres chacun en moyenne) :
// une approximation négligeable au regard du rayon de 80 km retenu.
func IngestAutoroutesPortuaires(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAutoroutesPortuaires)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "autoroutes-portuaires-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlAutoroutesFrance, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return fail(err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	for _, col := range []string{"highway", "ref", "the_geom"} {
		if _, ok := idx[col]; !ok {
			return fail(fmt.Errorf("colonne %q absente du fichier source", col))
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.autoroute_portuaire`); err != nil {
		return fail(err)
	}

	total, retenus := 0, 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(err)
		}
		total++
		highway := rec[idx["highway"]]
		if highway != "motorway" && highway != "motorway_link" {
			continue
		}
		wkt := rec[idx["the_geom"]]
		m := reWKTPremierPoint.FindStringSubmatch(wkt)
		if m == nil {
			continue
		}
		x, errX := strconv.ParseFloat(m[1], 64)
		y, errY := strconv.ParseFloat(m[2], 64)
		if errX != nil || errY != nil || !presLarge(x, y) {
			continue
		}
		ref := rec[idx["ref"]]
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.autoroute_portuaire (ref, highway, geom, source_id)
			VALUES (NULLIF($1,''), $2, ST_Transform(ST_SetSRID(ST_GeomFromText($3), 3857), 2154), $4)`,
			ref, highway, wkt, srcID); err != nil {
			return fail(fmt.Errorf("tronçon %q : insertion : %w", ref, err))
		}
		retenus++
	}
	if retenus == 0 {
		return fail(fmt.Errorf("autoroutes portuaires : aucun tronçon retenu sur %d lus", total))
	}

	// Filtre précis en distance réelle (grand cercle, via geography) : le
	// filtre ci-dessus (presLarge) n'est qu'une marge large en Web Mercator
	// pour limiter le volume inséré, déformée par la latitude — celui-ci
	// applique le rayon de 80 km exact annoncé dans SourceAutoroutesPortuaires.
	var conditions []string
	var args []any
	i := 1
	for _, a := range ancragesPorts {
		conditions = append(conditions, fmt.Sprintf(
			"ST_DWithin(ST_Transform(t.geom,4326)::geography, ST_SetSRID(ST_MakePoint($%d,$%d),4326)::geography, $%d)", i, i+1, i+2))
		args = append(args, a[0], a[1], rayonPortM)
		i += 3
	}
	supprimes, err := tx.Exec(ctx, fmt.Sprintf(`
		DELETE FROM geo.autoroute_portuaire t WHERE NOT (%s)`,
		strings.Join(conditions, " OR ")), args...)
	if err != nil {
		return fail(err)
	}
	final := retenus - int(supprimes.RowsAffected())

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lus": total, "retenus": final}, "")
	fmt.Printf("  autoroutes portuaires : %d tronçons retenus sur %d lus\n", final, total)
	return nil
}

var SourceVoiesFerreesPortuaires = archive.Source{
	Slug: "sncf-lignes-voie-portuaire", Label: "Lignes du réseau ferré national, type « voie portuaire »",
	Publisher: "SNCF Réseau", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : SNCF Réseau, ressources.data.sncf.com",
	Cadence:     "irrégulière",
	Notes: "Un raccordement physique classé « voie portuaire » (type_ligne=Vport) par SNCF " +
		"Réseau, sur tout le territoire — jamais une mesure de trafic fret : aucune donnée " +
		"ouverte ne distingue fret et voyageurs sur le réseau ferré national. Chargé en entier " +
		"(petit volume), filtré à l'affichage aux abords des quatre ports du dossier.",
}

const urlLignesParType = "https://ressources.data.sncf.com/api/explore/v2.1/catalog/datasets/lignes-par-type/exports/geojson?lang=fr&timezone=Europe%2FParis"

type featureLigneType struct {
	Properties struct {
		TypeLigne string `json:"type_ligne"`
		CodeLigne string `json:"code_ligne"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// IngestVoiesFerreesPortuaires charge les tronçons classés « voie
// portuaire » du réseau ferré national. Voir docs/ports-donnees.md.
func IngestVoiesFerreesPortuaires(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceVoiesFerreesPortuaires)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "voies-ferrees-portuaires-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlLignesParType, ".geojson")
	if err != nil {
		return fail(err)
	}

	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var fc struct {
		Features []featureLigneType `json:"features"`
	}
	if err := json.Unmarshal(raw, &fc); err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.voie_ferree_portuaire`); err != nil {
		return fail(err)
	}
	n, sansGeom := 0, 0
	for _, feat := range fc.Features {
		if feat.Properties.TypeLigne != "Vport" {
			continue
		}
		// Quelques tronçons Vport du fichier source n'ont aucune géométrie
		// (ex. code_ligne 583506) — un vrai trou de la source, pas une
		// erreur de lecture : ignorés plutôt que de faire échouer tout le
		// chargement pour une poignée de lignes.
		if len(feat.Geometry) == 0 || string(feat.Geometry) == "null" {
			sansGeom++
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.voie_ferree_portuaire (code_ligne, geom, source_id)
			VALUES ($1, ST_SetSRID(ST_GeomFromGeoJSON($2), 4326), $3)`,
			feat.Properties.CodeLigne, string(feat.Geometry), srcID); err != nil {
			return fail(fmt.Errorf("ligne %s : insertion : %w", feat.Properties.CodeLigne, err))
		}
		n++
	}
	if sansGeom > 0 {
		fmt.Printf("  voies ferrées portuaires : %d tronçons Vport sans géométrie dans la source, ignorés\n", sansGeom)
	}
	if n == 0 {
		return fail(fmt.Errorf("voies ferrées portuaires : aucun tronçon Vport trouvé"))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"troncons": n}, "")
	fmt.Printf("  voies ferrées portuaires : %d tronçons (type Vport)\n", n)
	return nil
}
