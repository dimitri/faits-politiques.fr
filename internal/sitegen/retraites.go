package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// loadAgeStartPension : la série 2004-2022 était déjà chargée (Drees,
// âge conjoncturel moyen), en U — un creux en 2010 juste avant la réforme
// qui recule progressivement l'âge légal — mais jamais tracée. Ne réutilise
// PAS courbePaliers : cette fonction ancre volontairement son axe à zéro
// (la question qu'elle pose est « quelle part »), ce qui est le bon choix
// pour un pourcentage mais écraserait ici tout le U dans les derniers pour
// cent d'un axe 0-68 ans — un âge n'a pas de zéro qui compare. dessinerAgePaliers
// reprend le même principe de bandes de régime, avec un axe borné à la
// série elle-même.
func loadAgeStartPension(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, age_ensemble FROM core.age_depart_retraite ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []PointYear
	for rows.Next() {
		var p PointYear
		if err := rows.Scan(&p.Year, &p.Value); err != nil {
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
	brackets := []Bracket{
		{Of: 2004, A: 2010, Label: "Avant la réforme de 2010"},
		{Of: 2010, A: 2022, Label: "Depuis la réforme de 2010"},
	}
	format := func(v float64) string { return Decimal(v, 1) + " ans" }
	return drawAgeBrackets(pts, brackets, format), nil
}

// drawAgeBrackets : même principe visuel que courbePaliers (bandes de
// régime datées, moyenne par bande) mais un axe borné au minimum et au
// maximum réels de la série, pas à zéro — adapté à une variable d'échelle
// (un âge), pas à une part.
func drawAgeBrackets(pts []PointYear, brackets []Bracket, format func(float64) string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 52.0, 26.0

	min, max := pts[0].Value, pts[0].Value
	for _, p := range pts {
		if p.Value < min {
			min = p.Value
		}
		if p.Value > max {
			max = p.Value
		}
	}
	margin := (max - min) * 0.15
	if margin <= 0 {
		margin = 1
	}
	bottom, top := min-margin, max+margin

	idx := map[int]int{}
	for i, p := range pts {
		idx[p.Year] = i
	}
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(len(pts)-1) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-(v-bottom)/(top-bottom)) }

	var seg strings.Builder
	for i, p := range pts {
		fmt.Fprintf(&seg, " L%.1f %.1f", x(i), y(p.Value))
	}
	trace := "M" + strings.TrimPrefix(strings.TrimSpace(seg.String()), "L")

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe paliers" viewBox="0 0 %.0f %.0f" role="img" aria-label="Série annuelle de %d à %d, de %s à %s">`,
		w, h, pts[0].Year, pts[len(pts)-1].Year, format(pts[0].Value), format(pts[len(pts)-1].Value))

	for n, pal := range brackets {
		i, ok := idx[pal.Of]
		j, ok2 := idx[pal.A]
		if !ok || !ok2 || j <= i {
			continue
		}
		var sum float64
		for k := i; k <= j; k++ {
			sum += pts[k].Value
		}
		moy := sum / float64(j-i+1)
		x1, x2 := x(i), x(j)
		if n%2 == 0 {
			fmt.Fprintf(&b, `<rect class="bande" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
				x1, mt, x2-x1, h-mt-mb)
		}
		middle := (x1 + x2) / 2
		fmt.Fprintf(&b, `<text class="pal" x="%.1f" y="%.1f">%s</text>`, middle, mt-30, template.HTMLEscapeString(pal.Label))
		fmt.Fprintf(&b, `<text class="pal moy" x="%.1f" y="%.1f">%s</text>`, middle, mt-14, template.HTMLEscapeString(format(moy)))
	}

	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, h-mb, w-mr, h-mb)
	fmt.Fprintf(&b, `<path class="trait" d="%s"/>`, trace)

	marker := func(i int, cl string) {
		fmt.Fprintf(&b, `<circle class="pt" cx="%.1f" cy="%.1f" r="4"><title>%d — %s</title></circle>`,
			x(i), y(pts[i].Value), pts[i].Year, template.HTMLEscapeString(format(pts[i].Value)))
		fmt.Fprintf(&b, `<text class="et%s" x="%.1f" y="%.1f">%s</text>`,
			cl, x(i), y(pts[i].Value)-10, template.HTMLEscapeString(format(pts[i].Value)))
	}
	imin := 0
	for i, p := range pts {
		if p.Value < pts[imin].Value {
			imin = i
		}
	}
	marker(0, "")
	if imin != 0 && imin != len(pts)-1 {
		marker(imin, " haut")
	}
	marker(len(pts)-1, " fin")

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, x(0), h-8, pts[0].Year)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, x(len(pts)-1), h-8, pts[len(pts)-1].Year)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type quantilesPension struct {
	Category                string
	Q10, Q25, Q50, Q75, Q90 float64
}

// loadRateReplacement : trois lignes (Ensemble/Femmes/Hommes), chacune
// avec sa dispersion complète (q10 à q90) plutôt que la seule médiane — la
// dispersion EST le sujet ("taux de remplacement" cache des situations très
// inégales derrière une moyenne).
func loadRateReplacement(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT categorie, taux_q10, taux_q25, taux_q50, taux_q75, taux_q90
		FROM core.taux_remplacement_retraite
		WHERE premiere_annee_retraite = 2020 AND revenu_reference = 'Niveau de vie' AND caracteristique = 'Sexe'
		ORDER BY CASE categorie WHEN 'Ensemble' THEN 0 WHEN 'Femme' THEN 1 ELSE 2 END`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var qs []quantilesPension
	for rows.Next() {
		var q quantilesPension
		if err := rows.Scan(&q.Category, &q.Q10, &q.Q25, &q.Q50, &q.Q75, &q.Q90); err != nil {
			return "", err
		}
		qs = append(qs, q)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(qs) == 0 {
		return "", nil
	}
	return drawQuantilesPension(qs), nil
}

// drawQuantilesPension : une boîte à moustaches par catégorie — la
// tige q10-q90, la boîte q25-q75, un trait pour la médiane — avec un repère
// vertical à 100 (pension égale au revenu d'avant la retraite), le seul
// point de comparaison que le chiffre lui-même appelle.
func drawQuantilesPension(qs []quantilesPension) template.HTML {
	const widthLabel, mRight, mTop, mBottom, heightLine = 90.0, 16.0, 14.0, 26.0, 46.0
	const width = 720.0
	height := mTop + mBottom + heightLine*float64(len(qs))
	widthAxis := width - widthLabel - mRight

	max := 0.0
	for _, q := range qs {
		if q.Q90 > max {
			max = q.Q90
		}
	}
	max = max * 1.08
	x := func(v float64) float64 { return widthLabel + widthAxis*v/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="boite-moustaches" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Dispersion du taux de remplacement à la retraite, par sexe, cohorte 2020">`, width, height)
	fmt.Fprintf(&b, `<line class="repere-100" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		x(100), mTop, x(100), height-mBottom)
	fmt.Fprintf(&b, `<text class="et-100" x="%.1f" y="%.1f">100 (revenu inchangé)</text>`, x(100), mTop-2)
	for i, q := range qs {
		cy := mTop + heightLine*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, widthLabel-10, cy+4, template.HTMLEscapeString(q.Category))
		fmt.Fprintf(&b, `<line class="tige" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"><title>%s : du 1ᵉʳ au 9ᵉ décile, %s à %s</title></line>`,
			x(q.Q10), cy, x(q.Q90), cy, template.HTMLEscapeString(q.Category), Decimal(q.Q10, 1), Decimal(q.Q90, 1))
		fmt.Fprintf(&b, `<rect class="boite" x="%.1f" y="%.1f" width="%.1f" height="16"><title>%s : entre le 1ᵉʳ et le 3ᵉ quartile, %s à %s</title></rect>`,
			x(q.Q25), cy-8, x(q.Q75)-x(q.Q25), template.HTMLEscapeString(q.Category), Decimal(q.Q25, 1), Decimal(q.Q75, 1))
		fmt.Fprintf(&b, `<line class="mediane" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"><title>%s : médiane %s</title></line>`,
			x(q.Q50), cy-8, x(q.Q50), cy+8, template.HTMLEscapeString(q.Category), Decimal(q.Q50, 1))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
