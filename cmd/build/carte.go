package main

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cartes choroplèthes rendues en SVG par la base.
//
// Aucune bibliothèque de cartographie, aucun serveur de tuiles, aucune requête
// vers un tiers : PostGIS projette et écrit le tracé, le gabarit l'affiche. La
// contrepartie de l'ODbL est que « © les contributeurs OpenStreetMap » doit
// accompagner CHAQUE carte affichée — la fonction l'inclut donc elle-même.
//
// La rampe est séquentielle, une seule teinte du clair au foncé : clarté OKLab
// monotone, pas d'au moins 9. Une rampe arc-en-ciel inventerait des ruptures.
var rampe = []string{"#DCE9EC", "#B0CFD5", "#7FB0BA", "#4A8894", "#1E5C69"}

// absentFill : « aucune donnée » n'est pas « la valeur la plus basse ». Un
// département où aucune liste ne s'est présentée n'a pas fait zéro pour cent.
const absentFill = "#EFEBE2"

type CaseCarte struct {
	Code, Nom string
	Valeur    float64
	Absent    bool
}

type Carte struct {
	SVG       template.HTML
	Bornes    []string
	Teintes   []string
	Vide      bool
	Unite     string
	Total     int
	NbAbsents int
}

// contoursDept renvoie le tracé SVG de chaque département métropolitain, en
// Lambert-93. Les outre-mer ont chacun leur projection et se dessinent en
// cartons : ils ne partagent pas ce repère.
func contoursDept(ctx context.Context, pool *pgxpool.Pool, tolerance float64) (map[string]string, []string, string, error) {
	rows, err := pool.Query(ctx, `
		SELECT code_insee, nom,
		       st_assvg(st_transform(st_simplifypreservetopology(geom, $1), 2154), 0, 0)
		FROM geo.contour
		WHERE niveau = 'DEPARTEMENT' AND code_insee !~ '^97'
		ORDER BY code_insee`, tolerance)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()
	d := map[string]string{}
	noms := map[string]string{}
	var codes []string
	for rows.Next() {
		var c, n, p string
		if err := rows.Scan(&c, &n, &p); err != nil {
			return nil, nil, "", err
		}
		d[c], noms[c] = p, n
		codes = append(codes, c)
	}
	_ = noms
	// La boîte englobante est fixe : elle ne dépend pas des données affichées,
	// donc toutes les cartes du site se superposent exactement.
	var vb string
	err = pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND code_insee !~ '^97') x`).Scan(&vb)
	return d, codes, vb, err
}

// choroplethe assemble une carte à partir de valeurs par code de département.
// Les classes sont des quintiles calculés sur les seules valeurs présentes.
func choroplethe(contours map[string]string, vb string, cases []CaseCarte,
	unite string, format func(float64) string) Carte {

	var vals []float64
	byCode := map[string]CaseCarte{}
	for _, c := range cases {
		byCode[c.Code] = c
		if !c.Absent {
			vals = append(vals, c.Valeur)
		}
	}
	if len(vals) == 0 {
		return Carte{Vide: true}
	}
	sort.Float64s(vals)

	// Autant de classes que la rampe en propose, mais jamais plus qu'il n'y a
	// de valeurs distinctes : cinq quintiles sur trois départements
	// produiraient des classes vides et un débordement d'indice.
	distinct := 1
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1] {
			distinct++
		}
	}
	nc := len(rampe)
	if distinct < nc {
		nc = distinct
	}

	seuils := make([]float64, 0, nc-1)
	for i := 1; i < nc; i++ {
		seuils = append(seuils, vals[len(vals)*i/nc])
	}
	classe := func(v float64) int {
		for i, s := range seuils {
			if v < s {
				return i
			}
		}
		return nc - 1
	}
	// La rampe garde ses extrêmes : avec trois classes on prend le clair, le
	// médian et le foncé, pas les trois premiers pas.
	teinte := func(c int) string {
		if nc == 1 {
			return rampe[len(rampe)-1]
		}
		return rampe[c*(len(rampe)-1)/(nc-1)]
	}

	var b strings.Builder
	var absents int
	codes := make([]string, 0, len(contours))
	for c := range contours {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, code := range codes {
		cc, ok := byCode[code]
		fill, titre := absentFill, cc.Nom+" — aucune donnée"
		if !ok {
			absents++
			titre = "aucune donnée"
		} else if cc.Absent {
			absents++
		} else {
			fill = teinte(classe(cc.Valeur))
			titre = cc.Nom + " — " + format(cc.Valeur)
		}
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			contours[code], fill, template.HTMLEscapeString(titre))
	}

	// Les bornes affichées sont celles des données, pas des nombres ronds
	// inventés : le lecteur doit pouvoir retrouver la classe d'une valeur.
	bornes := make([]string, 0, nc)
	teintes := make([]string, 0, nc)
	debut := 0
	for i := 0; i < nc; i++ {
		fin := len(vals)
		if i < nc-1 {
			fin = len(vals) * (i + 1) / nc
		}
		if fin <= debut {
			fin = debut + 1
		}
		if fin > len(vals) {
			fin = len(vals)
		}
		if debut >= len(vals) {
			break
		}
		bornes = append(bornes, format(vals[debut])+" – "+format(vals[fin-1]))
		teintes = append(teintes, teinte(i))
		debut = fin
	}
	return Carte{
		SVG: template.HTML(`<svg viewBox="` + vb + `" class="carte" role="img" ` +
			`aria-label="Carte par département">` + b.String() + `</svg>`),
		Bornes: bornes, Teintes: teintes, Unite: unite, Total: len(vals), NbAbsents: absents,
	}
}
