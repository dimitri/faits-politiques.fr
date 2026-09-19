package main

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type communeSRU struct {
	Commune, Categorie string
	Population, NbLLS  int
	TauxSRU, TauxCible float64
	X, Y               float64
}

type CarteSRU struct {
	SVG                                                  template.HTML
	NbCommunes, NbCarencees, NbDeficitaires, NbConformes int
}

// chargerCarteSRU : un cercle par commune soumise à la loi SRU, coloré par
// statut (carencée / déficitaire non carencée / conforme ou en avance),
// taille proportionnelle à la population — même patron géométrique que
// chargerCarteIFI (cmd/build/ifi.go), mais un statut catégoriel plutôt
// qu'une magnitude continue.
func chargerCarteSRU(ctx context.Context, pool *pgxpool.Pool) (*CarteSRU, error) {
	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.sru_commune`).Scan(&total); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}

	var vb string
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu=2154) x`).Scan(&vb); err != nil {
		return nil, err
	}

	// Le code Insee de la source SRU et celui du COG 2026 divergent pour
	// quelques communes nouvelles (ex. Orée d'Anjou, Porte des Pierres
	// Dorées — code de l'ex-commune-siège contre code de la commune
	// nouvelle) : on complète la jointure par code par un repli sur le nom
	// normalisé, contraint au même département (le nom seul est ambigu :
	// des dizaines de communes françaises partagent un même nom).
	rows, err := pool.Query(ctx, `
		SELECT s.commune, s.population, s.nombre_logements_sociaux,
		       coalesce(s.taux_sru_pct,0), coalesce(s.taux_cible_pct,0),
		       s.carencee, s.deficitaire,
		       st_x(st_transform(st_centroid(g.geom),2154)), st_y(st_transform(st_centroid(g.geom),2154))
		FROM core.sru_commune s
		JOIN geo.contour_cog g ON g.niveau='COMMUNE' AND g.cog_millesime=2026
			AND (g.code=s.code_insee
				OR (upper(unaccent(g.nom))=upper(unaccent(s.commune))
					AND g.code_departement = left(s.code_insee, CASE WHEN left(s.code_insee,2)='97' THEN 3 ELSE 2 END)))
		ORDER BY s.population DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	st := &CarteSRU{}
	var communes []communeSRU
	for rows.Next() {
		var c communeSRU
		var population, nbLLS *int
		var carencee, deficitaire bool
		if err := rows.Scan(&c.Commune, &population, &nbLLS, &c.TauxSRU, &c.TauxCible,
			&carencee, &deficitaire, &c.X, &c.Y); err != nil {
			return nil, err
		}
		if population != nil {
			c.Population = *population
		}
		if nbLLS != nil {
			c.NbLLS = *nbLLS
		}
		switch {
		case carencee:
			c.Categorie = "carencee"
			st.NbCarencees++
		case deficitaire:
			c.Categorie = "deficitaire"
			st.NbDeficitaires++
		default:
			c.Categorie = "conforme"
			st.NbConformes++
		}
		communes = append(communes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(communes) == 0 {
		return nil, nil
	}
	st.NbCommunes = len(communes)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo sru" role="img" `+
		`aria-label="Communes soumises à la loi SRU, par statut de conformité">`, vb)

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

	rayon := func(pop int) float64 { return 1300 + 90*math.Sqrt(float64(pop)) }
	// Conformes d'abord dessous, puis déficitaires, puis carencées par-dessus :
	// l'ordre de dessin ne doit jamais laisser une petite commune carencée
	// invisible sous une grande commune conforme voisine.
	for _, cat := range []string{"conforme", "deficitaire", "carencee"} {
		for _, c := range communes {
			if c.Categorie != cat {
				continue
			}
			titre := fmt.Sprintf("%s — %s %% de logements sociaux (cible %s %%), %s",
				template.HTMLEscapeString(c.Commune), Decimal(c.TauxSRU, 1), Decimal(c.TauxCible, 0),
				map[string]string{"carencee": "carencée", "deficitaire": "déficitaire", "conforme": "conforme ou au-delà"}[cat])
			fmt.Fprintf(&b, `<circle class="sru-c sru-%s" cx="%.0f" cy="%.0f" r="%.0f"><title>%s</title></circle>`,
				cat, c.X, -c.Y, rayon(c.Population), titre)
		}
	}
	b.WriteString(`</svg>`)

	st.SVG = template.HTML(b.String())
	return st, nil
}
