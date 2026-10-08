package sitegen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ce qu'il y a dans le dernier décile de niveau de vie (D10) : le dossier
// pauvreté (vieillesse... non, pauvrete-donnees.md) montre D1 à D9 avec un
// dixième décile délibérément « non borné ». Ce dossier ouvre ce dixième
// décile — 1 %, 0,1 %, 0,01 % les plus aisés — et lui adjoint le patrimoine,
// une notion distincte, jamais additionnée au revenu. Voir docs/
// repartition-richesse-donnees.md et la migration 0117.
type StatsWealth struct {
	Thresholds                     []ThresholdIncome
	Curve1Pct                      template.HTML
	Curve01Pct                     template.HTML
	YearStart, YearEnd             int
	Share1PctStart, Share1PctEnd   float64
	Share01PctStart, Share01PctEnd float64
	Wealth                         []BracketWealth
	YearWealth                     int

	// L'héritage comme facteur d'accès à la richesse (§ 4) et la
	// comparaison patrimoine/niveau de vie (§ 5) — même fiche Insee,
	// migration 0118.
	InheritedOverall, InheritedTop float64
	DonationOverall, DonationTop   float64
	GiniWealth, GiniLevelLife      float64
	Concentrations                 []Concentration
	SVGLorenz                      template.HTML
	Share10PctWealth               float64

	// Population de référence (France métropolitaine, tous âges,
	// core.population_age_departement, migration 0115) : sert à convertir
	// les pourcentages de seuils et de parts en nombres de personnes — un
	// ORDRE DE GRANDEUR, pas le chiffre exact que publierait Filosofi
	// lui-même, dont le champ (ménages fiscaux à revenu positif ou nul)
	// est légèrement plus étroit que la population totale.
	PopulationApprox       int
	YearPopulation         int
	SVGSummitIncome        template.HTML
	Distribution2021       []ShareGroup
	YearDistributionRecent int

	// Écart concret (en fois) entre le seuil du 0,1 % les plus aisés et la
	// médiane, § 1 — sert à donner un ancrage en euros aux points de
	// pourcentage du § 2, sans reconstituer une masse totale historique
	// (aucune source ne publie la masse des revenus déclarés par UC pour
	// 2004/2013/2018/2021 dans un champ comparable à celui du § 2).
	RatioSummitMedian float64
}

type Concentration struct {
	Position                  string
	MassWealth, MassLevelLife float64
}

type ShareGroup struct {
	Label, Color string
	SharePct     float64
	People       int
}

type ThresholdIncome struct {
	Label, Code               string
	IncomeBefore, LevelOfLife int
	SharePct                  float64
	People                    int
}

type BracketWealth struct {
	Label                        string
	Threshold2015, Threshold2021 int
	Average2015, Average2021     int
	ShareMass2021                float64
}

var labelThreshold = map[string]string{
	"D5": "Médiane (50 %)", "D9": "10 % les plus aisés", "Q99": "1 % les plus aisés",
	"Q99_9": "0,1 % les plus aisés", "Q99_99": "0,01 % les plus aisés",
}
var orderThreshold = []string{"D5", "D9", "Q99", "Q99_9", "Q99_99"}
var shareThreshold = map[string]float64{"D5": 50, "D9": 10, "Q99": 1, "Q99_9": 0.1, "Q99_99": 0.01}

var labelWealth = map[string]string{
	"P90_P95": "Du 90ᵉ au 95ᵉ centile", "P95_P99": "Du 95ᵉ au 99ᵉ centile", "SUP_P99": "Au-delà du 99ᵉ centile",
}
var orderWealth = []string{"P90_P95", "P95_P99", "SUP_P99"}

