package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type departementMusees struct {
	Nom  string
	Nb   int
	X, Y float64
}

type CarteMusees struct {
	SVG            template.HTML
	NbMusees       int
	NbDepartements int
}

// chargerCarteMusees : un cercle par département, proportionnel au nombre
// de musées labellisés « Musée de France » qui s'y trouvent — même patron
// que chargerCarteIFI (internal/sitegen/ifi.go), rayon en racine carrée pour une
// surface perceptivement juste. Le nom de département du fichier source
// est rapproché de geo.contour par nom normalisé (accents, espaces,
// apostrophes) plutôt que par code, ce fichier ne publiant pas de code
// INSEE de département.
func chargerCarteMusees(ctx context.Context, pool *pgxpool.Pool) (*CarteMusees, error) {
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
		var nom string
		var x, y float64
		if err := depRows.Scan(&nom, &x, &y); err != nil {
			depRows.Close()
			return nil, err
		}
		centroides[normaliserNomPays(nom)] = [2]float64{x, y}
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
	var deps []departementMusees
	var matches int
	for rows.Next() {
		var nom string
		var nb int
		if err := rows.Scan(&nom, &nb); err != nil {
			return nil, err
		}
		xy, ok := centroides[normaliserNomPays(strings.ReplaceAll(nom, " ", "-"))]
		if !ok {
			xy, ok = centroides[normaliserNomPays(nom)]
		}
		if !ok {
			continue
		}
		matches++
		deps = append(deps, departementMusees{Nom: nom, Nb: nb, X: xy[0], Y: xy[1]})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(deps) == 0 {
		return nil, nil
	}

	var vb string
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu=2154) x`).Scan(&vb); err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo musees" role="img" `+
		`aria-label="Musées labellisés Musée de France, par département">`, vb)

	fondRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, tolPleine)
	if err != nil {
		return nil, err
	}
	for fondRows.Next() {
		var d string
		if err := fondRows.Scan(&d); err != nil {
			fondRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	if err := fondRows.Err(); err != nil {
		fondRows.Close()
		return nil, err
	}
	fondRows.Close()
	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(fleuves)

	rayon := func(nb int) float64 { return 2000 + 1900*math.Sqrt(float64(nb)) }
	for _, d := range deps {
		titre := fmt.Sprintf("%s — %d musées labellisés « Musée de France »", d.Nom, d.Nb)
		fmt.Fprintf(&b, `<circle class="musee-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			d.X, -d.Y, rayon(d.Nb), template.HTMLEscapeString(titre))
	}
	b.WriteString(`</svg>`)

	return &CarteMusees{SVG: template.HTML(b.String()), NbMusees: total, NbDepartements: matches}, nil
}
