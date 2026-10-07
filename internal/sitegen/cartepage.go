package sitegen

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
)

// Une page par carte. La grille d'index ne montre plus que des vignettes ; le
// détail — tracé fin, infobulles, classement complet des départements, série
// annuelle quand elle existe — vit sur sa propre page.
//
// Ce découpage est d'abord un choix de lecture : une page de quinze cartes ne
// se lit pas, elle se survole. Il se trouve qu'il règle aussi le poids.
type PageMap struct {
	Slug, Title, Question, Source, Note string
	Section, SectionURL                 string
	// SectionIndexURL : l'adresse du fil d'Ariane, si elle diffère de
	// SectionURL (qui sert aussi de préfixe aux liens entre cartes
	// voisines). Les cartes départementales vivent sous /collectivites/carte/
	// pour ne pas entrer en collision avec /collectivites/region|departement/,
	// mais leur fil d'Ariane doit pointer vers /collectivites/, l'index réel.
	SectionIndexURL string
	Map             Map
	// Resume : nombre de départements, médiane, maximum et minimum — le
	// résumé chiffré posé à côté de LA carte, sur le modèle de la colonne
	// de droite de « Trois niveaux, trois cartes » sur
	// /sujets/collectivites/ (dl.legende-situation), plutôt qu'une simple
	// légende de couleurs renvoyant au classement complet plus bas.
	Summary      *SummaryRanking
	Ranking      []Rank
	Series       []PointYear
	SeriesLegend string
	Curve        template.HTML
	Neighboring  []LinkMap
}

type LinkMap struct{ Slug, Title string }

// SummaryRanking : nombre de départements couverts, médiane, maximum et
// minimum — tirés directement du classement déjà trié et déjà mis en forme
// par classement() (Rang.Valeur porte l'unité et les décimales choisies par
// l'appelant), plutôt que recalculés sur les valeurs brutes : jamais deux
// formatages différents du même chiffre sur une même page.
type SummaryRanking struct {
	Count             int
	MedianValue       string
	MaxName, MaxValue string
	MinName, MinValue string
}

func summarizeRanking(ranks []Rank) *SummaryRanking {
	if len(ranks) == 0 {
		return nil
	}
	r := &SummaryRanking{Count: len(ranks)}
	r.MaxName, r.MaxValue = ranks[0].Name, ranks[0].Value
	last := ranks[len(ranks)-1]
	r.MinName, r.MinValue = last.Name, last.Value
	r.MedianValue = ranks[len(ranks)/2].Value
	return r
}

type Rank struct {
	Rank       int
	Code, Name string
	Value      string
	Share      float64 // largeur de barre, en % du maximum
}

// ranking : tous les départements, du plus haut au plus bas. Aucun palmarès
// tronqué — un « top 10 » choisit pour le lecteur le bout de la distribution
// qui l'intéresse, et cache les 86 autres.
//
// Le nom vient de `noms`, c'est-à-dire de geo.contour, et jamais du champ Nom
// des cases : les requêtes agrègent des communes par département et en tirent
// un max(nom_clair) qui est le nom de la dernière commune dans l'ordre
// alphabétique. « Cambriolages : 1. VILLEGENON » n'était pas un département.
func ranking(cells []CellMap, noms map[string]string, format func(float64) string) []Rank {
	var out []Rank
	var max float64
	for _, c := range cells {
		if c.Absent {
			continue
		}
		if c.Value > max {
			max = c.Value
		}
	}
	dispo := make([]CellMap, 0, len(cells))
	for _, c := range cells {
		if !c.Absent {
			dispo = append(dispo, c)
		}
	}
	sort.Slice(dispo, func(i, j int) bool { return dispo[i].Value > dispo[j].Value })
	for i, c := range dispo {
		name := noms[c.Code]
		if name == "" {
			name = c.Name
		}
		r := Rank{Rank: i + 1, Code: c.Code, Name: name, Value: format(c.Value)}
		if max > 0 && c.Value > 0 {
			r.Share = 100 * c.Value / max
		}
		out = append(out, r)
	}
	return out
}

