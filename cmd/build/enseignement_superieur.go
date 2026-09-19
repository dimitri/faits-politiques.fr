package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type communeEtudiants struct {
	Commune  string
	Effectif int64
	X, Y     float64
}

type CarteEtudiants struct {
	SVG        template.HTML
	Annee      int
	NbCommunes int
	Total      int64
}

// chargerCarteEtudiants : un cercle par commune, proportionnel aux effectifs
// étudiants — même patron que chargerCarteIFI (cmd/build/ifi.go), à partir
// des coordonnées de la source elle-même (pas d'un fond de carte
// intermédiaire). Paris apparaît par arrondissement, comme dans la source :
// pas d'agrégation ici, à la différence de la carte de l'IFI.
func chargerCarteEtudiants(ctx context.Context, pool *pgxpool.Pool) (*CarteEtudiants, error) {
	// max(...) est une agrégation : la ligne existe même sans effectif encore
	// ingéré, avec une rentrée NULL.
	var anneeN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT max(rentree) FROM core.effectifs_etudiants_commune`).Scan(&anneeN); err != nil {
		return nil, err
	}
	if !anneeN.Valid {
		return nil, nil
	}
	annee := int(anneeN.Int64)

	// st_extent est une agrégation : la ligne existe même sans contour
	// encore ingéré, avec une valeur NULL (voir cmd/build/carte.go).
	var vbN sql.NullString
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu=2154) x`).Scan(&vbN); err != nil {
		return nil, err
	}
	vb := vbN.String

	rows, err := pool.Query(ctx, `
		SELECT commune, effectif, st_x(st_transform(geom,2154)), st_y(st_transform(geom,2154))
		FROM core.effectifs_etudiants_commune
		WHERE rentree=$1 AND geom IS NOT NULL ORDER BY effectif`, annee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var communes []communeEtudiants
	var total int64
	for rows.Next() {
		var c communeEtudiants
		if err := rows.Scan(&c.Commune, &c.Effectif, &c.X, &c.Y); err != nil {
			return nil, err
		}
		communes = append(communes, c)
		total += c.Effectif
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(communes) == 0 {
		return nil, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo etudiants" role="img" `+
		`aria-label="Effectifs étudiants par commune, %d">`, vb, annee)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, tolPleine)
	if err != nil {
		return nil, err
	}
	for depRows.Next() {
		var d string
		if err := depRows.Scan(&d); err != nil {
			depRows.Close()
			return nil, err
		}
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	if err := depRows.Err(); err != nil {
		depRows.Close()
		return nil, err
	}
	depRows.Close()
	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(fleuves)

	rayon := func(nb int64) float64 { return 1400 + 130*math.Sqrt(float64(nb)) }
	for _, c := range communes {
		titre := fmt.Sprintf("%s — %s étudiants (%d)", c.Commune, Nombre(int(c.Effectif)), annee)
		fmt.Fprintf(&b, `<circle class="etu-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			c.X, -c.Y, rayon(c.Effectif), template.HTMLEscapeString(titre))
	}
	b.WriteString(`</svg>`)

	return &CarteEtudiants{SVG: template.HTML(b.String()), Annee: annee, NbCommunes: len(communes), Total: total}, nil
}

type pointEffort struct {
	Annee      int
	Valeur     float64
	Estimation bool
}

// EffortRecherche : DIRD/PIB France et UE27, plus les derniers points de
// DIRDE/PIB (part des entreprises) cités en texte — deux séries seulement
// dans le graphique pour rester lisible, la troisième et la quatrième
// n'ajoutant qu'un chiffre de comparaison ponctuel.
type EffortRecherche struct {
	SVG                            template.HTML
	AnneeDebut, AnneeFin           int
	DirdFrDernier, DirdUeDernier   float64
	DirdeFrDernier, DirdeUeDernier float64
	Estimation                     bool
}

func chargerEffortRecherche(ctx context.Context, pool *pgxpool.Pool) (*EffortRecherche, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, dird_pib_fr, dird_pib_ue27, dirde_pib_fr, dirde_pib_ue27, estimation
		FROM core.effort_recherche ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fr, ue []pointEffort
	var dirdeFR, dirdeUE *float64
	var estimDerniere bool
	var anDebut, anFin int
	for rows.Next() {
		var annee int
		var dFR, dUE, deFR, deUE *float64
		var estim bool
		if err := rows.Scan(&annee, &dFR, &dUE, &deFR, &deUE, &estim); err != nil {
			return nil, err
		}
		if anDebut == 0 {
			anDebut = annee
		}
		anFin = annee
		if dFR != nil {
			fr = append(fr, pointEffort{annee, *dFR, estim})
		}
		if dUE != nil {
			ue = append(ue, pointEffort{annee, *dUE, estim})
		}
		if deFR != nil {
			dirdeFR = deFR
		}
		if deUE != nil {
			dirdeUE = deUE
		}
		estimDerniere = estim
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(fr) == 0 {
		return nil, nil
	}

	e := &EffortRecherche{AnneeDebut: anDebut, AnneeFin: anFin, Estimation: estimDerniere}
	e.DirdFrDernier = fr[len(fr)-1].Valeur
	if len(ue) > 0 {
		e.DirdUeDernier = ue[len(ue)-1].Valeur
	}
	if dirdeFR != nil {
		e.DirdeFrDernier = *dirdeFR
	}
	if dirdeUE != nil {
		e.DirdeUeDernier = *dirdeUE
	}
	e.SVG = dessinerEffortRecherche(fr, ue, anDebut, anFin)
	return e, nil
}

func dessinerEffortRecherche(fr, ue []pointEffort, anDebut, anFin int) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 34.0, 46.0, 14.0, 26.0
	maxVal := 0.0
	for _, p := range append(append([]pointEffort{}, fr...), ue...) {
		if p.Valeur > maxVal {
			maxVal = p.Valeur
		}
	}
	maxVal *= 1.15
	x := func(annee int) float64 { return ml + (w-ml-mr)*float64(annee-anDebut)/float64(anFin-anDebut) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxVal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe effort-recherche" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="DIRD rapportée au PIB, France et Union européenne, %d à %d">`, w, h, anDebut, anFin)
	for _, palier := range []float64{0, 1, 2, 3} {
		if palier > maxVal {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%.0f%%</text>`, ml-6, y(palier)+3, palier)
	}
	tracerLigneEffort(&b, fr, "fr", "France", x, y)
	tracerLigneEffort(&b, ue, "ue", "UE27", x, y)

	// Les deux étiquettes de fin de ligne se chevauchent quand les deux
	// dernières valeurs sont proches (2022-2023 : 2,19 % contre 2,11 %) —
	// on les écarte verticalement de part et d'autre de leur point milieu
	// plutôt que de laisser le texte se superposer.
	const ecartMin = 11.0
	if len(fr) > 0 && len(ue) > 0 {
		yFr := y(fr[len(fr)-1].Valeur)
		yUe := y(ue[len(ue)-1].Valeur)
		if math.Abs(yFr-yUe) < ecartMin {
			milieu := (yFr + yUe) / 2
			if yFr <= yUe {
				yFr, yUe = milieu-ecartMin/2, milieu+ecartMin/2
			} else {
				yFr, yUe = milieu+ecartMin/2, milieu-ecartMin/2
			}
		}
		etiquetteEffort(&b, "fr", "France", x(fr[len(fr)-1].Annee), yFr)
		etiquetteEffort(&b, "ue", "UE27", x(ue[len(ue)-1].Annee), yUe)
	} else if len(fr) > 0 {
		etiquetteEffort(&b, "fr", "France", x(fr[len(fr)-1].Annee), y(fr[len(fr)-1].Valeur))
	} else if len(ue) > 0 {
		etiquetteEffort(&b, "ue", "UE27", x(ue[len(ue)-1].Annee), y(ue[len(ue)-1].Valeur))
	}

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anDebut)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anFin)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func tracerLigneEffort(b *strings.Builder, pts []pointEffort, classe, label string, x func(int) float64, y func(float64) float64) {
	if len(pts) == 0 {
		return
	}
	var coords []string
	for _, p := range pts {
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Annee), y(p.Valeur)))
	}
	fmt.Fprintf(b, `<polyline class="ligne-%s" points="%s"/>`, classe, strings.Join(coords, " "))
	dernier := pts[len(pts)-1]
	titreDernier := fmt.Sprintf("%s, %d : %s %% du PIB", label, dernier.Annee, Decimal(dernier.Valeur, 2))
	if dernier.Estimation {
		titreDernier += " (estimation)"
	}
	cl := "pt-" + classe
	if dernier.Estimation {
		cl += " estim"
	}
	fmt.Fprintf(b, `<circle class="%s" cx="%.1f" cy="%.1f" r="2.6"><title>%s</title></circle>`,
		cl, x(dernier.Annee), y(dernier.Valeur), template.HTMLEscapeString(titreDernier))
}

func etiquetteEffort(b *strings.Builder, classe, label string, x, y float64) {
	fmt.Fprintf(b, `<text class="lbl-%s" x="%.1f" y="%.1f">%s</text>`, classe, x+5, y+3, template.HTMLEscapeString(label))
}
