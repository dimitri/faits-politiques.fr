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

type TerritoireColonial struct {
	Territoire, Region, Regime string
	AnneeRattachement          int
	NoteRattachement           string
	DateIndependance           time.Time
	NoteIndependance           string
	Chemin                     string // tracé SVG, "" si géométrie absente
	LabelX, LabelY, LabelR     float64
}

// nomCourtTerritoire : le nom affiché sur la carte, sans le pays actuel entre
// parenthèses (« Dahomey (Bénin) » → « Dahomey ») — le tableau qui suit
// garde le nom complet, l'étiquette n'a la place que pour le nom d'époque.
func nomCourtTerritoire(territoire string) string {
	if i := strings.IndexByte(territoire, '('); i > 1 {
		return strings.TrimSpace(territoire[:i])
	}
	return territoire
}

type StatsEmpireColonial struct {
	CarteSVG     template.HTML
	Table        template.HTML
	NbTotal      int
	NbCartes     int
	ExtensionSVG template.HTML
}

// territoireAnnee : un territoire tracé à une année repère (1900/1920/1938/
// 1946), pas à sa dernière extension avant indépendance comme
// TerritoireColonial — voir geo.empire_colonial_extension (migration 0154).
type territoireAnnee struct {
	territoire, chemin string
}

// chargerEmpireColonial : les 22 territoires (geo.territoire_colonial),
// triés par date d'indépendance — l'ordre chronologique est le fait
// principal que ce dossier montre (trois vagues, pas une évolution
// continue).
func chargerEmpireColonial(ctx context.Context, pool *pgxpool.Pool) (*StatsEmpireColonial, error) {
	const tol = 0.05 // degrés (EPSG:4326) : les territoires sont plus petits qu'un pays du fond Francophonie
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
		ORDER BY date_independance`, tol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tt []TerritoireColonial
	for rows.Next() {
		var t TerritoireColonial
		var chemin *string
		if err := rows.Scan(&t.Territoire, &t.Region, &t.Regime, &t.AnneeRattachement, &t.NoteRattachement,
			&t.DateIndependance, &t.NoteIndependance, &chemin, &t.LabelX, &t.LabelY, &t.LabelR); err != nil {
			return nil, err
		}
		if chemin != nil {
			t.Chemin = *chemin
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
	const tolFond = 0.15
	fondRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_simplifypreservetopology(geom, $1), 0, 2) FROM geo.contour_pays`, tolFond)
	if err != nil {
		return nil, err
	}
	var fonds []string
	for fondRows.Next() {
		var d string
		if err := fondRows.Scan(&d); err != nil {
			fondRows.Close()
			return nil, err
		}
		fonds = append(fonds, d)
	}
	if err := fondRows.Err(); err != nil {
		fondRows.Close()
		return nil, err
	}
	fondRows.Close()

	st := &StatsEmpireColonial{NbTotal: len(tt)}
	for _, t := range tt {
		if t.Chemin != "" {
			st.NbCartes++
		}
	}
	st.CarteSVG = dessinerCarteEmpireColonial(fonds, tt)
	st.Table = tableauEmpireColonial(tt)

	// L'empire à quatre dates fixes (1900/1920/1938/1946, migration 0154) :
	// montre la CROISSANCE de l'empire, pas seulement son rétrécissement
	// (seule chose que CarteSVG, ci-dessus, peut montrer — chaque territoire
	// n'y apparaît qu'à sa dernière extension avant indépendance).
	anRows, err := pool.Query(ctx, `
		SELECT annee_repere, territoire, st_assvg(st_simplifypreservetopology(geom, $1), 0, 2)
		FROM geo.empire_colonial_extension WHERE geom IS NOT NULL
		ORDER BY annee_repere, territoire`, tol)
	if err != nil {
		return nil, err
	}
	parAnnee := map[int][]territoireAnnee{}
	for anRows.Next() {
		var an int
		var ta territoireAnnee
		if err := anRows.Scan(&an, &ta.territoire, &ta.chemin); err != nil {
			anRows.Close()
			return nil, err
		}
		parAnnee[an] = append(parAnnee[an], ta)
	}
	if err := anRows.Err(); err != nil {
		anRows.Close()
		return nil, err
	}
	anRows.Close()
	if len(parAnnee) > 0 {
		st.ExtensionSVG = dessinerExtensionEmpire(fonds, parAnnee)
	}
	return st, nil
}

