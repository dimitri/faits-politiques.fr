package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TerritoryColonial struct {
	Territory, Region, Regime string
	YearAttachment            int
	NoteAttachment            string
	DateIndependance          time.Time
	NoteIndependance          string
	Path                      string // tracé SVG, "" si géométrie absente
	LabelX, LabelY, LabelR    float64
}

// nameShortTerritory : le nom affiché sur la carte, sans le pays actuel entre
// parenthèses (« Dahomey (Bénin) » → « Dahomey ») — le tableau qui suit
// garde le nom complet, l'étiquette n'a la place que pour le nom d'époque.
func nameShortTerritory(territory string) string {
	if i := strings.IndexByte(territory, '('); i > 1 {
		return strings.TrimSpace(territory[:i])
	}
	return territory
}

type StatsEmpireColonial struct {
	MapSVG       template.HTML
	Table        template.HTML
	CountTotal   int
	CountMaps    int
	ExtensionSVG template.HTML
}

// territoryYear : un territoire tracé à une année repère (1900/1920/1938/
// 1946), pas à sa dernière extension avant indépendance comme
// TerritoireColonial — voir geo.empire_colonial_extension (migration 0154).
type territoryYear struct {
	territory, path string
}

// loadEmpireColonial : les 22 territoires (geo.territoire_colonial),
// triés par date d'indépendance — l'ordre chronologique est le fait
// principal que ce dossier montre (trois vagues, pas une évolution
// continue).
func loadEmpireColonial(ctx context.Context, pool *pgxpool.Pool) (*StatsEmpireColonial, error) {
	const tolerance = 0.05 // degrés (EPSG:4326) : les territoires sont plus petits qu'un pays du fond Francophonie
	rows, err := pool.Query(ctx, `
		SELECT territoire, region, regime, annee_rattachement, note_rattachement,
		       date_independance, note_independance, chemin,
		       coalesce(st_x((ic).center), 0), coalesce(-st_y((ic).center), 0), coalesce((ic).radius, 0)
		FROM (
			SELECT territoire, region, regime, annee_rattachement, note_rattachement,
			       date_independance, note_independance,
			       CASE WHEN geom IS NOT NULL THEN st_assvg(st_simplifypreservetopology(geom, $1), 0, 2) END AS chemin,
			       CASE WHEN geom IS NOT NULL THEN st_simplifypreservetopology(geom, $1) END AS g
			FROM geo.territoire_colonial
		) x
		LEFT JOIN LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l ON true
		ORDER BY date_independance`, tolerance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tt []TerritoryColonial
	for rows.Next() {
		var t TerritoryColonial
		var path *string
		if err := rows.Scan(&t.Territory, &t.Region, &t.Regime, &t.YearAttachment, &t.NoteAttachment,
			&t.DateIndependance, &t.NoteIndependance, &path, &t.LabelX, &t.LabelY, &t.LabelR); err != nil {
			return nil, err
		}
		if path != nil {
			t.Path = *path
		}
		tt = append(tt, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(tt) == 0 {
		return nil, nil
	}

	// Le fond du monde (continents, frontières actuelles) : sans lui, les
	// territoires coloniaux flottaient seuls sur un fond vide, sans repère
	// ni océan — même source et même tolérance que le fond de la carte
	// Francophonie (geo.contour_pays), pour un rendu cohérent entre les
	// deux cartes « monde » du site.
	const toleranceBackground = 0.15
	backgroundRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_simplifypreservetopology(geom, $1), 0, 2) FROM geo.contour_pays`, toleranceBackground)
	if err != nil {
		return nil, err
	}
	var funds []string
	for backgroundRows.Next() {
		var d string
		if err := backgroundRows.Scan(&d); err != nil {
			backgroundRows.Close()
			return nil, err
		}
		funds = append(funds, d)
	}
	if err := backgroundRows.Err(); err != nil {
		backgroundRows.Close()
		return nil, err
	}
	backgroundRows.Close()

	st := &StatsEmpireColonial{CountTotal: len(tt)}
	for _, t := range tt {
		if t.Path != "" {
			st.CountMaps++
		}
	}
	st.MapSVG = drawMapEmpireColonial(funds, tt)
	st.Table = tableEmpireColonial(tt)

	// L'empire à quatre dates fixes (1900/1920/1938/1946, migration 0154) :
	// montre la CROISSANCE de l'empire, pas seulement son rétrécissement
	// (seule chose que CarteSVG, ci-dessus, peut montrer — chaque territoire
	// n'y apparaît qu'à sa dernière extension avant indépendance).
	anRows, err := pool.Query(ctx, `
		SELECT annee_repere, territoire, st_assvg(st_simplifypreservetopology(geom, $1), 0, 2)
		FROM geo.empire_colonial_extension WHERE geom IS NOT NULL
		ORDER BY annee_repere, territoire`, tolerance)
	if err != nil {
		return nil, err
	}
	perYear := map[int][]territoryYear{}
	for anRows.Next() {
		var an int
		var ta territoryYear
		if err := anRows.Scan(&an, &ta.territory, &ta.path); err != nil {
			anRows.Close()
			return nil, err
		}
		perYear[an] = append(perYear[an], ta)
	}
	if err := anRows.Err(); err != nil {
		anRows.Close()
		return nil, err
	}
	anRows.Close()
	if len(perYear) > 0 {
		st.ExtensionSVG = drawExtensionEmpire(funds, perYear)
	}
	return st, nil
}

// waveDecolonisation classe une date d'indépendance dans l'une des trois
// vagues identifiées dans les données elles-mêmes (§ 2 du dossier), pas une
// convention externe.
func waveDecolonisation(d time.Time) (class, label string) {
	an := d.Year()
	switch {
	case an <= 1956:
		return "vague-1", "1953-1956 : Indochine et Afrique du Nord"
	case an <= 1962:
		return "vague-2", "1958-1962 : Afrique de l'Ouest, Afrique équatoriale et Algérie"
	default:
		return "vague-3", "1975-1977 : Comores et Djibouti"
	}
}

// dessinerCarteEmpireColonial : chaque territoire à sa dernière extension
// avant l'indépendance (CShapes), coloré par vague de décolonisation — pas
// par région ni par régime juridique, parce que c'est le regroupement
// temporel qui est le fait marquant de ces données (voir § 2).
// seuilEtiquetteTerritoire : rayon minimal (degrés) du plus grand cercle
// inscriptible pour porter un nom — sous ce seuil (les Comores, 0,09°),
// aucun nom ne tiendrait lisiblement à l'échelle du monde entier.
const thresholdLabelTerritory = 0.5

func drawMapEmpireColonial(funds []string, tt []TerritoryColonial) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg viewBox="-90 -60 240 120" class="geo monde empire-colonial" role="img" ` +
		`aria-label="Territoires de l'empire colonial français, par vague de décolonisation, sur fond des pays actuels">`)
	for _, d := range funds {
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	for _, t := range tt {
		if t.Path == "" {
			continue
		}
		class, _ := waveDecolonisation(t.DateIndependance)
		title := fmt.Sprintf("%s — indépendance le %s. %s", t.Territory,
			dateDayFr(t.DateIndependance), t.NoteIndependance)
		fmt.Fprintf(&b, `<path class="territoire-p %s" d="%s"><title>%s</title></path>`,
			class, t.Path, template.HTMLEscapeString(title))
	}
	// Repéré à la vue des cartes déjà publiées (Seconde Guerre mondiale,
	// Indochine) : sans nom, chaque territoire n'est identifiable qu'en
	// survolant l'infobulle native, invisible sans interaction.
	for _, t := range tt {
		if t.Path == "" || t.LabelR < thresholdLabelTerritory {
			continue
		}
		fmt.Fprintf(&b, `<text class="nom-territoire" x="%.3f" y="%.3f" text-anchor="middle">%s</text>`,
			t.LabelX, t.LabelY, template.HTMLEscapeString(nameShortTerritory(t.Territory)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// drawExtensionEmpire : quatre petites cartes du monde côte à côte, une
// par année repère, même fond et même viewBox que la carte principale — la
// seule chose qui change d'une carte à l'autre est la liste des territoires
// français à cette date. Une seule teinte (pas une par vague, contrairement
// à dessinerCarteEmpireColonial) : ces quatre cartes ne parlent pas de
// décolonisation, seulement de superficie administrée à un instant donné.
func drawExtensionEmpire(funds []string, perYear map[int][]territoryYear) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="empire-extension">`)
	for _, an := range []int{1900, 1920, 1938, 1946} {
		territories, ok := perYear[an]
		fmt.Fprintf(&b, `<figure><svg viewBox="-90 -60 240 120" class="geo monde empire-colonial" role="img" `+
			`aria-label="L'empire colonial français en %d">`, an)
		for _, d := range funds {
			fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
		}
		if ok {
			for _, t := range territories {
				fmt.Fprintf(&b, `<path class="territoire-p annee-repere" d="%s"><title>%s en %d</title></path>`,
					t.path, template.HTMLEscapeString(t.territory), an)
			}
		}
		b.WriteString(`</svg>`)
		fmt.Fprintf(&b, `<figcaption>%d<span>%d territoires</span></figcaption></figure>`, an, len(territories))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// tableEmpireColonial : une ligne par territoire, groupée visuellement
// par vague via un attribut de ligne plutôt que trois tableaux séparés —
// l'ordre chronologique reste lisible d'un bout à l'autre.
func tableEmpireColonial(tt []TerritoryColonial) template.HTML {
	sorted := append([]TerritoryColonial(nil), tt...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].DateIndependance.Before(sorted[j].DateIndependance) })

	var t strings.Builder
	t.WriteString(`<div class="scroll"><table><thead><tr>` +
		`<th>Territoire</th><th>Région</th><th>Régime</th>` +
		`<th>Rattachement</th><th>Indépendance</th></tr></thead><tbody>`)
	var lastWave string
	for _, x := range sorted {
		class, labelWave := waveDecolonisation(x.DateIndependance)
		if labelWave != lastWave {
			fmt.Fprintf(&t, `<tr class="groupe"><td colspan="5">%s</td></tr>`, template.HTMLEscapeString(labelWave))
			lastWave = labelWave
		}
		fmt.Fprintf(&t, `<tr class="%s"><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			class, template.HTMLEscapeString(x.Territory), template.HTMLEscapeString(x.Region),
			template.HTMLEscapeString(x.Regime), x.YearAttachment,
			dateDayFr(x.DateIndependance))
	}
	t.WriteString(`</tbody></table></div>`)
	return template.HTML(t.String())
}
