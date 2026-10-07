package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

// aliasPaysFrancophonie : les six entités où le nom Francoscope diffère
// réellement du nom Natural Earth (au-delà d'un accent ou d'une apostrophe
// différente, normalisés à part) — vérifié une par une, pas deviné.
var aliasCountryFrancophonie = map[string]string{
	"Cabo Verde":                         "Cap-Vert",
	"Centrafrique":                       "République centrafricaine",
	"Congo":                              "République du Congo",
	"Congo (République démocratique du)": "République démocratique du Congo",
	"États-Unis d’Amérique":              "États-Unis",
	"Fédération de Russie":               "Russie",
}

// normalizeNameCountry réduit un nom à ses lettres et chiffres en minuscules,
// accents et apostrophes typographiques supprimés — pour rapprocher
// "Viet Nam" (Francoscope) de "Viêt Nam" (Natural Earth), ou "Côte d'Ivoire"
// de "Côte d’Ivoire", sans dépendre du caractère exact utilisé.
func normalizeNameCountry(s string) string {
	var b strings.Builder
	for _, r := range s {
		r = unicode.ToLower(r)
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'à', r == 'â', r == 'ä':
			b.WriteRune('a')
		case r == 'è', r == 'é', r == 'ê', r == 'ë':
			b.WriteRune('e')
		case r == 'î', r == 'ï':
			b.WriteRune('i')
		case r == 'ô', r == 'ö':
			b.WriteRune('o')
		case r == 'ù', r == 'û', r == 'ü':
			b.WriteRune('u')
		case r == 'ç':
			b.WriteRune('c')
		}
	}
	return b.String()
}

type CountryFrancophone struct {
	Name                                                      string
	PopulationThousands, FrancophonePct, FrancophoneThousands float64
	LabelX, LabelY, LabelR                                    float64
}

type StatsFrancophonie struct {
	MapSVG            template.HTML
	CountCountryMaps  int
	CountCountryTotal int
	TopPerPct         []CountryFrancophone
	TopPerCount       []CountryFrancophone
	TopPerPctTable    template.HTML
	TopPerCountTable  template.HTML
}

