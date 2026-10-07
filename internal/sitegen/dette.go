package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La dette publique : la série, et surtout ce qu'elle n'est pas.
//
// Deux confusions à désamorcer avant tout chiffre. La DETTE est un stock, le
// DÉFICIT un flux : la première est ce qui reste dû, le second ce qui manque
// cette année. Et « la dette de la France » n'est pas la dette de l'État : elle
// additionne l'État, les collectivités et la Sécurité sociale, qui empruntent
// séparément.
//
// « La dette par domaine » n'existe pas et ne peut pas être calculée : un
// emprunt n'est pas fléché vers une dépense. Ce que la comptabilité publie, et
// que cette page montre à la place, c'est la DÉPENSE par domaine — la
// nomenclature COFOG, dix fonctions, publiée par Eurostat.
type PointDebt struct {
	Year        int
	Meur, Gdp   float64
	Bar, Height float64
	X, Width    float64
}

type FunctionExpense struct {
	Code, Label string
	Amount      float64
	Share       float64
	ShareStart  float64
}

type StatsDebt struct {
	Start, End      int
	LastMeur        float64
	LastGdp         float64
	FirstMeur       float64
	FirstGdp        float64
	Points          []PointDebt
	Grid            template.HTML
	LineGdp         template.HTML
	Functions       []FunctionExpense
	YearFunctions   int
	StartFunctions  int
	BarsFunctions   template.HTML
	Sectors         []SubSector
	YearSectors     int
	BalanceS13      float64
	DebtService     float64
	YearDebtService int
}

// cofog : la nomenclature internationale des fonctions des administrations
// publiques. Les libellés sont ceux d'Eurostat, sans reformulation.
var cofog = []struct{ code, short string }{
	{"depense.GF10", "Protection sociale"},
	{"depense.GF07", "Santé"},
	{"depense.GF09", "Enseignement"},
	{"depense.GF01", "Services généraux"},
	{"depense.GF04", "Affaires économiques"},
	{"depense.GF03", "Ordre et sécurité"},
	{"depense.GF02", "Défense"},
	{"depense.GF06", "Logement et équipements"},
	{"depense.GF08", "Loisirs, culture, culte"},
	{"depense.GF05", "Protection de l'environnement"},
}

