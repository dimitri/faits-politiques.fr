package sitegen

import (
	"html/template"
	"strconv"
	"strings"
)

// La marque : une gaufre de 16 cases — 9 pour, 4 contre, 2 abstentions,
// 1 non-votant. C'est une forme de graphique réelle, identique à la grille des
// sièges des pages de scrutin : la marque et la donnée sont le même objet.
// Aucun hémicycle : sa forme suggérerait un axe gauche-droite que ce site
// refuse de produire.
//
// Elle est rendue en SVG inline plutôt qu'en <img> pour que le libellé
// « faits-politiques.fr » soit du vrai texte, dans la fonte du site.
var waffle = []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 4}

func Brand(size int) template.HTML {
	color := map[int]string{1: "#54BE86", 2: "#E4756F", 3: "#D6A93F"}
	var b strings.Builder
	b.WriteString(`<svg width="` + strconv.Itoa(size) + `" height="` + strconv.Itoa(size) +
		`" viewBox="0 0 48 48" aria-hidden="true" focusable="false">` +
		`<rect width="48" height="48" rx="11" fill="#15161B"/>`)
	for i, v := range waffle {
		x := 5.2 + float64(i%4)*10.2
		y := 5.2 + float64(i/4)*10.2
		fx := strconv.FormatFloat(x, 'f', -1, 64)
		fy := strconv.FormatFloat(y, 'f', -1, 64)
		if v == 4 {
			b.WriteString(`<rect x="` + strconv.FormatFloat(x+.75, 'f', -1, 64) +
				`" y="` + strconv.FormatFloat(y+.75, 'f', -1, 64) +
				`" width="5.5" height="5.5" rx="1.6" fill="none" stroke="#918D85" stroke-width="1.5"/>`)
			continue
		}
		b.WriteString(`<rect x="` + fx + `" y="` + fy + `" width="7" height="7" rx="2" fill="` +
			color[v] + `"/>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// Grid rend un carré par siège. L'ordre à l'intérieur d'un groupe est celui
// du relevé et n'a pas de signification : la grille dit COMBIEN et DANS QUEL
// ÉTAT, jamais QUI est où.
//
// Cinq états, pas quatre : « nr » (sans position enregistrée) n'est pas un
// non-votant recensé. Sur une motion de censure, la procédure ne fait voter
// que les soutiens ; les autres députés n'ont aucune position au relevé, ce
// qui est un fait sur la donnée et non une opinion prêtée à quiconque.
func Grid(inFavor, against, abst, nonVoter, withoutPosition int) template.HTML {
	var b strings.Builder
	b.Grow((inFavor + against + abst + nonVoter + withoutPosition) * 18)
	rep := func(class string, n int) {
		for i := 0; i < n; i++ {
			b.WriteString(`<b class="` + class + `"></b>`)
		}
	}
	rep("p", inFavor)
	rep("c", against)
	rep("a", abst)
	rep("nv", nonVoter)
	rep("nr", withoutPosition)
	return template.HTML(b.String())
}

// Percent donne une largeur CSS bornée à [0,100] avec deux décimales. Sans
// borne, une donnée aberrante ferait déborder une barre hors de son cadre.
func Percent(n, total int) string {
	if total <= 0 {
		return "0"
	}
	v := float64(n) * 100 / float64(total)
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}
