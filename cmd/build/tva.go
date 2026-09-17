package main

import (
	"fmt"
	"html/template"
	"strings"
)

// SchemaTVAEntreprises : comment la collecte par étages ne taxe que la
// valeur ajoutée, montrée sur une chaîne à trois entreprises — agriculteur,
// transformateur, distributeur — jusqu'au consommateur final.
//
// Les montants sont un EXEMPLE PÉDAGOGIQUE, explicitement étiqueté comme tel :
// aucune source ne publie de chaîne de transactions réelles à ce niveau de
// détail par filière (voir docs/tva-donnees.md). Ce que le schéma prouve n'en
// est pas moins réel : la somme des « TVA nette versée » à chaque étage
// retombe exactement sur la TVA payée par le consommateur final, quel que
// soit le nombre d'intermédiaires — la preuve chiffrée que seule la valeur
// ajoutée est taxée.
func schemaTVAEntreprises() template.HTML {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w(`<svg class="circuit tva" viewBox="0 0 800 470" role="img" aria-labelledby="tva-t tva-d">`)
	w(`<title id="tva-t">La TVA ne taxe que la valeur ajoutée à chaque étage</title>`)
	w(`<desc id="tva-d">Exemple pédagogique : un agriculteur vend pour 100 € HT, un transformateur pour 250 € HT, ` +
		`un distributeur pour 400 € HT au consommateur final (480 € TTC). À chaque étage, la TVA collectée sur la ` +
		`vente moins la TVA déductible sur les achats donne la TVA nette versée à l'État : 20 €, 30 € et 30 €, soit ` +
		`80 € au total — exactement la TVA payée par le consommateur final.</desc>`)
	w(`<defs><marker id="flt" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">` +
		`<path d="M0 0L10 5L0 10z" class="pointe"/></marker></defs>`)

	boite := func(x, y, wd, h float64, cl, titre, sous string) {
		w(`<rect class="noeud %s" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8"/>`, cl, x, y, wd, h)
		w(`<text class="nt" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`, x+wd/2, y+h/2-4, template.HTMLEscapeString(titre))
		w(`<text class="ns" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`, x+wd/2, y+h/2+13, template.HTMLEscapeString(sous))
	}
	fleche := func(d, cl string) { w(`<path class="flux %s" d="%s" marker-end="url(#flt)"/>`, cl, d) }
	etiq := func(x, y float64, anchor, txt string) {
		w(`<text class="et" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`, x, y, anchor, template.HTMLEscapeString(txt))
	}
	etiqFort := func(x, y float64, anchor, txt string) {
		w(`<text class="et fort" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`, x, y, anchor, template.HTMLEscapeString(txt))
	}

	type etage struct {
		nom, sousNom          string
		collectee, deductible float64
	}
	etages := []etage{
		{"Agriculteur", "vend 100 € HT", 20, 0},
		{"Transformateur", "vend 250 € HT", 50, 20},
		{"Distributeur", "vend 400 € HT", 80, 50},
	}

	const (
		boxW, boxH = 160.0, 90.0
		boxY       = 30.0
		gap        = 40.0
	)
	xs := []float64{20, 20 + boxW + gap, 20 + 2*(boxW+gap), 20 + 3*(boxW+gap)}

	// Les trois entreprises, puis le consommateur final.
	for i, e := range etages {
		boite(xs[i], boxY, boxW, boxH, "", e.nom, e.sousNom)
	}
	boite(xs[3], boxY, boxW, boxH, "secu", "Consommateur final", "paie 480 € TTC")

	// Flèches de vente entre étages, avec le prix HT au-dessus.
	for i := 0; i < 3; i++ {
		x1, x2 := xs[i]+boxW, xs[i+1]
		y := boxY + boxH/2
		fleche(fmt.Sprintf("M%.0f,%.0f L%.0f,%.0f", x1, y, x2, y), "verse")
	}
	etiq(xs[0]+boxW+gap/2, boxY+boxH/2-8, "middle", "100 € HT")
	etiq(xs[1]+boxW+gap/2, boxY+boxH/2-8, "middle", "250 € HT")
	etiqFort(xs[2]+boxW+gap/2, boxY+boxH/2-8, "middle", "400 € HT")

	// Une flèche verticale par entreprise vers l'État, avec le détail
	// collectée / déductible / nette.
	etatY := 380.0
	for i, e := range etages {
		xc := xs[i] + boxW/2
		fleche(fmt.Sprintf("M%.0f,%.0f L%.0f,%.0f", xc, boxY+boxH, xc, etatY), "exo")
		yLabel := boxY + boxH + 30
		etiq(xc, yLabel, "middle", fmt.Sprintf("TVA collectée : %s €", Nombre(int64(e.collectee))))
		if e.deductible > 0 {
			etiq(xc, yLabel+15, "middle", fmt.Sprintf("− déductible : %s €", Nombre(int64(e.deductible))))
		} else {
			etiq(xc, yLabel+15, "middle", "− déductible : 0 €")
		}
		etiqFort(xc, yLabel+32, "middle", fmt.Sprintf("= nette versée : %s €", Nombre(int64(e.collectee-e.deductible))))
	}

	boite(xs[1]-gap/2, etatY, boxW+gap+boxW, 60, "etat", "État", "20 € + 30 € + 30 € = 80 € reçus")
	etiqFort((xs[1]-gap/2+xs[2]+boxW+gap/2)/2, etatY+82, "middle",
		"Exactement les 80 € de TVA payés par le consommateur final")

	w(`</svg>`)
	return template.HTML(b.String())
}
