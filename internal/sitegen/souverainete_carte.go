package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// siteSemiConducteur : les sites français de production de semi-conducteurs
// identifiés par Sirene (internal/dossiers/souverainete_solutions.go) et
// déjà cités dans docs/souverainete-numerique.md § 2 — géocodés à la
// commune (api-adresse.data.gouv.fr, vérifié le 19 septembre 2026), pas à
// l'adresse exacte de l'usine : la Cour des comptes elle-même relève que
// l'État ne dispose d'aucune cartographie de cette filière (rapport avril
// 2026, cité au § 2), ce que cette carte comble partiellement.
type siteSemiConducteur struct {
	Nom, Commune, Note string
	Lon, Lat           float64
}

var sitesSemiConducteurs = []siteSemiConducteur{
	{"STMicroelectronics (Crolles 2)", "Crolles (Isère)",
		"Unité légale Sirene 399395581 ; projet « Liberty » avec GlobalFoundries, aide d'État plafonnée à 2,9 Md€.",
		5.883069, 45.283529},
	{"STMicroelectronics Rousset", "Rousset (Bouches-du-Rhône)",
		"Unité légale Sirene 414969584.", 5.620074, 43.481809},
	{"STMicroelectronics (Tours)", "Tours (Indre-et-Loire)",
		"Unité légale Sirene 380932590.", 0.695848, 47.395476},
	{"STMicroelectronics (Grenoble 2)", "Grenoble (Isère)",
		"Unité légale Sirene 504941337, recherche-développement.", 5.724301, 45.182828},
	{"Soitec", "Bernin (Isère)",
		"Unité légale Sirene 384711909 ; matériaux pour semi-conducteurs ; aide d'État de 231,8 M€.",
		5.867065, 45.267187},
}

// chargerCarteSemiConducteurs : le fond France métropolitaine (déjà utilisé
// pour la ligne de démarcation, internal/sitegen/seconde_guerre_mondiale.go) et
// cinq points géocodés à la commune — pas de taille proportionnelle : les
// montants d'aide publique ne sont connus que pour deux des cinq sites
// (Crolles, Soitec), les faire varier en taille aurait suggéré une
// précision que la source n'a que pour deux points sur cinq.
func chargerCarteSemiConducteurs(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	var fondChemin string
	err := pool.QueryRow(ctx, `
		SELECT st_assvg(st_union(g.geom), 0, 4)
		FROM (SELECT (ST_Dump(geom)).path AS path, (ST_Dump(geom)).geom AS geom
		      FROM geo.contour_pays WHERE nom_fr='France') g
		WHERE g.path[1] IN (1, 2)`).Scan(&fondChemin)
	if err != nil || fondChemin == "" {
		return "", err
	}
	fleuves, err := fleuvesSVG(ctx, pool, 4326, 0, 4)
	if err != nil {
		return "", err
	}
	return dessinerCarteSemiConducteurs(fondChemin, fleuves), nil
}

func dessinerCarteSemiConducteurs(fond string, fleuves string) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg viewBox="-6 -52 16 12" class="geo france semi-conducteurs" role="img" ` +
		`aria-label="Sites français de production de semi-conducteurs">`)
	fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, fond)
	b.WriteString(fleuves)
	for _, s := range sitesSemiConducteurs {
		x, y := s.Lon, -s.Lat
		titre := fmt.Sprintf("%s, %s — %s", s.Nom, s.Commune, s.Note)
		fmt.Fprintf(&b, `<circle class="site" cx="%.4f" cy="%.4f" r="0.12"><title>%s</title></circle>`,
			x, y, template.HTMLEscapeString(titre))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
