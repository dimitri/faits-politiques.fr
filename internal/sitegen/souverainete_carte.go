package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// siteSemiConducteur : les sites français de production de semi-conducteurs
// identifiés par Sirene (internal/dossiers/souverainete_solutions.go) et
// déjà cités dans docs/souverainete-numerique.md § 2 — géocodés à la
// commune (api-adresse.data.gouv.fr, vérifié le 19 septembre 2026), pas à
// l'adresse exacte de l'usine : la Cour des comptes elle-même relève que
// l'État ne dispose d'aucune cartographie de cette filière (rapport avril
// 2026, cité au § 2), ce que cette carte comble partiellement.
type siteSemiConducteur struct {
	Nom, Commune, Note string
	Lon, Lat           float64
}

var sitesSemiConducteurs = []siteSemiConducteur{
	{"STMicroelectronics (Crolles 2)", "Crolles (Isère)",
		"Unité légale Sirene 399395581 ; projet « Liberty » avec GlobalFoundries, aide d'État plafonnée à 2,9 Md€.",
		5.883069, 45.283529},
	{"STMicroelectronics Rousset", "Rousset (Bouches-du-Rhône)",
		"Unité légale Sirene 414969584.", 5.620074, 43.481809},
	{"STMicroelectronics (Tours)", "Tours (Indre-et-Loire)",
		"Unité légale Sirene 380932590.", 0.695848, 47.395476},
	{"STMicroelectronics (Grenoble 2)", "Grenoble (Isère)",
		"Unité légale Sirene 504941337, recherche-développement.", 5.724301, 45.182828},
	{"Soitec", "Bernin (Isère)",
		"Unité légale Sirene 384711909 ; matériaux pour semi-conducteurs ; aide d'État de 231,8 M€.",
		5.867065, 45.267187},
}

// chargerCarteSemiConducteurs : le fond France métropolitaine (même
// technique d'isolation des outre-mer que la carte de la Seconde Guerre
// mondiale, internal/sitegen/seconde_guerre_mondiale.go) et cinq points
// géocodés à la commune — pas de taille proportionnelle : les montants
// d'aide publique ne sont connus que pour deux des cinq sites (Crolles,
// Soitec), les faire varier en taille aurait suggéré une précision que la
// source n'a que pour deux points sur cinq.
//
// Projetée en Lambert-93 (2154), la convention de ce dépôt pour toute carte
// de la seule France, plutôt qu'en degrés WGS84 bruts : à la latitude de la
// France, un degré de longitude vaut environ 0,68 fois un degré de latitude
// en distance réelle (cosinus de 47°), et la carte paraissait environ 47 %
// trop large d'ouest en est avant cette correction.
func chargerCarteSemiConducteurs(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	var fondChemin, viewBox sql.NullString
	if err := pool.QueryRow(ctx, `
		WITH france AS (
			SELECT st_union(geom) g
			FROM (SELECT (ST_Dump(geom)).path AS path, (ST_Dump(geom)).geom AS geom
			      FROM geo.contour_pays WHERE nom_fr='France') d
			WHERE d.path[1] IN (1, 2)
		), proj AS (SELECT st_transform(g, 2154) AS g FROM france)
		SELECT st_assvg(g, 1, 0),
		       round(st_xmin(g))||' '||round(-st_ymax(g))||' '||
		       round(st_xmax(g)-st_xmin(g))||' '||round(st_ymax(g)-st_ymin(g))
		FROM proj`).Scan(&fondChemin, &viewBox); err != nil {
		return "", err
	}
	if !fondChemin.Valid || fondChemin.String == "" {
		return "", nil
	}
	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return "", err
	}

	lons := make([]float64, len(sitesSemiConducteurs))
	lats := make([]float64, len(sitesSemiConducteurs))
	for i, s := range sitesSemiConducteurs {
		lons[i], lats[i] = s.Lon, s.Lat
	}
	rows, err := pool.Query(ctx, `
		SELECT st_x(g), st_y(g)
		FROM unnest($1::float8[], $2::float8[]) WITH ORDINALITY AS v(lon, lat, ord)
		CROSS JOIN LATERAL (SELECT st_transform(st_setsrid(st_makepoint(v.lon, v.lat), 4326), 2154) g) t
		ORDER BY v.ord`, lons, lats)
	if err != nil {
		return "", err
	}
	points := make([]struct{ X, Y float64 }, 0, len(sitesSemiConducteurs))
	for rows.Next() {
		var p struct{ X, Y float64 }
		if err := rows.Scan(&p.X, &p.Y); err != nil {
			rows.Close()
			return "", err
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	return dessinerCarteSemiConducteurs(fondChemin.String, fleuves, viewBox.String, points), nil
}

func dessinerCarteSemiConducteurs(fond, fleuves, viewBox string, points []struct{ X, Y float64 }) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo france semi-conducteurs" role="img" `+
		`aria-label="Sites français de production de semi-conducteurs">`, viewBox)
	fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, fond)
	b.WriteString(fleuves)
	for i, s := range sitesSemiConducteurs {
		titre := fmt.Sprintf("%s, %s — %s", s.Nom, s.Commune, s.Note)
		fmt.Fprintf(&b, `<circle class="site" cx="%.0f" cy="%.0f" r="14000"><title>%s</title></circle>`,
			points[i].X, -points[i].Y, template.HTMLEscapeString(titre))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
