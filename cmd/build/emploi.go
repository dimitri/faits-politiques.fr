package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// chargerEmploiTotalSalarie : 51 valeurs par série (Eurostat nama_10_pe,
// 1975-2025) étaient chargées mais réduites à dix années choisies à la main
// en tableau. Seules deux courbes (total, salarié) sont tracées, sur la
// même échelle : les non-salariés (2,3 à 3,8 millions) sont un ordre de
// grandeur en dessous du total (22 à 31 millions) — une troisième courbe à
// cette échelle serait écrasée près de zéro, comme l'aurait été l'âge de
// départ à la retraite sur un axe à zéro (voir cmd/build/retraites.go).
// L'écart entre les deux courbes EST le nombre de non-salariés, lisible
// sans troisième trait.
func chargerEmploiTotalSalarie(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, serie_code, valeur FROM core.macro_value
		WHERE serie_code IN ('emploi.total', 'emploi.salarie') ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	parAnnee := map[int]map[string]float64{}
	var annees []int
	for rows.Next() {
		var annee int
		var code string
		var v float64
		if err := rows.Scan(&annee, &code, &v); err != nil {
			return "", err
		}
		if parAnnee[annee] == nil {
			parAnnee[annee] = map[string]float64{}
			annees = append(annees, annee)
		}
		parAnnee[annee][code] = v
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(annees) == 0 {
		return "", nil
	}

	type pt struct {
		Annee          int
		Total, Salarie float64
	}
	var pts []pt
	for _, a := range annees {
		total, okT := parAnnee[a]["emploi.total"]
		salarie, okS := parAnnee[a]["emploi.salarie"]
		if !okT || !okS {
			continue
		}
		pts = append(pts, pt{a, total, salarie})
	}
	if len(pts) == 0 {
		return "", nil
	}

	const w, h, ml, mr, mt, mb = 720.0, 320.0, 34.0, 78.0, 14.0, 26.0
	n := len(pts)
	max := 0.0
	for _, p := range pts {
		if p.Total > max {
			max = p.Total
		}
	}
	max *= 1.05
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(n-1) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe emploi-total" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Emploi total, salarié et non salarié en France, %d à %d">`, w, h, pts[0].Annee, pts[n-1].Annee)
	for _, palier := range []float64{0, 10000, 20000, 30000} {
		if palier > max {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s M</text>`, ml-6, y(palier)+3, Decimal(palier/1000, 0))
	}
	tracer := func(cl, nom string, sel func(pt) float64) {
		var coords []string
		for i, p := range pts {
			coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(i), y(sel(p))))
		}
		fmt.Fprintf(&b, `<polyline class="%s" points="%s"/>`, cl, strings.Join(coords, " "))
		dernier := pts[n-1]
		fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%s, %d : %s</title></circle>`,
			cl, x(n-1), y(sel(dernier)), nom, dernier.Annee, Decimal(sel(dernier)/1000, 2)+" M")
		fmt.Fprintf(&b, `<text class="%s-lbl" x="%.1f" y="%.1f">%s</text>`, cl, x(n-1)+4, y(sel(dernier))+3, template.HTMLEscapeString(nom))
	}
	tracer("total", "Total", func(p pt) float64 { return p.Total })
	tracer("salarie", "Salarié", func(p pt) float64 { return p.Salarie })
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, pts[0].Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, pts[n-1].Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), nil
}
