package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// portAnchor : un point de repère par grand port, pour situer les quatre
// ports sur la carte et nommer leurs cercles — HAROPA est représenté au
// Havre, la porte maritime de l'axe (Rouen et Paris ne sont pas des ports
// maritimes). Coordonnées vérifiées via le centroïde de la commune dans
// geo.contour_cog (IGN COG 2026), pas saisies à l'estime.
type portAnchor struct {
	ID, Name string
	Lon, Lat float64
}

var portsMap = []portAnchor{
	{"dunkerque", "Dunkerque", 2.3374, 51.0304},
	{"haropa", "HAROPA (Le Havre)", 0.1412, 49.4983},
	{"marseille-fos", "Marseille-Fos", 4.9041, 43.4559},
	{"nantes-saint-nazaire", "Nantes-Saint-Nazaire", -2.2510, 47.2799},
}

type MapInfrastructurePorts struct {
	SVG                           template.HTML
	CountHighways, CountLinesRail int
}

// loadMapInfrastructurePorts : fond des départements, grands cours
// d'eau (calque commun, internal/sitegen/carte.go), autoroutes et voies ferrées
// portuaires filtrées à 80 km des quatre ports (internal/macro/ports_infrastructure.go),
// et un point nommé par port — ni les débits ni la précision de la ligne à
// ligne des voies ferrées portuaires ne garantissent qu'un tronçon donné
// sert réellement le trafic fret : la carte montre l'accès physique, pas
// un flux mesuré.
func loadMapInfrastructurePorts(ctx context.Context, pool *pgxpool.Pool) (*MapInfrastructurePorts, error) {
	var countAuto int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM geo.autoroute_portuaire`).Scan(&countAuto); err != nil {
		return nil, err
	}
	if countAuto == 0 {
		return nil, nil
	}

	// st_extent est une agrégation : la ligne existe même sans contour
	// encore ingéré, avec une valeur NULL (voir internal/sitegen/carte.go).
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
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo ports-infra" role="img" `+
		`aria-label="Accès autoroutier et ferroviaire des quatre grands ports maritimes français">`, vb)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, toleranceFull)
	if err != nil {
		return nil, err
	}
	for depRows.Next() {
		var d string
		if err := depRows.Scan(&d); err != nil {
			depRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	if err := depRows.Err(); err != nil {
		depRows.Close()
		return nil, err
	}
	depRows.Close()

	rivers, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(rivers)

	autoRows, err := pool.Query(ctx, `SELECT st_assvg(geom,1,0) FROM geo.autoroute_portuaire`)
	if err != nil {
		return nil, err
	}
	for autoRows.Next() {
		var d string
		if err := autoRows.Scan(&d); err != nil {
			autoRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="autoroute-p" d="%s"/>`, d)
	}
	if err := autoRows.Err(); err != nil {
		autoRows.Close()
		return nil, err
	}
	autoRows.Close()

	var countRail int
	railRows, err := pool.Query(ctx, `SELECT st_assvg(st_transform(geom,2154),1,0) FROM geo.voie_ferree_portuaire`)
	if err != nil {
		return nil, err
	}
	for railRows.Next() {
		var d string
		if err := railRows.Scan(&d); err != nil {
			railRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="voie-ferree-p" d="%s"/>`, d)
		countRail++
	}
	if err := railRows.Err(); err != nil {
		railRows.Close()
		return nil, err
	}
	railRows.Close()

	type xy struct{ X, Y float64 }
	coords := make(map[string]xy, len(portsMap))
	rowsP, err := pool.Query(ctx, `
		SELECT v.id, st_x(g), st_y(g) FROM (VALUES ($1::text,$2::float8,$3::float8),
			($4,$5,$6),($7,$8,$9),($10,$11,$12)) v(id, lon, lat),
		LATERAL (SELECT st_transform(st_setsrid(st_makepoint(v.lon, v.lat), 4326), 2154) g) t`,
		portsMap[0].ID, portsMap[0].Lon, portsMap[0].Lat,
		portsMap[1].ID, portsMap[1].Lon, portsMap[1].Lat,
		portsMap[2].ID, portsMap[2].Lon, portsMap[2].Lat,
		portsMap[3].ID, portsMap[3].Lon, portsMap[3].Lat)
	if err != nil {
		return nil, err
	}
	for rowsP.Next() {
		var id string
		var c xy
		if err := rowsP.Scan(&id, &c.X, &c.Y); err != nil {
			rowsP.Close()
			return nil, err
		}
		coords[id] = c
	}
	if err := rowsP.Err(); err != nil {
		rowsP.Close()
		return nil, err
	}
	rowsP.Close()

	// Décalages d'étiquette réglés à l'écran par port, pour ne jamais
	// écrire un nom de port dans la mer ou hors du cadre.
	offsets := map[string][2]float64{
		"dunkerque":            {14000, 5000},
		"haropa":               {-14000, -18000},
		"marseille-fos":        {18000, 5000},
		"nantes-saint-nazaire": {-14000, 5000},
	}
	anchors := map[string]string{
		"dunkerque": "start", "haropa": "end", "marseille-fos": "start", "nantes-saint-nazaire": "end",
	}
	for _, p := range portsMap {
		c, ok := coords[p.ID]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, `<circle class="port-m" cx="%.0f" cy="%.0f" r="9000"><title>%s</title></circle>`,
			c.X, -c.Y, template.HTMLEscapeString(p.Name))
		dx, dy := offsets[p.ID][0], offsets[p.ID][1]
		// font-size en attribut SVG plutôt qu'en CSS : un font-size sans
		// unité est invalide en CSS (contrairement à stroke-width/r, que les
		// navigateurs acceptent nus dans ce contexte) et le texte ne
		// s'affichait pas du tout — repéré à l'écran avant publication.
		fmt.Fprintf(&b, `<text class="port-lbl" font-size="13000" text-anchor="%s" x="%.0f" y="%.0f">%s</text>`,
			anchors[p.ID], c.X+dx, -c.Y+dy, template.HTMLEscapeString(p.Name))
	}

	b.WriteString(`</svg>`)
	return &MapInfrastructurePorts{SVG: template.HTML(b.String()), CountHighways: countAuto, CountLinesRail: countRail}, nil
}