// tableFrancophonie : un classement simple, deux colonnes numériques —
// même patron que tableauDelocalisationCSP (appareil_productif.go).
func tableFrancophonie(country []CountryFrancophone, columnValue string, value func(CountryFrancophone) string) template.HTML {
	var t strings.Builder
	t.WriteString(`<div class="scroll"><table><thead><tr><th>Pays</th><th>` + columnValue + `</th></tr></thead><tbody>`)
	for _, p := range country {
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td></tr>`, template.HTMLEscapeString(p.Name), value(p))
	}
	t.WriteString(`</tbody></table></div>`)
	return template.HTML(t.String())
}

func loadFrancophonie(ctx context.Context, pool *pgxpool.Pool) (*StatsFrancophonie, error) {
	rows, err := pool.Query(ctx, `
		SELECT entite, population_2025_milliers, francophone_pct, francophone_milliers
		FROM core.francophonie_entite WHERE type_entite='pays'
		  AND population_2025_milliers IS NOT NULL AND francophone_pct IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	var all []CountryFrancophone
	for rows.Next() {
		var p CountryFrancophone
		if err := rows.Scan(&p.Name, &p.PopulationThousands, &p.FrancophonePct, &p.FrancophoneThousands); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(all) == 0 {
		return nil, nil
	}

	const toleranceFrancophonie = 0.15 // degrés (EPSG:4326) : assez pour un repère mondial
	geoRows, err := pool.Query(ctx, `
		SELECT nom_fr, st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT nom_fr, st_simplifypreservetopology(geom, $1) AS g FROM geo.contour_pays) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, toleranceFrancophonie)
	if err != nil {
		return nil, err
	}
	var fundsPaths []string
	type geoCountry struct {
		path                   string
		labelX, labelY, labelR float64
	}
	paths := map[string]geoCountry{}
	for geoRows.Next() {
		var name string
		var g geoCountry
		if err := geoRows.Scan(&name, &g.path, &g.labelX, &g.labelY, &g.labelR); err != nil {
			geoRows.Close()
			return nil, err
		}
		fundsPaths = append(fundsPaths, g.path)
		paths[normalizeNameCountry(name)] = g
	}
	if err := geoRows.Err(); err != nil {
		geoRows.Close()
		return nil, err
	}
	geoRows.Close()

	var maps []CountryFrancophone
	var pathsCountry []string
	for _, p := range all {
		nameSearch := p.Name
		if a, ok := aliasCountryFrancophonie[p.Name]; ok {
			nameSearch = a
		}
		if g, ok := paths[normalizeNameCountry(nameSearch)]; ok {
			p.LabelX, p.LabelY, p.LabelR = g.labelX, g.labelY, g.labelR
			maps = append(maps, p)
			pathsCountry = append(pathsCountry, g.path)
		}
	}

	st := &StatsFrancophonie{CountCountryMaps: len(maps), CountCountryTotal: len(all)}
	st.MapSVG = drawMapFrancophonie(fundsPaths, maps, pathsCountry)

	perPct := append([]CountryFrancophone(nil), all...)
	sort.Slice(perPct, func(i, j int) bool { return perPct[i].FrancophonePct > perPct[j].FrancophonePct })
	if len(perPct) > 12 {
		perPct = perPct[:12]
	}
	st.TopPerPct = perPct

	perCount := append([]CountryFrancophone(nil), all...)
	sort.Slice(perCount, func(i, j int) bool { return perCount[i].FrancophoneThousands > perCount[j].FrancophoneThousands })
	if len(perCount) > 12 {
		perCount = perCount[:12]
	}
	st.TopPerCount = perCount
	st.TopPerPctTable = tableFrancophonie(perPct, "Francophones",
		func(p CountryFrancophone) string { return Decimal(p.FrancophonePct, 1) + " %" })
	st.TopPerCountTable = tableFrancophonie(perCount, "Francophones",
		func(p CountryFrancophone) string { return Count(int(p.FrancophoneThousands * 1000)) })

	return st, nil
}

// drawMapFrancophonie : une choroplèthe par seuils — la part de
// francophones dans la population, pas leur nombre absolu (déjà montré par
// le classement à côté). Fond Natural Earth complet (tous les pays,
// francophones ou non, en gris neutre), les pays francophones repeints
// par-dessus selon leur seuil. ST_AsSVG inverse déjà l'axe Y (convention
// PostGIS), aucun retournement manuel à faire ici, à la différence des
// cercles proportionnels utilisés ailleurs sur ce site (ST_X/ST_Y bruts).
func drawMapFrancophonie(funds []string, country []CountryFrancophone, paths []string) template.HTML {
	if len(country) == 0 {
		return ""
	}
	threshold := func(pct float64) string {
		switch {
		case pct >= 90:
			return "fr-5"
		case pct >= 60:
			return "fr-4"
		case pct >= 30:
			return "fr-3"
		case pct >= 10:
			return "fr-2"
		case pct >= 1:
			return "fr-1"
		default:
			return "fr-0"
		}
	}
	var b strings.Builder
	b.WriteString(`<svg viewBox="-180 -85 360 170" class="geo monde francophonie" role="img" ` +
		`aria-label="Part de francophones dans la population, par pays, 2025">`)
	for _, d := range funds {
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	for i, p := range country {
		title := fmt.Sprintf("%s — %s %% de francophones (%s sur %s habitants)",
			p.Name, Decimal(p.FrancophonePct, 1), Count(int(p.FrancophoneThousands*1000)), Count(int(p.PopulationThousands*1000)))
		fmt.Fprintf(&b, `<path class="pays-p %s" d="%s"><title>%s</title></path>`,
			threshold(p.FrancophonePct), paths[i], template.HTMLEscapeString(title))
	}
	// Nommer les ~90 pays de cette carte les aurait tassés les uns sur les
	// autres (repéré sur les cartes déjà publiées) — seuls les pays où le
	// français est la langue d'une majorité substantielle de la population
	// (30 % ou plus, seuil fr-3 et au-dessus) sont donc nommés : c'est le
	// vrai sujet de cette carte, pas la liste des 88 membres et observateurs
	// de l'OIF, dont la plupart ont un pourcentage de francophones proche
	// de zéro.
	const thresholdPctLabel = 30.0
	const thresholdRadiusLabel = 0.15 // degrés : sous ce seuil (Monaco, les Seychelles...), aucun nom ne tient
	// « Congo (République démocratique du) » débordait largement de son
	// pays sur la carte, empiétant sur ses voisins (repéré à la vue de la
	// carte publiée) — le nom complet Francoscope reste dans l'infobulle.
	nameLabel := map[string]string{"Congo (République démocratique du)": "RD Congo"}
	for _, p := range country {
		if p.FrancophonePct < thresholdPctLabel || p.LabelR < thresholdRadiusLabel {
			continue
		}
		name := p.Name
		if short, ok := nameLabel[name]; ok {
			name = short
		}
		fmt.Fprintf(&b, `<text class="nom-francophone" x="%.3f" y="%.3f" text-anchor="middle">%s</text>`,
			p.LabelX, p.LabelY, template.HTMLEscapeString(name))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
