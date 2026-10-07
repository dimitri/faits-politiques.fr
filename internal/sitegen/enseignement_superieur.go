package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type municipalityStudents struct {
	Municipality string
	Headcount    int64
	X, Y         float64
}

type MapStudents struct {
	SVG                 template.HTML
	Year                int
	CountMunicipalities int
	Total               int64
}

// loadMapStudents : un cercle par commune, proportionnel aux effectifs
// étudiants — même patron que chargerCarteIFI (internal/sitegen/ifi.go), à partir
// des coordonnées de la source elle-même (pas d'un fond de carte
// intermédiaire). Paris apparaît par arrondissement, comme dans la source :
// pas d'agrégation ici, à la différence de la carte de l'IFI.
func loadMapStudents(ctx context.Context, pool *pgxpool.Pool) (*MapStudents, error) {
	// max(...) est une agrégation : la ligne existe même sans effectif encore
	// ingéré, avec une rentrée NULL.
	var yearN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT max(rentree) FROM core.effectifs_etudiants_commune`).Scan(&yearN); err != nil {
		return nil, err
	}
	if !yearN.Valid {
		return nil, nil
	}
	year := int(yearN.Int64)

	// st_extent est une agrégation : la ligne existe même sans contour
	// encore ingéré, avec une valeur NULL (voir internal/sitegen/carte.go).
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
		WHERE rentree=$1 AND geom IS NOT NULL ORDER BY effectif`, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var municipalities []municipalityStudents
	var total int64
	for rows.Next() {
		var c municipalityStudents
		if err := rows.Scan(&c.Municipality, &c.Headcount, &c.X, &c.Y); err != nil {
			return nil, err
		}
		municipalities = append(municipalities, c)
		total += c.Headcount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(municipalities) == 0 {
		return nil, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo etudiants" role="img" `+
		`aria-label="Effectifs étudiants par commune, %d">`, vb, year)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, toleranceFull)
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
	rivers, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(rivers)

	radius := func(count int64) float64 { return 1400 + 130*math.Sqrt(float64(count)) }
	for _, c := range municipalities {
		title := fmt.Sprintf("%s — %s étudiants (%d)", c.Municipality, Count(int(c.Headcount)), year)
		fmt.Fprintf(&b, `<circle class="etu-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			c.X, -c.Y, radius(c.Headcount), template.HTMLEscapeString(title))
	}
	b.WriteString(`</svg>`)

	return &MapStudents{SVG: template.HTML(b.String()), Year: year, CountMunicipalities: len(municipalities), Total: total}, nil
}

type pointEffort struct {
	Year     int
	Value    float64
	Estimate bool
}

// EffortSearch : DIRD/PIB France et UE27, plus les derniers points de
// DIRDE/PIB (part des entreprises) cités en texte — deux séries seulement
// dans le graphique pour rester lisible, la troisième et la quatrième
// n'ajoutant qu'un chiffre de comparaison ponctuel.
type EffortSearch struct {
	SVG                      template.HTML
	YearStart, YearEnd       int
	DirdFrLast, DirdEuLast   float64
	DirdeFrLast, DirdeEuLast float64
	Estimate                 bool
}

func loadEffortSearch(ctx context.Context, pool *pgxpool.Pool) (*EffortSearch, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, dird_pib_fr, dird_pib_ue27, dirde_pib_fr, dirde_pib_ue27, estimation
		FROM core.effort_recherche ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fr, eu []pointEffort
	var dirdeFR, dirdeEu *float64
	var estimateLast bool
	var anStart, anEnd int
	for rows.Next() {
		var year int
		var dFR, dEu, ofFR, ofEu *float64
		var estimate bool
		if err := rows.Scan(&year, &dFR, &dEu, &ofFR, &ofEu, &estimate); err != nil {
			return nil, err
		}
		if anStart == 0 {
			anStart = year
		}
		anEnd = year
		if dFR != nil {
			fr = append(fr, pointEffort{year, *dFR, estimate})
		}
		if dEu != nil {
			eu = append(eu, pointEffort{year, *dEu, estimate})
		}
		if ofFR != nil {
			dirdeFR = ofFR
		}
		if ofEu != nil {
			dirdeEu = ofEu
		}
		estimateLast = estimate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(fr) == 0 {
		return nil, nil
	}

	e := &EffortSearch{YearStart: anStart, YearEnd: anEnd, Estimate: estimateLast}
	e.DirdFrLast = fr[len(fr)-1].Value
	if len(eu) > 0 {
		e.DirdEuLast = eu[len(eu)-1].Value
	}
	if dirdeFR != nil {
		e.DirdeFrLast = *dirdeFR
	}
	if dirdeEu != nil {
		e.DirdeEuLast = *dirdeEu
	}
	e.SVG = drawEffortSearch(fr, eu, anStart, anEnd)
	return e, nil
}

func drawEffortSearch(fr, eu []pointEffort, anStart, anEnd int) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 34.0, 46.0, 14.0, 26.0
	maxVal := 0.0
	for _, p := range append(append([]pointEffort{}, fr...), eu...) {
		if p.Value > maxVal {
			maxVal = p.Value
		}
	}
	maxVal *= 1.15
	x := func(year int) float64 { return ml + (w-ml-mr)*float64(year-anStart)/float64(anEnd-anStart) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxVal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe effort-recherche" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="DIRD rapportée au PIB, France et Union européenne, %d à %d">`, w, h, anStart, anEnd)
	for _, bracket := range []float64{0, 1, 2, 3} {
		if bracket > maxVal {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(bracket), w-mr, y(bracket))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%.0f%%</text>`, ml-6, y(bracket)+3, bracket)
	}
	drawLineEffort(&b, fr, "fr", "France", x, y)
	drawLineEffort(&b, eu, "ue", "UE27", x, y)

	// Les deux étiquettes de fin de ligne se chevauchent quand les deux
	// dernières valeurs sont proches (2022-2023 : 2,19 % contre 2,11 %) —
	// on les écarte verticalement de part et d'autre de leur point milieu
	// plutôt que de laisser le texte se superposer.
	const gapMin = 11.0
	if len(fr) > 0 && len(eu) > 0 {
		yFr := y(fr[len(fr)-1].Value)
		yEu := y(eu[len(eu)-1].Value)
		if math.Abs(yFr-yEu) < gapMin {
			middle := (yFr + yEu) / 2
			if yFr <= yEu {
				yFr, yEu = middle-gapMin/2, middle+gapMin/2
			} else {
				yFr, yEu = middle+gapMin/2, middle-gapMin/2
			}
		}
		labelEffort(&b, "fr", "France", x(fr[len(fr)-1].Year), yFr)
		labelEffort(&b, "ue", "UE27", x(eu[len(eu)-1].Year), yEu)
	} else if len(fr) > 0 {
		labelEffort(&b, "fr", "France", x(fr[len(fr)-1].Year), y(fr[len(fr)-1].Value))
	} else if len(eu) > 0 {
		labelEffort(&b, "ue", "UE27", x(eu[len(eu)-1].Year), y(eu[len(eu)-1].Value))
	}

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anStart)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anEnd)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func drawLineEffort(b *strings.Builder, pts []pointEffort, class, label string, x func(int) float64, y func(float64) float64) {
	if len(pts) == 0 {
		return
	}
	var coords []string
	for _, p := range pts {
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Year), y(p.Value)))
	}
	fmt.Fprintf(b, `<polyline class="ligne-%s" points="%s"/>`, class, strings.Join(coords, " "))
	last := pts[len(pts)-1]
	titleLast := fmt.Sprintf("%s, %d : %s %% du PIB", label, last.Year, Decimal(last.Value, 2))
	if last.Estimate {
		titleLast += " (estimation)"
	}
	cl := "pt-" + class
	if last.Estimate {
		cl += " estim"
	}
	fmt.Fprintf(b, `<circle class="%s" cx="%.1f" cy="%.1f" r="2.6"><title>%s</title></circle>`,
		cl, x(last.Year), y(last.Value), template.HTMLEscapeString(titleLast))
}

func labelEffort(b *strings.Builder, class, label string, x, y float64) {
	fmt.Fprintf(b, `<text class="lbl-%s" x="%.1f" y="%.1f">%s</text>`, class, x+5, y+3, template.HTMLEscapeString(label))
}