func loadWealth(ctx context.Context, pool *pgxpool.Pool) (*StatsWealth, error) {
	st := &StatsWealth{}

	thresholdRows, err := pool.Query(ctx, `
		SELECT seuil, revenu_avant_redistribution_eur, niveau_de_vie_eur
		FROM core.filosofi_haut_revenu`)
	if err != nil {
		return nil, err
	}
	perThreshold := map[string]ThresholdIncome{}
	for thresholdRows.Next() {
		var code string
		var before, level int
		if err := thresholdRows.Scan(&code, &before, &level); err != nil {
			thresholdRows.Close()
			return nil, err
		}
		perThreshold[code] = ThresholdIncome{Label: labelThreshold[code], Code: code, IncomeBefore: before, LevelOfLife: level}
	}
	thresholdRows.Close()
	if err := thresholdRows.Err(); err != nil {
		return nil, err
	}
	// 2021 : le millésime des seuils de revenu (§ 1) et de la série de parts
	// la plus récente (§ 2) — la population de référence doit être la même
	// année, jamais la plus récente disponible par ailleurs.
	const yearReference = 2021
	var populationApprox sql.NullInt64
	if err := pool.QueryRow(ctx, `
		SELECT $1::smallint, sum(population) FROM core.population_age_departement
		WHERE code_departement !~ '^97' AND annee = $1`, yearReference).
		Scan(&st.YearPopulation, &populationApprox); err != nil {
		return nil, err
	}
	st.PopulationApprox = int(populationApprox.Int64)

	for _, code := range orderThreshold {
		if s, ok := perThreshold[code]; ok {
			s.SharePct = shareThreshold[code]
			s.People = int(s.SharePct / 100 * float64(st.PopulationApprox))
			st.Thresholds = append(st.Thresholds, s)
		}
	}
	// richesse est une source hors chaîne par défaut (internal/ingest) :
	// core.filosofi_haut_revenu reste vide après un simple « fpctl ingest
	// default », et la boucle ci-dessus ne construit alors aucun seuil.
	// richesse.gohtml indexe st.Seuils à des positions fixes (jusqu'à 3,
	// le 0,1 % le plus aisé) en supposant ordreSeuil au complet — un
	// sous-ensemble planterait le gabarit (index hors bornes) au lieu de
	// simplement ne rien publier. addPageNode (graphe_sections.go,
	// rienAPublier) sait sauter une page dont le chargeur renvoie nil :
	// c'est ce signal qu'il faut lui donner ici plutôt qu'un *StatsRichesse
	// non nil mais incomplet.
	if len(st.Thresholds) < len(orderThreshold) {
		return nil, nil
	}
	if median, ok := perThreshold["D5"]; ok {
		if summit, ok := perThreshold["Q99_9"]; ok && median.IncomeBefore > 0 {
			st.RatioSummitMedian = float64(summit.IncomeBefore) / float64(median.IncomeBefore)
		}
	}

	loadSeries := func(group string) ([]PointYear, error) {
		rows, err := pool.Query(ctx, `
			SELECT annee, part_pct FROM core.revenu_part_groupe
			WHERE groupe = $1 ORDER BY annee`, group)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var pts []PointYear
		for rows.Next() {
			var p PointYear
			if err := rows.Scan(&p.Year, &p.Value); err != nil {
				return nil, err
			}
			pts = append(pts, p)
		}
		return pts, rows.Err()
	}
	pct1, err := loadSeries("1_PLUS_AISES")
	if err != nil {
		return nil, err
	}
	pct01, err := loadSeries("0_1_PLUS_AISES")
	if err != nil {
		return nil, err
	}
	if len(pct1) > 0 {
		st.YearStart, st.YearEnd = pct1[0].Year, pct1[len(pct1)-1].Year
		st.Share1PctStart, st.Share1PctEnd = pct1[0].Value, pct1[len(pct1)-1].Value
		st.Curve1Pct = curve(pct1, func(v float64) string { return Decimal(v, 1) + " %" })
	}
	if len(pct01) > 0 {
		st.Share01PctStart, st.Share01PctEnd = pct01[0].Value, pct01[len(pct01)-1].Value
		st.Curve01Pct = curve(pct01, func(v float64) string { return Decimal(v, 1) + " %" })
	}

	// Répartition de la population pour l'année la plus récente (2021) des
	// quatre groupes non recouvrants, avec un nombre de personnes approché.
	repRows, err := pool.Query(ctx, `
		SELECT groupe, part_pct FROM core.revenu_part_groupe
		WHERE annee = (SELECT max(annee) FROM core.revenu_part_groupe)
		  AND groupe IN ('90_MODESTES','9_SUIVANTS','0_9_SUIVANTS','0_1_PLUS_AISES')`)
	if err != nil {
		return nil, err
	}
	labelGroup := map[string]string{
		"90_MODESTES": "90 % les plus modestes", "9_SUIVANTS": "9 % suivants",
		"0_9_SUIVANTS": "0,9 % suivants", "0_1_PLUS_AISES": "0,1 % les plus aisés",
	}
	colorGroup := map[string]string{
		"90_MODESTES": "#DCE9EC", "9_SUIVANTS": "#7FB0BA", "0_9_SUIVANTS": "#1E5C69", "0_1_PLUS_AISES": "#8C4B3A",
	}
	orderGroup := []string{"90_MODESTES", "9_SUIVANTS", "0_9_SUIVANTS", "0_1_PLUS_AISES"}
	perGroup := map[string]float64{}
	for repRows.Next() {
		var g string
		var p float64
		if err := repRows.Scan(&g, &p); err != nil {
			repRows.Close()
			return nil, err
		}
		perGroup[g] = p
	}
	repRows.Close()
	if err := repRows.Err(); err != nil {
		return nil, err
	}
	st.YearDistributionRecent = st.YearEnd
	for _, code := range orderGroup {
		p := perGroup[code]
		st.Distribution2021 = append(st.Distribution2021, ShareGroup{
			Label: labelGroup[code], Color: colorGroup[code], SharePct: p,
			People: int(p / 100 * float64(st.PopulationApprox)),
		})
	}

	patRows, err := pool.Query(ctx, `
		SELECT tranche, annee, seuil_bas_eur, patrimoine_moyen_eur, part_masse_pct
		FROM core.patrimoine_haut ORDER BY tranche, annee`)
	if err != nil {
		return nil, err
	}
	perBracket := map[string]*BracketWealth{}
	for patRows.Next() {
		var bracket string
		var year, threshold, average int
		var share *float64
		if err := patRows.Scan(&bracket, &year, &threshold, &average, &share); err != nil {
			patRows.Close()
			return nil, err
		}
		t, ok := perBracket[bracket]
		if !ok {
			t = &BracketWealth{Label: labelWealth[bracket]}
			perBracket[bracket] = t
		}
		if year == 2015 {
			t.Threshold2015, t.Average2015 = threshold, average
		} else {
			t.Threshold2021, t.Average2021 = threshold, average
			if share != nil {
				t.ShareMass2021 = *share
			}
			st.YearWealth = year
		}
	}
	patRows.Close()
	if err := patRows.Err(); err != nil {
		return nil, err
	}
	for _, code := range orderWealth {
		if t, ok := perBracket[code]; ok {
			st.Wealth = append(st.Wealth, *t)
		}
	}

	var inheritedOverall, inheritedTop, donationOverall, donationTop sql.NullFloat64
	if err := pool.QueryRow(ctx, `
		SELECT max(part_herite_pct) FILTER (WHERE categorie='ENSEMBLE'),
		       max(part_herite_pct) FILTER (WHERE categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE'),
		       max(part_donation_pct) FILTER (WHERE categorie='ENSEMBLE'),
		       max(part_donation_pct) FILTER (WHERE categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE')
		FROM core.menage_heritage WHERE tranche_age = 'TOUS_AGES'`).
		Scan(&inheritedOverall, &inheritedTop, &donationOverall, &donationTop); err != nil {
		return nil, err
	}
	st.InheritedOverall, st.InheritedTop = inheritedOverall.Float64, inheritedTop.Float64
	st.DonationOverall, st.DonationTop = donationOverall.Float64, donationTop.Float64

	// ORDER BY ... LIMIT 1 sur une table pas encore chargée ne renvoie aucune
	// ligne (pgx.ErrNoRows), pas une ligne NULL.
	if err := pool.QueryRow(ctx, `
		SELECT indice_patrimoine, indice_niveau_vie FROM core.gini_patrimoine_niveau_vie
		ORDER BY annee DESC LIMIT 1`).
		Scan(&st.GiniWealth, &st.GiniLevelLife); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	concRows, err := pool.Query(ctx, `
		SELECT position_distribution, masse_patrimoine_pct, masse_niveau_vie_pct
		FROM core.concentration_patrimoine_niveau_vie
		ORDER BY array_position(ARRAY[
			'Inférieure au 2e décile', 'Inférieure au 4e décile',
			'Inférieure au 5e décile (médiane)', 'Supérieure au 5e décile (médiane)',
			'Supérieure au 8e décile', 'Supérieure au 9e décile'
		], position_distribution)`)
	if err != nil {
		return nil, err
	}
	for concRows.Next() {
		var c Concentration
		if err := concRows.Scan(&c.Position, &c.MassWealth, &c.MassLevelLife); err != nil {
			concRows.Close()
			return nil, err
		}
		if strings.HasPrefix(c.Position, "Supérieure au 9") {
			st.Share10PctWealth = c.MassWealth
		}
		st.Concentrations = append(st.Concentrations, c)
	}
	concRows.Close()
	if err := concRows.Err(); err != nil {
		return nil, err
	}
	st.SVGLorenz = drawLorenz(st.Concentrations)

	// D9 à Q99,99 seulement : ce chapitre montre ce qu'il y a À L'INTÉRIEUR
	// du dixième décile (D5 est hors sujet ici, déjà dans le dossier
	// pauvreté). Même principe que le graphique des déciles D1-D9 de ce
	// dernier : un dégradé sur la dernière barre, parce que Q99,99 n'a pas
	// plus de plafond connu que D10 lui-même.
	var summit []ThresholdIncome
	for _, s := range st.Thresholds {
		if s.Code != "D5" {
			summit = append(summit, s)
		}
	}
	format := func(v float64) string { return Decimal(v, 0) + " €" }
	st.SVGSummitIncome = drawSummitIncome(summit, format)

	return st, nil
}

