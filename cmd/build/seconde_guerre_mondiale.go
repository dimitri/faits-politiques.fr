package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsSecondeGuerreMondiale struct {
	CarteSVG   template.HTML
	LongueurKm float64
}

// chargerSecondeGuerreMondiale : le tracé de la ligne de démarcation
// (geo.ligne_demarcation) superposé au contour de la France métropolitaine
// et de la Corse (geo.contour_pays, sous-géométries 1 et 2 — le reste du
// multipolygone France de Natural Earth couvre les outre-mer, hors sujet
// ici).
func chargerSecondeGuerreMondiale(ctx context.Context, pool *pgxpool.Pool) (*StatsSecondeGuerreMondiale, error) {
	var fondChemin string
	var ok1 bool
	if err := pool.QueryRow(ctx, `
		SELECT st_assvg(st_union(g.geom), 0, 4)
		FROM (SELECT (ST_Dump(geom)).path AS path, (ST_Dump(geom)).geom AS geom
		      FROM geo.contour_pays WHERE nom_fr='France') g
		WHERE g.path[1] IN (1, 2)`).Scan(&fondChemin); err == nil {
		ok1 = fondChemin != ""
	}
	if !ok1 {
		return nil, nil
	}

	var ligneChemin string
	var longueurM float64
	err := pool.QueryRow(ctx, `
		SELECT st_assvg(geom, 0, 4), longueur_m FROM geo.ligne_demarcation LIMIT 1`).
		Scan(&ligneChemin, &longueurM)
	if err != nil {
		return &StatsSecondeGuerreMondiale{CarteSVG: dessinerCarteSGM(fondChemin, "")}, nil
	}

	st := &StatsSecondeGuerreMondiale{LongueurKm: longueurM / 1000}
	st.CarteSVG = dessinerCarteSGM(fondChemin, ligneChemin)
	return st, nil
}

// dessinerCarteSGM : la France (fond neutre) et le tracé de la ligne de
// démarcation par-dessus — pas de remplissage par zone (occupée/libre),
// parce qu'aucune géométrie de zone vérifiée n'a été trouvée, seulement le
// tracé de la ligne elle-même (voir § 2 du dossier).
func dessinerCarteSGM(fond, ligne string) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg viewBox="-6 -52 16 12" class="geo france sgm" role="img" ` +
		`aria-label="Tracé de la ligne de démarcation, 1940-1942">`)
	fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, fond)
	if ligne != "" {
		fmt.Fprintf(&b, `<path class="ligne-demarcation" d="%s"><title>Ligne de démarcation, 1940-1942</title></path>`, ligne)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
