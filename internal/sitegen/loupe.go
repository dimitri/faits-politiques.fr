package sitegen

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La loupe de la carte de situation d'une commune.
//
// Une commune moyenne fait 14 km² : à l'échelle de la France elle tient en
// moins de deux pixels, et un anneau de la couleur de la page ne la désigne
// pas. Les cartographes y répondent par un « carton-zoom » : un cercle à
// grande échelle, relié à la commune, qui montre sa forme et ses voisines.
//
// Le fond de la loupe n'est pas écrit dans chaque page — 35 000 pages porteuses
// de leurs voisines pèseraient environ 1,7 Go — mais une fois par département
// (media/situation-communes-<dép>.svg), comme le fond de la France. La page ne
// porte que le contour fin de SA commune, posé sur ce fond.

const (
	radiusLens     = 165000.0 // rayon du cercle, dans le repère de la carte de France (mètres)
	minWindowLens  = 22000.0  // largeur du terrain montré, au moins
	factorWindow   = 2.4      // … et jamais moins de 2,4 fois la plus grande dimension de la commune
	srideMetropole = 2154
)

// writeCommuneFiles écrit, pour chaque département de métropole, le fichier des
// contours de ses communes et de celles qui le bordent à moins de 15 km : la
// loupe d'une commune frontalière ne doit pas montrer du vide au-delà de la
// limite.
func (f *backgroundSituation) writeCommuneFiles(ctx context.Context, pool *pgxpool.Pool, out string, mill int) error {
	f.communeFiles = map[string][4]float64{}
	for _, dept := range f.deps.Codes {
		if _, overseas := f.domDept[dept]; overseas {
			continue
		}
		rows, err := pool.Query(ctx, `
			WITH box AS (SELECT st_expand(st_extent(geom), 0.15) AS b FROM geo.contour_cog
			             WHERE niveau='COMMUNE' AND cog_millesime=$1 AND code_departement=$2)
			SELECT st_assvg(st_transform(st_simplifypreservetopology(c.geom,$3),c.srid_rendu),1,0),
			       st_xmin(t.g), st_xmax(t.g), -st_ymax(t.g), -st_ymin(t.g)
			FROM geo.contour_cog c CROSS JOIN box
			     CROSS JOIN LATERAL (SELECT st_transform(c.geom,c.srid_rendu) AS g) t
			WHERE c.niveau='COMMUNE' AND c.cog_millesime=$1 AND c.srid_rendu=$4 AND c.geom && box.b`,
			mill, dept, toleranceLoupe, srideMetropole)
		if err != nil {
			return err
		}
		var paths []string
		x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for rows.Next() {
			var d string
			var ax, bx, ay, by float64
			if err := rows.Scan(&d, &ax, &bx, &ay, &by); err != nil {
				rows.Close()
				return err
			}
			paths = append(paths, d)
			x0, x1 = math.Min(x0, ax), math.Max(x1, bx)
			y0, y1 = math.Min(y0, ay), math.Max(y1, by)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(paths) == 0 {
			continue
		}
		// Couleurs fixes et translucides, comme le fond de la France : ce fichier est
		// une image, il ne lit pas les variables CSS du thème.
		var b strings.Builder
		fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.0f %.0f %.0f %.0f">`, x0, y0, x1-x0, y1-y0)
		b.WriteString(`<g fill="#8C877D" fill-opacity=".2" stroke="#8C877D" stroke-opacity=".7" stroke-width="140" stroke-linejoin="round">`)
		for _, d := range paths {
			fmt.Fprintf(&b, `<path d="%s"/>`, d)
		}
		b.WriteString(`</g></svg>`)
		if err := os.WriteFile(filepath.Join(out, "media", "situation-communes-"+dept+".svg"), []byte(b.String()), 0o644); err != nil {
			return err
		}
		f.communeFiles[dept] = [4]float64{x0, y0, x1 - x0, y1 - y0}
	}
	return nil
}

// hasLens : la loupe n'existe que pour les communes de métropole, dans le repère
// de la carte de France ; celles d'outre-mer ont leur propre carton.
func (f *backgroundSituation) hasLens(c *geomSituation) bool {
	_, ok := f.communeFiles[c.dept]
	return ok && c.fine != "" && c.srid == srideMetropole
}

// lens dessine la loupe dans le calque de la commune : le point, le cône qui le
// relie au cercle, et le cercle, posé dans un coin vide de la carte (la Manche,
// le golfe de Gascogne, la Belgique) le plus proche de la commune.
func (f *backgroundSituation) lens(k *layer, c *geomSituation) {
	vx, vy, vw, vh := f.box[0], f.box[1], f.box[2], f.box[3]
	r := radiusLens
	coins := [][2]float64{
		{vx + vw - r - 10000, vy + r + 10000}, // nord-est
		{vx + r + 10000, vy + r + 10000},      // nord-ouest
		{vx + r + 10000, vy + vh - r - 20000}, // sud-ouest
	}
	best, bestDist := coins[0], math.Inf(1)
	for _, p := range coins {
		d := math.Hypot(p[0]-c.x, p[1]-c.y)
		if d > 1.15*r && d < bestDist {
			best, bestDist = p, d
		}
	}
	lx, ly := best[0], best[1]
	dist := math.Hypot(lx-c.x, ly-c.y)
	if dist <= r*1.05 { // la commune est sous le cercle : repli sur le point seul
		k.group("commune", []string{c.d})
		k.marker(c.x, c.y, 14000)
		return
	}
	// Les deux tangentes du point au cercle.
	th, al := math.Atan2(ly-c.y, lx-c.x), math.Asin(r/dist)
	t := math.Sqrt(dist*dist - r*r)
	p1 := [2]float64{c.x + t*math.Cos(th-al), c.y + t*math.Sin(th-al)}
	p2 := [2]float64{c.x + t*math.Cos(th+al), c.y + t*math.Sin(th+al)}

	win := math.Max(minWindowLens, factorWindow*math.Max(c.w, c.h))
	scale := 2 * r / win
	file := f.communeFiles[c.dept]

	fmt.Fprintf(&k.b, `<defs><clipPath id="lentille"><circle cx="%.0f" cy="%.0f" r="%.0f"/></clipPath></defs>`, lx, ly, r)
	fmt.Fprintf(&k.b, `<path class="cone" d="M%.0f %.0f L%.0f %.0f L%.0f %.0f Z"/>`, c.x, c.y, p1[0], p1[1], p2[0], p2[1])
	fmt.Fprintf(&k.b, `<circle class="point" cx="%.0f" cy="%.0f" r="9000"/>`, c.x, c.y)
	fmt.Fprintf(&k.b, `<circle class="lentille-fond" cx="%.0f" cy="%.0f" r="%.0f"/>`, lx, ly, r)
	fmt.Fprintf(&k.b, `<g clip-path="url(#lentille)"><g transform="translate(%.0f %.0f) scale(%.5f) translate(%.0f %.0f)">`+
		`<image href="%s/media/situation-communes-%s.svg" x="%.0f" y="%.0f" width="%.0f" height="%.0f"/>`+
		`<path class="commune-fine" d="%s"/></g></g>`,
		lx, ly, scale, -c.xc, -c.yc, f.root, c.dept, file[0], file[1], file[2], file[3], c.fine)
	fmt.Fprintf(&k.b, `<circle class="lentille-bord" cx="%.0f" cy="%.0f" r="%.0f"/>`, lx, ly, r)

	// L'échelle : la plus grande longueur ronde qui tient dans le tiers de la fenêtre.
	for _, m := range []float64{10000, 5000, 2000, 1000, 500} {
		if m <= win/3 {
			w := m * scale
			x, y := lx-w/2, ly+r*0.72
			fmt.Fprintf(&k.b, `<g class="echelle"><line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f"/><text x="%.0f" y="%.0f" text-anchor="middle">%s</text></g>`,
				x, y, x+w, y, lx, y+56000, kmLabel(m))
			break
		}
	}
}

func kmLabel(m float64) string {
	if m >= 1000 {
		return fmt.Sprintf("%.0f km", m/1000)
	}
	return fmt.Sprintf("%.0f m", m)
}