// drawSummitIncome : les seuils D9, Q99, Q99,9, Q99,99 en barres — même
// principe que dessinerSeuilsPauvrete (axe à zéro, jamais tronqué), sans les
// lignes de seuil (qui n'ont pas leur place ici) mais avec le même dégradé
// sur la dernière barre : Q99,99 n'a pas plus de plafond connu que D10.
func drawSummitIncome(thresholds []ThresholdIncome, format func(float64) string) template.HTML {
	if len(thresholds) == 0 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 30.0, 30.0
	max := 0.0
	for _, s := range thresholds {
		if float64(s.LevelOfLife) > max {
			max = float64(s.LevelOfLife)
		}
	}
	max *= 1.15
	// Une barre de plus, réservée au dégradé au-delà de la dernière valeur
	// connue — le même principe que D10 dans dessinerSeuilsPauvrete.
	n := float64(len(thresholds) + 1)
	step := (w - ml - mr) / n
	gap := step * 0.16
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an seuils-pauvrete" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="%s">`, w, h, template.HTMLEscapeString(fmt.Sprintf(
		"Du seuil des 10%% les plus aisés à celui des 0,01%% les plus aisés : de %s à %s",
		format(float64(thresholds[0].LevelOfLife)), format(float64(thresholds[len(thresholds)-1].LevelOfLife)))))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)

	for i, s := range thresholds {
		x := ml + step*float64(i) + gap/2
		top := y(float64(s.LevelOfLife))
		fmt.Fprintf(&b, `<rect class="b" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%s — %s/an</title></rect>`,
			x, top, step-gap, (h-mb)-top, s.Label, template.HTMLEscapeString(format(float64(s.LevelOfLife))))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+(step-gap)/2, top-6, template.HTMLEscapeString(format(float64(s.LevelOfLife))))
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+(step-gap)/2, h-8, template.HTMLEscapeString(strings.TrimPrefix(s.Code, "Q")))
	}

	xFade := ml + step*float64(len(thresholds)) + gap/2
	fmt.Fprintf(&b, `<rect class="d10" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="url(#sommetfade)">`+
		`<title>Au-delà du 0,01%% les plus aisés, aucun seuil publié — le patrimoine et le revenu les plus hauts ne sont pas bornés</title></rect>`,
		xFade, mt, step-gap, (h-mb)-mt)
	fmt.Fprintf(&b, `<text class="et d10-et" x="%.1f" y="%.1f" text-anchor="middle">non borné</text>`,
		xFade+(step-gap)/2, mt+34)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">au-delà</text>`,
		xFade+(step-gap)/2, h-8)

	b.WriteString(`<defs><linearGradient id="sommetfade" x1="0" y1="1" x2="0" y2="0">` +
		`<stop offset="0%" stop-color="#7FB0BA"/>` +
		`<stop offset="75%" stop-color="#7FB0BA" stop-opacity=".35"/>` +
		`<stop offset="100%" stop-color="#7FB0BA" stop-opacity="0"/></linearGradient></defs>`)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// drawLorenz : la courbe de Lorenz — la façon la plus reconnue de
