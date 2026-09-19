package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// portAncre : un point de repère par grand port, pour situer les quatre
// ports sur la carte et nommer leurs cercles — HAROPA est représenté au
// Havre, la porte maritime de l'axe (Rouen et Paris ne sont pas des ports
// maritimes). Coordonnées vérifiées via le centroïde de la commune dans
// geo.contour_cog (IGN COG 2026), pas saisies à l'estime.
type portAncre struct {
	ID, Nom  string
	Lon, Lat float64
}

var portsCarte = []portAncre{
	{"dunkerque", "Dunkerque", 2.3374, 51.0304},
	{"haropa", "HAROPA (Le Havre)", 0.1412, 49.4983},
	{"marseille-fos", "Marseille-Fos", 4.9041, 43.4559},
	{"nantes-saint-nazaire", "Nantes-Saint-Nazaire", -2.2510, 47.2799},
}

type CarteInfrastructurePorts struct {
	SVG                          template.HTML
	NbAutoroutes, NbVoiesFerrees int
}

// chargerCarteInfrastructurePorts : fond des départements, grands cours
// d'eau (calque commun, cmd/build/carte.go), autoroutes et voies ferrées
// portuaires filtrées à 80 km des quatre ports (internal/macro/ports_infrastructure.go),
// et un point nommé par port — ni les débits ni la précision de la ligne à
// ligne des voies ferrées portuaires ne garantissent qu'un tronçon donné
// sert réellement le trafic fret : la carte montre l'accès physique, pas
// un flux mesuré.
func chargerCarteInfrastructurePorts(ctx context.Context, pool *pgxpool.Pool) (*CarteInfrastructurePorts, error) {
	var nbAuto int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM geo.autoroute_portuaire`).Scan(&nbAuto); err != nil {
		return nil, err
	}
	if nbAuto == 0 {
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
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo ports-infra" role="img" `+
		`aria-label="Accès autoroutier et ferroviaire des quatre grands ports maritimes français">`, vb)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, tolPleine)
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

	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(fleuves)

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

	var nbFer int
	ferRows, err := pool.Query(ctx, `SELECT st_assvg(st_transform(geom,2154),1,0) FROM geo.voie_ferree_portuaire`)
	if err != nil {
		return nil, err
	}
	for ferRows.Next() {
		var d string
		if err := ferRows.Scan(&d); err != nil {
			ferRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="voie-ferree-p" d="%s"/>`, d)
		nbFer++
	}
	if err := ferRows.Err(); err != nil {
		ferRows.Close()
		return nil, err
	}
	ferRows.Close()

	type xy struct{ X, Y float64 }
	coords := make(map[string]xy, len(portsCarte))
	rowsP, err := pool.Query(ctx, `
		SELECT v.id, st_x(g), st_y(g) FROM (VALUES ($1::text,$2::float8,$3::float8),
			($4,$5,$6),($7,$8,$9),($10,$11,$12)) v(id, lon, lat),
		LATERAL (SELECT st_transform(st_setsrid(st_makepoint(v.lon, v.lat), 4326), 2154) g) t`,
		portsCarte[0].ID, portsCarte[0].Lon, portsCarte[0].Lat,
		portsCarte[1].ID, portsCarte[1].Lon, portsCarte[1].Lat,
		portsCarte[2].ID, portsCarte[2].Lon, portsCarte[2].Lat,
		portsCarte[3].ID, portsCarte[3].Lon, portsCarte[3].Lat)
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
	decalages := map[string][2]float64{
		"dunkerque":            {14000, 5000},
		"haropa":               {-14000, -18000},
		"marseille-fos":        {18000, 5000},
		"nantes-saint-nazaire": {-14000, 5000},
	}
	ancres := map[string]string{
		"dunkerque": "start", "haropa": "end", "marseille-fos": "start", "nantes-saint-nazaire": "end",
	}
	for _, p := range portsCarte {
		c, ok := coords[p.ID]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, `<circle class="port-m" cx="%.0f" cy="%.0f" r="9000"><title>%s</title></circle>`,
			c.X, -c.Y, template.HTMLEscapeString(p.Nom))
		dx, dy := decalages[p.ID][0], decalages[p.ID][1]
		// font-size en attribut SVG plutôt qu'en CSS : un font-size sans
		// unité est invalide en CSS (contrairement à stroke-width/r, que les
		// navigateurs acceptent nus dans ce contexte) et le texte ne
		// s'affichait pas du tout — repéré à l'écran avant publication.
		fmt.Fprintf(&b, `<text class="port-lbl" font-size="13000" text-anchor="%s" x="%.0f" y="%.0f">%s</text>`,
			ancres[p.ID], c.X+dx, -c.Y+dy, template.HTMLEscapeString(p.Nom))
	}

	b.WriteString(`</svg>`)
	return &CarteInfrastructurePorts{SVG: template.HTML(b.String()), NbAutoroutes: nbAuto, NbVoiesFerrees: nbFer}, nil
}

type pointModalPort struct {
	Port                    string
	Massifiee               float64
	EstPlafond              bool
	Fer, Fleuve             *float64
	AucunMassifiee          bool
}

// chargerReportModal charge le report modal (fer+fleuve vs route) par port,
// tel que publié par le rapport DGITM (une seule année, un chiffre par
// port — voir internal/macro/ports_report_modal.go).
func chargerReportModal(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
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
		var massifiee *float64
		if err := rows.Scan(&p.Port, &massifiee, &p.EstPlafond, &p.Fer, &p.Fleuve); err != nil {
			return "", err
		}
		if massifiee == nil {
			p.AucunMassifiee = true
		} else {
			p.Massifiee = *massifiee
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	return dessinerReportModal(pts), nil
}

func dessinerReportModal(pts []pointModalPort) template.HTML {
	const w, mr, ml, largeurBarre, gap = 720.0, 90.0, 190.0, 30.0, 16.0
	h := float64(len(pts))*(largeurBarre+gap) + gap
	largeurAxe := w - ml - mr
	const max = 60.0 // marge au-dessus du maximum observé (52 %)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="report-modal-port" role="img" `+
		`aria-label="Part du fer et du fleuve dans le pré- et post-acheminement, par port, 2023">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(largeurBarre+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+largeurBarre/2+4, template.HTMLEscapeString(p.Port))
		if p.AucunMassifiee {
			fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">non publié</text>`, ml+8, y+largeurBarre/2+4)
			continue
		}
		largeur := largeurAxe * p.Massifiee / max
		classe := "barre-plein"
		prefixe := ""
		if p.EstPlafond {
			classe = "barre-plafond"
			prefixe = "moins de "
		}
		titre := fmt.Sprintf("%s : %s%s %% massifié (fer + fleuve), 2023", p.Port, prefixe, Decimal(p.Massifiee, 0))
		if p.Fer != nil && p.Fleuve != nil {
			titre = fmt.Sprintf("%s — dont %s %% fer, %s %% fleuve", titre, Decimal(*p.Fer, 0), Decimal(*p.Fleuve, 0))
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f"><title>%s</title></rect>`,
			classe, ml, y, largeur, largeurBarre, template.HTMLEscapeString(titre))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">%s%s %%</text>`,
			ml+largeur+8, y+largeurBarre/2+4, prefixe, Decimal(p.Massifiee, 0))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointModalConteneurs struct {
	Port, Pays                    string
	Fer, Fleuve, Route            float64
	RouteCalculee                 bool
}

