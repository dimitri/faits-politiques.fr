package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// chargerAgeDepartRetraite : la série 2004-2022 était déjà chargée (Drees,
// âge conjoncturel moyen), en U — un creux en 2010 juste avant la réforme
// qui recule progressivement l'âge légal — mais jamais tracée. Ne réutilise
// PAS courbePaliers : cette fonction ancre volontairement son axe à zéro
// (la question qu'elle pose est « quelle part »), ce qui est le bon choix
// pour un pourcentage mais écraserait ici tout le U dans les derniers pour
// cent d'un axe 0-68 ans — un âge n'a pas de zéro qui compare. dessinerAgePaliers
// reprend le même principe de bandes de régime, avec un axe borné à la
// série elle-même.
func chargerAgeDepartRetraite(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, age_ensemble FROM core.age_depart_retraite ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []PointAnnee
	for rows.Next() {
		var p PointAnnee
		if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
			return "", err
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	paliers := []Palier{
		{De: 2004, A: 2010, Libelle: "Avant la réforme de 2010"},
		{De: 2010, A: 2022, Libelle: "Depuis la réforme de 2010"},
	}
	format := func(v float64) string { return Decimal(v, 1) + " ans" }
	return dessinerAgePaliers(pts, paliers, format), nil
}

// dessinerAgePaliers : même principe visuel que courbePaliers (bandes de
// régime datées, moyenne par bande) mais un axe borné au minimum et au
// maximum réels de la série, pas à zéro — adapté à une variable d'échelle
// (un âge), pas à une part.
func dessinerAgePaliers(pts []PointAnnee, paliers []Palier, format func(float64) string) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 52.0, 26.0

	min, max := pts[0].Valeur, pts[0].Valeur
	for _, p := range pts {
		if p.Valeur < min {
			min = p.Valeur
		}
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	marge := (max - min) * 0.15
	if marge <= 0 {
		marge = 1
	}
	bas, haut := min-marge, max+marge

	idx := map[int]int{}
	for i, p := range pts {
		idx[p.Annee] = i
	}
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(len(pts)-1) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-(v-bas)/(haut-bas)) }

	var seg strings.Builder
	for i, p := range pts {
		fmt.Fprintf(&seg, " L%.1f %.1f", x(i), y(p.Valeur))
	}
	trace := "M" + strings.TrimPrefix(strings.TrimSpace(seg.String()), "L")

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe paliers" viewBox="0 0 %.0f %.0f" role="img" aria-label="Série annuelle de %d à %d, de %s à %s">`,
		w, h, pts[0].Annee, pts[len(pts)-1].Annee, format(pts[0].Valeur), format(pts[len(pts)-1].Valeur))

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
		fmt.Fprintf(&b, `<text class="pal" x="%.1f" y="%.1f">%s</text>`, milieu, mt-30, template.HTMLEscapeString(pal.Libelle))
		fmt.Fprintf(&b, `<text class="pal moy" x="%.1f" y="%.1f">%s</text>`, milieu, mt-14, template.HTMLEscapeString(format(moy)))
	}

	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, h-mb, w-mr, h-mb)
	fmt.Fprintf(&b, `<path class="trait" d="%s"/>`, trace)

	repere := func(i int, cl string) {
		fmt.Fprintf(&b, `<circle class="pt" cx="%.1f" cy="%.1f" r="4"><title>%d — %s</title></circle>`,
			x(i), y(pts[i].Valeur), pts[i].Annee, template.HTMLEscapeString(format(pts[i].Valeur)))
		fmt.Fprintf(&b, `<text class="et%s" x="%.1f" y="%.1f">%s</text>`,
			cl, x(i), y(pts[i].Valeur)-10, template.HTMLEscapeString(format(pts[i].Valeur)))
	}
	imin := 0
	for i, p := range pts {
		if p.Valeur < pts[imin].Valeur {
			imin = i
		}
	}
	repere(0, "")
	if imin != 0 && imin != len(pts)-1 {
		repere(imin, " haut")
	}
	repere(len(pts)-1, " fin")

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, x(0), h-8, pts[0].Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, x(len(pts)-1), h-8, pts[len(pts)-1].Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type quantilesRetraite struct {
	Categorie              string
	Q10, Q25, Q50, Q75, Q90 float64
}