// curve : une série annuelle, en BARRES, une seule échelle, ancrée à zéro.
//
// Le nom est resté, la forme a changé. Une série annuelle est une suite de
// mesures distinctes — un taux au 31 décembre, un total d'exercice — et non un
// phénomène continu : un trait entre deux points laisse croire qu'on peut lire
// une valeur « à la mi-2019 », que la source ne publie pas. La barre dit
// exactement ce qui existe : une valeur par année, et rien entre elles.
//
// Ancrer à zéro est obligatoire pour une barre — sa longueur EST la valeur —
// et c'est aussi ce qui convient ici : la question posée est « combien ».
func curve(pts []PointYear, format func(float64) string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 210.0, 8.0, 8.0, 28.0, 26.0
	var max float64
	for _, p := range pts {
		if p.Value > max {
			max = p.Value
		}
	}
	if max <= 0 {
		return ""
	}
	n := float64(len(pts))
	step := (w - ml - mr) / n
	gap := step * 0.18
	if gap > 4 {
		gap = 4
	}
	if gap < 1 {
		gap = 1
	}
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("De %d à %d : %s puis %s",
			pts[0].Year, pts[len(pts)-1].Year, format(pts[0].Value),
			format(pts[len(pts)-1].Value))))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, p := range pts {
		x := ml + step*float64(i) + gap/2
		top := y(p.Value)
		cl := "b"
		if i == len(pts)-1 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%d — %s</title></rect>`,
			cl, x, top, step-gap, (h-mb)-top, p.Year, template.HTMLEscapeString(format(p.Value)))
	}
	// Deux étiquettes : la première et la dernière barre. Une valeur sur
	// chaque barre transformerait le graphique en tableau mal rangé ; les
	// autres sont dans l'infobulle.
	first, last := pts[0], pts[len(pts)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`,
		ml, y(first.Value)-8, template.HTMLEscapeString(format(first.Value)))
	fmt.Fprintf(&b, `<text class="et fin" x="%.1f" y="%.1f">%s</text>`,
		w-mr, y(last.Value)-8, template.HTMLEscapeString(format(last.Value)))
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, first.Year)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, last.Year)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// ── Barres appariées ──────────────────────────────────────────────────

// PairYear : deux mesures d'une même année, dans la même unité.
type PairYear struct {
	Year int
	A, B float64
}

