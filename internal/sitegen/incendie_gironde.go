package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// villesGironde2026 : les communes dont le nom aide à situer le périmètre
// brûlé — pas un critère de population, un choix de repères reconnaissables
// (le bourg le plus proche du feu, la ville la plus connue du pourtour du
// Bassin d'Arcachon) qui tiennent dans la boîte de la carte une fois le
// rembourrage appliqué (voir padding2026 plus bas). D'autres communes du
// delta de l'Eyre (Biscarrosse, Salles, Belin-Béliet) sont plus au sud,
// hors de cette boîte — les y faire entrer agrandirait la carte au point de
// perdre le périmètre brûlé dans le détail.
var villesGironde2026 = []string{"Saumos", "Le Porge", "Lacanau", "Andernos-les-Bains", "Arcachon"}

// padding2026 : marge autour du périmètre brûlé, en mètres (Lambert-93).
// 8 000 m plutôt que les 4 000 m d'une première version : juste assez pour
// faire entrer Arcachon et le Bassin d'Arcachon (le repère géographique le
// plus parlant de la zone), sans diluer le périmètre brûlé dans une boîte
// démesurée.
const padding2026 = 8000.0

// loadFireGironde dessine le périmètre brûlé de l'incendie de Gironde
// (juillet-août 2026, Copernicus EMS activation EMSR899) sur un fond
// local — le département, pas la grille entière des cartes choroplèthes
// (carte.go) : une seule zone à montrer, pas 95 à comparer, donc sa propre
// boîte, ajustée au périmètre plutôt qu'à la France entière.
//
// Le filet du département vient de geo.contour (OSM, la même source que
// toutes les autres cartes du site) ; le périmètre brûlé, de
// geo.zone_brulee_gironde_2026 ; l'eau et les cours d'eau locaux, de
// geo.eau_gironde_2026 et geo.riviere_gironde_2026 (voir leur migration
// pour la provenance) — sans eux, la forme brûlée n'a ni orientation ni
// échelle reconnaissables, un défaut signalé après la première version de
// cette carte.
func loadFireGironde(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM geo.zone_brulee_gironde_2026`).Scan(&n); err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}

	var x0, y0, x1, y1 float64
	if err := pool.QueryRow(ctx, `
		SELECT st_xmin(e), st_ymin(e), st_xmax(e), st_ymax(e)
		  FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.zone_brulee_gironde_2026) b`).
		Scan(&x0, &y0, &x1, &y1); err != nil {
		return "", err
	}
	x0 -= padding2026
	y0 -= padding2026
	x1 += padding2026
	y1 += padding2026
	width := x1 - x0
	scale := width / 700 // voir labelsRegions (carte.go) : un <g transform="scale(...)"> autour d'un texte à taille normale, pas un font-size direct à l'échelle Lambert-93.

	var eauSVG string
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(st_assvg(st_simplifypreservetopology(
		         st_intersection(st_transform(geom,2154), st_makeenvelope($1,$2,$3,$4,2154)), 15),1,0), '')
		  FROM geo.eau_gironde_2026`, x0, y0, x1, y1).Scan(&eauSVG); err != nil {
		return "", err
	}

	type dept struct {
		code, nom string
		fondSVG   string
		lx, ly    float64
	}
	// Les Landes (département 40) ne touchent pas la boîte de cette carte
	// (vérifié par st_intersects : le feu et son rembourrage restent au nord
	// de la frontière) — un seul département à montrer, pas deux dont un
	// resterait invisible.
	depts := []dept{{code: "33", nom: "Gironde"}}
	for i := range depts {
		if err := pool.QueryRow(ctx, `
			SELECT coalesce(st_assvg(st_intersection(
			         st_transform(st_simplifypreservetopology(geom,0.001),2154),
			         st_makeenvelope($2,$3,$4,$5,2154)),1,0), '')
			  FROM geo.contour WHERE niveau='DEPARTEMENT' AND code_insee=$1`,
			depts[i].code, x0, y0, x1, y1).Scan(&depts[i].fondSVG); err != nil {
			return "", err
		}
		// Le point d'étiquette évite la zone brûlée et l'eau (st_difference) :
		// un simple point-on-surface du département, sans cette exclusion,
		// est tombé en plein sur le périmètre brûlé lors du premier essai —
		// illisible, du texte sombre sur un aplat sombre. La boîte englobante
		// de la zone brûlée (st_envelope, sans union) sert d'exclusion plutôt
		// que sa géométrie exacte : un st_union/st_difference sur ses 2 061
		// polygones sources (la plupart des détections isolées de quelques
		// mètres carrés) est restée bloquée plus de 30 s lors de l'essai —
		// la boîte suffit largement à écarter le texte du brûlé.
		var lx, ly sql.NullFloat64
		if err := pool.QueryRow(ctx, `
			WITH brule AS (SELECT st_expand(st_envelope(st_transform(geom,2154)), 3000) g FROM geo.zone_brulee_gironde_2026),
			     eau   AS (SELECT st_simplifypreservetopology(st_intersection(
			                        st_transform(geom,2154), st_makeenvelope($2,$3,$4,$5,2154)), 15) g
			                 FROM geo.eau_gironde_2026),
			     dept  AS (SELECT st_intersection(st_transform(st_simplifypreservetopology(geom,0.001),2154),
			                        st_makeenvelope($2,$3,$4,$5,2154)) g
			                 FROM geo.contour WHERE niveau='DEPARTEMENT' AND code_insee=$1)
			SELECT st_x(p), st_y(p) FROM (
			  SELECT st_pointonsurface(st_difference(dept.g, st_union(brule.g, eau.g))) p
			    FROM dept, brule, eau) e`,
			depts[i].code, x0, y0, x1, y1).Scan(&lx, &ly); err != nil {
			return "", err
		}
		depts[i].lx, depts[i].ly = lx.Float64, ly.Float64
	}

	riviereRows, err := pool.Query(ctx, `
		SELECT nom, coalesce(st_assvg(st_transform(st_simplifypreservetopology(geom,0.0003),2154),1,0), '')
		  FROM geo.riviere_gironde_2026`)
	if err != nil {
		return "", err
	}
	defer riviereRows.Close()
	var rivieresSVG strings.Builder
	for riviereRows.Next() {
		var nom, d string
		if err := riviereRows.Scan(&nom, &d); err != nil {
			return "", err
		}
		if d == "" {
			continue
		}
		fmt.Fprintf(&rivieresSVG, `<path class="fleuve" d="%s"><title>%s</title></path>`,
			d, template.HTMLEscapeString(nom))
	}
	if err := riviereRows.Err(); err != nil {
		return "", err
	}

	var surfaceHa float64
	var burntSVG string
	if err := pool.QueryRow(ctx, `
		SELECT surface_ha, st_assvg(st_transform(geom,2154),1,0)
		  FROM geo.zone_brulee_gironde_2026`).Scan(&surfaceHa, &burntSVG); err != nil {
		return "", err
	}

	villeRows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (nom) nom,
		       st_x(st_transform(st_centroid(geom),2154)), st_y(st_transform(st_centroid(geom),2154))
		  FROM geo.contour_cog
		 WHERE niveau='COMMUNE' AND code_departement='33' AND nom = ANY($1)
		 ORDER BY nom, cog_millesime DESC`, villesGironde2026)
	if err != nil {
		return "", err
	}
	defer villeRows.Close()
	type ville struct {
		nom  string
		x, y float64
	}
	var villes []ville
	for villeRows.Next() {
		var v ville
		if err := villeRows.Scan(&v.nom, &v.x, &v.y); err != nil {
			return "", err
		}
		villes = append(villes, v)
	}
	if err := villeRows.Err(); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<figure class="schema"><svg viewBox="%.0f %.0f %.0f %.0f" class="geo incendie" role="img" `+
		`aria-label="Périmètre brûlé par l'incendie de Gironde, juillet-août 2026">`,
		x0, -y1, width, y1-y0)
	if eauSVG != "" {
		fmt.Fprintf(&b, `<path class="eau" d="%s"/>`, eauSVG)
	}
	for _, d := range depts {
		if d.fondSVG != "" {
			fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d.fondSVG)
		}
	}
	b.WriteString(rivieresSVG.String())
	fmt.Fprintf(&b, `<path class="brule" d="%s"><title>Zone brûlée — %s hectares</title></path>`,
		burntSVG, Decimal(surfaceHa, 0))
	for _, d := range depts {
		if d.lx == 0 && d.ly == 0 {
			continue
		}
		fmt.Fprintf(&b, `<g transform="translate(%.0f,%.0f) scale(%.2f)">`+
			`<text class="nom-region" x="0" y="0" text-anchor="middle">%s</text></g>`,
			d.lx, -d.ly, scale, template.HTMLEscapeString(d.nom))
	}
	for _, v := range villes {
		fmt.Fprintf(&b, `<g transform="translate(%.0f,%.0f) scale(%.2f)">`+
			`<circle class="ville-point" r="3"/>`+
			`<text class="ville-nom" x="6" y="4">%s</text></g>`,
			v.x, -v.y, scale, template.HTMLEscapeString(v.nom))
	}
	b.WriteString(`</svg>`)
	fmt.Fprintf(&b, `<figcaption>Périmètre brûlé détecté par télédétection, %s hectares — Copernicus EMS, `+
		`cartographie rapide, activation EMSR899 (« Wildfire in Saumos, France »), produit de délinéation, `+
		`deuxième passage de suivi (fin juillet 2026). Un chiffre plus élevé (42 000 ha) a circulé dans la `+
		`presse à la même période&nbsp;: il peut inclure une extension postérieure à ce passage, ou le feu `+
		`distinct des Landes, jamais confondu avec celui-ci dans ce dossier. Fond, cours d'eau (l'Eyre, le `+
		`Lacanau) et contour départemental&nbsp;: © les contributeurs OpenStreetMap, ODbL 1.0 ; polygones `+
		`d'eau, osmdata.openstreetmap.de.</figcaption></figure>`, Decimal(surfaceHa, 0))
	return template.HTML(b.String()), nil
}
