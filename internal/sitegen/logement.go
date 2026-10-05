package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
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
	Annee                                                int
	NbCommunes, NbCarencees, NbDeficitaires, NbConformes int
	// NbExemptees : une dimension à part, jamais un quatrième statut sur la
	// carte — le fichier source la place elle-même en « 4 bis », distincte de
	// carencée/déficitaire (colonne 4), et les deux se recoupent réellement
	// (139 communes à la fois déficitaires et exemptées au millésime 2025,
	// vérifié directement) : une commune exemptée n'a pas forcément atteint
	// son taux cible, elle est seulement dispensée d'y être tenue. Un
	// cinquième statut « exemptée et carencée » sur la carte (prélèvement dû
	// malgré l'exemption, NbExempteesPrelevees) resterait à ajouter si le
	// besoin s'en fait sentir — pour l'instant un simple chiffre, en texte.
	NbExemptees, NbExempteesPrelevees int
}

// chargerCarteSRU : un cercle par commune soumise à la loi SRU, coloré par
// statut (carencée / déficitaire non carencée / conforme ou en avance),
// taille proportionnelle à la population — même patron géométrique que
// chargerCarteIFI (internal/sitegen/ifi.go), mais un statut catégoriel plutôt
// qu'une magnitude continue.
//
// core.sru_commune porte l'historique 2023-2026 (0191_sru_pluriannuel.sql) ;
// la carte n'a besoin que d'une seule photographie cohérente, donnée par la
// vue core.sru_commune_dernier (un seul millésime, le plus récent).
func chargerCarteSRU(ctx context.Context, pool *pgxpool.Pool) (*CarteSRU, error) {
	var total int
	var annee int
	if err := pool.QueryRow(ctx, `SELECT count(*), coalesce(max(annee),0) FROM core.sru_commune_dernier`).Scan(&total, &annee); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	st := &CarteSRU{}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE exemptee), count(*) FILTER (WHERE exemptee AND prelevement_net > 0)
		FROM core.sru_commune_dernier`).Scan(&st.NbExemptees, &st.NbExempteesPrelevees); err != nil {
		return nil, err
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

	// Le code Insee de la source SRU et celui du COG 2026 divergent pour
	// quelques communes nouvelles (ex. Orée d'Anjou, Porte des Pierres
	// Dorées — code de l'ex-commune-siège contre code de la commune
	// nouvelle) : on complète la jointure par code par un repli sur le nom
	// normalisé, contraint au même département (le nom seul est ambigu :
	// des dizaines de communes françaises partagent un même nom).
	//
	// Un OR unique entre `g.code=s.code_insee` et un repli sur nom normalisé
	// empêche PostgreSQL d'utiliser la clé (niveau,code,cog_millesime) : le
	// planificateur retombe sur un balayage croisé des ~2 200 communes SRU
	// contre les ~34 900 communes du COG (unaccent() calculé à chaque paire),
	// plusieurs minutes pour une poignée de communes réellement concernées
	// par le repli. Séparer les deux voies en UNION ALL garde l'index pour
	// l'écrasante majorité des lignes et ne paie le balayage coûteux que pour
	// les communes que le code seul n'a pas trouvées.
	rows, err := pool.Query(ctx, `
		WITH par_code AS (
			SELECT s.code_insee, s.commune, s.population, s.nombre_logements_sociaux,
			       coalesce(s.taux_sru_pct,0) AS taux_sru_pct, coalesce(s.taux_cible_pct,0) AS taux_cible_pct,
			       s.carencee, s.deficitaire,
			       st_x(st_transform(st_centroid(g.geom),2154)) AS x, st_y(st_transform(st_centroid(g.geom),2154)) AS y
			FROM core.sru_commune_dernier s
			JOIN geo.contour_cog g ON g.niveau='COMMUNE' AND g.cog_millesime=2026 AND g.code=s.code_insee
		)
		SELECT commune, population, nombre_logements_sociaux, taux_sru_pct, taux_cible_pct, carencee, deficitaire, x, y
		FROM par_code
		UNION ALL
		SELECT s.commune, s.population, s.nombre_logements_sociaux,
		       coalesce(s.taux_sru_pct,0), coalesce(s.taux_cible_pct,0),
		       s.carencee, s.deficitaire,
		       st_x(st_transform(st_centroid(g.geom),2154)), st_y(st_transform(st_centroid(g.geom),2154))
		FROM core.sru_commune_dernier s
		JOIN geo.contour_cog g ON g.niveau='COMMUNE' AND g.cog_millesime=2026
			AND upper(unaccent(g.nom))=upper(unaccent(s.commune))
			AND g.code_departement = left(s.code_insee, CASE WHEN left(s.code_insee,2)='97' THEN 3 ELSE 2 END)
		WHERE NOT EXISTS (SELECT 1 FROM par_code pc WHERE pc.code_insee = s.code_insee)
		ORDER BY population DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
	st.Annee = annee
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
	fleuves, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	b.WriteString(fleuves)

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

