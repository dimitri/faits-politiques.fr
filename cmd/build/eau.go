package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// carteBassins : les 7 bassins hydrographiques de France métropolitaine
// (geo.contour_bassin, migration 0088) — pas une choroplèthe, aucune valeur
// numérique ne varie d'un bassin à l'autre ici, seule l'identité du bassin
// est encodée par une couleur catégorielle. Voir
// docs/bassins-versants-donnees.md § 3.
const tolBassins = 0.015 // ≈ 1,5 km — sept polygones simples, pleine page

var couleursBassins = []string{
	"#0D3B43", "#8C4B3A", "#1E5C69", "#B0763A", "#4A8894", "#5C2E1F", "#7FB0BA",
}

type Bassin struct {
	Code, Nom, Couleur string
}

type CarteBassins struct {
	SVG     template.HTML
	Bassins []Bassin
}

func chargerCarteBassins(ctx context.Context, pool *pgxpool.Pool) (*CarteBassins, error) {
	var vb string
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour_bassin) x`).Scan(&vb); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT code, nom, st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour_bassin ORDER BY code`, tolBassins)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ct := &CarteBassins{}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo bassins" role="img" `+
		`aria-label="Les sept bassins hydrographiques de France métropolitaine">`, vb)
	i := 0
	for rows.Next() {
		var code, nom, d string
		if err := rows.Scan(&code, &nom, &d); err != nil {
			return nil, err
		}
		coul := couleursBassins[i%len(couleursBassins)]
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			d, coul, template.HTMLEscapeString(nom))
		ct.Bassins = append(ct.Bassins, Bassin{Code: code, Nom: nom, Couleur: coul})
		i++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ct.Bassins) == 0 {
		return nil, nil // table absente ou vide : le schéma est simplement omis
	}
	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(fleuves)
	b.WriteString(`</svg>`)
	ct.SVG = template.HTML(b.String())
	return ct, nil
}

// carteEPTBEPAGE : les EPTB/EPAGE dont le contour reconstruit (migration
// 0122, internal/eau/eptb_epage.go) couvre au moins 80 % de leurs membres —
// en dessous, l'union ne serait qu'un fragment épars, plus trompeur qu'utile
// pris pour le territoire entier. Un fond des départements donne un repère
// national : la couverture reste partielle par construction (67 structures
// trouvées dans BANATIC, une fraction seulement affichée ici), voir
// docs/bassins-versants-donnees.md § 1.2.
const (
	tolEPTBEPAGE            = tolPleine
	seuilResolutionEPTBEPAGE = 0.8
)

var couleursTypeEPTBEPAGE = map[string]string{
	"EPTB": "#1E5C69", "EPAGE": "#B0763A", "EPTB_EPAGE": "#7A3B8C",
}

type CarteEPTBEPAGE struct {
	SVG                              template.HTML
	NbAffiches, NbTrouves            int
	NbEPTB, NbEPAGE, NbDouble        int
}

func chargerCarteEPTBEPAGE(ctx context.Context, pool *pgxpool.Pool) (*CarteEPTBEPAGE, error) {
	var nbTrouves int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.eptb_epage`).Scan(&nbTrouves); err != nil {
		return nil, err
	}
	if nbTrouves == 0 {
		return nil, nil // table absente ou vide : le schéma est simplement omis
	}

	var vb string
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu=2154) x`).Scan(&vb); err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo eptb-epage" role="img" `+
		`aria-label="Les établissements publics territoriaux de bassin et d'aménagement et de gestion des eaux dont le contour a pu être reconstruit">`, vb)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, tolEPTBEPAGE)
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

	rows, err := pool.Query(ctx, `
		SELECT e.nom, e.type, c.nb_membres_resolus, c.nb_membres_total,
		       st_assvg(st_transform(st_simplifypreservetopology(c.geom,$1),2154),1,0)
		FROM geo.contour_eptb_epage c JOIN core.eptb_epage e ON e.siren = c.siren
		WHERE c.nb_membres_resolus::float / c.nb_membres_total >= $2
		ORDER BY e.type, e.nom`, tolEPTBEPAGE, seuilResolutionEPTBEPAGE)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ct := &CarteEPTBEPAGE{NbTrouves: nbTrouves}
	for rows.Next() {
		var nom, typ, d string
		var resolus, total int
		if err := rows.Scan(&nom, &typ, &resolus, &total, &d); err != nil {
			return nil, err
		}
		titre := nom
		if resolus < total {
			titre = fmt.Sprintf("%s (contour partiel : %d membres sur %d résolus)", nom, resolus, total)
		}
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			d, couleursTypeEPTBEPAGE[typ], template.HTMLEscapeString(titre))
		ct.NbAffiches++
		switch typ {
		case "EPTB":
			ct.NbEPTB++
		case "EPAGE":
			ct.NbEPAGE++
		case "EPTB_EPAGE":
			ct.NbDouble++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if ct.NbAffiches == 0 {
		return nil, nil
	}
	b.WriteString(`</svg>`)
	ct.SVG = template.HTML(b.String())
	return ct, nil
}
