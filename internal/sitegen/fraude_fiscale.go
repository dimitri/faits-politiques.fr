package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pointControlFiscal struct {
	Year               float64 // en Md€, converti depuis M€
	Notified, Cash     *float64
	NotifiedCalculated bool
}

type ControlFiscal struct {
	SVG                    template.HTML
	LastYear               int
	LastNotified, LastCash float64
	LastNotifiedCalculated bool
	GapAveragePct          float64
}

// loadControlFiscal : les résultats du contrôle fiscal, 2015-2024
// (core.controle_fiscal_resultats) — notifié (droits et pénalités mis en
// recouvrement) contre encaissé (effectivement recouvré). Le notifié 2022
// et 2023 est absent de la table (non retrouvé dans une source primaire) :
// la ligne notifiée est tracée en deux segments plutôt que de relier 2021
// à 2024 par une droite qui inventerait deux années de données.
func loadControlFiscal(ctx context.Context, pool *pgxpool.Pool) (*ControlFiscal, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, montant_notifie_m, notifie_calcule, montant_encaisse_m
		FROM core.controle_fiscal_resultats ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type line struct {
		year               int
		notified           *float64
		notifiedCalculated bool
		cash               float64
	}
	var lines []line
	var sumNotified, sumCash float64
	var nComparable int
	for rows.Next() {
		var l line
		var notifiedM *float64
		if err := rows.Scan(&l.year, &notifiedM, &l.notifiedCalculated, &l.cash); err != nil {
			return nil, err
		}
		if notifiedM != nil {
			v := *notifiedM / 1000
			l.notified = &v
			sumNotified += v
			sumCash += l.cash / 1000
			nComparable++
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}

	cf := &ControlFiscal{}
	last := lines[len(lines)-1]
	cf.LastYear = last.year
	cf.LastCash = last.cash / 1000
	if last.notified != nil {
		cf.LastNotified = *last.notified
		cf.LastNotifiedCalculated = last.notifiedCalculated
	}
	if nComparable > 0 && sumNotified > 0 {
		cf.GapAveragePct = (sumNotified - sumCash) / sumNotified * 100
	}

	var notifiedPts, cashPts []pointControlFiscal
	for _, l := range lines {
		e := l.cash / 1000
		cashPts = append(cashPts, pointControlFiscal{Year: float64(l.year), Cash: &e})
		if l.notified != nil {
			notifiedPts = append(notifiedPts, pointControlFiscal{Year: float64(l.year), Notified: l.notified, NotifiedCalculated: l.notifiedCalculated})
		}
	}
	cf.SVG = drawControlFiscal(notifiedPts, cashPts, lines[0].year, last.year)
	return cf, nil
}

func drawControlFiscal(notifiedPts, cashPts []pointControlFiscal, anStart, anEnd int) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 34.0, 62.0, 14.0, 26.0
	maxVal := 0.0
	for _, p := range cashPts {
		if p.Cash != nil && *p.Cash > maxVal {
			maxVal = *p.Cash
		}
	}
	for _, p := range notifiedPts {
		if p.Notified != nil && *p.Notified > maxVal {
			maxVal = *p.Notified
		}
	}
	maxVal *= 1.15
	x := func(year float64) float64 { return ml + (w-ml-mr)*(year-float64(anStart))/float64(anEnd-anStart) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxVal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe controle-fiscal" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Résultats du contrôle fiscal, notifié et encaissé, %d à %d">`, w, h, anStart, anEnd)
	for _, bracket := range []float64{0, 5, 10, 15} {
		if bracket > maxVal {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(bracket), w-mr, y(bracket))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d Md€</text>`, ml-6, y(bracket)+3, int(bracket))
	}

	// Le notifié se dessine en segments contigus séparés — jamais une droite
	// reliant deux années dont la valeur intermédiaire est inconnue.
	traceSegments(&b, notifiedPts, "notifie", func(p pointControlFiscal) *float64 { return p.Notified }, x, y)
	traceSegments(&b, cashPts, "encaisse", func(p pointControlFiscal) *float64 { return p.Cash }, x, y)

	if len(notifiedPts) > 0 {
		last := notifiedPts[len(notifiedPts)-1]
		cl := "pt-notifie"
		if last.NotifiedCalculated {
			cl += " calcule"
		}
		fmt.Fprintf(&b, `<circle class="%s" cx="%.1f" cy="%.1f" r="2.6"/>`, cl, x(last.Year), y(*last.Notified))
		fmt.Fprintf(&b, `<text class="lbl-notifie" x="%.1f" y="%.1f">Notifié</text>`, x(last.Year)+5, y(*last.Notified)+3)
	}
	if len(cashPts) > 0 {
		last := cashPts[len(cashPts)-1]
		fmt.Fprintf(&b, `<circle class="pt-encaisse" cx="%.1f" cy="%.1f" r="2.6"/>`, x(last.Year), y(*last.Cash))
		fmt.Fprintf(&b, `<text class="lbl-encaisse" x="%.1f" y="%.1f">Encaissé</text>`, x(last.Year)+5, y(*last.Cash)+3)
	}

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anStart)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anEnd)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// traceSegments : dessine une ligne par groupe d'années consécutives (pas
// de saut) — une année manquante (ex. 2022-2023 pour le notifié) coupe le
// tracé plutôt que d'être comblée par une interpolation visuelle.
func traceSegments(b *strings.Builder, pts []pointControlFiscal, class string,
	value func(pointControlFiscal) *float64, x func(float64) float64, y func(float64) float64) {
	var segment []pointControlFiscal
	flush := func() {
		if len(segment) < 2 {
			segment = nil
			return
		}
		var coords []string
		for _, p := range segment {
			coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Year), y(*value(p))))
		}
		fmt.Fprintf(b, `<polyline class="ligne-%s" points="%s"/>`, class, strings.Join(coords, " "))
		segment = nil
	}
	yearPrevious := 0.0
	for i, p := range pts {
		if i > 0 && p.Year-yearPrevious > 1 {
			flush()
		}
		segment = append(segment, p)
		yearPrevious = p.Year
	}
	flush()
}
