package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le budget de l'État et celui de la Sécurité sociale, à partir des deux seules
// séries que ce site ait ingérées en propre :
//
//	DGFiP   situations mensuelles du budget de l'État — CUMUL depuis janvier
//	DREES   comptes de la protection sociale — prestations, 1959→2024
//
// Deux avertissements structurent ces pages, parce qu'ils sont la première
// source d'erreur de lecture :
//
//  1. une situation mensuelle est un CUMUL depuis le 1er janvier, pas un flux
//     du mois ni un total d'année. « 145,9 Md€ de déficit en juillet 2026 »
//     veut dire « de janvier à juillet », et se compare à juillet 2025, jamais
//     à l'année 2025 entière ;
//  2. une situation mensuelle n'est pas la loi de règlement. Elle est
//     provisoire, en comptabilité budgétaire (encaissements-décaissements),
//     et le chiffre définitif de l'exercice sera différent.
type LineBudget struct {
	Level                       int
	Category, SubCategory, Line string
	Amount, Previous            float64
	Gap                         float64
	WithPrevious                bool
}

type CurveFiscalYear struct {
	FiscalYear int
	Path       template.HTML
	Last       string
	MonthEnd   int
	X, Y       float64
	Incomplete bool
}

type StatsBudget struct {
	Arrete, MonthName           string
	Month, FiscalYear           int
	Revenues, Expenses, Balance float64
	Lines                       []LineBudget
	Curves                      []CurveFiscalYear
	Grid                        template.HTML
	FiscalYears                 []int
	// RecettesDetail / DepensesDetail : les lignes NOMMÉES de la situation
	// mensuelle (TVA, IR, IS, TICPE… ; personnel, intervention, dette…), en
	// dehors du tableau hiérarchique complet — la réponse à « où passe l'argent »
	// sans avoir à dérouler les 26 lignes de la source.
	RevenuesDetail []SubSector
	ExpensesDetail []SubSector
	BarsRevenues   template.HTML
	BarsExpenses   template.HTML
}

var monthFr = [...]string{"", "janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre"}

// mdEur : un montant en euros, écrit en milliards. Les budgets se lisent en
// milliards ; les afficher à l'euro près donnerait une précision que la
// situation mensuelle, provisoire, n'a pas.
func mdEur(v float64) string { return Decimal(v/1e9, 1) + "\u202fMd€" }

