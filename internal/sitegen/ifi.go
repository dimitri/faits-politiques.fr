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

// carteIFI : la répartition communale de l'IFI (core.ifi_commune, millésime
// le plus récent chargé) — un cercle par commune, sur un fond des
// départements pour repère national. Pas une choroplèthe : le rayon encode
// le nombre de redevables (échelle en racine carrée, pour une surface
// perceptivement juste), la couleur est uniforme. Couverture partielle par
// construction : seules les communes de plus de 20 000 habitants comptant
// plus de 50 redevables sont publiées par la DGFiP (voir
// docs/sci-holding-donnees.md).
const tolIFI = tolPleine

type CommuneIFI struct {
	Nom                               string
	NombreRedevables                  int
	PatrimoineMoyenEur, ImpotMoyenEur float64
	X, Y                              float64
}

type CarteIFI struct {
	SVG        template.HTML
	Annee      int
	NbCommunes int
}

func chargerCarteIFI(ctx context.Context, pool *pgxpool.Pool) (*CarteIFI, error) {
	// max(...) est une agrégation : la ligne existe même sans IFI encore
	// ingéré, avec une année NULL.
	var anneeN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT max(annee) FROM core.ifi_commune`).Scan(&anneeN); err != nil {
		return nil, err
	}
	if !anneeN.Valid {
		return nil, nil // table absente ou vide : le schéma est simplement omis
	}
	annee := int(anneeN.Int64)

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
		WITH agrege AS (
		  -- Paris est publié par arrondissement (751xx) : agrégé sur le
		  -- centroïde de Paris (75056), seule géométrie que ce dépôt connaisse
		  -- à ce niveau — voir la migration 0123.
		  SELECT CASE WHEN code_insee LIKE '751%' THEN '75056' ELSE code_insee END AS code_insee,
		         max(nom_commune) FILTER (WHERE code_insee NOT LIKE '751%') AS nom_direct,
		         sum(nombre_redevables) AS nb,
		         sum(patrimoine_moyen_eur * nombre_redevables) / sum(nombre_redevables) AS patrimoine_moyen,
		         sum(impot_moyen_eur * nombre_redevables) / sum(nombre_redevables) AS impot_moyen
		  FROM core.ifi_commune WHERE annee = $1
		  GROUP BY 1
		)
		SELECT coalesce(a.nom_direct, 'Paris (tous arrondissements publiés)'), a.nb, a.patrimoine_moyen, a.impot_moyen,
		       st_x(st_transform(st_centroid(g.geom), 2154)), st_y(st_transform(st_centroid(g.geom), 2154))
		FROM agrege a JOIN geo.contour_cog g ON g.niveau = 'COMMUNE' AND g.cog_millesime = 2026 AND g.code = a.code_insee
		ORDER BY a.nb`, annee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var communes []CommuneIFI
	for rows.Next() {
		var c CommuneIFI
		if err := rows.Scan(&c.Nom, &c.NombreRedevables, &c.PatrimoineMoyenEur, &c.ImpotMoyenEur, &c.X, &c.Y); err != nil {
			return nil, err
		}
		communes = append(communes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(communes) == 0 {
		return nil, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo ifi" role="img" `+
		`aria-label="Nombre de redevables à l'impôt sur la fortune immobilière, par commune, %d">`, vb, annee)

	depRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),2154),1,0)
		FROM geo.contour WHERE niveau='DEPARTEMENT' AND srid_rendu=2154`, tolIFI)
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

	// Rayon en racine carrée du nombre de redevables (surface proportionnelle,
	// pas le rayon) : calé pour que la plus petite commune publiée (51
	// redevables) et la plus grande (plusieurs milliers) restent toutes deux
	// lisibles sur une carte de France entière.
	rayon := func(nb int) float64 { return 2400 + 224*math.Sqrt(float64(nb)) }
	for _, c := range communes {
		titre := fmt.Sprintf("%s — %s redevables, patrimoine moyen %s M€", c.Nom, Nombre(c.NombreRedevables), Decimal(c.PatrimoineMoyenEur/1e6, 1))
		fmt.Fprintf(&b, `<circle class="ifi-c" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
			c.X, -c.Y, rayon(c.NombreRedevables), template.HTMLEscapeString(titre))
	}

	b.WriteString(`</svg>`)
	return &CarteIFI{SVG: template.HTML(b.String()), Annee: annee, NbCommunes: len(communes)}, nil
}