// chargerReportModalConteneurs charge la comparaison conteneurs seuls
// (Marseille-Fos, Anvers-Bruges, Rotterdam) — jamais à comparer aux
// chiffres tous-trafics de chargerReportModal (voir la table core.report_modal_conteneurs).
func chargerReportModalConteneurs(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT port, pays, part_fer_pct, part_fleuve_pct, part_route_pct, route_calculee
		FROM core.report_modal_conteneurs ORDER BY part_fer_pct DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []pointModalConteneurs
	for rows.Next() {
		var p pointModalConteneurs
		if err := rows.Scan(&p.Port, &p.Pays, &p.Fer, &p.Fleuve, &p.Route, &p.RouteCalculee); err != nil {
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
	return dessinerReportModalConteneurs(pts), nil
}

func dessinerReportModalConteneurs(pts []pointModalConteneurs) template.HTML {
	const w, mr, ml, largeurBarre, gap = 720.0, 12.0, 130.0, 30.0, 16.0
	h := float64(len(pts))*(largeurBarre+gap) + gap
	largeurAxe := w - ml - mr

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="report-modal-conteneurs" role="img" `+
		`aria-label="Répartition modale du transport de conteneurs vers l'arrière-pays, trois ports">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(largeurBarre+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+largeurBarre/2+4, template.HTMLEscapeString(p.Port))
		x := ml
		seg := func(classe string, valeur float64, label string) {
			largeur := largeurAxe * valeur / 100
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f"><title>%s, %s : %s %%</title></rect>`,
				classe, x, y, largeur, largeurBarre, template.HTMLEscapeString(p.Port), label, Decimal(valeur, 1))
			x += largeur
		}
		seg("seg-fer", p.Fer, "fer")
		seg("seg-fleuve", p.Fleuve, "fleuve")
		routeNote := ""
		if p.RouteCalculee {
			routeNote = " (calculée par complément)"
		}
		seg("seg-route", p.Route, "route"+routeNote)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
