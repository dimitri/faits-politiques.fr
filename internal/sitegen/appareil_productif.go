package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StatsApparatusProductive rassemble les trois volets du dossier : le
// glissement sectoriel de l'emploi sur cinquante ans (Eurostat), les
// délocalisations d'emplois détectées par le modèle Insee (une série
// annuelle à trois scénarios, une carte départementale, un tableau par
// catégorie socioprofessionnelle).
type StatsApparatusProductive struct {
	ChangeSVG                                        template.HTML
	YearStartChange, YearEndChange                   int
	OffshoringAnnualSVG                              template.HTML
	OffshoringDeptSVG                                template.HTML
	CountDepartments                                 int
	OffshoringCSPTable                               template.HTML
	TradeAutomobile, TradeTextile, TradeElectronicTV *StatsTradeSector
}

// PartnerShare : la part d'un pays partenaire dans les importations
// françaises d'un secteur, à deux dates — le couple, pas la valeur seule,
// est ce que le graphique en haltère (dumbbell) montre.
type PartnerShare struct {
	Name                 string
	Share2013, ShareLast float64
}

type StatsTradeSector struct {
	Sector                     string
	Label                      string
	YearStart, YearEnd         int
	TotalUSDStart, TotalUSDEnd float64
	Partners                   []PartnerShare
	SVG                        template.HTML
}

