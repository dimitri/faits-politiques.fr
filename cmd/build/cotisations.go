package main

import (
	"fmt"
	"html/template"
	"strings"
)

// risqueContributif : les six risques de la protection sociale (DREES,
// comptes de la protection sociale 2024) et leur répartition
// contributif/non contributif/mixte — les mêmes chiffres, déjà publiés et
// vérifiés (docs/cotisations-et-droits.md § 4), rangés ici pour la barre
// empilée plutôt que redérivés d'une classification poste par poste (37
// postes) que ce dossier qualifie lui-même de « décision de lecture », pas
// d'un calcul objectif à rejouer depuis core.protection_sociale.
type risqueContributif struct {
	Risque                                      string
	Contributif, NonContributif, MixteNonClasse float64
}

var risquesContributifs = []risqueContributif{
	{"Vieillesse-survie", 402.9, 16.2, 7.5},
	{"Santé", 41.7, 297.2, 0},
	{"Famille", 4.5, 61.3, 0},
	{"Emploi", 37.5, 0.6, 13.0},
	{"Pauvreté-exclusion", 0, 34.0, 0},
	{"Logement", 0, 16.1, 0},
}

// dessinerContributifNonContributif : une barre par risque, empilée en
// trois segments — le même patron que dessinerSecteursBloc
// (cmd/build/union_europeenne.go), une échelle commune en milliards d'euros
// plutôt qu'en pourcentages, pour montrer aussi bien la composition que le
// poids réel de chaque risque.
func dessinerContributifNonContributif() template.HTML {
	max := 0.0
	for _, r := range risquesContributifs {
		total := r.Contributif + r.NonContributif + r.MixteNonClasse
		if total > max {
			max = total
		}
	}
	max *= 1.05
	const w, mr, ml, largeurBarre, gap = 720.0, 90.0, 150.0, 34.0, 16.0
	h := float64(len(risquesContributifs))*(largeurBarre+gap) + gap
	largeurAxe := w - ml - mr
	x := func(v float64) float64 { return largeurAxe * v / max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="barres-contributif" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Protection sociale par risque, part contributive et non contributive, 2024">`, w, h)
	for i, r := range risquesContributifs {
		y := gap + float64(i)*(largeurBarre+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+largeurBarre/2+4, template.HTMLEscapeString(r.Risque))
		xx := ml
		seg := func(cl string, v float64, libelle string) {
			if v <= 0 {
				return
			}
			largeur := x(v)
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f">`+
				`<title>%s, %s : %s Md€</title></rect>`,
				cl, xx, y, largeur, largeurBarre, template.HTMLEscapeString(r.Risque), libelle, Decimal(v, 1))
			if largeur > 40 {
				fmt.Fprintf(&b, `<text class="et-seg" x="%.1f" y="%.1f">%s</text>`, xx+largeur/2, y+largeurBarre/2+4, Decimal(v, 0))
			}
			xx += largeur
		}
		seg("contrib", r.Contributif, "contributif")
		seg("noncontrib", r.NonContributif, "non contributif")
		seg("mixte", r.MixteNonClasse, "mixte ou non classé")
	}
	for _, palier := range []float64{0, 100, 200, 300, 400} {
		if palier > max {
			continue
		}
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d Md€</text>`, ml+x(palier), h-4, int(palier))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
