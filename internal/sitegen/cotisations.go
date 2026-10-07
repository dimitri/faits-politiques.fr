package sitegen

import (
	"fmt"
	"html/template"
	"strings"
)

// riskContributory : les six risques de la protection sociale (DREES,
// comptes de la protection sociale 2024) et leur répartition
// contributif/non contributif/mixte — les mêmes chiffres, déjà publiés et
// vérifiés (docs/cotisations-et-droits.md § 4), rangés ici pour la barre
// empilée plutôt que redérivés d'une classification poste par poste (37
// postes) que ce dossier qualifie lui-même de « décision de lecture », pas
// d'un calcul objectif à rejouer depuis core.protection_sociale.
type riskContributory struct {
	Risk                                         string
	Contributory, NonContributory, MixedNonClass float64
}

var risksContributory = []riskContributory{
	{"Vieillesse-survie", 402.9, 16.2, 7.5},
	{"Santé", 41.7, 297.2, 0},
	{"Famille", 4.5, 61.3, 0},
	{"Emploi", 37.5, 0.6, 13.0},
	{"Pauvreté-exclusion", 0, 34.0, 0},
	{"Logement", 0, 16.1, 0},
}

// drawContributoryNonContributory : une barre par risque, empilée en
// trois segments — le même patron que dessinerSecteursBloc
// (internal/sitegen/union_europeenne.go), une échelle commune en milliards d'euros
// plutôt qu'en pourcentages, pour montrer aussi bien la composition que le
// poids réel de chaque risque.
func drawContributoryNonContributory() template.HTML {
	max := 0.0
	for _, r := range risksContributory {
		total := r.Contributory + r.NonContributory + r.MixedNonClass
		if total > max {
			max = total
		}
	}
	max *= 1.05
	const w, mr, ml, widthBar, gap = 720.0, 90.0, 150.0, 34.0, 16.0
	h := float64(len(risksContributory))*(widthBar+gap) + gap
	widthAxis := w - ml - mr
	x := func(v float64) float64 { return widthAxis * v / max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="barres-contributif" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Protection sociale par risque, part contributive et non contributive, 2024">`, w, h)
	for i, r := range risksContributory {
		y := gap + float64(i)*(widthBar+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+widthBar/2+4, template.HTMLEscapeString(r.Risk))
		xx := ml
		seg := func(cl string, v float64, label string) {
			if v <= 0 {
				return
			}
			width := x(v)
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f">`+
				`<title>%s, %s : %s Md€</title></rect>`,
				cl, xx, y, width, widthBar, template.HTMLEscapeString(r.Risk), label, Decimal(v, 1))
			if width > 40 {
				fmt.Fprintf(&b, `<text class="et-seg" x="%.1f" y="%.1f">%s</text>`, xx+width/2, y+widthBar/2+4, Decimal(v, 0))
			}
			xx += width
		}
		seg("contrib", r.Contributory, "contributif")
		seg("noncontrib", r.NonContributory, "non contributif")
		seg("mixte", r.MixedNonClass, "mixte ou non classé")
	}
	for _, bracket := range []float64{0, 100, 200, 300, 400} {
		if bracket > max {
			continue
		}
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d Md€</text>`, ml+x(bracket), h-4, int(bracket))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
