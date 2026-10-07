package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le taux de chômage au sens du BIT, trimestre par trimestre, depuis l'INSEE
// directement (core.chomage_taux_trimestriel) — pas la republication annuelle
// d'Eurostat qu'utilise la frise (chomage.taux dans core.macro_value). C'est
// la mesure que les gouvernements et les médias citent à chaque publication
// trimestrielle, et la page que /frise/ renvoie pour le détail.
type PointQuarter struct {
	Quarter string
	Year    int
	Rate    float64
}

type StatsUnemployment struct {
	Series             template.HTML
	Points             []PointQuarter
	Start, End         string
	RateStart, RateEnd float64
	TrimMax, TrimMin   string
	RateMax, RateMin   float64
	CountQuarters      int
	// Comparaison : la moyenne annuelle de la série trimestrielle INSEE contre
	// la série annuelle qu'Eurostat republie (chomage.taux) — même mesure,
	// deux publications, pour rendre visible l'écart que la note du haut de
	// page ne fait qu'affirmer.
	BarsComparison      template.HTML
	YearComparisonStart int
	YearComparisonEnd   int
	// Tranches : la répartition des allocataires de l'Assurance chômage par
	// montant d'indemnisation — la question « combien touchent-ils vraiment ? »
	// que la seule mesure du TAUX de chômage ne peut pas répondre.
	Brackets     template.HTML
	DateBrackets string
}

func loadUnemployment(ctx context.Context, pool *pgxpool.Pool) (*StatsUnemployment, error) {
	rows, err := pool.Query(ctx, `
		SELECT trimestre, annee, taux FROM core.chomage_taux_trimestriel
		ORDER BY trimestre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	st := &StatsUnemployment{}
	for rows.Next() {
		var p PointQuarter
		if err := rows.Scan(&p.Quarter, &p.Year, &p.Rate); err != nil {
			return nil, err
		}
		st.Points = append(st.Points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(st.Points) == 0 {
		return st, nil
	}
	st.CountQuarters = len(st.Points)
	st.Start, st.RateStart = st.Points[0].Quarter, st.Points[0].Rate
	last := st.Points[len(st.Points)-1]
	st.End, st.RateEnd = last.Quarter, last.Rate
	st.TrimMax, st.RateMax = st.Points[0].Quarter, st.Points[0].Rate
	st.TrimMin, st.RateMin = st.Points[0].Quarter, st.Points[0].Rate
	for _, p := range st.Points {
		if p.Rate > st.RateMax {
			st.TrimMax, st.RateMax = p.Quarter, p.Rate
		}
		if p.Rate < st.RateMin {
			st.TrimMin, st.RateMin = p.Quarter, p.Rate
		}
	}
	st.Series = curveQuarterly(st.Points)

	crows, err := pool.Query(ctx, `
		SELECT t.annee, avg(t.taux)::float8, e.valeur::float8
		FROM core.chomage_taux_trimestriel t
		JOIN core.macro_value e ON e.annee=t.annee AND e.serie_code='chomage.taux'
		GROUP BY t.annee, e.valeur HAVING count(*)=4
		ORDER BY t.annee`)
	if err != nil {
		return nil, err
	}
	var comparison []PairYear
	for crows.Next() {
		var an int
		var moyINSEE, eurostat float64
		if err := crows.Scan(&an, &moyINSEE, &eurostat); err != nil {
			crows.Close()
			return nil, err
		}
		comparison = append(comparison, PairYear{Year: an, A: moyINSEE, B: eurostat})
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}
	if n := len(comparison); n > 0 {
		st.YearComparisonStart, st.YearComparisonEnd = comparison[0].Year, comparison[n-1].Year
		st.BarsComparison = barsMatched(comparison,
			"Moyenne annuelle de la série trimestrielle (INSEE)", "Série annuelle republiée (Eurostat)",
			func(v float64) string { return Decimal(v, 1) + " %" }, 5)
	}

	var dateBrackets time.Time
	if err := pool.QueryRow(ctx,
		`SELECT max(date_reference) FROM core.chomage_tranche_unedic`).Scan(&dateBrackets); err == nil && !dateBrackets.IsZero() {
		trows, err := pool.Query(ctx, `
			SELECT tranche_min, tranche_max, pct::float8 FROM core.chomage_tranche_unedic
			WHERE date_reference=$1::date ORDER BY tranche_min`, dateBrackets)
		if err != nil {
			return nil, err
		}
		var brackets []bracketUnedic
		for trows.Next() {
			var t bracketUnedic
			var tmax *int
			if err := trows.Scan(&t.Min, &tmax, &t.Pct); err != nil {
				trows.Close()
				return nil, err
			}
			t.Max = tmax
			brackets = append(brackets, t)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return nil, err
		}
		st.DateBrackets = dateFr(dateBrackets)
		st.Brackets = histogramBrackets(brackets)
	}
	return st, nil
}

// curveQuarterly : une barre par trimestre, ancrée à zéro comme les
// autres graphes du site — c'est un taux, une grandeur qui se lit en
// proportion, pas un écart où le zéro n'aurait pas de sens.
func curveQuarterly(pts []PointQuarter) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 210.0, 8.0, 8.0, 28.0, 26.0
	var max float64
	for _, p := range pts {
		if p.Rate > max {
			max = p.Rate
		}
	}
	if max <= 0 {
		return ""
	}
	n := float64(len(pts))
	step := (w - ml - mr) / n
	gap := step * 0.18
	if gap > 2 {
		gap = 2
	}
	if gap < 0.3 {
		gap = 0.3
	}
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("De %s à %s : %s puis %s",
			pts[0].Quarter, pts[len(pts)-1].Quarter,
			Decimal(pts[0].Rate, 1)+" %", Decimal(pts[len(pts)-1].Rate, 1)+" %")))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, p := range pts {
		x := ml + step*float64(i) + gap/2
		top := y(p.Rate)
		cl := "b"
		if i == len(pts)-1 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.2f" y="%.1f" width="%.2f" height="%.1f">`+
			`<title>%s — %s %%</title></rect>`,
			cl, x, top, step-gap, (h-mb)-top, p.Quarter, template.HTMLEscapeString(Decimal(p.Rate, 1)))
	}
	first, last := pts[0], pts[len(pts)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s %%</text>`,
		ml, y(first.Rate)-8, template.HTMLEscapeString(Decimal(first.Rate, 1)))
	fmt.Fprintf(&b, `<text class="et fin" x="%.1f" y="%.1f">%s %%</text>`,
		w-mr, y(last.Rate)-8, template.HTMLEscapeString(Decimal(last.Rate, 1)))
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, first.Year)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, last.Year)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type bracketUnedic struct {
	Min int
	Max *int
	Pct float64
}

func (t bracketUnedic) label() string {
	if t.Max == nil {
		return Count(t.Min) + " € et plus"
	}
	return Count(t.Min) + "–" + Count(*t.Max) + " €"
}

// histogramBrackets : une distribution, pas quinze catégories — un seul
// accent, jamais une rampe. L'ordre est celui des tranches elles-mêmes, pas
// celui des valeurs : la question posée est « comment c'est réparti », pas
// « quelle tranche est la plus grosse ».
func histogramBrackets(brackets []bracketUnedic) template.HTML {
	if len(brackets) == 0 {
		return ""
	}
	max := 0.0
	for _, t := range brackets {
		if t.Pct > max {
			max = t.Pct
		}
	}
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="barres">`)
	for _, t := range brackets {
		fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s %%</span></div>`,
			template.HTMLEscapeString(t.label()), 100*t.Pct/max, Decimal(t.Pct, 1))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}
