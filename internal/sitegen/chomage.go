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
type PointTrimestre struct {
	Trimestre string
	Annee     int
	Taux      float64
}

type StatsChomage struct {
	Serie              template.HTML
	Points             []PointTrimestre
	Debut, Fin         string
	TauxDebut, TauxFin float64
	TrimMax, TrimMin   string
	TauxMax, TauxMin   float64
	NbTrimestres       int
	// Comparaison : la moyenne annuelle de la série trimestrielle INSEE contre
	// la série annuelle qu'Eurostat republie (chomage.taux) — même mesure,
	// deux publications, pour rendre visible l'écart que la note du haut de
	// page ne fait qu'affirmer.
	BarresComparaison     template.HTML
	AnneeComparaisonDebut int
	AnneeComparaisonFin   int
	// Tranches : la répartition des allocataires de l'Assurance chômage par
	// montant d'indemnisation — la question « combien touchent-ils vraiment ? »
	// que la seule mesure du TAUX de chômage ne peut pas répondre.
	Tranches     template.HTML
	DateTranches string
}

func loadChomage(ctx context.Context, pool *pgxpool.Pool) (*StatsChomage, error) {
	rows, err := pool.Query(ctx, `
		SELECT trimestre, annee, taux FROM core.chomage_taux_trimestriel
		ORDER BY trimestre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	st := &StatsChomage{}
	for rows.Next() {
		var p PointTrimestre
		if err := rows.Scan(&p.Trimestre, &p.Annee, &p.Taux); err != nil {
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
	st.NbTrimestres = len(st.Points)
	st.Debut, st.TauxDebut = st.Points[0].Trimestre, st.Points[0].Taux
	dernier := st.Points[len(st.Points)-1]
	st.Fin, st.TauxFin = dernier.Trimestre, dernier.Taux
	st.TrimMax, st.TauxMax = st.Points[0].Trimestre, st.Points[0].Taux
	st.TrimMin, st.TauxMin = st.Points[0].Trimestre, st.Points[0].Taux
	for _, p := range st.Points {
		if p.Taux > st.TauxMax {
			st.TrimMax, st.TauxMax = p.Trimestre, p.Taux
		}
		if p.Taux < st.TauxMin {
			st.TrimMin, st.TauxMin = p.Trimestre, p.Taux
		}
	}
	st.Serie = courbeTrimestrielle(st.Points)

	crows, err := pool.Query(ctx, `
		SELECT t.annee, avg(t.taux)::float8, e.valeur::float8
		FROM core.chomage_taux_trimestriel t
		JOIN core.macro_value e ON e.annee=t.annee AND e.serie_code='chomage.taux'
		GROUP BY t.annee, e.valeur HAVING count(*)=4
		ORDER BY t.annee`)
	if err != nil {
		return nil, err
	}
	var comparaison []PaireAnnee
	for crows.Next() {
		var an int
		var moyINSEE, eurostat float64
		if err := crows.Scan(&an, &moyINSEE, &eurostat); err != nil {
			crows.Close()
			return nil, err
		}
		comparaison = append(comparaison, PaireAnnee{Annee: an, A: moyINSEE, B: eurostat})
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}
	if n := len(comparaison); n > 0 {
		st.AnneeComparaisonDebut, st.AnneeComparaisonFin = comparaison[0].Annee, comparaison[n-1].Annee
		st.BarresComparaison = barresAppariees(comparaison,
			"Moyenne annuelle de la série trimestrielle (INSEE)", "Série annuelle republiée (Eurostat)",
			func(v float64) string { return Decimal(v, 1) + " %" }, 5)
	}

	var dateTranches time.Time
	if err := pool.QueryRow(ctx,
		`SELECT max(date_reference) FROM core.chomage_tranche_unedic`).Scan(&dateTranches); err == nil && !dateTranches.IsZero() {
		trows, err := pool.Query(ctx, `
			SELECT tranche_min, tranche_max, pct::float8 FROM core.chomage_tranche_unedic
			WHERE date_reference=$1::date ORDER BY tranche_min`, dateTranches)
		if err != nil {
			return nil, err
		}
		var tranches []trancheUnedic
		for trows.Next() {
			var t trancheUnedic
			var tmax *int
			if err := trows.Scan(&t.Min, &tmax, &t.Pct); err != nil {
				trows.Close()
				return nil, err
			}
			t.Max = tmax
			tranches = append(tranches, t)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return nil, err
		}
		st.DateTranches = dateFr(dateTranches)
		st.Tranches = histogrammeTranches(tranches)
	}
	return st, nil
}

// courbeTrimestrielle : une barre par trimestre, ancrée à zéro comme les
// autres graphes du site — c'est un taux, une grandeur qui se lit en
// proportion, pas un écart où le zéro n'aurait pas de sens.
func courbeTrimestrielle(pts []PointTrimestre) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 210.0, 8.0, 8.0, 28.0, 26.0
	var max float64
	for _, p := range pts {
		if p.Taux > max {
			max = p.Taux
		}
	}
	if max <= 0 {
		return ""
	}
	n := float64(len(pts))
	pas := (w - ml - mr) / n
	gap := pas * 0.18
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
			pts[0].Trimestre, pts[len(pts)-1].Trimestre,
			Decimal(pts[0].Taux, 1)+" %", Decimal(pts[len(pts)-1].Taux, 1)+" %")))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, p := range pts {
		x := ml + pas*float64(i) + gap/2
		top := y(p.Taux)
		cl := "b"
		if i == len(pts)-1 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.2f" y="%.1f" width="%.2f" height="%.1f">`+
			`<title>%s — %s %%</title></rect>`,
			cl, x, top, pas-gap, (h-mb)-top, p.Trimestre, template.HTMLEscapeString(Decimal(p.Taux, 1)))
	}
	premier, dernier := pts[0], pts[len(pts)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s %%</text>`,
		ml, y(premier.Taux)-8, template.HTMLEscapeString(Decimal(premier.Taux, 1)))
	fmt.Fprintf(&b, `<text class="et fin" x="%.1f" y="%.1f">%s %%</text>`,
		w-mr, y(dernier.Taux)-8, template.HTMLEscapeString(Decimal(dernier.Taux, 1)))
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, premier.Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, dernier.Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type trancheUnedic struct {
	Min int
	Max *int
	Pct float64
}

func (t trancheUnedic) label() string {
	if t.Max == nil {
		return Nombre(t.Min) + " € et plus"
	}
	return Nombre(t.Min) + "–" + Nombre(*t.Max) + " €"
}

// histogrammeTranches : une distribution, pas quinze catégories — un seul
// accent, jamais une rampe. L'ordre est celui des tranches elles-mêmes, pas
// celui des valeurs : la question posée est « comment c'est réparti », pas
// « quelle tranche est la plus grosse ».
func histogrammeTranches(tranches []trancheUnedic) template.HTML {
	if len(tranches) == 0 {
		return ""
	}
	max := 0.0
	for _, t := range tranches {
		if t.Pct > max {
			max = t.Pct
		}
	}
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="barres">`)
	for _, t := range tranches {
		fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s %%</span></div>`,
			template.HTMLEscapeString(t.label()), 100*t.Pct/max, Decimal(t.Pct, 1))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}