type pointModalPort struct {
	Port        string
	Scaled      float64
	EstCap      bool
	Rail, River *float64
	NoneScaled  bool
}

// loadReportModal charge le report modal (fer+fleuve vs route) par port,
// tel que publié par le rapport DGITM (une seule année, un chiffre par
// port — voir internal/macro/ports_report_modal.go).
func loadReportModal(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT port, part_massifiee_pct, est_plafond, part_fer_pct, part_fleuve_pct
		FROM core.report_modal_port ORDER BY part_massifiee_pct DESC NULLS LAST`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []pointModalPort
	for rows.Next() {
		var p pointModalPort
		var scaled *float64
		if err := rows.Scan(&p.Port, &scaled, &p.EstCap, &p.Rail, &p.River); err != nil {
			return "", err
		}
		if scaled == nil {
			p.NoneScaled = true
		} else {
			p.Scaled = *scaled
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	return drawReportModal(pts), nil
}

func drawReportModal(pts []pointModalPort) template.HTML {
	const w, mr, ml, widthBar, gap = 720.0, 90.0, 190.0, 30.0, 16.0
	h := float64(len(pts))*(widthBar+gap) + gap
	widthAxis := w - ml - mr
	const max = 60.0 // marge au-dessus du maximum observé (52 %)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="report-modal-port" role="img" `+
		`aria-label="Part du fer et du fleuve dans le pré- et post-acheminement, par port, 2023">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(widthBar+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+widthBar/2+4, template.HTMLEscapeString(p.Port))
		if p.NoneScaled {
			fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">non publié</text>`, ml+8, y+widthBar/2+4)
			continue
		}
		width := widthAxis * p.Scaled / max
		class := "barre-plein"
		prefix := ""
		if p.EstCap {
			class = "barre-plafond"
			prefix = "moins de "
		}
		title := fmt.Sprintf("%s : %s%s %% massifié (fer + fleuve), 2023", p.Port, prefix, Decimal(p.Scaled, 0))
		if p.Rail != nil && p.River != nil {
			title = fmt.Sprintf("%s — dont %s %% fer, %s %% fleuve", title, Decimal(*p.Rail, 0), Decimal(*p.River, 0))
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f"><title>%s</title></rect>`,
			class, ml, y, width, widthBar, template.HTMLEscapeString(title))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">%s%s %%</text>`,
			ml+width+8, y+widthBar/2+4, prefix, Decimal(p.Scaled, 0))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointModalContainers struct {
	Port, Country      string
	Rail, River, Route float64
	RouteCalculated    bool
}

// loadReportModalContainers charge la comparaison conteneurs seuls
// (Marseille-Fos, Anvers-Bruges, Rotterdam) — jamais à comparer aux
// chiffres tous-trafics de chargerReportModal (voir la table core.report_modal_conteneurs).
func loadReportModalContainers(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT port, pays, part_fer_pct, part_fleuve_pct, part_route_pct, route_calculee
		FROM core.report_modal_conteneurs ORDER BY part_fer_pct DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []pointModalContainers
	for rows.Next() {
		var p pointModalContainers
		if err := rows.Scan(&p.Port, &p.Country, &p.Rail, &p.River, &p.Route, &p.RouteCalculated); err != nil {
			return "", err
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	return drawReportModalContainers(pts), nil
}

func drawReportModalContainers(pts []pointModalContainers) template.HTML {
	const w, mr, ml, widthBar, gap = 720.0, 12.0, 130.0, 30.0, 16.0
	h := float64(len(pts))*(widthBar+gap) + gap
	widthAxis := w - ml - mr

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="report-modal-conteneurs" role="img" `+
		`aria-label="Répartition modale du transport de conteneurs vers l'arrière-pays, trois ports">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(widthBar+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+widthBar/2+4, template.HTMLEscapeString(p.Port))
		x := ml
		seg := func(class string, value float64, label string) {
			width := widthAxis * value / 100
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f"><title>%s, %s : %s %%</title></rect>`,
				class, x, y, width, widthBar, template.HTMLEscapeString(p.Port), label, Decimal(value, 1))
			x += width
		}
		seg("seg-fer", p.Rail, "fer")
		seg("seg-fleuve", p.River, "fleuve")
		routeNote := ""
		if p.RouteCalculated {
			routeNote = " (calculée par complément)"
		}
		seg("seg-route", p.Route, "route"+routeNote)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
