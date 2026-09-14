package main

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
type PageCarte struct {
	Slug, Titre, Question, Source, Note string
	Section, SectionURL                 string
	Carte                               Carte
	Classement                          []Rang
	Serie                               []PointAnnee
	SerieLegende                        string
	Courbe                              template.HTML
	Voisines                            []LienCarte
}

type LienCarte struct{ Slug, Titre string }

type Rang struct {
	Rang      int
	Code, Nom string
	Valeur    string
	Part      float64 // largeur de barre, en % du maximum
}

// classement : tous les départements, du plus haut au plus bas. Aucun palmarès
// tronqué — un « top 10 » choisit pour le lecteur le bout de la distribution
// qui l'intéresse, et cache les 86 autres.
//
// Le nom vient de `noms`, c'est-à-dire de geo.contour, et jamais du champ Nom
// des cases : les requêtes agrègent des communes par département et en tirent
// un max(nom_clair) qui est le nom de la dernière commune dans l'ordre
// alphabétique. « Cambriolages : 1. VILLEGENON » n'était pas un département.
func classement(cases []CaseCarte, noms map[string]string, format func(float64) string) []Rang {
	var out []Rang
	var max float64
	for _, c := range cases {
		if c.Absent {
			continue
		}
		if c.Valeur > max {
			max = c.Valeur
		}
	}
	dispo := make([]CaseCarte, 0, len(cases))
	for _, c := range cases {
		if !c.Absent {
			dispo = append(dispo, c)
		}
	}
	sort.Slice(dispo, func(i, j int) bool { return dispo[i].Valeur > dispo[j].Valeur })
	for i, c := range dispo {
		nom := noms[c.Code]
		if nom == "" {
			nom = c.Nom
		}
		r := Rang{Rang: i + 1, Code: c.Code, Nom: nom, Valeur: format(c.Valeur)}
		if max > 0 && c.Valeur > 0 {
			r.Part = 100 * c.Valeur / max
		}
		out = append(out, r)
	}
	return out
}

// courbe : une série annuelle, en BARRES, une seule échelle, ancrée à zéro.
//
// Le nom est resté, la forme a changé. Une série annuelle est une suite de
// mesures distinctes — un taux au 31 décembre, un total d'exercice — et non un
// phénomène continu : un trait entre deux points laisse croire qu'on peut lire
// une valeur « à la mi-2019 », que la source ne publie pas. La barre dit
// exactement ce qui existe : une valeur par année, et rien entre elles.
//
// Ancrer à zéro est obligatoire pour une barre — sa longueur EST la valeur —
// et c'est aussi ce qui convient ici : la question posée est « combien ».
func courbe(pts []PointAnnee, format func(float64) string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 210.0, 8.0, 8.0, 28.0, 26.0
	var max float64
	for _, p := range pts {
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	if max <= 0 {
		return ""
	}
	n := float64(len(pts))
	pas := (w - ml - mr) / n
	gap := pas * 0.18
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
			pts[0].Annee, pts[len(pts)-1].Annee, format(pts[0].Valeur),
			format(pts[len(pts)-1].Valeur))))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, p := range pts {
		x := ml + pas*float64(i) + gap/2
		top := y(p.Valeur)
		cl := "b"
		if i == len(pts)-1 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%d — %s</title></rect>`,
			cl, x, top, pas-gap, (h-mb)-top, p.Annee, template.HTMLEscapeString(format(p.Valeur)))
	}
	// Deux étiquettes : la première et la dernière barre. Une valeur sur
	// chaque barre transformerait le graphique en tableau mal rangé ; les
	// autres sont dans l'infobulle.
	premier, dernier := pts[0], pts[len(pts)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`,
		ml, y(premier.Valeur)-8, template.HTMLEscapeString(format(premier.Valeur)))
	fmt.Fprintf(&b, `<text class="et fin" x="%.1f" y="%.1f">%s</text>`,
		w-mr, y(dernier.Valeur)-8, template.HTMLEscapeString(format(dernier.Valeur)))
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, premier.Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, dernier.Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// ── Barres appariées ──────────────────────────────────────────────────

// PaireAnnee : deux mesures d'une même année, dans la même unité.
type PaireAnnee struct {
	Annee int
	A, B  float64
}

