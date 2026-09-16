package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le § 1 de docs/pauvrete-donnees.md décrit les seuils à 50 % et 60 % de la
// médiane, et les compare aux déciles du niveau de vie — tout en abstrait,
// en euros cités dans le texte. Ce fichier construit le même repère en
// graphique, à partir des mêmes tables (core.filosofi_decile_national,
// core.pauvrete_seuil_annuel), inséré dans le document par le marqueur
// <!-- schema:seuils-pauvrete --> (cmd/build/main.go).
type SeuilsPauvrete struct {
	Annee           int
	Seuil50, Seuil60 float64
	SVG             template.HTML
}

type pointDecile struct {
	Decile int
	Valeur float64
}

func chargerSeuilsPauvrete(ctx context.Context, pool *pgxpool.Pool) (*SeuilsPauvrete, error) {
	var annee int
	if err := pool.QueryRow(ctx,
		`SELECT max(annee) FROM core.filosofi_decile_national`).Scan(&annee); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT decile, niveau_vie_mensuel FROM core.filosofi_decile_national
		WHERE annee = $1 ORDER BY decile`, annee)
	if err != nil {
		return nil, err
	}
	var deciles []pointDecile
	for rows.Next() {
		var p pointDecile
		if err := rows.Scan(&p.Decile, &p.Valeur); err != nil {
			rows.Close()
			return nil, err
		}
		deciles = append(deciles, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(deciles) == 0 {
		return nil, nil
	}

	st := &SeuilsPauvrete{Annee: annee}
	if err := pool.QueryRow(ctx, `
		SELECT max(seuil_euros) FILTER (WHERE seuil_relatif = 0.5),
		       max(seuil_euros) FILTER (WHERE seuil_relatif = 0.6)
		FROM core.pauvrete_seuil_annuel WHERE annee = $1`, annee).
		Scan(&st.Seuil50, &st.Seuil60); err != nil {
		return nil, err
	}

	format := func(v float64) string { return Decimal(v, 0) + " €" }
	st.SVG = dessinerSeuilsPauvrete(deciles, st.Seuil50, st.Seuil60, format)
	return st, nil
}

// dessinerSeuilsPauvrete : neuf barres (les plafonds de chaque décile de
// niveau de vie, D1 à D9, sur une échelle qui part de zéro — jamais tronquée,
// comme partout ailleurs sur ce site) et deux lignes pointillées, aux seuils
// à 50 % et 60 % de la médiane. Les deux sont dans la même unité (€/mois) :
// pas besoin d'une seconde échelle, contrairement à courbeAvecLigne.
func dessinerSeuilsPauvrete(deciles []pointDecile, seuil50, seuil60 float64, format func(float64) string) template.HTML {
	if len(deciles) == 0 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 20.0, 30.0
	max := seuil60
	for _, p := range deciles {
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	max *= 1.08 // un peu d'air au-dessus de la barre la plus haute
	n := float64(len(deciles))
	pas := (w - ml - mr) / n
	gap := pas * 0.16
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an seuils-pauvrete" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="%s">`, w, h, template.HTMLEscapeString(fmt.Sprintf(
		"Déciles de niveau de vie et seuils de pauvreté : le premier décile plafonne à %s, "+
			"le seuil à 60%% de la médiane est à %s", format(deciles[0].Valeur), format(seuil60))))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)

	for i, p := range deciles {
		x := ml + pas*float64(i) + gap/2
		top := y(p.Valeur)
		// D1 seul plafonne sous le seuil à 60 % : la teinte le distingue des
		// huit autres barres, sans qu'aucun texte n'ait besoin de le répéter.
		cl := "b"
		if p.Valeur < seuil60 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>D%d — plafond à %s</title></rect>`,
			cl, x, top, pas-gap, (h-mb)-top, p.Decile, template.HTMLEscapeString(format(p.Valeur)))
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">D%d</text>`,
			x+(pas-gap)/2, h-8, p.Decile)
	}
	// Valeur explicite sur D1 et D9 seulement (comme courbe()) : une valeur
	// par barre transformerait le graphique en tableau mal rangé.
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
		ml+pas*0.5, y(deciles[0].Valeur)-8, template.HTMLEscapeString(format(deciles[0].Valeur)))
	dernier := deciles[len(deciles)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
		ml+pas*(n-0.5), y(dernier.Valeur)-8, template.HTMLEscapeString(format(dernier.Valeur)))

	ligneSeuil := func(seuil float64, pct string, decale float64) {
		yy := y(seuil)
		fmt.Fprintf(&b, `<line class="ligne-seuil" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			ml, yy, w-mr, yy)
		fmt.Fprintf(&b, `<text class="et-seuil" x="%.1f" y="%.1f">Seuil à %s de la médiane — %s</text>`,
			ml+4, yy+decale, pct, template.HTMLEscapeString(format(seuil)))
	}
	ligneSeuil(seuil60, "60 %", -5)
	ligneSeuil(seuil50, "50 %", 13)

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