// loadTradeSector : additionne, par partenaire, tous les codes HS du
// secteur (le textile-habillement en a deux — bonneterie et habillement
// classique — jamais fusionnés au chargement, voir la migration 0125), puis
// retient les N partenaires les plus importants à la dernière année pour le
// graphique — le classement se fait ici, sur la donnée complète, pas au
// chargement.
func loadTradeSector(ctx context.Context, pool *pgxpool.Pool, sector, label string, topN int) (*StatsTradeSector, error) {
	// min/max sont des agrégations : la ligne existe même sans ce secteur
	// encore ingéré, avec des bornes NULL.
	var yearStartN, yearEndN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT min(annee), max(annee) FROM core.commerce_partenaire_secteur WHERE secteur=$1`, sector).
		Scan(&yearStartN, &yearEndN); err != nil {
		return nil, err
	}
	if !yearStartN.Valid {
		return nil, nil
	}
	yearStart, yearEnd := int(yearStartN.Int64), int(yearEndN.Int64)

	totals := map[int]float64{}
	rows, err := pool.Query(ctx, `
		SELECT annee, sum(valeur_usd) FROM core.commerce_partenaire_secteur
		WHERE secteur=$1 AND code_partenaire=0 GROUP BY annee`, sector)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a int
		var v float64
		if err := rows.Scan(&a, &v); err != nil {
			rows.Close()
			return nil, err
		}
		totals[a] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if totals[yearStart] == 0 || totals[yearEnd] == 0 {
		return nil, fmt.Errorf("commerce %s : total mondial manquant pour %d ou %d", sector, yearStart, yearEnd)
	}

	type values struct{ start, end float64 }
	perPartner := map[string]*values{}
	prows, err := pool.Query(ctx, `
		SELECT nom_partenaire, annee, sum(valeur_usd) FROM core.commerce_partenaire_secteur
		WHERE secteur=$1 AND code_partenaire<>0 GROUP BY nom_partenaire, annee`, sector)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var name string
		var year int
		var v float64
		if err := prows.Scan(&name, &year, &v); err != nil {
			prows.Close()
			return nil, err
		}
		if perPartner[name] == nil {
			perPartner[name] = &values{}
		}
		if year == yearStart {
			perPartner[name].start = v
		} else if year == yearEnd {
			perPartner[name].end = v
		}
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, err
	}
	prows.Close()

	var all []PartnerShare
	for name, v := range perPartner {
		all = append(all, PartnerShare{Name: name, Share2013: 100 * v.start / totals[yearStart], ShareLast: 100 * v.end / totals[yearEnd]})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ShareLast > all[j].ShareLast })
	if len(all) > topN {
		all = all[:topN]
	}
	// Le graphique se lit du plus petit au plus grand de haut en bas d'un
	// <svg> (y croissant vers le bas) : inverser l'ordre pour que le premier
	// partenaire apparaisse en haut.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	st := &StatsTradeSector{
		Sector: sector, Label: label, YearStart: yearStart, YearEnd: yearEnd,
		TotalUSDStart: totals[yearStart], TotalUSDEnd: totals[yearEnd], Partners: all,
	}
	st.SVG = drawDumbbellTrade(st)
	return st, nil
}

// drawDumbbellTrade : un graphique en haltère (dumbbell) — un point
// pour la part de marché de départ, un point pour la part d'arrivée, reliés
// par un trait — plutôt qu'une carte du monde, qui aurait mis en avant les
// plus gros volumes absolus (Allemagne, Espagne) plutôt que le déplacement
// réel vers des partenaires plus récents.
func drawDumbbellTrade(st *StatsTradeSector) template.HTML {
	if len(st.Partners) == 0 {
		return ""
	}
	const widthLabel, mRight, mTop, mBottom, heightLine = 132.0, 16.0, 10.0, 24.0, 30.0
	const width = 720.0
	height := mTop + mBottom + heightLine*float64(len(st.Partners))
	widthAxis := width - widthLabel - mRight

	max := 0.0
	for _, p := range st.Partners {
		if p.Share2013 > max {
			max = p.Share2013
		}
		if p.ShareLast > max {
			max = p.ShareLast
		}
	}
	max = max * 1.2
	x := func(pct float64) float64 { return widthLabel + widthAxis*pct/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="haltere-commerce" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Part de chaque partenaire dans les importations françaises, %s, %d et %d">`,
		width, height, template.HTMLEscapeString(st.Label), st.YearStart, st.YearEnd)
	for i, p := range st.Partners {
		cy := mTop + heightLine*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="pays" x="%.1f" y="%.1f">%s</text>`,
			widthLabel-10, cy+4, template.HTMLEscapeString(p.Name))
		fmt.Fprintf(&b, `<line class="trait" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			x(p.Share2013), cy, x(p.ShareLast), cy)
		fmt.Fprintf(&b, `<circle class="pt-debut" cx="%.1f" cy="%.1f" r="4.5"><title>%s, %d : %s %%</title></circle>`,
			x(p.Share2013), cy, template.HTMLEscapeString(p.Name), st.YearStart, template.HTMLEscapeString(Decimal(p.Share2013, 1)))
		fmt.Fprintf(&b, `<circle class="pt-fin" cx="%.1f" cy="%.1f" r="4.5"><title>%s, %d : %s %%</title></circle>`,
			x(p.ShareLast), cy, template.HTMLEscapeString(p.Name), st.YearEnd, template.HTMLEscapeString(Decimal(p.ShareLast, 1)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointSector struct {
	Year                                                      int
	PctAgriculture, PctIndustry, PctConstruction, PctServices float64
}

func loadApparatusProductive(ctx context.Context, pool *pgxpool.Pool) (*StatsApparatusProductive, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee,
		       max(emploi_milliers) FILTER (WHERE code_nace='TOTAL') AS total,
		       max(emploi_milliers) FILTER (WHERE code_nace='A') AS agri,
		       max(emploi_milliers) FILTER (WHERE code_nace='B-E') AS indus,
		       max(emploi_milliers) FILTER (WHERE code_nace='F') AS constr
		FROM core.emploi_secteur_nace
		GROUP BY annee
		HAVING max(emploi_milliers) FILTER (WHERE code_nace='TOTAL') IS NOT NULL
		ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	var pts []pointSector
	for rows.Next() {
		var year int
		var total, agri, indus, constr float64
		if err := rows.Scan(&year, &total, &agri, &indus, &constr); err != nil {
			rows.Close()
			return nil, err
		}
		if total <= 0 {
			continue
		}
		pts = append(pts, pointSector{
			Year: year, PctAgriculture: 100 * agri / total, PctIndustry: 100 * indus / total,
			PctConstruction: 100 * constr / total,
			PctServices:     100 * (total - agri - indus - constr) / total,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(pts) == 0 {
		return nil, nil
	}

	st := &StatsApparatusProductive{
		YearStartChange: pts[0].Year, YearEndChange: pts[len(pts)-1].Year,
		ChangeSVG: drawChangeSectoral(pts),
	}

	if st.OffshoringAnnualSVG, err = drawOffshoringAnnual(ctx, pool); err != nil {
		return nil, err
	}
	if st.OffshoringDeptSVG, st.CountDepartments, err = drawOffshoringDept(ctx, pool); err != nil {
		return nil, err
	}
	if st.OffshoringCSPTable, err = tableOffshoringCSP(ctx, pool); err != nil {
		return nil, err
	}
	if st.TradeAutomobile, err = loadTradeSector(ctx, pool, "automobile", "automobiles (HS 8703)", 8); err != nil {
		return nil, err
	}
	if st.TradeTextile, err = loadTradeSector(ctx, pool, "textile-habillement", "textile-habillement (HS 61+62)", 8); err != nil {
		return nil, err
	}
	if st.TradeElectronicTV, err = loadTradeSector(ctx, pool, "electronique-tv", "télévisions et écrans (HS 8528)", 8); err != nil {
		return nil, err
	}
	return st, nil
}

// drawChangeSectoral : quatre courbes (part de l'emploi total, en
// %) — agriculture, industrie (y compris énergie), construction, et
// services calculés par soustraction (total moins les trois autres : c'est
// une identité arithmétique sur des données réelles, pas une estimation).
func drawChangeSectoral(pts []pointSector) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 320.0, 34.0, 92.0, 14.0, 26.0
	n := len(pts)
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(n-1) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/100) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe glissement-sectoriel" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Part de l'emploi total par secteur, France, %d à %d">`, w, h, pts[0].Year, pts[n-1].Year)
	for _, bracket := range []float64{0, 25, 50, 75, 100} {
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(bracket), w-mr, y(bracket))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d%%</text>`, ml-6, y(bracket)+3, int(bracket))
	}
	traceLine := func(cl string, sel func(pointSector) float64, nameShort string) {
		var coords []string
		for i, p := range pts {
			coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(i), y(sel(p))))
		}
		fmt.Fprintf(&b, `<polyline class="%s" points="%s"/>`, cl, strings.Join(coords, " "))
		last := pts[n-1]
		fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%s, %d : %s %%</title></circle>`,
			cl, x(n-1), y(sel(last)), nameShort, last.Year, template.HTMLEscapeString(Decimal(sel(last), 1)))
		first := pts[0]
		fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%s, %d : %s %%</title></circle>`,
			cl, x(0), y(sel(first)), nameShort, first.Year, template.HTMLEscapeString(Decimal(sel(first), 1)))
		fmt.Fprintf(&b, `<text class="%s-lbl" x="%.1f" y="%.1f">%s</text>`, cl, x(n-1)+4, y(sel(last))+3, nameShort)
	}
	traceLine("ligne-services", func(p pointSector) float64 { return p.PctServices }, "Services")
	traceLine("ligne-industrie", func(p pointSector) float64 { return p.PctIndustry }, "Industrie")
	traceLine("ligne-construction", func(p pointSector) float64 { return p.PctConstruction }, "Construction")
	traceLine("ligne-agriculture", func(p pointSector) float64 { return p.PctAgriculture }, "Agriculture")
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, pts[0].Year)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, pts[n-1].Year)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// drawOffshoringAnnual : la bande bas-haut et la ligne centrale du
// scénario Insee pour les emplois ETP délocalisés, 2001-2017 — jamais une
// seule courbe qui ferait croire à un chiffre certain.
func drawOffshoringAnnual(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, emplois_etp_bas, emplois_etp_central, emplois_etp_haut
		FROM core.delocalisation_annuelle
		WHERE emplois_etp_central IS NOT NULL
		ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type pt struct{ Year, Bottom, Central, Top int }
	var pts []pt
	for rows.Next() {
		var p pt
		if err := rows.Scan(&p.Year, &p.Bottom, &p.Central, &p.Top); err != nil {
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

	const w, h, ml, mr, mt, mb = 720.0, 260.0, 46.0, 10.0, 14.0, 26.0
	n := len(pts)
	max := 0
	for _, p := range pts {
		if p.Top > max {
			max = p.Top
		}
	}
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(n-1) }
	y := func(v int) float64 { return mt + (h-mt-mb)*(1-float64(v)/float64(max)) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe delocalisation-annuelle" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Emplois délocalisés par an, scénarios bas à haut, %d à %d">`, w, h, pts[0].Year, pts[n-1].Year)
	var band []string
	for i, p := range pts {
		band = append(band, fmt.Sprintf("%.2f,%.2f", x(i), y(p.Top)))
	}
	for i := n - 1; i >= 0; i-- {
		band = append(band, fmt.Sprintf("%.2f,%.2f", x(i), y(pts[i].Bottom)))
	}
	fmt.Fprintf(&b, `<polygon class="bande" points="%s"/>`, strings.Join(band, " "))
	var centre []string
	for i, p := range pts {
		centre = append(centre, fmt.Sprintf("%.2f,%.2f", x(i), y(p.Central)))
	}
	fmt.Fprintf(&b, `<polyline class="ligne-centrale" points="%s"/>`, strings.Join(centre, " "))
	for i, p := range pts {
		fmt.Fprintf(&b, `<circle class="pt-central" cx="%.1f" cy="%.1f" r="2.6"><title>%d : entre %s et %s (scénario central %s)</title></circle>`,
			x(i), y(p.Central), p.Year, Count(p.Bottom), Count(p.Top), Count(p.Central))
	}
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`, ml-6, y(max)+3, Count(max))
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">0</text>`, ml-6, y(0)+3)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, pts[0].Year)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, pts[n-1].Year)
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), nil
}

