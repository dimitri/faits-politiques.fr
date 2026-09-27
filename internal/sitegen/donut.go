package sitegen

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

// SegmentAnneau : une part d'un anneau proportionnel, avec sa teinte FIXE —
// jamais recalculée par valeur, pour qu'une catégorie garde sa couleur d'un
// dessin à l'autre.
type SegmentAnneau struct {
	Libelle, Couleur string
	Valeur, Part     float64 // Part en % du total, déjà calculée par l'appelant
}

// dessinerAnneau trace un anneau proportionnel générique. C'est le moteur
// commun à toutes les répartitions en donut du site : seules les couleurs, les
// valeurs et le total affiché au centre changent d'un usage à l'autre — la
// géométrie ne devrait jamais être réécrite deux fois.
func dessinerAnneau(segments []SegmentAnneau, formatValeur func(float64) string,
	totalTexte, totalLegende, ariaLabel string, seuilEtiquette float64) template.HTML {

	if len(segments) == 0 {
		return ""
	}
	// cx est décalé vers la gauche : la plus grosse part tombe souvent vers
	// 3 heures et son étiquette, ancrée à gauche, a besoin de place à droite —
	// sans cette marge, un libellé long sort du viewBox et se fait couper.
	const cx, cy, r, sw = 140.0, 120.0, 74.0, 34.0
	circonf := 2 * math.Pi * r

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="donut" viewBox="0 0 400 240" role="img" aria-label="%s">`,
		template.HTMLEscapeString(ariaLabel))

	cumul := 0.0
	for _, seg := range segments {
		part := seg.Part / 100
		dash := part * circonf
		coul := seg.Couleur
		if coul == "" {
			coul = "#8A7F6B"
		}
		fmt.Fprintf(&b, `<circle class="tranche" cx="%.0f" cy="%.0f" r="%.1f" `+
			`fill="none" stroke="%s" stroke-width="%.0f" `+
			`stroke-dasharray="%.2f %.2f" stroke-dashoffset="%.2f" `+
			`transform="rotate(-90 %.0f %.0f)"><title>%s — %s (%s%%)</title></circle>`,
			cx, cy, r, coul, sw, dash, circonf-dash, -cumul*circonf, cx, cy,
			template.HTMLEscapeString(seg.Libelle), template.HTMLEscapeString(formatValeur(seg.Valeur)),
			template.HTMLEscapeString(Decimal(seg.Part, 1)))

		if seg.Part >= seuilEtiquette {
			mid := (cumul + part/2) * 2 * math.Pi
			ang := mid - math.Pi/2 // on part du haut, sens horaire
			lr := r + sw/2 + 20
			lx := cx + lr*math.Cos(ang)
			ly := cy + lr*math.Sin(ang)
			anchor := "middle"
			if lx > cx+8 {
				anchor = "start"
			} else if lx < cx-8 {
				anchor = "end"
			}
			fmt.Fprintf(&b, `<text class="tr-lib" x="%.1f" y="%.1f" text-anchor="%s">%s</text>`,
				lx, ly-5, anchor, template.HTMLEscapeString(seg.Libelle))
			fmt.Fprintf(&b, `<text class="tr-val" x="%.1f" y="%.1f" text-anchor="%s">%s</text>`,
				lx, ly+10, anchor, template.HTMLEscapeString(Decimal(seg.Part, 1))+" %")
		}
		cumul += part
	}
	fmt.Fprintf(&b, `<text class="don-total" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`,
		cx, cy-4, template.HTMLEscapeString(totalTexte))
	fmt.Fprintf(&b, `<text class="don-total-l" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`,
		cx, cy+14, template.HTMLEscapeString(totalLegende))
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// donutRisques dessine l'anneau des six risques de la protection sociale —
// une seconde lecture du tableau qui le précède, jamais un remplacement.
//
// Six teintes FIXES par code de risque. Aucune ne reprend le vert, le rouge ou
// l'ocre — réservés aux positions de vote — ni la rampe séquentielle des
// cartes, réservée aux magnitudes.
var couleurRisque = map[string]string{
	"E11-2": "#1E5C69", // Vieillesse-survie
	"E11-1": "#4A8894", // Santé
	"E11-3": "#6B5CA5", // Famille
	"E11-4": "#3D6FA0", // Emploi
	"E11-6": "#A34F86", // Pauvreté-exclusion sociale
	"E11-5": "#8A7F6B", // Logement
}

func donutRisques(risques []RisqueSocial, total float64, formatTotal string) template.HTML {
	if total <= 0 {
		return ""
	}
	var segs []SegmentAnneau
	for _, r0 := range risques {
		segs = append(segs, SegmentAnneau{
			Libelle: r0.Libelle, Couleur: couleurRisque[r0.Code],
			Valeur: r0.Montant, Part: r0.Part,
		})
	}
	return dessinerAnneau(segs, mdEur, formatTotal, "prestations",
		"Répartition des prestations entre les six risques, total "+formatTotal, 6)
}

// fluxPartageVA : le partage de la valeur ajoutée en flux, pour UNE année —
// une seule source (la valeur ajoutée produite) qui se scinde en trois
// bandes proportionnelles vers trois destinations, dont une seule est un
// dividende potentiel : l'excédent brut d'exploitation, avant tout partage
// entre actionnaires, prêteurs et investissement. Toujours les mêmes trois
// teintes, dans le même ordre, pour comparer deux années d'un coup d'œil.
var couleursPartageVA = []string{"#4A8894", "#B0763A", "#1E5C69"} // Rémunération, Impôts, EBE

func fluxPartageVA(annee int, remun, impots, ebe, va float64) template.HTML {
	if va <= 0 {
		return ""
	}
	type cible struct {
		Libelle, Sous, Couleur string
		Valeur                 float64
	}
	cibles := []cible{
		{"Rémunération des salariés", "salaires et cotisations", couleursPartageVA[0], remun},
		{"Impôts sur la production", "net des subventions", couleursPartageVA[1], impots},
		{"Excédent brut d'exploitation", "dont une part en dividendes", couleursPartageVA[2], ebe},
	}

	const hTotal = 150.0 // hauteur représentant les 100 % de VA
	const yTop = 34.0
	const xSrcL, xSrcR = 14.0, 104.0
	const xDstL, xDstR = 250.0, 280.0
	const xm = (xSrcR + xDstL) / 2
	const gap = 5.0

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="flux-va" viewBox="0 0 460 220" role="img" `+
		`aria-label="Partage de la valeur ajoutée en %d, total %s : %s"><defs>`+
		`<marker id="fva%d" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" `+
		`orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" class="fva-pointe"/></marker></defs>`,
		annee, mdEur(va), template.HTMLEscapeString(fmt.Sprintf(
			"%s rémunération (%s), %s impôts sur la production (%s), %s excédent brut d'exploitation (%s)",
			Decimal(100*remun/va, 1)+" %", mdEur(remun), Decimal(100*impots/va, 1)+" %",
			mdEur(impots), Decimal(100*ebe/va, 1)+" %", mdEur(ebe))), annee)

	fmt.Fprintf(&b, `<rect class="fva-source" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="6"/>`,
		xSrcL, yTop, xSrcR-xSrcL, hTotal)
	fmt.Fprintf(&b, `<text class="fva-t fva-src-t" x="%.0f" y="%.0f" text-anchor="middle">%d</text>`,
		(xSrcL+xSrcR)/2, yTop+hTotal/2-6, annee)
	fmt.Fprintf(&b, `<text class="fva-s" x="%.0f" y="%.0f" text-anchor="middle">valeur ajoutée</text>`,
		(xSrcL+xSrcR)/2, yTop+hTotal/2+11)

	cum := 0.0
	for i, c := range cibles {
		part := 100 * c.Valeur / va
		h := hTotal * part / 100
		if h < 0 {
			h = 0 // un impôt net négatif (subventions > impôts) n'a pas de largeur à dessiner
		}
		y0t, y0b := yTop+cum, yTop+cum+h
		// Bande à largeur constante : la courbe du haut va du bord source au
		// bord destination À LA MÊME HAUTEUR (y0t), celle du bas les relie en
		// sens inverse (y0b) — un bord vertical droit à chaque bout, comme le
		// faisceau des exonérations du circuit budgétaire.
		fmt.Fprintf(&b, `<path class="fva-ruban r%d" d="M%.1f,%.1f C%.1f,%.1f %.1f,%.1f %.1f,%.1f `+
			`L%.1f,%.1f C%.1f,%.1f %.1f,%.1f %.1f,%.1f Z" marker-end="url(#fva%d)"><title>%s — %s (%s %%)</title></path>`,
			i, xSrcR, y0t, xm, y0t, xm, y0t, xDstL, y0t,
			xDstL, y0b, xm, y0b, xm, y0b, xSrcR, y0b, annee,
			template.HTMLEscapeString(c.Libelle), template.HTMLEscapeString(mdEur(c.Valeur)),
			Decimal(part, 1))
		by0, by1 := y0t, y0b
		if by1-by0 > gap*2 {
			by0, by1 = by0+gap/2, by1-gap/2
		}
		fmt.Fprintf(&b, `<rect class="fva-dest r%d" x="%.0f" y="%.1f" width="%.0f" height="%.1f" rx="4"/>`,
			i, xDstL, by0, xDstR-xDstL, by1-by0)
		my := (by0 + by1) / 2
		// L'étiquette part toujours À DROITE de la case, jamais centrée dedans :
		// une case de 90 px de large ne tient pas « Excédent brut d'exploitation ».
		// En dessous de 18 px (les impôts sur la production avoisinent 2 % de la
		// VA), deux lignes de texte se chevaucheraient avec le libellé voisin —
		// la bande colorée et la légende suffisent, le montant exact reste dans
		// le tableau qui suit.
		if h >= 18 {
			fmt.Fprintf(&b, `<text class="fva-dt" x="%.0f" y="%.1f">%s</text>`,
				xDstR+8, my-2, template.HTMLEscapeString(c.Libelle))
			fmt.Fprintf(&b, `<text class="fva-ds" x="%.0f" y="%.1f">%s — %s %%</text>`,
				xDstR+8, my+13, mdEur(c.Valeur), Decimal(part, 1))
		}
		cum += h
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