// chargerTauxRemplacement : trois lignes (Ensemble/Femmes/Hommes), chacune
// avec sa dispersion complète (q10 à q90) plutôt que la seule médiane — la
// dispersion EST le sujet ("taux de remplacement" cache des situations très
// inégales derrière une moyenne).
func chargerTauxRemplacement(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT categorie, taux_q10, taux_q25, taux_q50, taux_q75, taux_q90
		FROM core.taux_remplacement_retraite
		WHERE premiere_annee_retraite = 2020 AND revenu_reference = 'Niveau de vie' AND caracteristique = 'Sexe'
		ORDER BY CASE categorie WHEN 'Ensemble' THEN 0 WHEN 'Femme' THEN 1 ELSE 2 END`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var qs []quantilesRetraite
	for rows.Next() {
		var q quantilesRetraite
		if err := rows.Scan(&q.Categorie, &q.Q10, &q.Q25, &q.Q50, &q.Q75, &q.Q90); err != nil {
			return "", err
		}
		qs = append(qs, q)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(qs) == 0 {
		return "", nil
	}
	return dessinerQuantilesRetraite(qs), nil
}

// dessinerQuantilesRetraite : une boîte à moustaches par catégorie — la
// tige q10-q90, la boîte q25-q75, un trait pour la médiane — avec un repère
// vertical à 100 (pension égale au revenu d'avant la retraite), le seul
// point de comparaison que le chiffre lui-même appelle.
func dessinerQuantilesRetraite(qs []quantilesRetraite) template.HTML {
	const largeurEtiquette, mDroite, mHaut, mBas, hauteurLigne = 90.0, 16.0, 14.0, 26.0, 46.0
	const largeur = 720.0
	hauteur := mHaut + mBas + hauteurLigne*float64(len(qs))
	largeurAxe := largeur - largeurEtiquette - mDroite

	max := 0.0
	for _, q := range qs {
		if q.Q90 > max {
			max = q.Q90
		}
	}
	max = max * 1.08
	x := func(v float64) float64 { return largeurEtiquette + largeurAxe*v/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="boite-moustaches" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Dispersion du taux de remplacement à la retraite, par sexe, cohorte 2020">`, largeur, hauteur)
	fmt.Fprintf(&b, `<line class="repere-100" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		x(100), mHaut, x(100), hauteur-mBas)
	fmt.Fprintf(&b, `<text class="et-100" x="%.1f" y="%.1f">100 (revenu inchangé)</text>`, x(100), mHaut-2)
	for i, q := range qs {
		cy := mHaut + hauteurLigne*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, largeurEtiquette-10, cy+4, template.HTMLEscapeString(q.Categorie))
		fmt.Fprintf(&b, `<line class="tige" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"><title>%s : du 1ᵉʳ au 9ᵉ décile, %s à %s</title></line>`,
			x(q.Q10), cy, x(q.Q90), cy, template.HTMLEscapeString(q.Categorie), Decimal(q.Q10, 1), Decimal(q.Q90, 1))
		fmt.Fprintf(&b, `<rect class="boite" x="%.1f" y="%.1f" width="%.1f" height="16"><title>%s : entre le 1ᵉʳ et le 3ᵉ quartile, %s à %s</title></rect>`,
			x(q.Q25), cy-8, x(q.Q75)-x(q.Q25), template.HTMLEscapeString(q.Categorie), Decimal(q.Q25, 1), Decimal(q.Q75, 1))
		fmt.Fprintf(&b, `<line class="mediane" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"><title>%s : médiane %s</title></line>`,
			x(q.Q50), cy-8, x(q.Q50), cy+8, template.HTMLEscapeString(q.Categorie), Decimal(q.Q50, 1))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