// drawOffshoringDept : un cercle proportionnel par département,
// même patron que la carte IFI (internal/sitegen/ifi.go) — le cumul 1995-2017 du
// scénario central, pas une série temporelle par département.
func drawOffshoringDept(ctx context.Context, pool *pgxpool.Pool) (template.HTML, int, error) {
	rows, err := pool.Query(ctx, `
		SELECT d.nom_departement, d.emplois_delocalises_1995_2017,
		       st_x(st_transform(st_centroid(g.geom), 2154)), st_y(st_transform(st_centroid(g.geom), 2154))
		FROM core.delocalisation_departement d
		JOIN geo.contour g ON g.niveau='DEPARTEMENT' AND g.code_insee = d.code_departement
		ORDER BY d.emplois_delocalises_1995_2017`)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	type c struct {
		Name string
		Jobs int
		X, Y float64
	}
	var cs []c
	for rows.Next() {
		var v c
		if err := rows.Scan(&v.Name, &v.Jobs, &v.X, &v.Y); err != nil {
			return "", 0, err
		}
		cs = append(cs, v)
	}
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	if len(cs) == 0 {
		return "", 0, nil
	}

	// st_extent est une agrégation : la ligne existe même sans contour
	// encore ingéré, avec une valeur NULL (voir internal/sitegen/carte.go).
	var vbN sql.NullString
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu=2154) x`).Scan(&vbN); err != nil {
		return "", 0, err
	}
	vb := vbN.String

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo delocalisation" role="img" `+
		`aria-label="Emplois délocalisés par département, cumul 1995-2017">`, vb)
	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, toleranceFull)
	if err != nil {
		return "", 0, err
	}
	for depRows.Next() {
		var d string
		if err := depRows.Scan(&d); err != nil {
			depRows.Close()
			return "", 0, err
		}
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	if err := depRows.Err(); err != nil {
		depRows.Close()
		return "", 0, err
	}
	depRows.Close()
	rivers, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return "", 0, err
	}
	b.WriteString(rivers)

	radius := func(count int) float64 { return 1800 + 130*math.Sqrt(float64(count)) }
	for _, v := range cs {
		title := fmt.Sprintf("%s — %s emplois délocalisés (1995-2017)", v.Name, Count(v.Jobs))
		fmt.Fprintf(&b, `<circle class="deloc-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			v.X, -v.Y, radius(v.Jobs), template.HTMLEscapeString(title))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), len(cs), nil
}

// tableOffshoringCSP : la comparaison champ général / postes
// délocalisés par catégorie socioprofessionnelle (Figure 7 de l'étude
// Insee) — c'est ce tableau qui montre la surreprésentation des postes
// qualifiés, pas seulement ouvriers, parmi les emplois délocalisés.
func tableOffshoringCSP(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT categorie_socioprofessionnelle, part_champ_general_pct, part_postes_delocalises_pct
		FROM core.delocalisation_csp`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type line struct {
		CSP              string
		General, Delocal float64
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.CSP, &l.General, &l.Delocal); err != nil {
			return "", err
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", nil
	}
	sort.Slice(lines, func(i, j int) bool {
		return (lines[i].Delocal - lines[i].General) > (lines[j].Delocal - lines[j].General)
	})

	var t strings.Builder
	t.WriteString(`<div class="scroll"><table><thead><tr>` +
		`<th>Catégorie socioprofessionnelle</th><th>Emploi général</th>` +
		`<th>Postes délocalisés</th><th>Écart</th></tr></thead><tbody>`)
	for _, l := range lines {
		gap := l.Delocal - l.General
		signe := "+"
		if gap < 0 {
			signe = ""
		}
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s %%</td><td>%s %%</td><td class="ecart">%s%s pt</td></tr>`,
			template.HTMLEscapeString(l.CSP), Decimal(l.General, 1), Decimal(l.Delocal, 1), signe, Decimal(gap, 1))
	}
	t.WriteString(`</tbody></table></div>`)
	return template.HTML(t.String()), nil
}