func loadDebt(ctx context.Context, pool *pgxpool.Pool) (*StatsDebt, error) {
	st := &StatsDebt{}
	rows, err := pool.Query(ctx, `
		SELECT m.annee, m.valeur::float8, p.valeur::float8
		FROM core.macro_value m
		LEFT JOIN core.macro_value p ON p.annee=m.annee AND p.serie_code='dette.publique.pib'
		WHERE m.serie_code='dette.publique.meur' ORDER BY m.annee`)
	if err != nil {
		return nil, err
	}
	var max float64
	for rows.Next() {
		var pt PointDebt
		var gdp *float64
		if err := rows.Scan(&pt.Year, &pt.Meur, &gdp); err != nil {
			break
		}
		pt.Meur *= 1e6
		if gdp != nil {
			pt.Gdp = *gdp
		}
		if pt.Meur > max {
			max = pt.Meur
		}
		st.Points = append(st.Points, pt)
	}
	rows.Close()
	if len(st.Points) == 0 {
		return st, nil
	}
	st.Start, st.End = st.Points[0].Year, st.Points[len(st.Points)-1].Year
	st.FirstMeur, st.FirstGdp = st.Points[0].Meur, st.Points[0].Gdp
	st.LastMeur = st.Points[len(st.Points)-1].Meur
	st.LastGdp = st.Points[len(st.Points)-1].Gdp

	// Barres : un stock annuel est une mesure au 31 décembre, pas un continuum.
	// gl : la place des étiquettes de l'axe. « 3 460,5 Md€ » fait onze signes ;
	// à 66 px elle sortait du viewBox et se lisait « 460,5 Md€ ».
	const gw, gh, gl, gt, gb = 720.0, 250.0, 100.0, 16.0, 30.0
	n := float64(len(st.Points))
	step := (gw - gl - 8) / n
	var g strings.Builder
	for _, frac := range []float64{0, 0.5, 1} {
		v := max * frac
		y := gt + (gh-gt-gb)*(1-frac)
		fmt.Fprintf(&g, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			gl, y, gw-8, y)
		fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			gl-8, y+3, mdEur(v))
	}
	for i := range st.Points {
		pt := &st.Points[i]
		pt.X = gl + step*float64(i) + 1
		pt.Width = step - 2
		h := (gh - gt - gb) * pt.Meur / max
		pt.Height = h
		pt.Bar = gt + (gh - gt - gb) - h
		if pt.Year%5 == 0 {
			fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d</text>`,
				pt.X+pt.Width/2, gh-10, pt.Year)
		}
	}
	st.Grid = template.HTML(g.String())

	// Le ratio au PIB, en ligne : jusqu'ici cité seulement dans l'infobulle de
	// chaque barre, alors que la note qui suit affirme que c'est LUI la lecture
	// insensible à l'inflation. Échelle propre, non zéro-basée, comme la
	// population sur la page protection sociale — ce n'est pas la même unité
	// que les barres, donc pas le même repère.
	minP, maxP := st.Points[0].Gdp, st.Points[0].Gdp
	for _, pt := range st.Points {
		if pt.Gdp < minP {
			minP = pt.Gdp
		}
		if pt.Gdp > maxP {
			maxP = pt.Gdp
		}
	}
	if maxP == minP {
		maxP = minP + 1
	}
	bottomP := minP - (maxP-minP)*0.15
	topP := maxP + (maxP-minP)*0.15
	yP := func(v float64) float64 { return gt + (gh-gt-gb)*(1-(v-bottomP)/(topP-bottomP)) }
	var trace strings.Builder
	for i, pt := range st.Points {
		op := "L"
		if i == 0 {
			op = "M"
		}
		fmt.Fprintf(&trace, "%s%.1f,%.1f", op, pt.X+pt.Width/2, yP(pt.Gdp))
	}
	var lp strings.Builder
	fmt.Fprintf(&lp, `<path class="ligne-pib" d="%s" fill="none"/>`, trace.String())
	first, last := st.Points[0], st.Points[len(st.Points)-1]
	fmt.Fprintf(&lp, `<circle class="pt-pib" cx="%.1f" cy="%.1f" r="2.6"/>`,
		first.X+first.Width/2, yP(first.Gdp))
	fmt.Fprintf(&lp, `<circle class="pt-pib" cx="%.1f" cy="%.1f" r="2.6"/>`,
		last.X+last.Width/2, yP(last.Gdp))
	fmt.Fprintf(&lp, `<text class="et pib" x="%.1f" y="%.1f">%s %% du PIB</text>`,
		first.X+first.Width/2, yP(first.Gdp)-8, Decimal(first.Gdp, 1))
	fmt.Fprintf(&lp, `<text class="et pib pib-fin" x="%.1f" y="%.1f" text-anchor="end">%s %% du PIB</text>`,
		last.X+last.Width/2, yP(last.Gdp)-8, Decimal(last.Gdp, 1))
	st.LineGdp = template.HTML(lp.String())

	// La dépense par fonction : ce qui remplace la « dette par domaine ».
	_ = pool.QueryRow(ctx, `
		SELECT max(annee), min(annee) FROM core.macro_value WHERE serie_code='depense.GF10'`).
		Scan(&st.YearFunctions, &st.StartFunctions)
	var totalEnd, totalDeb float64
	for _, f := range cofog {
		var a, b *float64
		_ = pool.QueryRow(ctx, `
			SELECT max(valeur) FILTER (WHERE annee=$2)::float8,
			       max(valeur) FILTER (WHERE annee=$3)::float8
			FROM core.macro_value WHERE serie_code=$1`,
			f.code, st.YearFunctions, st.StartFunctions).Scan(&a, &b)
		if a != nil {
			totalEnd += *a
		}
		if b != nil {
			totalDeb += *b
		}
	}
	for _, f := range cofog {
		var a, b *float64
		var lib string
		_ = pool.QueryRow(ctx, `SELECT label FROM ref.macro_serie WHERE code=$1`, f.code).Scan(&lib)
		_ = pool.QueryRow(ctx, `
			SELECT max(valeur) FILTER (WHERE annee=$2)::float8,
			       max(valeur) FILTER (WHERE annee=$3)::float8
			FROM core.macro_value WHERE serie_code=$1`,
			f.code, st.YearFunctions, st.StartFunctions).Scan(&a, &b)
		fd := FunctionExpense{Code: f.code, Label: f.short}
		if a != nil {
			fd.Amount = *a * 1e6
			if totalEnd > 0 {
				fd.Share = 100 * *a / totalEnd
			}
		}
		if b != nil && totalDeb > 0 {
			fd.ShareStart = 100 * *b / totalDeb
		}
		st.Functions = append(st.Functions, fd)
	}
	sort.Slice(st.Functions, func(i, j int) bool {
		return st.Functions[i].Amount > st.Functions[j].Amount
	})
	var maxF float64
	for _, f := range st.Functions {
		if f.Amount > maxF {
			maxF = f.Amount
		}
	}
	var bf strings.Builder
	bf.WriteString(`<div class="barres">`)
	for _, f := range st.Functions {
		fmt.Fprintf(&bf, `<div class="ligne"><span class="n">%s</span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s</span><span class="c">%s</span></div>`,
			template.HTMLEscapeString(f.Label), 100*f.Amount/maxF,
			mdEur(f.Amount), Decimal(f.Share, 1)+" %")
	}
	bf.WriteString(`</div>`)
	st.BarsFunctions = template.HTML(bf.String())

	// Qui emprunte : le solde des trois sous-secteurs.
	srows, err := pool.Query(ctx, `
		SELECT annee, secteur, perimetre_label, depenses_meur::float8*1e6,
		       recettes_meur::float8*1e6, solde_meur::float8*1e6
		FROM derived.budget_sous_secteur
		WHERE annee=(SELECT max(annee) FROM derived.budget_sous_secteur)
		ORDER BY solde_meur`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var s SubSector
		if err := srows.Scan(&st.YearSectors, &s.Code, &s.Label, &s.Expenses,
			&s.Revenues, &s.Balance); err != nil {
			break
		}
		if s.Code == "S13" {
			st.BalanceS13 = s.Balance
			continue
		}
		st.Sectors = append(st.Sectors, s)
	}
	srows.Close()

	// La charge de la dette : le seul montant que l'État paie POUR la dette, et
	// il vient de la situation mensuelle, donc en cumul.
	_ = pool.QueryRow(ctx, `
		SELECT montant_eur, exercice FROM core.execution_etat
		WHERE ligne='Charges de la dette de l’Etat'
		ORDER BY date_arrete DESC LIMIT 1`).Scan(&st.DebtService, &st.YearDebtService)
	return st, nil
}
