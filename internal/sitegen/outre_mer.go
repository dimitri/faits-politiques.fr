package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ecartPrixDOM struct {
	Territoire           string
	Ecart2010, Ecart2022 float64
}

// chargerEcartPrixDOM : l'écart de prix (indice de Fisher) entre chaque DOM
// et la France métropolitaine, 2010 et 2022 (Insee ECSP) — un graphique en
// haltère, même patron que dessinerHaltereCommerce
// (internal/sitegen/appareil_productif.go).
func chargerEcartPrixDOM(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
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
	var pts []ecartPrixDOM
	for rows.Next() {
		var p ecartPrixDOM
		var e2010 *float64
		if err := rows.Scan(&p.Territoire, &e2010, &p.Ecart2022); err != nil {
			return "", err
		}
		if e2010 != nil {
			p.Ecart2010 = *e2010
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	return dessinerEcartPrixDOM(pts), nil
}

func dessinerEcartPrixDOM(pts []ecartPrixDOM) template.HTML {
	const largeurEtiquette, mDroite, mHaut, mBas, hauteurLigne = 160.0, 60.0, 10.0, 24.0, 32.0
	const largeur = 720.0
	hauteur := mHaut + mBas + hauteurLigne*float64(len(pts))
	largeurAxe := largeur - largeurEtiquette - mDroite

	max := 0.0
	for _, p := range pts {
		if p.Ecart2022 > max {
			max = p.Ecart2022
		}
	}
	max *= 1.25
	x := func(pct float64) float64 { return largeurEtiquette + largeurAxe*pct/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="haltere-commerce ecart-dom" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Écart de prix (indice de Fisher) entre les DOM et la France métropolitaine, 2010 et 2022">`,
		largeur, hauteur)
	for i, p := range pts {
		cy := mHaut + hauteurLigne*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="pays" x="%.1f" y="%.1f">%s</text>`,
			largeurEtiquette-10, cy+4, template.HTMLEscapeString(p.Territoire))
		if p.Ecart2010 > 0 {
			fmt.Fprintf(&b, `<line class="trait" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				x(p.Ecart2010), cy, x(p.Ecart2022), cy)
			fmt.Fprintf(&b, `<circle class="pt-debut" cx="%.1f" cy="%.1f" r="4.5"><title>%s, 2010 : +%s %%</title></circle>`,
				x(p.Ecart2010), cy, template.HTMLEscapeString(p.Territoire), Decimal(p.Ecart2010, 1))
		}
		fmt.Fprintf(&b, `<circle class="pt-fin" cx="%.1f" cy="%.1f" r="4.5"><title>%s, 2022 : +%s %%</title></circle>`,
			x(p.Ecart2022), cy, template.HTMLEscapeString(p.Territoire), Decimal(p.Ecart2022, 1))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">+%s %%</text>`,
			x(p.Ecart2022)+10, cy+4, Decimal(p.Ecart2022, 1))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type alimentaireDOM struct {
	Territoire           string
	General, Alimentaire float64
}

// chargerAlimentaireDOM : l'écart général contre l'écart sur les seuls
// produits alimentaires, 2022 — le point que docs/outre-mer-donnees.md
// souligne : les deux séries ne se ressemblent pas et ne doivent jamais
// être confondues.
func chargerAlimentaireDOM(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT territoire, fisher_general_pct, fisher_alimentaire_pct
		FROM core.ecart_prix_dom WHERE annee=2022 AND fisher_alimentaire_pct IS NOT NULL
		ORDER BY fisher_alimentaire_pct DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []alimentaireDOM
	for rows.Next() {
		var p alimentaireDOM
		if err := rows.Scan(&p.Territoire, &p.General, &p.Alimentaire); err != nil {
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
	sort.Slice(pts, func(i, j int) bool { return pts[i].Alimentaire > pts[j].Alimentaire })
	return dessinerAlimentaireDOM(pts), nil
}

func dessinerAlimentaireDOM(pts []alimentaireDOM) template.HTML {
	const largeurEtiquette, mDroite, mHaut, mBas, hauteurLigne = 160.0, 60.0, 10.0, 24.0, 32.0
	const largeur = 720.0
	hauteur := mHaut + mBas + hauteurLigne*float64(len(pts))
	largeurAxe := largeur - largeurEtiquette - mDroite

	max := 0.0
	for _, p := range pts {
		if p.Alimentaire > max {
			max = p.Alimentaire
		}
	}
	max *= 1.15
	x := func(pct float64) float64 { return largeurEtiquette + largeurAxe*pct/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="haltere-commerce ecart-dom-alim" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Écart de prix général contre écart sur les produits alimentaires, DOM, 2022">`,
		largeur, hauteur)
	for i, p := range pts {
		cy := mHaut + hauteurLigne*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="pays" x="%.1f" y="%.1f">%s</text>`,
			largeurEtiquette-10, cy+4, template.HTMLEscapeString(p.Territoire))
		fmt.Fprintf(&b, `<line class="trait" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			x(p.General), cy, x(p.Alimentaire), cy)
		fmt.Fprintf(&b, `<circle class="pt-debut" cx="%.1f" cy="%.1f" r="4.5"><title>%s, écart général : +%s %%</title></circle>`,
			x(p.General), cy, template.HTMLEscapeString(p.Territoire), Decimal(p.General, 1))
		fmt.Fprintf(&b, `<circle class="pt-fin" cx="%.1f" cy="%.1f" r="4.5"><title>%s, écart alimentaire : +%s %%</title></circle>`,
			x(p.Alimentaire), cy, template.HTMLEscapeString(p.Territoire), Decimal(p.Alimentaire, 1))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">+%s %%</text>`,
			x(p.Alimentaire)+10, cy+4, Decimal(p.Alimentaire, 1))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
