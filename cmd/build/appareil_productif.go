package main

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

// StatsAppareilProductif rassemble les trois volets du dossier : le
// glissement sectoriel de l'emploi sur cinquante ans (Eurostat), les
// délocalisations d'emplois détectées par le modèle Insee (une série
// annuelle à trois scénarios, une carte départementale, un tableau par
// catégorie socioprofessionnelle).
type StatsAppareilProductif struct {
	GlissementSVG                    template.HTML
	AnneeDebutGlissement, AnneeFinGlissement int
	DelocalisationAnnuelleSVG        template.HTML
	DelocalisationDeptSVG            template.HTML
	NbDepartements                   int
	DelocalisationCSPTable           template.HTML
	CommerceAutomobile, CommerceTextile, CommerceElectroniqueTV *StatsCommerceSecteur
}

// PartenairePart : la part d'un pays partenaire dans les importations
// françaises d'un secteur, à deux dates — le couple, pas la valeur seule,
// est ce que le graphique en haltère (dumbbell) montre.
type PartenairePart struct {
	Nom                  string
	Part2013, PartDerniere float64
}

type StatsCommerceSecteur struct {
	Secteur                        string
	Libelle                        string
	AnneeDebut, AnneeFin            int
	TotalUSDDebut, TotalUSDFin      float64
	Partenaires                     []PartenairePart
	SVG                             template.HTML
}

