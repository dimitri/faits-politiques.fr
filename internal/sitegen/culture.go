package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type departmentMuseums struct {
	Name  string
	Count int
	X, Y  float64
}

type MapMuseums struct {
	SVG              template.HTML
	CountMuseums     int
	CountDepartments int
}

// loadMapMuseums : un cercle par département, proportionnel au nombre
// de musées labellisés « Musée de France » qui s'y trouvent — même patron
// que chargerCarteIFI (internal/sitegen/ifi.go), rayon en racine carrée pour une
// surface perceptivement juste. Le nom de département du fichier source
// est rapproché de geo.contour par nom normalisé (accents, espaces,
// apostrophes) plutôt que par code, ce fichier ne publiant pas de code
// INSEE de département.
func loadMapMuseums(ctx context.Context, pool *pgxpool.Pool) (*MapMuseums, error) {
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.musee_france`).Scan(&total); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}

	depRows, err := pool.Query(ctx, `
		SELECT nom, st_x(st_transform(st_centroid(geom),2154)), st_y(st_transform(st_centroid(geom),2154))
		FROM geo.contour WHERE niveau='DEPARTEMENT'`)
	if err != nil {
		return nil, err
	}
	centroides := map[string][2]float64{}
	for depRows.Next() {
		var name string
		var x, y float64
		if err := depRows.Scan(&name, &x, &y); err != nil {
			depRows.Close()
			return nil, err
		}
		centroides[normalizeNameCountry(name)] = [2]float64{x, y}
	}
	if err := depRows.Err(); err != nil {
		depRows.Close()
		return nil, err
	}
	depRows.Close()

	rows, err := pool.Query(ctx, `
		SELECT departement, count(*) FROM core.musee_france GROUP BY departement ORDER BY departement`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deps []departmentMuseums
	var matches int
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		xy, ok := centroides[normalizeNameCountry(strings.ReplaceAll(name, " ", "-"))]
		if !ok {
			xy, ok = centroides[normalizeNameCountry(name)]
		}
		if !ok {
			continue
		}
		matches++
		deps = append(deps, departmentMuseums{Name: name, Count: count, X: xy[0], Y: xy[1]})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(deps) == 0 {
		return nil, nil
	}

	// st_extent est une agrégation : la ligne existe même sans contour
	// départemental, avec une valeur NULL — et geo.contour n'a AUCUNE source
	// dans le catalogue (contours OpenStreetMap de la migration 0058, chargés
	// hors pipeline), alors que core.musee_france, lui, vient de la source
	// « museofile » : une base où museofile a tourné mais où geo.contour est
	// resté vide est donc le cas normal, pas une anomalie. Même défense que
	// jeuContours (internal/sitegen/carte.go) sur cette même table : une boîte
	// vide plutôt qu'un échec de toute la construction.
	var vbN sql.NullString
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu=2154) x`).Scan(&vbN); err != nil {
		return nil, err
	}
	vb := vbN.String

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo musees" role="img" `+
		`aria-label="Musées labellisés Musée de France, par département">`, vb)

	backgroundRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, toleranceFull)
	if err != nil {
		return nil, err
	}
	for backgroundRows.Next() {
		var d string
		if err := backgroundRows.Scan(&d); err != nil {
			backgroundRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	if err := backgroundRows.Err(); err != nil {
		backgroundRows.Close()
		return nil, err
	}
	backgroundRows.Close()
	rivers, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(rivers)

	radius := func(count int) float64 { return 2000 + 1900*math.Sqrt(float64(count)) }
	for _, d := range deps {
		title := fmt.Sprintf("%s — %d musées labellisés « Musée de France »", d.Name, d.Count)
		fmt.Fprintf(&b, `<circle class="musee-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			d.X, -d.Y, radius(d.Count), template.HTMLEscapeString(title))
	}
	b.WriteString(`</svg>`)

	return &MapMuseums{SVG: template.HTML(b.String()), CountMuseums: total, CountDepartments: matches}, nil
}