// montrer une concentration (population cumulée en abscisse, masse détenue
// cumulée en ordonnée), ici pour le patrimoine ET le niveau de vie sur le
// même repère, contre la diagonale d'égalité parfaite. Les points viennent
// des lignes « Inférieure au Nᵉ décile » de la fiche Insee, complétées par
// les deux extrémités (0,0) et (100,100), vraies par construction.
func drawLorenz(conc []Concentration) template.HTML {
	if len(conc) == 0 {
		return ""
	}
	// (population cumulée, patrimoine cumulé, niveau de vie cumulé), déduit
	// des libellés « Inférieure au Nᵉ décile » (population cumulée = le
	// centile) et « Supérieure au Nᵉ décile » (population cumulée = 100 -
	// le centile ; masse cumulée = 100 - la masse « supérieure » publiée).
	type pt struct{ x, wealth, levelLife float64 }
	pts := []pt{{0, 0, 0}}
	for _, c := range conc {
		switch {
		case strings.HasPrefix(c.Position, "Inférieure au 2"):
			pts = append(pts, pt{20, c.MassWealth, c.MassLevelLife})
		case strings.HasPrefix(c.Position, "Inférieure au 4"):
			pts = append(pts, pt{40, c.MassWealth, c.MassLevelLife})
		case strings.HasPrefix(c.Position, "Inférieure au 5"):
			pts = append(pts, pt{50, c.MassWealth, c.MassLevelLife})
		case strings.HasPrefix(c.Position, "Supérieure au 8"):
			pts = append(pts, pt{80, 100 - c.MassWealth, 100 - c.MassLevelLife})
		case strings.HasPrefix(c.Position, "Supérieure au 9"):
			pts = append(pts, pt{90, 100 - c.MassWealth, 100 - c.MassLevelLife})
		}
	}
	pts = append(pts, pt{100, 100, 100})

	const w, h, m = 400.0, 400.0, 30.0
	side := w - 2*m
	x := func(v float64) float64 { return m + side*v/100 }
	y := func(v float64) float64 { return h - m - side*v/100 }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="lorenz" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Courbe de Lorenz : le patrimoine s'écarte beaucoup plus de l'égalité parfaite que le niveau de vie">`, w, h)
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, m, h-m, w-m, h-m)
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, m, h-m, m, m)
	fmt.Fprintf(&b, `<line class="egalite" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, x(0), y(0), x(100), y(100))

	traceLine := func(cl string, sel func(pt) float64) {
		var pts2 []string
		for _, p := range pts {
			pts2 = append(pts2, fmt.Sprintf("%.1f,%.1f", x(p.x), y(sel(p))))
		}
		fmt.Fprintf(&b, `<polyline class="%s" points="%s"/>`, cl, strings.Join(pts2, " "))
		for _, p := range pts {
			fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%d%% des ménages, %s%% de la masse</title></circle>`,
				cl, x(p.x), y(sel(p)), int(p.x), Decimal(sel(p), 1))
		}
	}
	traceLine("ligne-patrimoine", func(p pt) float64 { return p.wealth })
	traceLine("ligne-niveau-vie", func(p pt) float64 { return p.levelLife })

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">0</text>`, x(0), h-m+16)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">100%%</text>`, x(100), h-m+16)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">100%%</text>`, m-6, y(100)+4)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">0</text>`, m-6, y(0)+4)

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
