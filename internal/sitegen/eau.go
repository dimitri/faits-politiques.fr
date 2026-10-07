package sitegen

import (
	"context"
	"database/sql"
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
const toleranceBasins = 0.015 // ≈ 1,5 km — sept polygones simples, pleine page

var colorsBasins = []string{
	"#0D3B43", "#8C4B3A", "#1E5C69", "#B0763A", "#4A8894", "#5C2E1F", "#7FB0BA",
}

type Basin struct {
	Code, Name, Color string
}

type MapBasins struct {
	SVG    template.HTML
	Basins []Basin
}

func loadMapBasins(ctx context.Context, pool *pgxpool.Pool) (*MapBasins, error) {
	// Filtré à la métropole : les deux bassins d'outre-mer (Martinique,
	// Mayotte, § 3) ne sont ni adjacents à la métropole ni valides en
	// Lambert-93 — un st_extent qui les inclurait ferait exploser le
	// cadrage de cette carte pour un gain visuel nul (des points minuscules
	// à des milliers de km). Ils sont listés à part, pas sur cette carte.
	// st_extent est une agrégation : la ligne existe même sans bassin encore
	// ingéré, avec une valeur NULL (voir internal/sitegen/carte.go).
	var vbN sql.NullString
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour_bassin
		      WHERE territoire = 'metropole') x`).Scan(&vbN); err != nil {
		return nil, err
	}
	vb := vbN.String

	rows, err := pool.Query(ctx, `
		SELECT code, nom, st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour_bassin WHERE territoire = 'metropole' ORDER BY code`, toleranceBasins)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ct := &MapBasins{}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo bassins" role="img" `+
		`aria-label="Les sept bassins hydrographiques de France métropolitaine">`, vb)
	i := 0
	for rows.Next() {
		var code, name, d string
		if err := rows.Scan(&code, &name, &d); err != nil {
			return nil, err
		}
		coul := colorsBasins[i%len(colorsBasins)]
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			d, coul, template.HTMLEscapeString(name))
		ct.Basins = append(ct.Basins, Basin{Code: code, Name: name, Color: coul})
		i++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ct.Basins) == 0 {
		return nil, nil // table absente ou vide : le schéma est simplement omis
	}
	rivers, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(rivers)
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
	toleranceEPTBEPAGE           = toleranceFull
	thresholdResolutionEPTBEPAGE = 0.8
)

var colorsTypeEPTBEPAGE = map[string]string{
	"EPTB": "#1E5C69", "EPAGE": "#B0763A", "EPTB_EPAGE": "#7A3B8C",
}

type MapEPTBEPAGE struct {
	SVG                                template.HTML
	CountDisplayed, CountFound         int
	CountEPTB, CountEPAGE, CountDouble int
}

func loadMapEPTBEPAGE(ctx context.Context, pool *pgxpool.Pool) (*MapEPTBEPAGE, error) {
	var countFound int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.eptb_epage`).Scan(&countFound); err != nil {
		return nil, err
	}
	if countFound == 0 {
		return nil, nil // table absente ou vide : le schéma est simplement omis
	}

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

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo eptb-epage" role="img" `+
		`aria-label="Les établissements publics territoriaux de bassin et d'aménagement et de gestion des eaux dont le contour a pu être reconstruit">`, vb)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, toleranceEPTBEPAGE)
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

	rows, err := pool.Query(ctx, `
		SELECT e.nom, e.type, c.nb_membres_resolus, c.nb_membres_total,
		       st_assvg(st_transform(st_simplifypreservetopology(c.geom,$1),2154),1,0)
		FROM geo.contour_eptb_epage c JOIN core.eptb_epage e ON e.siren = c.siren
		WHERE c.nb_membres_resolus::float / c.nb_membres_total >= $2
		ORDER BY e.type, e.nom`, toleranceEPTBEPAGE, thresholdResolutionEPTBEPAGE)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ct := &MapEPTBEPAGE{CountFound: countFound}
	for rows.Next() {
		var name, typ, d string
		var resolved, total int
		if err := rows.Scan(&name, &typ, &resolved, &total, &d); err != nil {
			return nil, err
		}
		title := name
		if resolved < total {
			title = fmt.Sprintf("%s (contour partiel : %d membres sur %d résolus)", name, resolved, total)
		}
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			d, colorsTypeEPTBEPAGE[typ], template.HTMLEscapeString(title))
		ct.CountDisplayed++
		switch typ {
		case "EPTB":
			ct.CountEPTB++
		case "EPAGE":
			ct.CountEPAGE++
		case "EPTB_EPAGE":
			ct.CountDouble++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if ct.CountDisplayed == 0 {
		return nil, nil
	}
	b.WriteString(`</svg>`)
	ct.SVG = template.HTML(b.String())
	return ct, nil
}
