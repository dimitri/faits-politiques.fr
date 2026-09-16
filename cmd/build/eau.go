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
	b.WriteString(`</svg>`)
	ct.SVG = template.HTML(b.String())
	return ct, nil
}