// barsMatched : deux barres par année, côte à côte, sur UN axe ancré à
// zéro. C'est la forme qui répond à « combien l'un, combien l'autre, et quel
// écart » sans calcul : l'écart entre les deux barres d'une année se voit.
//
// Chaque année est un groupe séparé de ses voisins par un blanc d'un tiers de
// sa largeur : sans ce blanc, trente paires de barres collées forment une
// seule masse où l'on ne retrouve plus quelle barre appartient à quelle année.
func barsMatched(pts []PairYear, libA, libB string, format func(float64) string,
	labelAll int) template.HTML {

	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 270.0, 86.0, 8.0, 14.0, 28.0
	var max float64
	for _, p := range pts {
		if p.A > max {
			max = p.A
		}
		if p.B > max {
			max = p.B
		}
	}
	if max <= 0 {
		return ""
	}
	step := (w - ml - mr) / float64(len(pts))
	blank := step / 3
	larg := (step - blank) / 2
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="graphe apparie" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("%s et %s, %d à %d",
			libA, libB, pts[0].Year, pts[len(pts)-1].Year)))
	for _, frac := range []float64{0, 0.5, 1} {
		yy := mt + (h-mt-mb)*(1-frac)
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			ml, yy, w-mr, yy)
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			ml-8, yy+3, template.HTMLEscapeString(format(max*frac)))
	}
	for i, p := range pts {
		x := ml + step*float64(i) + blank/2
		for j, v := range []float64{p.A, p.B} {
			cl, lib := "pa", libA
			if j == 1 {
				cl, lib = "pb", libB
			}
			top := y(v)
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
				`<title>%d — %s : %s</title></rect>`,
				cl, x+larg*float64(j), top, larg, (h-mb)-top, p.Year,
				template.HTMLEscapeString(lib), template.HTMLEscapeString(format(v)))
		}
		if labelAll > 0 && p.Year%labelAll == 0 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d</text>`,
				x+larg, h-10, p.Year)
		}
	}
	b.WriteString(`</svg>`)
	fmt.Fprintf(&b, `<div class="legende"><span><i class="pa"></i>%s</span>`+
		`<span><i class="pb"></i>%s</span></div>`,
		template.HTMLEscapeString(libA), template.HTMLEscapeString(libB))
	return template.HTML(b.String())
}

// ── Barres avec une ligne à seconde échelle ────────────────────────────

// curveWithLine superpose une série en barres (prestations, échelle de
// gauche) et une série en ligne à points (population, échelle de droite). Les
// deux axes sont réels et indépendants — c'est le seul cas sur ce site où deux
// échelles coexistent sur un même graphique, et c'est justement pour ça
// qu'elles sont dessinées dans des couleurs et des styles n'ayant rien de
// commun : un lecteur ne doit jamais lire un repère de la ligne sur l'échelle
// des barres, ni l'inverse.
//
// La ligne ne couvre que les années où `ligne` a une valeur : si la série de
// population est plus courte que celle des barres, elle s'arrête net et ne
// s'invente aucun point.
func curveWithLine(bars []PointYear, line []PointYear,
	formatBars, formatLine func(float64) string, descriptionLine string) template.HTML {

	if len(bars) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 230.0, 44.0, 44.0, 28.0, 26.0
	var maxB float64
	for _, p := range bars {
		if p.Value > maxB {
			maxB = p.Value
		}
	}
	if maxB <= 0 {
		return ""
	}
	perYear := map[int]float64{}
	var minL, maxL float64
	isFirst := true
	for _, p := range line {
		perYear[p.Year] = p.Value
		if isFirst || p.Value < minL {
			minL = p.Value
		}
		if isFirst || p.Value > maxL {
			maxL = p.Value
		}
		isFirst = false
	}
	// L'échelle de la ligne part d'un peu sous son minimum, pas de zéro : une
	// population ne varie que de quelques pour cent sur la période, et un axe
	// à zéro l'aurait aplatie en trait droit.
	if maxL == minL {
		maxL = minL + 1
	}
	bottomL := minL - (maxL-minL)*0.15
	topL := maxL + (maxL-minL)*0.15

	n := float64(len(bars))
	step := (w - ml - mr) / n
	gap := step * 0.18
	if gap > 4 {
		gap = 4
	}
	if gap < 1 {
		gap = 1
	}
	yB := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxB) }
	yL := func(v float64) float64 { return mt + (h-mt-mb)*(1-(v-bottomL)/(topL-bottomL)) }
	xAt := func(i int) float64 { return ml + step*float64(i) + step/2 }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an avec-ligne" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("De %d à %d, %s",
			bars[0].Year, bars[len(bars)-1].Year, descriptionLine)))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, p := range bars {
		x := ml + step*float64(i) + gap/2
		top := yB(p.Value)
		cl := "b"
		if i == len(bars)-1 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%d — %s</title></rect>`,
			cl, x, top, step-gap, (h-mb)-top, p.Year, template.HTMLEscapeString(formatBars(p.Value)))
	}
	first, last := bars[0], bars[len(bars)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`,
		ml, yB(first.Value)-8, template.HTMLEscapeString(formatBars(first.Value)))
	fmt.Fprintf(&b, `<text class="et fin" x="%.1f" y="%.1f">%s</text>`,
		w-mr, yB(last.Value)-8, template.HTMLEscapeString(formatBars(last.Value)))
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, first.Year)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, last.Year)

	// La ligne : un point par année couverte, relié, avec un marqueur discret.
	var trace strings.Builder
	var pts []struct {
		x, y  float64
		year  int
		value float64
	}
	for i, p := range bars {
		if v, ok := perYear[p.Year]; ok {
			pts = append(pts, struct {
				x, y  float64
				year  int
				value float64
			}{xAt(i), yL(v), p.Year, v})
		}
	}
	for i, pt := range pts {
		op := "L"
		if i == 0 {
			op = "M"
		}
		fmt.Fprintf(&trace, "%s%.1f,%.1f", op, pt.x, pt.y)
	}
	if trace.Len() > 0 {
		fmt.Fprintf(&b, `<path class="ligne-population" d="%s" fill="none"/>`, trace.String())
		// Un point par année créerait un chapelet de perles sur soixante ans de
		// données — le trait pointillé suffit à porter la forme. Seule une
		// cible invisible, plus large que le trait, garde l'infobulle par année
		// au survol ; les deux extrémités seules restent visibles, à l'image
		// des deux étiquettes qui les accompagnent.
		for i, pt := range pts {
			fmt.Fprintf(&b, `<circle class="pt-hit" cx="%.1f" cy="%.1f" r="6"><title>%d — %s</title></circle>`,
				pt.x, pt.y, pt.year, template.HTMLEscapeString(formatLine(pt.value)))
			if i == 0 || i == len(pts)-1 {
				fmt.Fprintf(&b, `<circle class="pt-population" cx="%.1f" cy="%.1f" r="3"/>`, pt.x, pt.y)
			}
		}
		// Étiquettes de la population : à GAUCHE, jamais à droite — c'est là
		// que finissent déjà les plus grandes valeurs des barres (l'échelle de
		// gauche démarre à zéro et les prestations montent bien plus qu'elles),
		// et les deux étiquettes se sont un jour superposées dans ce coin.
		fmt.Fprintf(&b, `<text class="et droite" x="2" y="%.1f">%s</text>`,
			yL(pts[0].value)-8, template.HTMLEscapeString(formatLine(pts[0].value)))
		fmt.Fprintf(&b, `<text class="et droite fin" x="2" y="%.1f">%s</text>`,
			yL(pts[len(pts)-1].value)-8, template.HTMLEscapeString(formatLine(pts[len(pts)-1].value)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// ── Barres empilées annuelles ───────────────────────────────────────────

// SegmentAnnuel : une catégorie, sa valeur pour une année, dans une série
// empilée.
type SeriesStacked struct {
	Label  string
	Color  string
	Values map[int]float64
}

// barsStackedAnnual : un total empilé par année, catégories dans un
// ordre FIXE (jamais recalculé par valeur) — l'identité d'une catégorie ne
// doit pas changer de couleur d'une année à l'autre. Les segments d'une même
// barre se somment exactement au total ; c'est ce que ce type de graphique
// promet, et la seule raison de l'utiliser.
func barsStackedAnnual(years []int, series []SeriesStacked,
	format func(float64) string) template.HTML {

	if len(years) < 2 || len(series) == 0 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 16.0, 26.0
	var maxTotal float64
	for _, an := range years {
		var total float64
		for _, s := range series {
			total += s.Values[an]
		}
		if total > maxTotal {
			maxTotal = total
		}
	}
	if maxTotal <= 0 {
		return ""
	}
	n := float64(len(years))
	step := (w - ml - mr) / n
	gap := step * 0.18
	if gap > 5 {
		gap = 5
	}
	if gap < 1 {
		gap = 1
	}
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxTotal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an empilees" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("De %d à %d, en catégories empilées",
			years[0], years[len(years)-1])))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, an := range years {
		x := ml + step*float64(i) + gap/2
		cumulative := 0.0
		var total float64
		for _, s := range series {
			total += s.Values[an]
		}
		for si, s := range series {
			v := s.Values[an]
			bottom := y(cumulative)
			top := y(cumulative + v)
			cl := fmt.Sprintf("seg s%d", si)
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" `+
				`style="fill:%s"><title>%d — %s : %s</title></rect>`,
				cl, x, top, step-gap, bottom-top, s.Color, an,
				template.HTMLEscapeString(s.Label), template.HTMLEscapeString(format(v)))
			cumulative += v
		}
		// Les étiquettes de la première et de la dernière barre s'ancrent à son
		// bord extérieur, jamais centrées : une barre à l'extrémité du graphe a
		// son centre presque sur le bord du viewBox, et un texte centré y
		// déborderait — coupé, jamais visible en entier.
		if i == 0 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, x, h-8, an)
			fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`,
				x, y(total)-8, template.HTMLEscapeString(format(total)))
		} else if i == len(years)-1 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%d</text>`,
				x+step-gap, h-8, an)
			fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
				x+step-gap, y(total)-8, template.HTMLEscapeString(format(total)))
		} else if an%2 == 0 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d</text>`,
				x+(step-gap)/2, h-8, an)
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// ── Mini-courbe ──────────────────────────────────────────────────────

// sparkline : une forme sans axe ni graduation, pour accompagner un texte qui
// donne déjà les deux bornes (« min → max »). Échelle relative — JAMAIS
// zéro — parce qu'ici c'est la variation qui compte, pas l'ordre de
// grandeur : un sparkline de dette part de son propre minimum, comme les
// colonnes de la frise dont il est la miniature.
// couleur : vide pour la teinte par défaut (var(--accent), sur les cartes de
// la frise) ; une valeur fixe pour un sparkline groupé par catégorie — jamais
// recalculée par la hauteur de la courbe elle-même.
func sparkline(pts []PointYear, color string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, pad = 132.0, 30.0, 3.0
	min, max := pts[0].Value, pts[0].Value
	for _, p := range pts {
		if p.Value < min {
			min = p.Value
		}
		if p.Value > max {
			max = p.Value
		}
	}
	if max == min {
		max = min + 1
	}
	n := float64(len(pts) - 1)
	x := func(i int) float64 { return pad + (w-2*pad)*float64(i)/n }
	y := func(v float64) float64 { return pad + (h-2*pad)*(1-(v-min)/(max-min)) }

	var trace strings.Builder
	for i, p := range pts {
		op := "L"
		if i == 0 {
			op = "M"
		}
		fmt.Fprintf(&trace, "%s%.1f,%.1f", op, x(i), y(p.Value))
	}
	last := pts[len(pts)-1]
	style := ""
	if color != "" {
		style = fmt.Sprintf(` style="--sp:%s"`, color)
	}
	return template.HTML(fmt.Sprintf(
		`<svg class="sparkline" viewBox="0 0 %.0f %.0f" aria-hidden="true" focusable="false"%s>`+
			`<path class="ligne" d="%s" fill="none"/><circle class="pt" cx="%.1f" cy="%.1f" r="2"/></svg>`,
		w, h, style, trace.String(), x(len(pts)-1), y(last.Value)))
}

// ── Deux courbes, un seul axe ────────────────────────────────────────

// twoCurves superpose deux séries annuelles DE LA MÊME UNITÉ sur UN SEUL
// axe partagé — le cas légitime où deux lignes ont un sens, à la différence
// de courbeAvecLigne (deux unités, deux échelles). Deux teintes fixes,
// jamais recalculées, une légende toujours présente.
func twoCurves(a, b []PointYear, libA, libB string, format func(float64) string) template.HTML {
	if len(a) < 2 || len(b) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 230.0, 8.0, 8.0, 28.0, 26.0
	max := 0.0
	for _, p := range a {
		if p.Value > max {
			max = p.Value
		}
	}
	for _, p := range b {
		if p.Value > max {
			max = p.Value
		}
	}
	if max <= 0 {
		return ""
	}
	start, end := a[0].Year, a[len(a)-1].Year
	x := func(an int) float64 { return ml + (w-ml-mr)*float64(an-start)/float64(end-start) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	trace := func(pts []PointYear) string {
		var t strings.Builder
		for i, p := range pts {
			op := "L"
			if i == 0 {
				op = "M"
			}
			fmt.Fprintf(&t, "%s%.1f,%.1f", op, x(p.Year), y(p.Value))
		}
		return t.String()
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, `<svg class="courbe deux-lignes" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("%s et %s, %d à %d", libA, libB, start, end)))
	fmt.Fprintf(&buf, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, h-mb, w-mr, h-mb)
	fmt.Fprintf(&buf, `<path class="ligne l1" d="%s" fill="none"/>`, trace(a))
	fmt.Fprintf(&buf, `<path class="ligne l2" d="%s" fill="none"/>`, trace(b))
	// Les deux étiquettes de fin peuvent tomber à la même hauteur quand les
	// deux séries se rejoignent (c'est justement le fait que ce genre de
	// graphe sert à montrer) — sans écart minimal, elles se superposent et
	// deviennent illisibles au moment précis où le croisement est le plus
	// intéressant à lire.
	yEndA, yEndB := y(a[len(a)-1].Value), y(b[len(b)-1].Value)
	const gapMin = 13.0
	if d := yEndA - yEndB; d > -gapMin && d < gapMin {
		middle := (yEndA + yEndB) / 2
		if yEndA <= yEndB {
			yEndA, yEndB = middle-gapMin/2, middle+gapMin/2
		} else {
			yEndA, yEndB = middle+gapMin/2, middle-gapMin/2
		}
	}
	for i, p := range []struct {
		pts []PointYear
		cl  string
	}{{a, "l1"}, {b, "l2"}} {
		first, last := p.pts[0], p.pts[len(p.pts)-1]
		yEt := yEndA
		if i == 1 {
			yEt = yEndB
		}
		fmt.Fprintf(&buf, `<circle class="pt %s" cx="%.1f" cy="%.1f" r="2.6"><title>%d — %s</title></circle>`,
			p.cl, x(first.Year), y(first.Value), first.Year, template.HTMLEscapeString(format(first.Value)))
		fmt.Fprintf(&buf, `<circle class="pt %s" cx="%.1f" cy="%.1f" r="2.6"><title>%d — %s</title></circle>`,
			p.cl, x(last.Year), y(last.Value), last.Year, template.HTMLEscapeString(format(last.Value)))
		fmt.Fprintf(&buf, `<text class="et %s" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			p.cl, x(last.Year)-4, yEt-8, template.HTMLEscapeString(format(last.Value)))
	}
	fmt.Fprintf(&buf, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, start)
	fmt.Fprintf(&buf, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, end)
	buf.WriteString(`</svg>`)
	fmt.Fprintf(&buf, `<div class="legende"><span><i class="l1"></i>%s</span>`+
		`<span><i class="l2"></i>%s</span></div>`,
		template.HTMLEscapeString(libA), template.HTMLEscapeString(libB))
	return template.HTML(buf.String())
}