// vagueDecolonisation classe une date d'indépendance dans l'une des trois
// vagues identifiées dans les données elles-mêmes (§ 2 du dossier), pas une
// convention externe.
func vagueDecolonisation(d time.Time) (classe, libelle string) {
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
const seuilEtiquetteTerritoire = 0.5

func dessinerCarteEmpireColonial(fonds []string, tt []TerritoireColonial) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg viewBox="-90 -60 240 120" class="geo monde empire-colonial" role="img" ` +
		`aria-label="Territoires de l'empire colonial français, par vague de décolonisation, sur fond des pays actuels">`)
	for _, d := range fonds {
		fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
	}
	for _, t := range tt {
		if t.Chemin == "" {
			continue
		}
		classe, _ := vagueDecolonisation(t.DateIndependance)
		titre := fmt.Sprintf("%s — indépendance le %s. %s", t.Territoire,
			dateJourFr(t.DateIndependance), t.NoteIndependance)
		fmt.Fprintf(&b, `<path class="territoire-p %s" d="%s"><title>%s</title></path>`,
			classe, t.Chemin, template.HTMLEscapeString(titre))
	}
	// Repéré à la vue des cartes déjà publiées (Seconde Guerre mondiale,
	// Indochine) : sans nom, chaque territoire n'est identifiable qu'en
	// survolant l'infobulle native, invisible sans interaction.
	for _, t := range tt {
		if t.Chemin == "" || t.LabelR < seuilEtiquetteTerritoire {
			continue
		}
		fmt.Fprintf(&b, `<text class="nom-territoire" x="%.3f" y="%.3f" text-anchor="middle">%s</text>`,
			t.LabelX, t.LabelY, template.HTMLEscapeString(nomCourtTerritoire(t.Territoire)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// dessinerExtensionEmpire : quatre petites cartes du monde côte à côte, une
// par année repère, même fond et même viewBox que la carte principale — la
// seule chose qui change d'une carte à l'autre est la liste des territoires
// français à cette date. Une seule teinte (pas une par vague, contrairement
// à dessinerCarteEmpireColonial) : ces quatre cartes ne parlent pas de
// décolonisation, seulement de superficie administrée à un instant donné.
func dessinerExtensionEmpire(fonds []string, parAnnee map[int][]territoireAnnee) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="empire-extension">`)
	for _, an := range []int{1900, 1920, 1938, 1946} {
		territoires, ok := parAnnee[an]
		fmt.Fprintf(&b, `<figure><svg viewBox="-90 -60 240 120" class="geo monde empire-colonial" role="img" `+
			`aria-label="L'empire colonial français en %d">`, an)
		for _, d := range fonds {
			fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, d)
		}
		if ok {
			for _, t := range territoires {
				fmt.Fprintf(&b, `<path class="territoire-p annee-repere" d="%s"><title>%s en %d</title></path>`,
					t.chemin, template.HTMLEscapeString(t.territoire), an)
			}
		}
		b.WriteString(`</svg>`)
		fmt.Fprintf(&b, `<figcaption>%d<span>%d territoires</span></figcaption></figure>`, an, len(territoires))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// tableauEmpireColonial : une ligne par territoire, groupée visuellement
// par vague via un attribut de ligne plutôt que trois tableaux séparés —
// l'ordre chronologique reste lisible d'un bout à l'autre.
func tableauEmpireColonial(tt []TerritoireColonial) template.HTML {
	sorted := append([]TerritoireColonial(nil), tt...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].DateIndependance.Before(sorted[j].DateIndependance) })

	var t strings.Builder
	t.WriteString(`<div class="scroll"><table><thead><tr>` +
		`<th>Territoire</th><th>Région</th><th>Régime</th>` +
		`<th>Rattachement</th><th>Indépendance</th></tr></thead><tbody>`)
	var derniereVague string
	for _, x := range sorted {
		classe, libelleVague := vagueDecolonisation(x.DateIndependance)
		if libelleVague != derniereVague {
			fmt.Fprintf(&t, `<tr class="groupe"><td colspan="5">%s</td></tr>`, template.HTMLEscapeString(libelleVague))
			derniereVague = libelleVague
		}
		fmt.Fprintf(&t, `<tr class="%s"><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			classe, template.HTMLEscapeString(x.Territoire), template.HTMLEscapeString(x.Region),
			template.HTMLEscapeString(x.Regime), x.AnneeRattachement,
			dateJourFr(x.DateIndependance))
	}
	t.WriteString(`</tbody></table></div>`)
	return template.HTML(t.String())
}
