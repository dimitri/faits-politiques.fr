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
var aliasPaysFrancophonie = map[string]string{
	"Cabo Verde":                         "Cap-Vert",
	"Centrafrique":                       "République centrafricaine",
	"Congo":                              "République du Congo",
	"Congo (République démocratique du)": "République démocratique du Congo",
	"États-Unis d’Amérique":              "États-Unis",
	"Fédération de Russie":               "Russie",
}

// normaliserNomPays réduit un nom à ses lettres et chiffres en minuscules,
// accents et apostrophes typographiques supprimés — pour rapprocher
// "Viet Nam" (Francoscope) de "Viêt Nam" (Natural Earth), ou "Côte d'Ivoire"
// de "Côte d’Ivoire", sans dépendre du caractère exact utilisé.
func normaliserNomPays(s string) string {
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

type PaysFrancophone struct {
	Nom                                                     string
	PopulationMilliers, FrancophonePct, FrancophoneMilliers float64
	LabelX, LabelY, LabelR                                  float64
}

type StatsFrancophonie struct {
	CarteSVG          template.HTML
	NbPaysCartes      int
	NbPaysTotal       int
	TopParPct         []PaysFrancophone
	TopParNombre      []PaysFrancophone
	TopParPctTable    template.HTML
	TopParNombreTable template.HTML
}

// tableauFrancophonie : un classement simple, deux colonnes numériques —
// même patron que tableauDelocalisationCSP (appareil_productif.go).
func tableauFrancophonie(pays []PaysFrancophone, colonneValeur string, valeur func(PaysFrancophone) string) template.HTML {
	var t strings.Builder
	t.WriteString(`<div class="scroll"><table><thead><tr><th>Pays</th><th>` + colonneValeur + `</th></tr></thead><tbody>`)
	for _, p := range pays {
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td></tr>`, template.HTMLEscapeString(p.Nom), valeur(p))
	}
	t.WriteString(`</tbody></table></div>`)
	return template.HTML(t.String())
}

func chargerFrancophonie(ctx context.Context, pool *pgxpool.Pool) (*StatsFrancophonie, error) {
	rows, err := pool.Query(ctx, `
		SELECT entite, population_2025_milliers, francophone_pct, francophone_milliers
		FROM core.francophonie_entite WHERE type_entite='pays'
		  AND population_2025_milliers IS NOT NULL AND francophone_pct IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	var tous []PaysFrancophone
	for rows.Next() {
		var p PaysFrancophone
		if err := rows.Scan(&p.Nom, &p.PopulationMilliers, &p.FrancophonePct, &p.FrancophoneMilliers); err != nil {
			rows.Close()
			return nil, err
		}
		tous = append(tous, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(tous) == 0 {
		return nil, nil
	}

	const tolFrancophonie = 0.15 // degrés (EPSG:4326) : assez pour un repère mondial
	geoRows, err := pool.Query(ctx, `
		SELECT nom_fr, st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT nom_fr, st_simplifypreservetopology(geom, $1) AS g FROM geo.contour_pays) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tolFrancophonie)
	if err != nil {
		return nil, err
	}
	var fondsChemins []string
	type geoPays struct {
		chemin                 string
		labelX, labelY, labelR float64
	}
	chemins := map[string]geoPays{}
	for geoRows.Next() {
		var nom string
		var g geoPays
		if err := geoRows.Scan(&nom, &g.chemin, &g.labelX, &g.labelY, &g.labelR); err != nil {
			geoRows.Close()
			return nil, err
		}
		fondsChemins = append(fondsChemins, g.chemin)
		chemins[normaliserNomPays(nom)] = g
	}
	if err := geoRows.Err(); err != nil {
		geoRows.Close()
		return nil, err
	}
	geoRows.Close()

	var cartographies []PaysFrancophone
	var cheminsPays []string
	for _, p := range tous {
		nomRecherche := p.Nom
		if a, ok := aliasPaysFrancophonie[p.Nom]; ok {
			nomRecherche = a
		}
		if g, ok := chemins[normaliserNomPays(nomRecherche)]; ok {
			p.LabelX, p.LabelY, p.LabelR = g.labelX, g.labelY, g.labelR
			cartographies = append(cartographies, p)
			cheminsPays = append(cheminsPays, g.chemin)
		}
	}

	st := &StatsFrancophonie{NbPaysCartes: len(cartographies), NbPaysTotal: len(tous)}
	st.CarteSVG = dessinerCarteFrancophonie(fondsChemins, cartographies, cheminsPays)

	parPct := append([]PaysFrancophone(nil), tous...)
	sort.Slice(parPct, func(i, j int) bool { return parPct[i].FrancophonePct > parPct[j].FrancophonePct })
	if len(parPct) > 12 {
		parPct = parPct[:12]
	}
	st.TopParPct = parPct

	parNombre := append([]PaysFrancophone(nil), tous...)
	sort.Slice(parNombre, func(i, j int) bool { return parNombre[i].FrancophoneMilliers > parNombre[j].FrancophoneMilliers })
	if len(parNombre) > 12 {
		parNombre = parNombre[:12]
	}
	st.TopParNombre = parNombre
	st.TopParPctTable = tableauFrancophonie(parPct, "Francophones",
		func(p PaysFrancophone) string { return Decimal(p.FrancophonePct, 1) + " %" })
	st.TopParNombreTable = tableauFrancophonie(parNombre, "Francophones",
		func(p PaysFrancophone) string { return Nombre(int(p.FrancophoneMilliers * 1000)) })

	return st, nil
}

// dessinerCarteFrancophonie : une choroplèthe par seuils — la part de
// francophones dans la population, pas leur nombre absolu (déjà montré par
// le classement à côté). Fond Natural Earth complet (tous les pays,
// francophones ou non, en gris neutre), les pays francophones repeints
// par-dessus selon leur seuil. ST_AsSVG inverse déjà l'axe Y (convention
// PostGIS), aucun retournement manuel à faire ici, à la différence des
// cercles proportionnels utilisés ailleurs sur ce site (ST_X/ST_Y bruts).
func dessinerCarteFrancophonie(fonds []string, pays []PaysFrancophone, chemins []string) template.HTML {
	if len(pays) == 0 {
		return ""
	}
	seuil := func(pct float64) string {
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
	for _, d := range fonds {
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	for i, p := range pays {
		titre := fmt.Sprintf("%s — %s %% de francophones (%s sur %s habitants)",
			p.Nom, Decimal(p.FrancophonePct, 1), Nombre(int(p.FrancophoneMilliers*1000)), Nombre(int(p.PopulationMilliers*1000)))
		fmt.Fprintf(&b, `<path class="pays-p %s" d="%s"><title>%s</title></path>`,
			seuil(p.FrancophonePct), chemins[i], template.HTMLEscapeString(titre))
	}
	// Nommer les ~90 pays de cette carte les aurait tassés les uns sur les
	// autres (repéré sur les cartes déjà publiées) — seuls les pays où le
	// français est la langue d'une majorité substantielle de la population
	// (30 % ou plus, seuil fr-3 et au-dessus) sont donc nommés : c'est le
	// vrai sujet de cette carte, pas la liste des 88 membres et observateurs
	// de l'OIF, dont la plupart ont un pourcentage de francophones proche
	// de zéro.
	const seuilPctEtiquette = 30.0
	const seuilRayonEtiquette = 0.15 // degrés : sous ce seuil (Monaco, les Seychelles...), aucun nom ne tient
	// « Congo (République démocratique du) » débordait largement de son
	// pays sur la carte, empiétant sur ses voisins (repéré à la vue de la
	// carte publiée) — le nom complet Francoscope reste dans l'infobulle.
	nomEtiquette := map[string]string{"Congo (République démocratique du)": "RD Congo"}
	for _, p := range pays {
		if p.FrancophonePct < seuilPctEtiquette || p.LabelR < seuilRayonEtiquette {
			continue
		}
		nom := p.Nom
		if court, ok := nomEtiquette[nom]; ok {
			nom = court
		}
		fmt.Fprintf(&b, `<text class="nom-francophone" x="%.3f" y="%.3f" text-anchor="middle">%s</text>`,
			p.LabelX, p.LabelY, template.HTMLEscapeString(nom))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
