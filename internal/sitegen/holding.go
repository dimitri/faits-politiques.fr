package sitegen

import (
	"fmt"
	"html/template"
	"strings"
)

// schemaHoldingParentSubsidiary : le régime mère-fille montré depuis le bénéfice
// AVANT impôt de la filiale — pas depuis le dividende déjà net d'IS, un point
// presque toujours absent du débat public (voir docs/sci-holding-donnees.md) :
// un dividende n'est jamais de l'argent qui n'a pas encore été taxé. Les
// pourcentages (25 % d'IS, 5 % de quote-part) sont réels et sourcés ; seul le
// montant de 100 000 € de bénéfice initial est arbitraire, choisi pour que
// l'arithmétique reste lisible.
func schemaHoldingParentSubsidiary() template.HTML {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w(`<svg class="circuit holding" viewBox="0 0 800 380" role="img" aria-labelledby="hold-t hold-d">`)
	w(`<title id="hold-t">L'impôt déjà payé avant que le dividende n'existe</title>`)
	w(`<desc id="hold-d">Une filiale réalise 100 000 € de bénéfice avant impôt. Elle paie d'abord 25 000 € d'impôt ` +
		`sur les sociétés (25 %%) : il ne reste que 75 000 € à distribuer à la holding. Sur cette somme, 95 %% sont ` +
		`exonérés d'impôt sur les sociétés (régime mère-fille), mais 5 %% (la quote-part pour frais et charges) sont ` +
		`réintégrés et taxés à 25 %%, soit 937,50 € de plus. Au total, 25 937,50 € — près de 26 %% du bénéfice ` +
		`initial — sont déjà payés avant que l'argent ne soit ne serait-ce que dans la holding. Si le solde est ` +
		`ensuite distribué à l'associé personne physique, le prélèvement forfaitaire unique de 30 %% s'applique à ` +
		`son tour, portant le total cumulé à environ 48 %% du bénéfice initial.</desc>`)
	w(`<defs><marker id="flh" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">` +
		`<path d="M0 0L10 5L0 10z" class="pointe"/></marker></defs>`)

	box := func(x, y, wd, h float64, cl, title, sub string) {
		w(`<rect class="noeud %s" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8"/>`, cl, x, y, wd, h)
		w(`<text class="nt" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`, x+wd/2, y+h/2-4, template.HTMLEscapeString(title))
		w(`<text class="ns" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`, x+wd/2, y+h/2+13, template.HTMLEscapeString(sub))
	}
	arrow := func(d, cl string) { w(`<path class="flux %s" d="%s" marker-end="url(#flh)"/>`, cl, d) }
	label := func(x, y float64, anchor, txt string) {
		w(`<text class="et" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`, x, y, anchor, template.HTMLEscapeString(txt))
	}
	labelStrong := func(x, y float64, anchor, txt string) {
		w(`<text class="et fort" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`, x, y, anchor, template.HTMLEscapeString(txt))
	}

	box(20, 30, 180, 90, "", "Filiale", "100 000 € de bénéfice avant IS")
	box(310, 30, 180, 90, "etat", "Holding", "régime mère-fille (art. 145, 216 CGI)")
	box(600, 30, 180, 90, "secu", "Associé personne physique", "si l'argent est distribué")

	// IS de la filiale, déjà payé avant que le dividende n'existe.
	label(110, 145, "middle", "− 25 000 € d'IS (25 %)")
	labelStrong(110, 162, "middle", "déjà payés avant toute distribution")

	arrow("M200,75 L310,75", "verse")
	label(255, 65, "middle", "75 000 € distribuables")

	label(400, 145, "middle", "95 % exonérés (régime mère-fille)")
	labelStrong(400, 162, "middle", "5 % de quote-part → 937,50 € d'IS (25 %)")
	label(400, 179, "middle", "soit 1,25 % sur les 75 000 € remontés")

	labelStrong(400, 205, "middle", "Cumul déjà payé : 25 937,50 € (≈ 26 % du bénéfice initial)")
	label(400, 222, "middle", "Avec intégration fiscale (art. 223 A CGI) : cumul ≈ 25 187,50 € (≈ 25 %)")

	arrow("M490,75 L600,75", "exo")
	label(545, 65, "middle", "PFU 30 %")
	label(545, 98, "middle", "si distribué — pas avant")

	labelStrong(400, 260, "middle", "Si le solde est ensuite distribué à l'associé : total cumulé ≈ 48 156 € (≈ 48 % du bénéfice initial)")

	w(`</svg>`)
	return template.HTML(b.String())
}
