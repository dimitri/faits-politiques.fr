package sitegen

import (
	"fmt"
	"html/template"
	"strings"
)

// Palier : un régime daté, avec la valeur moyenne qui le caractérise.
//
// Un palier n'est pas une décoration : c'est une affirmation sur la série —
// « entre ces deux dates, la valeur tient autour de X ». Il est donc CALCULÉ
// sur les points de l'intervalle, jamais saisi à la main, et la fonction
// refuse un palier dont les bornes ne correspondent à aucun point.
type Palier struct {
	De, A   int    // années incluses
	Libelle string // ce que la période désigne, pas ce qu'elle prouve
}

// courbePaliers : une série annuelle longue, découpée en régimes datés.
//
// Motif d'exister, à côté de courbe() : sur cinquante ans, une courbe seule ne
// se lit pas. L'œil voit une montée et s'arrête là. Les paliers disent où la
// pente change et ce que vaut chaque régime — ce sont eux qui portent
// l'information, la courbe n'en est que la trace continue.
//
// L'axe est ancré à zéro pour la même raison que courbe() : la question posée
// est « quelle part », pas « quelle variation ».
//
// Aucune couleur n'est écrite ici. Tout passe par les classes CSS de .courbe,
// qui suivent --accent, --filet et --encre, donc les deux thèmes.
func courbePaliers(pts []PointAnnee, paliers []Palier, format func(float64) string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 52.0, 26.0

	var max float64
	for _, p := range pts {
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	if max <= 0 {
		return ""
	}
	// Un peu d'air au-dessus du maximum : sans cela le pic touche le bord et
	// son étiquette sort du viewBox.
	ech := max * 1.08

	idx := map[int]int{}
	for i, p := range pts {
		idx[p.Annee] = i
	}
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(len(pts)-1) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/ech) }

	var seg strings.Builder
	for i, p := range pts {
		fmt.Fprintf(&seg, " L%.1f %.1f", x(i), y(p.Valeur))
	}
	trace := "M" + strings.TrimPrefix(strings.TrimSpace(seg.String()), "L")
	aire := fmt.Sprintf("M%.1f %.1f%s L%.1f %.1f Z",
		x(0), h-mb, seg.String(), x(len(pts)-1), h-mb)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe paliers" viewBox="0 0 %.0f %.0f" role="img" aria-label="Série annuelle de %d à %d, de %s à %s">`,
		w, h, pts[0].Annee, pts[len(pts)-1].Annee,
		format(pts[0].Valeur), format(pts[len(pts)-1].Valeur))

	// Les bandes d'abord : elles passent DERRIÈRE la courbe, sinon elles la
	// voilent. Une bande sur deux est teintée — alterner suffit à séparer les
	// régimes sans tracer de frontière, qui suggérerait une rupture nette là
	// où il n'y a qu'un changement de régime.
	for n, pal := range paliers {
		i, ok := idx[pal.De]
		j, ok2 := idx[pal.A]
		if !ok || !ok2 || j <= i {
			continue
		}
		var somme float64
		for k := i; k <= j; k++ {
			somme += pts[k].Valeur
		}
		moy := somme / float64(j-i+1)
		x1, x2 := x(i), x(j)
		if n%2 == 0 {
			fmt.Fprintf(&b, `<rect class="bande" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
				x1, mt, x2-x1, h-mt-mb)
		}
		milieu := (x1 + x2) / 2
		fmt.Fprintf(&b, `<text class="pal" x="%.1f" y="%.1f">%s</text>`,
			milieu, mt-30, template.HTMLEscapeString(pal.Libelle))
		fmt.Fprintf(&b, `<text class="pal moy" x="%.1f" y="%.1f">%s</text>`,
			milieu, mt-14, template.HTMLEscapeString(format(moy)))
	}

	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)
	fmt.Fprintf(&b, `<path class="aire" d="%s"/><path class="trait" d="%s"/>`, aire, trace)

	// Les trois points qui se citent : départ, maximum, arrivée. Sur une série
	// de cinquante ans, en marquer davantage revient à ne rien marquer.
	repere := func(i int, cl string) {
		fmt.Fprintf(&b, `<circle class="pt" cx="%.1f" cy="%.1f" r="4"><title>%d — %s</title></circle>`,
			x(i), y(pts[i].Valeur), pts[i].Annee, template.HTMLEscapeString(format(pts[i].Valeur)))
		fmt.Fprintf(&b, `<text class="et%s" x="%.1f" y="%.1f">%s</text>`,
			cl, x(i), y(pts[i].Valeur)-10, template.HTMLEscapeString(format(pts[i].Valeur)))
	}
	imax := 0
	for i, p := range pts {
		if p.Valeur > pts[imax].Valeur {
			imax = i
		}
	}
	repere(0, "")
	if imax != 0 && imax != len(pts)-1 {
		repere(imax, " haut")
	}
	repere(len(pts)-1, " fin")

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, x(0), h-8, pts[0].Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`,
		x(len(pts)-1), h-8, pts[len(pts)-1].Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