// chargerCommerceSecteur : additionne, par partenaire, tous les codes HS du
// secteur (le textile-habillement en a deux — bonneterie et habillement
// classique — jamais fusionnés au chargement, voir la migration 0125), puis
// retient les N partenaires les plus importants à la dernière année pour le
// graphique — le classement se fait ici, sur la donnée complète, pas au
// chargement.
func chargerCommerceSecteur(ctx context.Context, pool *pgxpool.Pool, secteur, libelle string, topN int) (*StatsCommerceSecteur, error) {
	// min/max sont des agrégations : la ligne existe même sans ce secteur
	// encore ingéré, avec des bornes NULL.
	var anneeDebutN, anneeFinN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT min(annee), max(annee) FROM core.commerce_partenaire_secteur WHERE secteur=$1`, secteur).
		Scan(&anneeDebutN, &anneeFinN); err != nil {
		return nil, err
	}
	if !anneeDebutN.Valid {
		return nil, nil
	}
	anneeDebut, anneeFin := int(anneeDebutN.Int64), int(anneeFinN.Int64)

	totaux := map[int]float64{}
	rows, err := pool.Query(ctx, `
		SELECT annee, sum(valeur_usd) FROM core.commerce_partenaire_secteur
		WHERE secteur=$1 AND code_partenaire=0 GROUP BY annee`, secteur)
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
		totaux[a] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if totaux[anneeDebut] == 0 || totaux[anneeFin] == 0 {
		return nil, fmt.Errorf("commerce %s : total mondial manquant pour %d ou %d", secteur, anneeDebut, anneeFin)
	}

	type valeurs struct{ debut, fin float64 }
	parPartenaire := map[string]*valeurs{}
	prows, err := pool.Query(ctx, `
		SELECT nom_partenaire, annee, sum(valeur_usd) FROM core.commerce_partenaire_secteur
		WHERE secteur=$1 AND code_partenaire<>0 GROUP BY nom_partenaire, annee`, secteur)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var nom string
		var annee int
		var v float64
		if err := prows.Scan(&nom, &annee, &v); err != nil {
			prows.Close()
			return nil, err
		}
		if parPartenaire[nom] == nil {
			parPartenaire[nom] = &valeurs{}
		}
		if annee == anneeDebut {
			parPartenaire[nom].debut = v
		} else if annee == anneeFin {
			parPartenaire[nom].fin = v
		}
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, err
	}
	prows.Close()

	var tous []PartenairePart
	for nom, v := range parPartenaire {
		tous = append(tous, PartenairePart{Nom: nom, Part2013: 100 * v.debut / totaux[anneeDebut], PartDerniere: 100 * v.fin / totaux[anneeFin]})
	}
	sort.Slice(tous, func(i, j int) bool { return tous[i].PartDerniere > tous[j].PartDerniere })
	if len(tous) > topN {
		tous = tous[:topN]
	}
	// Le graphique se lit du plus petit au plus grand de haut en bas d'un
	// <svg> (y croissant vers le bas) : inverser l'ordre pour que le premier
	// partenaire apparaisse en haut.
	for i, j := 0, len(tous)-1; i < j; i, j = i+1, j-1 {
		tous[i], tous[j] = tous[j], tous[i]
	}

	st := &StatsCommerceSecteur{
		Secteur: secteur, Libelle: libelle, AnneeDebut: anneeDebut, AnneeFin: anneeFin,
		TotalUSDDebut: totaux[anneeDebut], TotalUSDFin: totaux[anneeFin], Partenaires: tous,
	}
	st.SVG = dessinerHaltereCommerce(st)
	return st, nil
}

// dessinerHaltereCommerce : un graphique en haltère (dumbbell) — un point
// pour la part de marché de départ, un point pour la part d'arrivée, reliés
// par un trait — plutôt qu'une carte du monde, qui aurait mis en avant les
// plus gros volumes absolus (Allemagne, Espagne) plutôt que le déplacement
// réel vers des partenaires plus récents.
func dessinerHaltereCommerce(st *StatsCommerceSecteur) template.HTML {
	if len(st.Partenaires) == 0 {
		return ""
	}
	const largeurEtiquette, mDroite, mHaut, mBas, hauteurLigne = 132.0, 16.0, 10.0, 24.0, 30.0
	const largeur = 720.0
	hauteur := mHaut + mBas + hauteurLigne*float64(len(st.Partenaires))
	largeurAxe := largeur - largeurEtiquette - mDroite

	max := 0.0
	for _, p := range st.Partenaires {
		if p.Part2013 > max {
			max = p.Part2013
		}
		if p.PartDerniere > max {
			max = p.PartDerniere
		}
	}
	max = max * 1.2
	x := func(pct float64) float64 { return largeurEtiquette + largeurAxe*pct/max }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="haltere-commerce" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Part de chaque partenaire dans les importations françaises, %s, %d et %d">`,
		largeur, hauteur, template.HTMLEscapeString(st.Libelle), st.AnneeDebut, st.AnneeFin)
	for i, p := range st.Partenaires {
		cy := mHaut + hauteurLigne*(float64(i)+0.5)
		fmt.Fprintf(&b, `<text class="pays" x="%.1f" y="%.1f">%s</text>`,
			largeurEtiquette-10, cy+4, template.HTMLEscapeString(p.Nom))
		fmt.Fprintf(&b, `<line class="trait" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			x(p.Part2013), cy, x(p.PartDerniere), cy)
		fmt.Fprintf(&b, `<circle class="pt-debut" cx="%.1f" cy="%.1f" r="4.5"><title>%s, %d : %s %%</title></circle>`,
			x(p.Part2013), cy, template.HTMLEscapeString(p.Nom), st.AnneeDebut, template.HTMLEscapeString(Decimal(p.Part2013, 1)))
		fmt.Fprintf(&b, `<circle class="pt-fin" cx="%.1f" cy="%.1f" r="4.5"><title>%s, %d : %s %%</title></circle>`,
			x(p.PartDerniere), cy, template.HTMLEscapeString(p.Nom), st.AnneeFin, template.HTMLEscapeString(Decimal(p.PartDerniere, 1)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointSecteur struct {
	Annee                              int
	PctAgriculture, PctIndustrie, PctConstruction, PctServices float64
}

func chargerAppareilProductif(ctx context.Context, pool *pgxpool.Pool) (*StatsAppareilProductif, error) {
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
	var pts []pointSecteur
	for rows.Next() {
		var annee int
		var total, agri, indus, constr float64
		if err := rows.Scan(&annee, &total, &agri, &indus, &constr); err != nil {
			rows.Close()
			return nil, err
		}
		if total <= 0 {
			continue
		}
		pts = append(pts, pointSecteur{
			Annee: annee, PctAgriculture: 100 * agri / total, PctIndustrie: 100 * indus / total,
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

	st := &StatsAppareilProductif{
		AnneeDebutGlissement: pts[0].Annee, AnneeFinGlissement: pts[len(pts)-1].Annee,
		GlissementSVG: dessinerGlissementSectoriel(pts),
	}

	if st.DelocalisationAnnuelleSVG, err = dessinerDelocalisationAnnuelle(ctx, pool); err != nil {
		return nil, err
	}
	if st.DelocalisationDeptSVG, st.NbDepartements, err = dessinerDelocalisationDept(ctx, pool); err != nil {
		return nil, err
	}
	if st.DelocalisationCSPTable, err = tableauDelocalisationCSP(ctx, pool); err != nil {
		return nil, err
	}
	if st.CommerceAutomobile, err = chargerCommerceSecteur(ctx, pool, "automobile", "automobiles (HS 8703)", 8); err != nil {
		return nil, err
	}
	if st.CommerceTextile, err = chargerCommerceSecteur(ctx, pool, "textile-habillement", "textile-habillement (HS 61+62)", 8); err != nil {
		return nil, err
	}
	if st.CommerceElectroniqueTV, err = chargerCommerceSecteur(ctx, pool, "electronique-tv", "télévisions et écrans (HS 8528)", 8); err != nil {
		return nil, err
	}
	return st, nil
}

// dessinerGlissementSectoriel : quatre courbes (part de l'emploi total, en
// %) — agriculture, industrie (y compris énergie), construction, et
// services calculés par soustraction (total moins les trois autres : c'est
// une identité arithmétique sur des données réelles, pas une estimation).
func dessinerGlissementSectoriel(pts []pointSecteur) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 320.0, 34.0, 92.0, 14.0, 26.0
	n := len(pts)
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(n-1) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/100) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe glissement-sectoriel" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Part de l'emploi total par secteur, France, %d à %d">`, w, h, pts[0].Annee, pts[n-1].Annee)
	for _, palier := range []float64{0, 25, 50, 75, 100} {
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d%%</text>`, ml-6, y(palier)+3, int(palier))
	}
	traceLigne := func(cl string, sel func(pointSecteur) float64, nomCourt string) {
		var coords []string
		for i, p := range pts {
			coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(i), y(sel(p))))
		}
		fmt.Fprintf(&b, `<polyline class="%s" points="%s"/>`, cl, strings.Join(coords, " "))
		dernier := pts[n-1]
		fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%s, %d : %s %%</title></circle>`,
			cl, x(n-1), y(sel(dernier)), nomCourt, dernier.Annee, template.HTMLEscapeString(Decimal(sel(dernier), 1)))
		premier := pts[0]
		fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%s, %d : %s %%</title></circle>`,
			cl, x(0), y(sel(premier)), nomCourt, premier.Annee, template.HTMLEscapeString(Decimal(sel(premier), 1)))
		fmt.Fprintf(&b, `<text class="%s-lbl" x="%.1f" y="%.1f">%s</text>`, cl, x(n-1)+4, y(sel(dernier))+3, nomCourt)
	}
	traceLigne("ligne-services", func(p pointSecteur) float64 { return p.PctServices }, "Services")
	traceLigne("ligne-industrie", func(p pointSecteur) float64 { return p.PctIndustrie }, "Industrie")
	traceLigne("ligne-construction", func(p pointSecteur) float64 { return p.PctConstruction }, "Construction")
	traceLigne("ligne-agriculture", func(p pointSecteur) float64 { return p.PctAgriculture }, "Agriculture")
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, pts[0].Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, pts[n-1].Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// dessinerDelocalisationAnnuelle : la bande bas-haut et la ligne centrale du
// scénario Insee pour les emplois ETP délocalisés, 2001-2017 — jamais une
// seule courbe qui ferait croire à un chiffre certain.
func dessinerDelocalisationAnnuelle(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, emplois_etp_bas, emplois_etp_central, emplois_etp_haut
		FROM core.delocalisation_annuelle
		WHERE emplois_etp_central IS NOT NULL
		ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type pt struct{ Annee, Bas, Central, Haut int }
	var pts []pt
	for rows.Next() {
		var p pt
		if err := rows.Scan(&p.Annee, &p.Bas, &p.Central, &p.Haut); err != nil {
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
		if p.Haut > max {
			max = p.Haut
		}
	}
	x := func(i int) float64 { return ml + (w-ml-mr)*float64(i)/float64(n-1) }
	y := func(v int) float64 { return mt + (h-mt-mb)*(1-float64(v)/float64(max)) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe delocalisation-annuelle" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Emplois délocalisés par an, scénarios bas à haut, %d à %d">`, w, h, pts[0].Annee, pts[n-1].Annee)
	var bande []string
	for i, p := range pts {
		bande = append(bande, fmt.Sprintf("%.2f,%.2f", x(i), y(p.Haut)))
	}
	for i := n - 1; i >= 0; i-- {
		bande = append(bande, fmt.Sprintf("%.2f,%.2f", x(i), y(pts[i].Bas)))
	}
	fmt.Fprintf(&b, `<polygon class="bande" points="%s"/>`, strings.Join(bande, " "))
	var centre []string
	for i, p := range pts {
		centre = append(centre, fmt.Sprintf("%.2f,%.2f", x(i), y(p.Central)))
	}
	fmt.Fprintf(&b, `<polyline class="ligne-centrale" points="%s"/>`, strings.Join(centre, " "))
	for i, p := range pts {
		fmt.Fprintf(&b, `<circle class="pt-central" cx="%.1f" cy="%.1f" r="2.6"><title>%d : entre %s et %s (scénario central %s)</title></circle>`,
			x(i), y(p.Central), p.Annee, Nombre(p.Bas), Nombre(p.Haut), Nombre(p.Central))
	}
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%s</text>`, ml-6, y(max)+3, Nombre(max))
	fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">0</text>`, ml-6, y(0)+3)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, pts[0].Annee)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, pts[n-1].Annee)
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), nil
}

