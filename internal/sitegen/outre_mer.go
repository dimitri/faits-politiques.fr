package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type gapPriceDOM struct {
	Territory        string
	Gap2010, Gap2022 float64
}

// loadGapPriceDOM : l'écart de prix (indice de Fisher) entre chaque DOM
// et la France métropolitaine, 2010 et 2022 (Insee ECSP) — un graphique en
// haltère, même patron que dessinerHaltereCommerce
// (internal/sitegen/appareil_productif.go).
func loadGapPriceDOM(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT territoire,
		       max(fisher_general_pct) FILTER (WHERE annee=2010),
		       max(fisher_general_pct) FILTER (WHERE annee=2022)
		FROM core.ecart_prix_dom
		GROUP BY territoire
		HAVING max(fisher_general_pct) FILTER (WHERE annee=2022) IS NOT NULL
		ORDER BY max(fisher_general_pct) FILTER (WHERE annee=2022) DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []gapPriceDOM
	for rows.Next() {
		var p gapPriceDOM
		var e2010 *float64
		if err := rows.Scan(&p.Territory, &e2010, &p.Gap2022); err != nil {
			return "", err
		}
		if e2010 != nil {
			p.Gap2010 = *e2010
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	return drawGapPriceDOM(pts), nil
}

func drawGapPriceDOM(pts []gapPriceDOM) template.HTML {
	const widthLabel, mRight, mTop, mBottom, heightLine = 160.0, 60.0, 10.0, 24.0, 32.0
	const width = 720.0
	height := mTop + mBottom + heightLine*float64(len(pts))
	widthAxis := width - widthLabel - mRight

	max := 0.0
	for _, p := range pts {
		if p.Gap2022 > max {
			max = p.Gap2022
		}
	}
	max *= 1.25
	x := func(pct float64) float64 { return widthLabel + widthAxis*pct/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="haltere-commerce ecart-dom" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Écart de prix (indice de Fisher) entre les DOM et la France métropolitaine, 2010 et 2022">`,
		width, height)
	for i, p := range pts {
		cy := mTop + heightLine*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="pays" x="%.1f" y="%.1f">%s</text>`,
			widthLabel-10, cy+4, template.HTMLEscapeString(p.Territory))
		if p.Gap2010 > 0 {
			fmt.Fprintf(&b, `<line class="trait" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				x(p.Gap2010), cy, x(p.Gap2022), cy)
			fmt.Fprintf(&b, `<circle class="pt-debut" cx="%.1f" cy="%.1f" r="4.5"><title>%s, 2010 : +%s %%</title></circle>`,
				x(p.Gap2010), cy, template.HTMLEscapeString(p.Territory), Decimal(p.Gap2010, 1))
		}
		fmt.Fprintf(&b, `<circle class="pt-fin" cx="%.1f" cy="%.1f" r="4.5"><title>%s, 2022 : +%s %%</title></circle>`,
			x(p.Gap2022), cy, template.HTMLEscapeString(p.Territory), Decimal(p.Gap2022, 1))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">+%s %%</text>`,
			x(p.Gap2022)+10, cy+4, Decimal(p.Gap2022, 1))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type foodDOM struct {
	Territory     string
	General, Food float64
}

// loadFoodDOM : l'écart général contre l'écart sur les seuls
// produits alimentaires, 2022 — le point que docs/outre-mer-donnees.md
// souligne : les deux séries ne se ressemblent pas et ne doivent jamais
// être confondues.
func loadFoodDOM(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT territoire, fisher_general_pct, fisher_alimentaire_pct
		FROM core.ecart_prix_dom WHERE annee=2022 AND fisher_alimentaire_pct IS NOT NULL
		ORDER BY fisher_alimentaire_pct DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []foodDOM
	for rows.Next() {
		var p foodDOM
		if err := rows.Scan(&p.Territory, &p.General, &p.Food); err != nil {
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
	sort.Slice(pts, func(i, j int) bool { return pts[i].Food > pts[j].Food })
	return drawFoodDOM(pts), nil
}

func drawFoodDOM(pts []foodDOM) template.HTML {
	const widthLabel, mRight, mTop, mBottom, heightLine = 160.0, 60.0, 10.0, 24.0, 32.0
	const width = 720.0
	height := mTop + mBottom + heightLine*float64(len(pts))
	widthAxis := width - widthLabel - mRight

	max := 0.0
	for _, p := range pts {
		if p.Food > max {
			max = p.Food
		}
	}
	max *= 1.15
	x := func(pct float64) float64 { return widthLabel + widthAxis*pct/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="haltere-commerce ecart-dom-alim" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Écart de prix général contre écart sur les produits alimentaires, DOM, 2022">`,
		width, height)
	for i, p := range pts {
		cy := mTop + heightLine*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="pays" x="%.1f" y="%.1f">%s</text>`,
			widthLabel-10, cy+4, template.HTMLEscapeString(p.Territory))
		fmt.Fprintf(&b, `<line class="trait" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			x(p.General), cy, x(p.Food), cy)
		fmt.Fprintf(&b, `<circle class="pt-debut" cx="%.1f" cy="%.1f" r="4.5"><title>%s, écart général : +%s %%</title></circle>`,
			x(p.General), cy, template.HTMLEscapeString(p.Territory), Decimal(p.General, 1))
		fmt.Fprintf(&b, `<circle class="pt-fin" cx="%.1f" cy="%.1f" r="4.5"><title>%s, écart alimentaire : +%s %%</title></circle>`,
			x(p.Food), cy, template.HTMLEscapeString(p.Territory), Decimal(p.Food, 1))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">+%s %%</text>`,
			x(p.Food)+10, cy+4, Decimal(p.Food, 1))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
