package main

import (
	"fmt"
	"html/template"
	"strings"
)

// schemaHoldingMereFille : le régime mère-fille montré sur 100 000 € de
// dividendes remontés d'une filiale vers sa holding — un exemple pédagogique
// dont les pourcentages (5 % de quote-part, 25 % d'IS) sont réels et sourcés
// (docs/sci-holding-donnees.md), seul le montant de 100 000 € est arbitraire,
// choisi pour que l'arithmétique reste lisible.
func schemaHoldingMereFille() template.HTML {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w(`<svg class="circuit holding" viewBox="0 0 760 300" role="img" aria-labelledby="hold-t hold-d">`)
	w(`<title id="hold-t">Le régime mère-fille : une exonération à 95 %%, pas à 100 %%</title>`)
	w(`<desc id="hold-d">Une filiale verse 100 000 € de dividendes à sa holding. 95 000 € sont exonérés ` +
		`d'impôt sur les sociétés (régime mère-fille), mais 5 000 € (la quote-part pour frais et charges) ` +
		`sont réintégrés et taxés à 25 %%, soit 1 250 € — un taux effectif de 1,25 %%, pas zéro. Si l'argent ` +
		`reste dans la holding, rien d'autre n'est dû ; s'il est distribué à l'associé personne physique, ` +
		`le prélèvement forfaitaire unique de 30 %% s'applique alors, mais pas avant.</desc>`)
	w(`<defs><marker id="flh" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">` +
		`<path d="M0 0L10 5L0 10z" class="pointe"/></marker></defs>`)

	boite := func(x, y, wd, h float64, cl, titre, sous string) {
		w(`<rect class="noeud %s" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8"/>`, cl, x, y, wd, h)
		w(`<text class="nt" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`, x+wd/2, y+h/2-4, template.HTMLEscapeString(titre))
		w(`<text class="ns" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`, x+wd/2, y+h/2+13, template.HTMLEscapeString(sous))
	}
	fleche := func(d, cl string) { w(`<path class="flux %s" d="%s" marker-end="url(#flh)"/>`, cl, d) }
	etiq := func(x, y float64, anchor, txt string) {
		w(`<text class="et" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`, x, y, anchor, template.HTMLEscapeString(txt))
	}
	etiqFort := func(x, y float64, anchor, txt string) {
		w(`<text class="et fort" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`, x, y, anchor, template.HTMLEscapeString(txt))
	}

	boite(20, 40, 180, 90, "", "Filiale", "verse 100 000 € de dividendes")
	boite(290, 40, 180, 90, "etat", "Holding", "régime mère-fille (art. 145, 216 CGI)")
	boite(560, 40, 180, 90, "secu", "Associé personne physique", "si l'argent est distribué")

	fleche("M200,85 L290,85", "verse")
	etiq(245, 75, "middle", "100 000 €")

	etiq(380, 155, "middle", "95 000 € exonérés (95 %)")
	etiqFort(380, 172, "middle", "5 000 € de quote-part → 1 250 € d'IS (25 %)")
	etiq(380, 189, "middle", "soit un taux effectif de 1,25 %")

	fleche("M470,85 L560,85", "exo")
	etiq(515, 75, "middle", "PFU 30 %")
	etiq(515, 108, "middle", "si distribué — pas avant")

	etiq(380, 230, "middle", "Avec intégration fiscale (détention ≥ 95 %, art. 223 A CGI) :")
	etiqFort(380, 247, "middle", "quote-part à 1 % → 250 € d'IS, taux effectif de 0,25 %")

	w(`</svg>`)
	return template.HTML(b.String())
}