// RangPrelevementSRU : une commune et le prélèvement SRU net qu'elle a
// supporté au dernier millésime chargé (majoration de carence comprise).
type RangPrelevementSRU struct {
	Commune, Departement string
	Montant              float64
}

// PrelevementSRU : le prélèvement SRU agrégé au dernier millésime, et son
// évolution 2023-2026 — la donnée que 0135_sru_communes.sql chargeait déjà
// dans son fichier source sans jamais la lire (voir 0191_sru_pluriannuel.sql).
type PrelevementSRU struct {
	Annee                 int
	Total                 float64
	NbCommunesPrelevement int
	Top                   []RangPrelevementSRU
	Trend                 template.HTML
	AnneeDebut, AnneeFin  int
}

// chargerPrelevementSRU : le montant réel de la sanction SRU (le
// « prélèvement net », majoration de carence comprise), qui existe comme
// colonne dans les fichiers sources depuis le millésime 2024 mais n'était
// lu par aucun connecteur avant 0191_sru_pluriannuel.sql — plus de 130
// millions d'euros par an, jamais montrés sur le site jusqu'ici.
func chargerPrelevementSRU(ctx context.Context, pool *pgxpool.Pool) (*PrelevementSRU, error) {
	st := &PrelevementSRU{}
	if err := pool.QueryRow(ctx, `
		SELECT annee, coalesce(sum(prelevement_net),0), count(*) FILTER (WHERE prelevement_net > 0)
		FROM core.sru_commune_dernier GROUP BY annee`).Scan(&st.Annee, &st.Total, &st.NbCommunesPrelevement); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT commune, departement, prelevement_net FROM core.sru_commune_dernier
		WHERE prelevement_net > 0 ORDER BY prelevement_net DESC LIMIT 5`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r RangPrelevementSRU
		if err := rows.Scan(&r.Commune, &r.Departement, &r.Montant); err != nil {
			rows.Close()
			return nil, err
		}
		st.Top = append(st.Top, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// La tendance 2023-2026 : le nombre de communes carencées existe pour
	// les quatre millésimes, le prélèvement seulement depuis 2024 — la
	// ligne s'arrête donc net avant 2023 (voir le commentaire de
	// courbeAvecLigne, cmd/build/cartepage.go), elle ne l'invente pas.
	trendRows, err := pool.Query(ctx, `
		SELECT annee, count(*) FILTER (WHERE carencee) FROM core.sru_commune GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	var barres []PointAnnee
	for trendRows.Next() {
		var p PointAnnee
		if err := trendRows.Scan(&p.Annee, &p.Valeur); err != nil {
			trendRows.Close()
			return nil, err
		}
		barres = append(barres, p)
	}
	if err := trendRows.Err(); err != nil {
		return nil, err
	}
	trendRows.Close()

	prelRows, err := pool.Query(ctx, `
		SELECT annee, sum(prelevement_net) FROM core.sru_commune
		WHERE prelevement_net IS NOT NULL GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	var ligne []PointAnnee
	for prelRows.Next() {
		var p PointAnnee
		if err := prelRows.Scan(&p.Annee, &p.Valeur); err != nil {
			prelRows.Close()
			return nil, err
		}
		ligne = append(ligne, p)
	}
	if err := prelRows.Err(); err != nil {
		return nil, err
	}
	prelRows.Close()

	if len(barres) > 0 {
		st.AnneeDebut, st.AnneeFin = barres[0].Annee, barres[len(barres)-1].Annee
	}
	meur := func(v float64) string { return Decimal(v/1e6, 1) + " M€" }
	st.Trend = courbeAvecLigne(barres, ligne, func(v float64) string { return Nombre(int(v)) }, meur,
		"avec le prélèvement SRU net total sur une échelle séparée")

	return st, nil
}
