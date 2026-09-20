package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsSecondeGuerreMondiale struct {
	CarteSVG   template.HTML
	LongueurKm float64
}

// chargerSecondeGuerreMondiale : le tracé de la ligne de démarcation
// (geo.ligne_demarcation) superposé au contour de la France métropolitaine
// et de la Corse (geo.contour_pays, sous-géométries 1 et 2 — le reste du
// multipolygone France de Natural Earth couvre les outre-mer, hors sujet
// ici).
func chargerSecondeGuerreMondiale(ctx context.Context, pool *pgxpool.Pool) (*StatsSecondeGuerreMondiale, error) {
	var fondChemin string
	var ok1 bool
	if err := pool.QueryRow(ctx, `
		SELECT st_assvg(st_union(g.geom), 0, 4)
		FROM (SELECT (ST_Dump(geom)).path AS path, (ST_Dump(geom)).geom AS geom
		      FROM geo.contour_pays WHERE nom_fr='France') g
		WHERE g.path[1] IN (1, 2)`).Scan(&fondChemin); err == nil {
		ok1 = fondChemin != ""
	}
	if !ok1 {
		return nil, nil
	}

	fleuves, err := fleuvesSVG(ctx, pool, 4326, 0, 4)
	if err != nil {
		return nil, err
	}

	var ligneChemin string
	var longueurM float64
	err = pool.QueryRow(ctx, `
		SELECT st_assvg(geom, 0, 4), longueur_m FROM geo.ligne_demarcation LIMIT 1`).
		Scan(&ligneChemin, &longueurM)
	if err != nil {
		return &StatsSecondeGuerreMondiale{CarteSVG: dessinerCarteSGM(fondChemin, fleuves, "")}, nil
	}

	st := &StatsSecondeGuerreMondiale{LongueurKm: longueurM / 1000}
	st.CarteSVG = dessinerCarteSGM(fondChemin, fleuves, ligneChemin)
	return st, nil
}

// dessinerCarteSGM : la France (fond neutre), les grands cours d'eau comme
// repère, et le tracé de la ligne de démarcation par-dessus — pas de
// remplissage par zone (occupée/libre), parce qu'aucune géométrie de zone
// vérifiée n'a été trouvée, seulement le tracé de la ligne elle-même (voir
// § 2 du dossier).
func dessinerCarteSGM(fond, fleuves, ligne string) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg viewBox="-6 -52 16 12" class="geo france sgm" role="img" ` +
		`aria-label="Tracé de la ligne de démarcation, 1940-1942">`)
	fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, fond)
	b.WriteString(fleuves)
	if ligne != "" {
		fmt.Fprintf(&b, `<path class="ligne-demarcation" d="%s"><title>Ligne de démarcation, 1940-1942</title></path>`, ligne)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointPopulation struct {
	Annee      int
	Population int64
}

type PopulationGuerres struct {
	SVG                            template.HTML
	Pop1911, Pop1921               int64
	BaisseAbsolue                  int64
	BaissePct                      float64
	Pop1936, Pop1954               int64
}

// chargerPopulationGuerres : la population communale agrégée au niveau
// national (core.population_historique_commune, Insee 1876-1999) — la
// seule série de ce dossier qui montre un choc démographique mesuré
// indépendamment de tout dénombrement militaire ou civil. Le creux de la
// Première Guerre mondiale (1911→1921) est directement lisible ; celui de
// la Seconde ne l'est pas, la source sautant de 1936 à 1954 sans point en
// 1946.
func chargerPopulationGuerres(ctx context.Context, pool *pgxpool.Pool) (*PopulationGuerres, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, sum(population) FROM core.population_historique_commune
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []pointPopulation
	valeurs := map[int]int64{}
	for rows.Next() {
		var p pointPopulation
		if err := rows.Scan(&p.Annee, &p.Population); err != nil {
			return nil, err
		}
		pts = append(pts, p)
		valeurs[p.Annee] = p.Population
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, nil
	}
	pg := &PopulationGuerres{
		Pop1911: valeurs[1911], Pop1921: valeurs[1921],
		Pop1936: valeurs[1936], Pop1954: valeurs[1954],
	}
	if pg.Pop1911 > 0 {
		pg.BaisseAbsolue = pg.Pop1911 - pg.Pop1921
		pg.BaissePct = float64(pg.BaisseAbsolue) / float64(pg.Pop1911) * 100
	}
	pg.SVG = dessinerPopulationGuerres(pts)
	return pg, nil
}

func dessinerPopulationGuerres(pts []pointPopulation) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 40.0, 14.0, 14.0, 26.0
	anneeDebut, anneeFin := pts[0].Annee, pts[len(pts)-1].Annee
	maxVal := int64(0)
	for _, p := range pts {
		if p.Population > maxVal {
			maxVal = p.Population
		}
	}
	maxValM := float64(maxVal) / 1e6 * 1.1
	x := func(annee int) float64 { return ml + (w-ml-mr)*float64(annee-anneeDebut)/float64(anneeFin-anneeDebut) }
	y := func(popM float64) float64 { return mt + (h-mt-mb)*(1-popM/maxValM) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe population-guerres" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Population de la France, %d à %d">`, w, h, anneeDebut, anneeFin)

	// Deux bandes : 1914-1918 et 1939-1945 — pas des zones de rupture de
	// série (comme dans le graphique immigration), mais les deux guerres
	// elles-mêmes, pour lire le creux de 1921 et l'absence de creux visible
	// autour de 1954 dans leur contexte.
	for _, guerre := range [][2]int{{1914, 1918}, {1939, 1945}} {
		fmt.Fprintf(&b, `<rect class="bande-guerre" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
			x(guerre[0]), mt, x(guerre[1])-x(guerre[0]), h-mt-mb)
	}
	for _, palier := range []float64{0, 20, 40, 60} {
		if palier > maxValM {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d M</text>`, ml-6, y(palier)+3, int(palier))
	}

	var coords []string
	for _, p := range pts {
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Annee), y(float64(p.Population)/1e6)))
	}
	fmt.Fprintf(&b, `<polyline class="ligne-pop" points="%s"/>`, strings.Join(coords, " "))
	for _, p := range pts {
		fmt.Fprintf(&b, `<circle class="pt-pop" cx="%.2f" cy="%.2f" r="2.6"><title>%d : %s habitants</title></circle>`,
			x(p.Annee), y(float64(p.Population)/1e6), p.Annee, Nombre(int(p.Population)))
	}
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anneeDebut)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anneeFin)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