func loadBudget(ctx context.Context, pool *pgxpool.Pool) (*StatsBudget, error) {
	st := &StatsBudget{}
	// max(...) est une agrégation : la ligne existe même sans exécution
	// budgétaire encore ingérée, avec des valeurs NULL.
	var arreteN sql.NullString
	var monthN, fiscalYearN sql.NullInt64
	err := pool.QueryRow(ctx, `
		SELECT to_char(max(date_arrete),'YYYY-MM-DD'),
		       extract(month FROM max(date_arrete))::int,
		       max(exercice)::int
		FROM core.execution_etat`).Scan(&arreteN, &monthN, &fiscalYearN)
	if err != nil || !arreteN.Valid {
		return nil, err
	}
	arrete := arreteN.String
	st.Month, st.FiscalYear = int(monthN.Int64), int(fiscalYearN.Int64)
	st.MonthName = monthFr[st.Month]
	st.Arrete = st.MonthName + " " + fmt.Sprint(st.FiscalYear)

	// Les trois nombres de tête. Ils viennent de lignes nommées, jamais d'une
	// somme faite ici : additionner des lignes hiérarchisées double-compterait.
	head := func(cat, line string) float64 {
		var v *float64
		_ = pool.QueryRow(ctx, `
			SELECT montant_eur FROM core.execution_etat
			WHERE date_arrete=$1::date AND categorie=$2 AND ligne=$3`,
			arrete, cat, line).Scan(&v)
		if v == nil {
			return 0
		}
		return *v
	}
	st.Revenues = head("Recettes", "Total recettes nettes du budget général")
	st.Expenses = head("Dépenses", "Total dépenses nettes du budget général")
	st.Balance = head("Solde budgétaire", "Solde budgétaire")

	// Le même mois de l'exercice précédent : c'est la seule comparaison qui ait
	// un sens sur un cumul. Comparer juillet à l'année pleine d'avant serait
	// comparer sept mois à douze.
	rows, err := pool.Query(ctx, `
		SELECT e.niveau, e.categorie, e.sous_categorie, e.ligne, e.montant_eur, p.montant_eur
		FROM core.execution_etat e
		LEFT JOIN core.execution_etat p
		  ON p.categorie=e.categorie AND p.sous_categorie=e.sous_categorie
		 AND p.ligne=e.ligne
		 AND p.date_arrete = (e.date_arrete - interval '1 year')::date
		WHERE e.date_arrete=$1::date
		ORDER BY e.categorie, e.sous_categorie, e.niveau, e.ligne`, arrete)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l LineBudget
		var m, p *float64
		if err := rows.Scan(&l.Level, &l.Category, &l.SubCategory, &l.Line, &m, &p); err != nil {
			rows.Close()
			return nil, err
		}
		if m != nil {
			l.Amount = *m
		}
		if p != nil && *p != 0 {
			l.Previous, l.WithPrevious = *p, true
			l.Gap = 100 * (l.Amount - *p) / abs(*p)
		}
		st.Lines = append(st.Lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Le solde cumulé, mois par mois. En BARRES : un cumul mensuel est une
	// série de douze mesures distinctes, pas un phénomène continu — une courbe
	// suggère qu'on peut lire une valeur « à la mi-mars », ce que la source ne
	// publie pas. Trois barres par mois, une par exercice, même axe, origine à
	// zéro puisqu'il s'agit d'un solde.
	srows, err := pool.Query(ctx, `
		SELECT exercice::int, extract(month FROM date_arrete)::int, montant_eur
		FROM core.execution_etat
		WHERE categorie='Solde budgétaire' AND ligne='Solde budgétaire'
		ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	per := map[int]map[int]float64{}
	var bottom, top float64
	for srows.Next() {
		var e, m int
		var v *float64
		if err := srows.Scan(&e, &m, &v); err != nil {
			break
		}
		if v == nil {
			continue
		}
		if per[e] == nil {
			per[e] = map[int]float64{}
		}
		per[e][m] = *v
		if *v < bottom {
			bottom = *v
		}
		if *v > top {
			top = *v
		}
	}
	srows.Close()
	for e := range per {
		st.FiscalYears = append(st.FiscalYears, e)
	}
	sort.Ints(st.FiscalYears)

	const gw, gh, gl, gt, gb, gr = 720.0, 260.0, 96.0, 16.0, 30.0, 10.0
	if top < 0 {
		top = 0
	}
	if bottom == top {
		bottom = top - 1
	}
	y := func(v float64) float64 { return gt + (gh-gt-gb)*(top-v)/(top-bottom) }
	step := (gw - gl - gr) / 12
	n := len(st.FiscalYears)
	if n == 0 {
		n = 1
	}
	larg := (step - 6) / float64(n)

	var g strings.Builder
	// Zéro n'est pas une graduation comme les autres sur un solde : c'est la
	// frontière entre déficit et excédent, donc un trait plein.
	fmt.Fprintf(&g, `<line class="zero" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		gl-4, y(0), gw-gr, y(0))
	for _, v := range []float64{bottom, bottom / 2} {
		fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			gl-8, y(v)+3, mdEur(v))
	}
	fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">0</text>`, gl-8, y(0)+3)
	month := []string{"", "J", "F", "M", "A", "M", "J", "J", "A", "S", "O", "N", "D"}
	for m := 1; m <= 12; m++ {
		fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			gl+step*(float64(m)-0.5), gh-10, month[m])
	}
	st.Grid = template.HTML(g.String())

	for i, e := range st.FiscalYears {
		var b strings.Builder
		c := CurveFiscalYear{FiscalYear: e}
		for m := 1; m <= 12; m++ {
			v, ok := per[e][m]
			if !ok {
				continue
			}
			x := gl + step*float64(m-1) + 3 + larg*float64(i)
			y0, y1 := y(0), y(v)
			if y1 < y0 {
				y0, y1 = y1, y0
			}
			h := y1 - y0
			if h < 1 {
				h = 1
			}
			fmt.Fprintf(&b, `<rect class="b e%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
				`<title>%d, cumul à fin %s : %s</title></rect>`,
				i, x, y0, larg-1, h, e, monthFr[m], mdEur(v))
			c.Last, c.MonthEnd = mdEur(v), m
		}
		c.Incomplete = c.MonthEnd < 12
		c.Path = template.HTML(b.String())
		st.Curves = append(st.Curves, c)
	}

	// Les lignes nommées de la situation mensuelle, niveau par niveau : les
	// quatre grandes recettes fiscales, puis les natures de dépense. Ce sont
	// des lignes RÉELLES de la source (categorie/niveau), pas une somme faite
	// ici — une case de la même table, jamais recalculée.
	detail := func(category string, level int) ([]SubSector, float64) {
		drows, err := pool.Query(ctx, `
			SELECT ligne, montant_eur FROM core.execution_etat
			WHERE date_arrete=$1::date AND categorie=$2 AND niveau=$3
			ORDER BY montant_eur DESC`, arrete, category, level)
		if err != nil {
			return nil, 0
		}
		defer drows.Close()
		var out []SubSector
		var total float64
		for drows.Next() {
			var s SubSector
			if err := drows.Scan(&s.Label, &s.Expenses); err != nil {
				break
			}
			out = append(out, s)
			total += s.Expenses
		}
		for i := range out {
			if total > 0 {
				out[i].ShareExpenses = 100 * out[i].Expenses / total
			}
		}
		return out, total
	}
	st.RevenuesDetail, _ = detail("Recettes", 3)
	st.ExpensesDetail, _ = detail("Dépenses", 2)
	st.BarsRevenues = barsSectors(st.RevenuesDetail)
	st.BarsExpenses = barsSectors(st.ExpensesDetail)
	return st, nil
}

// pctFr : un pourcentage signé, virgule décimale. « +4.6 % » est un anglicisme
// typographique dans une page française.
func pctFr(v float64) string {
	signe := "+"
	if v < 0 {
		signe = ""
	}
	return signe + Decimal(v, 1) + "\u202f%"
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ── Protection sociale ────────────────────────────────────────────────

type RiskSocial struct {
	Code, Label string
	Amount      float64
	Share       float64
}

type StatsSocial struct {
	Year, Start               int
	Total                     float64
	Risks                     []RiskSocial
	Series                    []PointYear
	Curve                     template.HTML
	SubRisks                  []RiskSocial
	CountRegimes              int
	ShareSocialSecurityStrict float64
	Break                     int
	Population                []PointYear
	CurveWithPop              template.HTML
	Donut                     template.HTML
	// Part65, Part20 : la structure par âge que la courbe de population totale
	// ne montre pas (voir la note abs de la page) — deux parts de la MÊME
	// population, sur le MÊME axe 0-100, donc comparables directement.
	Share65, Share20 []PointYear
	CurveAges        template.HTML
	YearIntersection int
}

func loadSocial(ctx context.Context, pool *pgxpool.Pool) (*StatsSocial, error) {
	st := &StatsSocial{}
	// Debut est celui de la SÉRIE affichée, pas de la table : le compte remonte
	// à 1959 pour certains postes, mais le total tous régimes ne commence qu'en
	// 1981. Titrer « depuis 1959 » sur une courbe qui part de 1981 serait faux.
	// max(...) est une agrégation : la ligne existe même sans protection
	// sociale encore ingérée, avec une année NULL.
	var yearN sql.NullInt64
	err := pool.QueryRow(ctx, `
		SELECT max(annee)::int, count(DISTINCT regime) FROM core.protection_sociale`).
		Scan(&yearN, &st.CountRegimes)
	if err != nil {
		return nil, err
	}
	st.Year = int(yearN.Int64)

	// « Tous régimes » et le niveau 1 : six risques qui se somment exactement au
	// total. Descendre plus bas ou mélanger les niveaux double-compterait.
	const filter = `si_code='S1' AND regime='Total tous régimes'`
	_ = pool.QueryRow(ctx, `SELECT valeur_meur*1e6 FROM core.protection_sociale
		WHERE annee=$1 AND ps_niveau=0 AND `+filter, st.Year).Scan(&st.Total)

	for _, niv := range []int{1, 2} {
		rows, err := pool.Query(ctx, `
			SELECT ps_code, ps_libelle, valeur_meur*1e6 FROM core.protection_sociale
			WHERE annee=$1 AND ps_niveau=$2 AND `+filter+` ORDER BY 3 DESC`, st.Year, niv)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r RiskSocial
			if err := rows.Scan(&r.Code, &r.Label, &r.Amount); err != nil {
				break
			}
			if st.Total > 0 {
				r.Share = 100 * r.Amount / st.Total
			}
			if niv == 1 {
				st.Risks = append(st.Risks, r)
			} else {
				st.SubRisks = append(st.SubRisks, r)
			}
		}
		rows.Close()
	}
	st.Donut = donutRisks(st.Risks, st.Total, mdEur(st.Total))

	// La série longue vient de la vue derived.protection_sociale_total, somme
	// des six risques : elle rejoint EXACTEMENT le total « tous régimes » à
	// partir de 1981 (127,4 Md€) et le prolonge jusqu'en 1959. Avant 1981, le
	// libellé du périmètre change dans la source — « tous secteurs
	// institutionnels » au lieu de « total tous régimes » — et l'année de
	// rupture est affichée sur la page plutôt que lissée.
	srows, err := pool.Query(ctx, `
		SELECT annee::int, sum(prestations_meur)*1e6 FROM derived.protection_sociale_total
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var p PointYear
		if err := srows.Scan(&p.Year, &p.Value); err != nil {
			break
		}
		st.Series = append(st.Series, p)
	}
	srows.Close()
	_ = pool.QueryRow(ctx, `
		SELECT min(annee) FROM core.protection_sociale
		WHERE ps_niveau=0 AND si_code='S1' AND regime='Total tous régimes'`).Scan(&st.Break)
	if len(st.Series) > 0 {
		st.Start = st.Series[0].Year
	}
	st.Curve = curve(st.Series, mdEur)

	// La population totale, en superposition : la même hausse des prestations
	// se lit très différemment selon qu'elle vient de plus de bénéficiaires ou
	// de prestations plus généreuses par personne. Champ METRO parce que sa
	// série remonte à 1901, donc couvre tout l'historique des prestations ;
	// FRANCE entière ne commence qu'en 1991.
	prows, err := pool.Query(ctx, `
		SELECT annee, sum(population)::float8 FROM core.population_age
		WHERE champ='METRO' GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var p PointYear
		if err := prows.Scan(&p.Year, &p.Value); err != nil {
			break
		}
		st.Population = append(st.Population, p)
	}
	prows.Close()
	millions := func(v float64) string { return Decimal(v/1e6, 1) + "\u202fM" }
	st.CurveWithPop = curveWithLine(st.Series, st.Population, mdEur, millions,
		"avec la population sur une échelle séparée")

	// La structure par \u00e2ge : \u00ab la population a aussi vieilli \u00bb, affirm\u00e9 dans
	// la note ci-dessous mais jamais montr\u00e9 ailleurs sur le site avant cette
	// courbe. Part des moins de 20 ans et des 65 ans et plus, sur la m\u00eame
	// population et le m\u00eame axe 0-100 \u2014 directement comparables, contrairement
	// \u00e0 la courbe de population totale ci-dessus.
	arows, err := pool.Query(ctx, `
		SELECT annee, sum(population) FILTER (WHERE age<20)::float8,
		       sum(population) FILTER (WHERE age>=65)::float8, sum(population)::float8
		FROM core.population_age WHERE champ='METRO' GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	pct := "%"
	for arows.Next() {
		var an int
		var p20, p65, total float64
		if err := arows.Scan(&an, &p20, &p65, &total); err != nil {
			arows.Close()
			return nil, err
		}
		if total <= 0 {
			continue
		}
		st.Share20 = append(st.Share20, PointYear{Year: an, Value: 100 * p20 / total})
		st.Share65 = append(st.Share65, PointYear{Year: an, Value: 100 * p65 / total})
	}
	arows.Close()
	if err := arows.Err(); err != nil {
		return nil, err
	}
	// L'ann\u00e9e de croisement : la premi\u00e8re o\u00f9 les 65 ans et plus d\u00e9passent les
	// moins de 20 ans \u2014 un fait dat\u00e9, pas une tendance qu'on affirme \u00e0 l'\u0153il.
	for i, p := range st.Share65 {
		if p.Value > st.Share20[i].Value {
			st.YearIntersection = p.Year
			break
		}
	}
	st.CurveAges = twoCurves(st.Share65, st.Share20,
		"65 ans et plus", "Moins de 20 ans", func(v float64) string { return Decimal(v, 1) + pct })
	return st, nil
}

// ── Les quatre bourses publiques ──────────────────────────────────────

// Le budget de l'État n'est PAS le budget public. Il en fait 40 % ; la Sécurité
// sociale en fait presque la moitié, et les collectivités un cinquième. Les
// trois sont votés par des assemblées différentes, tenus dans des comptabilités
// différentes, et le déficit dont on parle au journal est la somme des trois.
type SubSector struct {
	Code, Label                 string
	Expenses, Revenues, Balance float64
	ShareExpenses               float64
}

type SeriesSector struct {
	Year                        int
	Expenses, Revenues, Balance float64
}

type StatsSectors struct {
	// Consolidation : la somme des dépenses des trois sous-secteurs dépasse le
	// total consolidé, parce qu'un transfert de l'État à une collectivité est
	// une dépense de l'un ET finance une dépense de l'autre. Les soldes, eux,
	// s'additionnent exactement.
	SumExpenses      float64
	GapConsolidation float64
	// DepRec : dépenses et recettes des administrations publiques, année par
	// année, en barres appariées. Le solde se lit dans l'écart entre les deux.
	DepRec   template.HTML
	Year     int
	Start    int
	Sectors  []SubSector
	Total    SubSector
	Series   map[string][]SeriesSector
	Bars     template.HTML
	Financem []LineFinancing
	AnnEnd   int
	StartEnd int
	BarsEnd  template.HTML
	// CourbeS1311S1314 : les dépenses de l'administration centrale contre
	// celles de la Sécurité sociale, sur toute la série — le fait que la
	// seconde dépense plus que la première n'est montré nulle part ailleurs
	// qu'en un instantané d'une seule année (Secteurs, ci-dessus).
	CurveS1311S1314      template.HTML
	YearIntersection1314 int
	// EmpileesFinancement : les quatre postes de financement de la protection
	// sociale, empilés à 100 % sur TOUTE la série (1990–2023) — barresFin,
	// ci-dessus, ne montre que deux dates.
	StackedFinancing template.HTML
}

type LineFinancing struct {
	Code, Label string
	Amount      float64
	Share       float64
	ShareStart  float64
}

func loadSectors(ctx context.Context, pool *pgxpool.Pool) (*StatsSectors, error) {
	st := &StatsSectors{Series: map[string][]SeriesSector{}}
	// max/min sont des agrégations : la ligne existe même sans ce dérivé
	// encore calculé, avec des bornes NULL.
	var yearN, startN sql.NullInt64
	if err := pool.QueryRow(ctx,
		`SELECT max(annee), min(annee) FROM derived.budget_sous_secteur`).
		Scan(&yearN, &startN); err != nil {
		return nil, err
	}
	st.Year, st.Start = int(yearN.Int64), int(startN.Int64)
	rows, err := pool.Query(ctx, `
		SELECT annee, secteur, perimetre_label,
		       depenses_meur::float8*1e6, recettes_meur::float8*1e6, solde_meur::float8*1e6
		FROM derived.budget_sous_secteur ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var an int
		var code, lib string
		var d, r, so float64
		if err := rows.Scan(&an, &code, &lib, &d, &r, &so); err != nil {
			break
		}
		st.Series[code] = append(st.Series[code], SeriesSector{an, d, r, so})
		if an == st.Year {
			s := SubSector{Code: code, Label: lib, Expenses: d, Revenues: r, Balance: so}
			if code == "S13" {
				st.Total = s
			} else {
				st.Sectors = append(st.Sectors, s)
			}
		}
	}
	rows.Close()
	for _, x := range st.Sectors {
		st.SumExpenses += x.Expenses
	}
	st.GapConsolidation = st.SumExpenses - st.Total.Expenses
	for i := range st.Sectors {
		if st.Total.Expenses > 0 {
			st.Sectors[i].ShareExpenses = 100 * st.Sectors[i].Expenses / st.Total.Expenses
		}
	}
	sort.Slice(st.Sectors, func(i, j int) bool {
		return st.Sectors[i].Expenses > st.Sectors[j].Expenses
	})
	st.Bars = barsSectors(st.Sectors)
	var pairs []PairYear
	for _, x := range st.Series["S13"] {
		pairs = append(pairs, PairYear{x.Year, x.Expenses, x.Revenues})
	}
	st.DepRec = barsMatched(pairs, "Dépenses", "Recettes", mdEur, 5)

	// Administration centrale contre Sécurité sociale, dépenses, toute la
	// série : « c'est la Sécurité sociale qui dépense le plus » (la note plus
	// bas) devient une forme, pas seulement un chiffre pour la dernière année.
	var s1311, s1314 []PointYear
	for _, x := range st.Series["S1311"] {
		s1311 = append(s1311, PointYear{Year: x.Year, Value: x.Expenses})
	}
	for _, x := range st.Series["S1314"] {
		s1314 = append(s1314, PointYear{Year: x.Year, Value: x.Expenses})
	}
	if len(s1311) == len(s1314) {
		for i, p := range s1314 {
			if p.Value > s1311[i].Value {
				st.YearIntersection1314 = p.Year
				break
			}
		}
		st.CurveS1311S1314 = twoCurves(s1314, s1311,
			"Sécurité sociale (S1314)", "Administration centrale (S1311)", mdEur)
	}

	// Le financement de la protection sociale : la bascule cotisations → impôt.
	// C'est le fait le plus mal connu du budget social, et il est publié tel
	// quel par la DREES — aucune interprétation n'est nécessaire pour le voir.
	positions := []struct{ code, lib string }{
		{"protection.financement.cotisations.employeurs", "Cotisations des employeurs"},
		{"protection.financement.cotisations.protegees", "Cotisations des assurés"},
		{"protection.financement.impot.affecte", "Recettes fiscales affectées"},
		{"protection.financement.impot.general", "Recettes fiscales générales"},
	}
	_ = pool.QueryRow(ctx, `
		SELECT max(annee), min(annee) FROM core.macro_value
		WHERE serie_code='protection.financement.total'`).Scan(&st.AnnEnd, &st.StartEnd)
	var totalEnd, totalDeb float64
	_ = pool.QueryRow(ctx, `
		SELECT valeur::float8 FROM core.macro_value
		WHERE serie_code='protection.financement.total' AND annee=$1`, st.AnnEnd).Scan(&totalEnd)
	_ = pool.QueryRow(ctx, `
		SELECT valeur::float8 FROM core.macro_value
		WHERE serie_code='protection.financement.total' AND annee=$1`, st.StartEnd).Scan(&totalDeb)
	for _, p := range positions {
		var end, deb *float64
		_ = pool.QueryRow(ctx, `
			SELECT max(valeur) FILTER (WHERE annee=$2)::float8,
			       max(valeur) FILTER (WHERE annee=$3)::float8
			FROM core.macro_value WHERE serie_code=$1`, p.code, st.AnnEnd, st.StartEnd).
			Scan(&end, &deb)
		l := LineFinancing{Code: p.code, Label: p.lib}
		if end != nil {
			l.Amount = *end * 1e6
			if totalEnd > 0 {
				l.Share = 100 * *end / totalEnd
			}
		}
		if deb != nil && totalDeb > 0 {
			l.ShareStart = 100 * *deb / totalDeb
		}
		st.Financem = append(st.Financem, l)
	}
	st.BarsEnd = barsFinancing(st.Financem, st.StartEnd, st.AnnEnd)

	// La même bascule, sur les 34 années, en 100 % empilé — barresFinancement
	// ne compare que deux dates parce qu'un empilement en série devient un mur
	// de couleurs ; ici il ne l'est pas trop, quatre postes sur 34 ans.
	frows, err := pool.Query(ctx, `
		SELECT annee, serie_code, valeur::float8 FROM core.macro_value
		WHERE serie_code = ANY($1) ORDER BY annee`,
		[]string{"protection.financement.cotisations.employeurs",
			"protection.financement.cotisations.protegees",
			"protection.financement.impot.affecte",
			"protection.financement.impot.general"})
	if err != nil {
		return nil, err
	}
	valuesEnd := map[string]map[int]float64{}
	var yearsEnd []int
	seenYearEnd := map[int]bool{}
	for frows.Next() {
		var an int
		var code string
		var v float64
		if err := frows.Scan(&an, &code, &v); err != nil {
			frows.Close()
			return nil, err
		}
		if valuesEnd[code] == nil {
			valuesEnd[code] = map[int]float64{}
		}
		valuesEnd[code][an] = v
		if !seenYearEnd[an] {
			seenYearEnd[an] = true
			yearsEnd = append(yearsEnd, an)
		}
	}
	frows.Close()
	if err := frows.Err(); err != nil {
		return nil, err
	}
	sort.Ints(yearsEnd)
	// 100 % empilé : chaque poste en PART de l'année, pas en montant — sinon
	// l'inflation ferait grossir la barre entière, et le propos est la
	// COMPOSITION, pas le total.
	totalYear := map[int]float64{}
	for _, code := range []string{"protection.financement.cotisations.employeurs",
		"protection.financement.cotisations.protegees", "protection.financement.impot.affecte",
		"protection.financement.impot.general"} {
		for an, v := range valuesEnd[code] {
			totalYear[an] += v
		}
	}
	pctEnd := map[string]map[int]float64{}
	for _, p := range positions {
		pctEnd[p.code] = map[int]float64{}
		for an, v := range valuesEnd[p.code] {
			if totalYear[an] > 0 {
				pctEnd[p.code][an] = 100 * v / totalYear[an]
			}
		}
	}
	st.StackedFinancing = barsStackedAnnual(yearsEnd, []SeriesStacked{
		{Label: positions[0].lib, Color: "#1E5C69", Values: pctEnd[positions[0].code]},
		{Label: positions[1].lib, Color: "#4A8894", Values: pctEnd[positions[1].code]},
		{Label: positions[2].lib, Color: "#B0CFD5", Values: pctEnd[positions[2].code]},
		{Label: positions[3].lib, Color: "#DCE9EC", Values: pctEnd[positions[3].code]},
	}, func(v float64) string { return Decimal(v, 1) + " %" })
	return st, nil
}

func barsSectors(ss []SubSector) template.HTML {
	var max float64
	for _, s := range ss {
		if s.Expenses > max {
			max = s.Expenses
		}
	}
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="barres">`)
	for _, s := range ss {
		fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s</span><span class="c">%s</span></div>`,
			template.HTMLEscapeString(s.Label), 100*s.Expenses/max,
			mdEur(s.Expenses), Decimal(s.ShareExpenses, 0)+" %")
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// barsFinancing : deux barres empilées à 100 %, la première année et la
// dernière. Un empilement en pourcentage ne se lit bien qu'à deux ou trois
// dates ; en série annuelle il devient un mur de couleurs.
func barsFinancing(ls []LineFinancing, start, end int) template.HTML {
	if len(ls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, cas := range []struct {
		an    int
		share func(LineFinancing) float64
	}{{start, func(l LineFinancing) float64 { return l.ShareStart }},
		{end, func(l LineFinancing) float64 { return l.Share }}} {
		fmt.Fprintf(&b, `<div class="empil"><span class="an">%d</span><span class="pile">`, cas.an)
		for i, l := range ls {
			fmt.Fprintf(&b, `<i class="f%d" style="width:%.2f%%" title="%s : %s"></i>`,
				i, cas.share(l), template.HTMLEscapeString(l.Label),
				Decimal(cas.share(l), 1)+" %")
		}
		b.WriteString(`</span></div>`)
	}
	b.WriteString(`<div class="legende">`)
	for i, l := range ls {
		fmt.Fprintf(&b, `<span><i class="f%d"></i>%s</span>`, i,
			template.HTMLEscapeString(l.Label))
	}
	b.WriteString(`</div>`)
	return template.HTML(`<div class="empils">` + b.String() + `</div>`)
}