// barresAppariees : deux barres par année, côte à côte, sur UN axe ancré à
// zéro. C'est la forme qui répond à « combien l'un, combien l'autre, et quel
// écart » sans calcul : l'écart entre les deux barres d'une année se voit.
//
// Chaque année est un groupe séparé de ses voisins par un blanc d'un tiers de
// sa largeur : sans ce blanc, trente paires de barres collées forment une
// seule masse où l'on ne retrouve plus quelle barre appartient à quelle année.
func barresAppariees(pts []PaireAnnee, libA, libB string, format func(float64) string,
	etiquetteTous int) template.HTML {

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
	pas := (w - ml - mr) / float64(len(pts))
	blanc := pas / 3
	larg := (pas - blanc) / 2
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="graphe apparie" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("%s et %s, %d à %d",
			libA, libB, pts[0].Annee, pts[len(pts)-1].Annee)))
	for _, frac := range []float64{0, 0.5, 1} {
		yy := mt + (h-mt-mb)*(1-frac)
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			ml, yy, w-mr, yy)
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			ml-8, yy+3, template.HTMLEscapeString(format(max*frac)))
	}
	for i, p := range pts {
		x := ml + pas*float64(i) + blanc/2
		for j, v := range []float64{p.A, p.B} {
			cl, lib := "pa", libA
			if j == 1 {
				cl, lib = "pb", libB
			}
			top := y(v)
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
				`<title>%d — %s : %s</title></rect>`,
				cl, x+larg*float64(j), top, larg, (h-mb)-top, p.Annee,
				template.HTMLEscapeString(lib), template.HTMLEscapeString(format(v)))
		}
		if etiquetteTous > 0 && p.Annee%etiquetteTous == 0 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d</text>`,
				x+larg, h-10, p.Annee)
		}
	}
	b.WriteString(`</svg>`)
	fmt.Fprintf(&b, `<div class="legende"><span><i class="pa"></i>%s</span>`+
		`<span><i class="pb"></i>%s</span></div>`,
		template.HTMLEscapeString(libA), template.HTMLEscapeString(libB))
	return template.HTML(b.String())
}

// ── Barres avec une ligne à seconde échelle ────────────────────────────

// courbeAvecLigne superpose une série en barres (prestations, échelle de
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
func courbeAvecLigne(barres []PointAnnee, ligne []PointAnnee,
	formatBarres, formatLigne func(float64) string) template.HTML {

	if len(barres) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 230.0, 44.0, 44.0, 28.0, 26.0
	var maxB float64
	for _, p := range barres {
		if p.Valeur > maxB {
			maxB = p.Valeur
		}
	}
	if maxB <= 0 {
		return ""
	}
	parAnnee := map[int]float64{}
	var minL, maxL float64
	first := true
	for _, p := range ligne {
		parAnnee[p.Annee] = p.Valeur
		if first || p.Valeur < minL {
			minL = p.Valeur
		}
		if first || p.Valeur > maxL {
			maxL = p.Valeur
		}
		first = false
	}
	// L'échelle de la ligne part d'un peu sous son minimum, pas de zéro : une
	// population ne varie que de quelques pour cent sur la période, et un axe
	// à zéro l'aurait aplatie en trait droit.
	if maxL == minL {
		maxL = minL + 1
	}
	basL := minL - (maxL-minL)*0.15
	hautL := maxL + (maxL-minL)*0.15

	n := float64(len(barres))
	pas := (w - ml - mr) / n
	gap := pas * 0.18
	if gap > 4 {
		gap = 4
	}
	if gap < 1 {
		gap = 1
	}
	yB := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxB) }
	yL := func(v float64) float64 { return mt + (h-mt-mb)*(1-(v-basL)/(hautL-basL)) }
	xAt := func(i int) float64 { return ml + pas*float64(i) + pas/2 }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an avec-ligne" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("De %d à %d, avec la population sur une échelle séparée",
			barres[0].Annee, barres[len(barres)-1].Annee)))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, p := range barres {
		x := ml + pas*float64(i) + gap/2
		top := yB(p.Valeur)
		cl := "b"
		if i == len(barres)-1 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%d — %s</title></rect>`,
			cl, x, top, pas-gap, (h-mb)-top, p.Annee, template.HTMLEscapeString(formatBarres(p.Valeur)))
	}
	premier, dernier := barres[0], barres[len(barres)-1]
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`,
		ml, yB(premier.Valeur)-8, template.HTMLEscapeString(formatBarres(premier.Valeur)))
	fmt.Fprintf(&b, `<text class="et fin" x="%.1f" y="%.1f">%s</text>`,
		w-mr, yB(dernier.Valeur)-8, template.HTMLEscapeString(formatBarres(dernier.Valeur)))
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, premier.Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, dernier.Annee)

	// La ligne : un point par année couverte, relié, avec un marqueur discret.
	var trace strings.Builder
	var pts []struct {
		x, y   float64
		annee  int
		valeur float64
	}
	for i, p := range barres {
		if v, ok := parAnnee[p.Annee]; ok {
			pts = append(pts, struct {
				x, y   float64
				annee  int
				valeur float64
			}{xAt(i), yL(v), p.Annee, v})
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
				pt.x, pt.y, pt.annee, template.HTMLEscapeString(formatLigne(pt.valeur)))
			if i == 0 || i == len(pts)-1 {
				fmt.Fprintf(&b, `<circle class="pt-population" cx="%.1f" cy="%.1f" r="3"/>`, pt.x, pt.y)
			}
		}
		// Étiquettes de la population : à GAUCHE, jamais à droite — c'est là
		// que finissent déjà les plus grandes valeurs des barres (l'échelle de
		// gauche démarre à zéro et les prestations montent bien plus qu'elles),
		// et les deux étiquettes se sont un jour superposées dans ce coin.
		fmt.Fprintf(&b, `<text class="et droite" x="2" y="%.1f">%s</text>`,
			yL(pts[0].valeur)-8, template.HTMLEscapeString(formatLigne(pts[0].valeur)))
		fmt.Fprintf(&b, `<text class="et droite fin" x="2" y="%.1f">%s</text>`,
			yL(pts[len(pts)-1].valeur)-8, template.HTMLEscapeString(formatLigne(pts[len(pts)-1].valeur)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// ── Barres empilées annuelles ───────────────────────────────────────────

// SegmentAnnuel : une catégorie, sa valeur pour une année, dans une série
// empilée.
type SerieEmpilee struct {
	Libelle string
	Couleur string
	Valeurs map[int]float64
}

// barresEmpileesAnnuelles : un total empilé par année, catégories dans un
// ordre FIXE (jamais recalculé par valeur) — l'identité d'une catégorie ne
// doit pas changer de couleur d'une année à l'autre. Les segments d'une même
// barre se somment exactement au total ; c'est ce que ce type de graphique
// promet, et la seule raison de l'utiliser.
func barresEmpileesAnnuelles(annees []int, series []SerieEmpilee,
	format func(float64) string) template.HTML {

	if len(annees) < 2 || len(series) == 0 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 16.0, 26.0
	var maxTotal float64
	for _, an := range annees {
		var total float64
		for _, s := range series {
			total += s.Valeurs[an]
		}
		if total > maxTotal {
			maxTotal = total
		}
	}
	if maxTotal <= 0 {
		return ""
	}
	n := float64(len(annees))
	pas := (w - ml - mr) / n
	gap := pas * 0.18
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
			annees[0], annees[len(annees)-1])))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	for i, an := range annees {
		x := ml + pas*float64(i) + gap/2
		cumul := 0.0
		var total float64
		for _, s := range series {
			total += s.Valeurs[an]
		}
		for si, s := range series {
			v := s.Valeurs[an]
			bas := y(cumul)
			haut := y(cumul + v)
			cl := fmt.Sprintf("seg s%d", si)
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" `+
				`style="fill:%s"><title>%d — %s : %s</title></rect>`,
				cl, x, haut, pas-gap, bas-haut, s.Couleur, an,
				template.HTMLEscapeString(s.Libelle), template.HTMLEscapeString(format(v)))
			cumul += v
		}
		// Les étiquettes de la première et de la dernière barre s'ancrent à son
		// bord extérieur, jamais centrées : une barre à l'extrémité du graphe a
		// son centre presque sur le bord du viewBox, et un texte centré y
		// déborderait — coupé, jamais visible en entier.
		if i == 0 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, x, h-8, an)
			fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`,
				x, y(total)-8, template.HTMLEscapeString(format(total)))
		} else if i == len(annees)-1 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%d</text>`,
				x+pas-gap, h-8, an)
			fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
				x+pas-gap, y(total)-8, template.HTMLEscapeString(format(total)))
		} else if an%2 == 0 {
			fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d</text>`,
				x+(pas-gap)/2, h-8, an)
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
func sparkline(pts []PointAnnee, couleur string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, pad = 132.0, 30.0, 3.0
	min, max := pts[0].Valeur, pts[0].Valeur
	for _, p := range pts {
		if p.Valeur < min {
			min = p.Valeur
		}
		if p.Valeur > max {
			max = p.Valeur
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
		fmt.Fprintf(&trace, "%s%.1f,%.1f", op, x(i), y(p.Valeur))
	}
	last := pts[len(pts)-1]
	style := ""
	if couleur != "" {
		style = fmt.Sprintf(` style="--sp:%s"`, couleur)
	}
	return template.HTML(fmt.Sprintf(
		`<svg class="sparkline" viewBox="0 0 %.0f %.0f" aria-hidden="true" focusable="false"%s>`+
			`<path class="ligne" d="%s" fill="none"/><circle class="pt" cx="%.1f" cy="%.1f" r="2"/></svg>`,
		w, h, style, trace.String(), x(len(pts)-1), y(last.Valeur)))
}

// ── Deux courbes, un seul axe ────────────────────────────────────────

// deuxCourbes superpose deux séries annuelles DE LA MÊME UNITÉ sur UN SEUL
// axe partagé — le cas légitime où deux lignes ont un sens, à la différence
// de courbeAvecLigne (deux unités, deux échelles). Deux teintes fixes,
// jamais recalculées, une légende toujours présente.
func deuxCourbes(a, b []PointAnnee, libA, libB string, format func(float64) string) template.HTML {
	if len(a) < 2 || len(b) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 230.0, 8.0, 8.0, 28.0, 26.0
	max := 0.0
	for _, p := range a {
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	for _, p := range b {
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	if max <= 0 {
		return ""
	}
	debut, fin := a[0].Annee, a[len(a)-1].Annee
	x := func(an int) float64 { return ml + (w-ml-mr)*float64(an-debut)/float64(fin-debut) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	trace := func(pts []PointAnnee) string {
		var t strings.Builder
		for i, p := range pts {
			op := "L"
			if i == 0 {
				op = "M"
			}
			fmt.Fprintf(&t, "%s%.1f,%.1f", op, x(p.Annee), y(p.Valeur))
		}
		return t.String()
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, `<svg class="courbe deux-lignes" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		w, h, template.HTMLEscapeString(fmt.Sprintf("%s et %s, %d à %d", libA, libB, debut, fin)))
	fmt.Fprintf(&buf, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, h-mb, w-mr, h-mb)
	fmt.Fprintf(&buf, `<path class="ligne l1" d="%s" fill="none"/>`, trace(a))
	fmt.Fprintf(&buf, `<path class="ligne l2" d="%s" fill="none"/>`, trace(b))
	// Les deux étiquettes de fin peuvent tomber à la même hauteur quand les
	// deux séries se rejoignent (c'est justement le fait que ce genre de
	// graphe sert à montrer) — sans écart minimal, elles se superposent et
	// deviennent illisibles au moment précis où le croisement est le plus
	// intéressant à lire.
	yFinA, yFinB := y(a[len(a)-1].Valeur), y(b[len(b)-1].Valeur)
	const ecartMin = 13.0
	if d := yFinA - yFinB; d > -ecartMin && d < ecartMin {
		milieu := (yFinA + yFinB) / 2
		if yFinA <= yFinB {
			yFinA, yFinB = milieu-ecartMin/2, milieu+ecartMin/2
		} else {
			yFinA, yFinB = milieu+ecartMin/2, milieu-ecartMin/2
		}
	}
	for i, p := range []struct {
		pts []PointAnnee
		cl  string
	}{{a, "l1"}, {b, "l2"}} {
		premier, dernier := p.pts[0], p.pts[len(p.pts)-1]
		yEt := yFinA
		if i == 1 {
			yEt = yFinB
		}
		fmt.Fprintf(&buf, `<circle class="pt %s" cx="%.1f" cy="%.1f" r="2.6"><title>%d — %s</title></circle>`,
			p.cl, x(premier.Annee), y(premier.Valeur), premier.Annee, template.HTMLEscapeString(format(premier.Valeur)))
		fmt.Fprintf(&buf, `<circle class="pt %s" cx="%.1f" cy="%.1f" r="2.6"><title>%d — %s</title></circle>`,
			p.cl, x(dernier.Annee), y(dernier.Valeur), dernier.Annee, template.HTMLEscapeString(format(dernier.Valeur)))
		fmt.Fprintf(&buf, `<text class="et %s" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			p.cl, x(dernier.Annee)-4, yEt-8, template.HTMLEscapeString(format(dernier.Valeur)))
	}
	fmt.Fprintf(&buf, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, debut)
	fmt.Fprintf(&buf, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, fin)
	buf.WriteString(`</svg>`)
	fmt.Fprintf(&buf, `<div class="legende"><span><i class="l1"></i>%s</span>`+
		`<span><i class="l2"></i>%s</span></div>`,
		template.HTMLEscapeString(libA), template.HTMLEscapeString(libB))
	return template.HTML(buf.String())
}
