package main

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
}

type StatsEmpireColonial struct {
	CarteSVG template.HTML
	Table    template.HTML
	NbTotal  int
	NbCartes int
}

// chargerEmpireColonial : les 22 territoires (geo.territoire_colonial),
// triés par date d'indépendance — l'ordre chronologique est le fait
// principal que ce dossier montre (trois vagues, pas une évolution
// continue).
func chargerEmpireColonial(ctx context.Context, pool *pgxpool.Pool) (*StatsEmpireColonial, error) {
	const tol = 0.05 // degrés (EPSG:4326) : les territoires sont plus petits qu'un pays du fond Francophonie
	rows, err := pool.Query(ctx, `
		SELECT territoire, region, regime, annee_rattachement, note_rattachement,
		       date_independance, note_independance,
		       CASE WHEN geom IS NOT NULL THEN st_assvg(st_simplifypreservetopology(geom, $1), 0, 2) END
		FROM geo.territoire_colonial
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
			&t.DateIndependance, &t.NoteIndependance, &chemin); err != nil {
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

	st := &StatsEmpireColonial{NbTotal: len(tt)}
	for _, t := range tt {
		if t.Chemin != "" {
			st.NbCartes++
		}
	}
	st.CarteSVG = dessinerCarteEmpireColonial(tt)
	st.Table = tableauEmpireColonial(tt)
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
func dessinerCarteEmpireColonial(tt []TerritoireColonial) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg viewBox="-90 -60 240 120" class="geo monde empire-colonial" role="img" ` +
		`aria-label="Territoires de l'empire colonial français, par vague de décolonisation">`)
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
	b.WriteString(`</svg>`)
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