// dessinerDelocalisationDept : un cercle proportionnel par département,
// même patron que la carte IFI (cmd/build/ifi.go) — le cumul 1995-2017 du
// scénario central, pas une série temporelle par département.
func dessinerDelocalisationDept(ctx context.Context, pool *pgxpool.Pool) (template.HTML, int, error) {
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
		Nom     string
		Emplois int
		X, Y    float64
	}
	var cs []c
	for rows.Next() {
		var v c
		if err := rows.Scan(&v.Nom, &v.Emplois, &v.X, &v.Y); err != nil {
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
	// encore ingéré, avec une valeur NULL (voir cmd/build/carte.go).
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
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, tolPleine)
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
	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return "", 0, err
	}
	b.WriteString(fleuves)

	rayon := func(nb int) float64 { return 1800 + 130*math.Sqrt(float64(nb)) }
	for _, v := range cs {
		titre := fmt.Sprintf("%s — %s emplois délocalisés (1995-2017)", v.Nom, Nombre(v.Emplois))
		fmt.Fprintf(&b, `<circle class="deloc-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			v.X, -v.Y, rayon(v.Emplois), template.HTMLEscapeString(titre))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), len(cs), nil
}

// tableauDelocalisationCSP : la comparaison champ général / postes
// délocalisés par catégorie socioprofessionnelle (Figure 7 de l'étude
// Insee) — c'est ce tableau qui montre la surreprésentation des postes
// qualifiés, pas seulement ouvriers, parmi les emplois délocalisés.
func tableauDelocalisationCSP(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT categorie_socioprofessionnelle, part_champ_general_pct, part_postes_delocalises_pct
		FROM core.delocalisation_csp`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type ligne struct {
		CSP              string
		General, Delocal float64
	}
	var lignes []ligne
	for rows.Next() {
		var l ligne
		if err := rows.Scan(&l.CSP, &l.General, &l.Delocal); err != nil {
			return "", err
		}
		lignes = append(lignes, l)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(lignes) == 0 {
		return "", nil
	}
	sort.Slice(lignes, func(i, j int) bool {
		return (lignes[i].Delocal - lignes[i].General) > (lignes[j].Delocal - lignes[j].General)
	})

	var t strings.Builder
	t.WriteString(`<div class="scroll"><table><thead><tr>` +
		`<th>Catégorie socioprofessionnelle</th><th>Emploi général</th>` +
		`<th>Postes délocalisés</th><th>Écart</th></tr></thead><tbody>`)
	for _, l := range lignes {
		ecart := l.Delocal - l.General
		signe := "+"
		if ecart < 0 {
			signe = ""
		}
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s %%</td><td>%s %%</td><td class="ecart">%s%s pt</td></tr>`,
			template.HTMLEscapeString(l.CSP), Decimal(l.General, 1), Decimal(l.Delocal, 1), signe, Decimal(ecart, 1))
	}
	t.WriteString(`</tbody></table></div>`)
	return template.HTML(t.String()), nil
}
