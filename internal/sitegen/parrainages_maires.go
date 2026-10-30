package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les maires qui ont parrainé un candidat à l'élection présidentielle.
//
// Le répertoire national des élus ne donne à un maire aucune nuance politique,
// et la nuance des listes municipales est « divers » pour l'essentiel des
// sièges nuancés ; le parrainage, publié par le Conseil constitutionnel, est
// l'un des rares actes publics d'un maire qui le situe. Le site en montre le
// FAIT BRUT — « a parrainé X en 2022 » —, jamais une couleur : un maire peut
// parrainer un candidat dont il ne partage pas les idées, par souci de
// pluralisme ou par courtoisie, et 19 % de ceux qu'on retrouve aux deux
// scrutins ont parrainé deux candidats différents (migration 0208).
//
// Deux usages : la page de la commune dit ce que son maire EN EXERCICE a
// parrainé ; la fiche d'un candidat dit combien de maires l'ont parrainé et
// où se trouvent leurs communes.

// MayorSponsorship : un parrainage donné par le maire en exercice de la commune.
type MayorSponsorship struct {
	Election  int
	Candidate string // « Jean-Luc Mélenchon »
	Slug      string // fiche du candidat 2027, vide si ce n'en est pas un
	Date      string
}

// SponsoringMayor : une ligne de la liste nominative d'une fiche candidat.
type SponsoringMayor struct {
	Name, Commune, Dept, URL, Date string
}

// SponsorshipElection : les maires d'un scrutin qui ont parrainé ce candidat.
type SponsorshipElection struct {
	Election int
	Mayors   int // tous les maires qui l'ont parrainé
	Total    int // tous les maires qui ont parrainé, tous candidats confondus
	OffMap   int // dont communes d'outre-mer ou introuvables, non dessinées
	List     []SponsoringMayor
}

// SponsorMap : la carte de France des communes de ces maires.
type SponsorMap struct {
	Background, ViewBox, Width, Height string
	Layer                              template.HTML
}

// CandidateSponsorships : ce que la fiche d'un candidat montre.
type CandidateSponsorships struct {
	Elections []SponsorshipElection // du plus récent au plus ancien
	Map       *SponsorMap
}

type SponsorshipsMayors struct {
	Mayor     map[string][]MayorSponsorship // code INSEE → parrainages du maire en exercice
	Candidate map[string]*CandidateSponsorships
}

// prettyCandidate : « MÉLENCHON Jean-Luc » → « Jean-Luc Mélenchon ». Le Conseil
// écrit le NOM en capitales puis le prénom ; le nom peut compter plusieurs mots
// (« LE PEN », « DUPONT-AIGNAN »).
func prettyCandidate(s string) string {
	var nom, prenom []string
	for _, w := range strings.Fields(s) {
		if len(prenom) == 0 && w == strings.ToUpper(w) {
			nom = append(nom, titleCase(w))
		} else {
			prenom = append(prenom, w)
		}
	}
	return strings.TrimSpace(strings.Join(prenom, " ") + " " + strings.Join(nom, " "))
}

func titleCase(w string) string {
	var b strings.Builder
	start := true
	for _, r := range strings.ToLower(w) {
		if start {
			r = unicode.ToUpper(r)
		}
		start = r == '-' || r == '\''
		b.WriteRune(r)
	}
	return b.String()
}

// loadSponsorshipsMayors rapproche les maires parrains de leur commune (nom de
// commune et de département sans accent ni ponctuation, au COG courant), puis
// du maire en exercice (nom, prénom et commune identiques). Un rapprochement
// ambigu — deux communes de même nom dans le même département — est écarté
// plutôt que deviné. Le rapprochement se fait ici, à la lecture, jamais au
// chargement : ni le COG ni le répertoire des élus n'y sont forcément à jour.
func loadSponsorshipsMayors(ctx context.Context, pool *pgxpool.Pool, r *Resolver, bg *backgroundSituation,
	cands []*Candidate) (*SponsorshipsMayors, error) {

	var depCodes, depNames []string
	for code, name := range r.depts {
		// « Rhône (avec Métropole de Lyon) » : le Conseil écrit « Rhône ».
		name, _, _ = strings.Cut(name, " (")
		depCodes, depNames = append(depCodes, code), append(depNames, name)
	}
	var slugs, names []string
	bySlug := map[string]*Candidate{}
	for _, c := range cands {
		slugs, names = append(slugs, c.Slug), append(names, c.Name+" "+c.FirstName)
		bySlug[c.Slug] = c
	}

	rows, err := pool.Query(ctx, `
		WITH dep AS (
		  SELECT code, core.f_cle_nom(nom) AS cle FROM unnest($1::text[], $2::text[]) AS t(code, nom)),
		cand AS (
		  SELECT slug, core.f_cle_nom(nom) AS cle FROM unnest($3::text[], $4::text[]) AS t(slug, nom)),
		com AS (
		  SELECT code_insee, code_departement, core.f_cle_nom(nom) AS cle
		  FROM ref.commune WHERE cog_millesime = (SELECT max(cog_millesime) FROM ref.commune))
		SELECT pm.election, pm.candidat, coalesce(cand.slug, ''), pm.nom, pm.prenom, pm.commune,
		       pm.departement, pm.date_publication, pm.nom_cle, pm.prenom_cle,
		       coalesce(array_agg(DISTINCT com.code_insee) FILTER (WHERE com.code_insee IS NOT NULL), '{}')
		FROM core.parrainage_maire pm
		LEFT JOIN cand ON cand.cle = core.f_cle_nom(pm.candidat)
		LEFT JOIN dep ON dep.cle = pm.departement_cle
		LEFT JOIN com ON com.cle = pm.commune_cle AND com.code_departement = dep.code
		GROUP BY pm.election, pm.candidat, cand.slug, pm.nom, pm.prenom, pm.commune, pm.departement,
		         pm.date_publication, pm.nom_cle, pm.prenom_cle`,
		depCodes, depNames, slugs, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type parrainage struct {
		election                          int
		candidat, slug, nom, prenom       string
		commune, dept, date, nomKey, preK string
		code                              string // vide si introuvable ou ambigu
	}
	var all []parrainage
	totals := map[int]int{}
	for rows.Next() {
		var p parrainage
		var codes []string
		var date time.Time
		if err := rows.Scan(&p.election, &p.candidat, &p.slug, &p.nom, &p.prenom, &p.commune, &p.dept,
			&date, &p.nomKey, &p.preK, &codes); err != nil {
			return nil, err
		}
		p.date = date.Format("02/01/2006")
		if len(codes) == 1 {
			p.code = codes[0]
		}
		all = append(all, p)
		totals[p.election]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Les maires en exercice, par commune.
	mrows, err := pool.Query(ctx, `
		SELECT m.commune_code, core.f_cle_nom(p.family_name), core.f_cle_nom(p.given_name)
		FROM core.mandate m JOIN core.person p ON p.id = m.person_id
		WHERE m.mandate_type = 'MAIRE' AND upper(m.validity) IS NULL AND m.commune_code IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	type cle struct{ nom, prenom string }
	mayors := map[string]cle{}
	for mrows.Next() {
		var code string
		var k cle
		if err := mrows.Scan(&code, &k.nom, &k.prenom); err != nil {
			return nil, err
		}
		mayors[code] = k
	}
	if err := mrows.Err(); err != nil {
		return nil, err
	}

	out := &SponsorshipsMayors{Mayor: map[string][]MayorSponsorship{}, Candidate: map[string]*CandidateSponsorships{}}
	display := func(p parrainage) (string, string) {
		if c := bySlug[p.slug]; c != nil {
			return c.FirstName + " " + c.Name, c.Slug
		}
		return prettyCandidate(p.candidat), ""
	}
	type perCand struct {
		byElection map[int][]parrainage
	}
	grouped := map[string]*perCand{}
	for _, p := range all {
		if p.code != "" && mayors[p.code] == (cle{p.nomKey, p.preK}) {
			name, slug := display(p)
			out.Mayor[p.code] = append(out.Mayor[p.code], MayorSponsorship{p.election, name, slug, p.date})
		}
		if p.slug == "" {
			continue
		}
		g := grouped[p.slug]
		if g == nil {
			g = &perCand{byElection: map[int][]parrainage{}}
			grouped[p.slug] = g
		}
		g.byElection[p.election] = append(g.byElection[p.election], p)
	}
	for _, l := range out.Mayor {
		sort.Slice(l, func(i, j int) bool { return l[i].Election > l[j].Election })
	}

	for slug, g := range grouped {
		cs := &CandidateSponsorships{}
		var years []int
		for y := range g.byElection {
			years = append(years, y)
		}
		sort.Ints(years)
		ly := &layer{}
		for _, y := range years {
			ps := g.byElection[y]
			el := SponsorshipElection{Election: y, Mayors: len(ps), Total: totals[y]}
			fmt.Fprintf(&ly.b, `<g class="parrains e%d">`, y)
			for _, p := range ps {
				c := bg.municipalities[p.code]
				if p.code == "" || c == nil {
					el.OffMap++
				} else if _, overseas := bg.domDept[c.dept]; overseas {
					el.OffMap++
				} else {
					fmt.Fprintf(&ly.b, `<circle cx="%.0f" cy="%.0f" r="4500"/>`, c.x, c.y)
				}
				url := ""
				if p.code != "" {
					url = r.urlMunicipality(p.code)
				}
				el.List = append(el.List, SponsoringMayor{
					Name: NameClean(p.prenom + " " + p.nom), Commune: p.commune, Dept: p.dept, URL: url, Date: p.date})
			}
			ly.b.WriteString(`</g>`)
			sort.Slice(el.List, func(i, j int) bool {
				a, b := el.List[i], el.List[j]
				if a.Dept != b.Dept {
					return KeySort(a.Dept) < KeySort(b.Dept)
				}
				return KeySort(a.Commune) < KeySort(b.Commune)
			})
			cs.Elections = append([]SponsorshipElection{el}, cs.Elections...)
		}
		cs.Map = &SponsorMap{Background: bg.url, ViewBox: bg.vb}
		if v := strings.Fields(bg.vb); len(v) == 4 {
			cs.Map.Width, cs.Map.Height = v[2], v[3]
		}
		cs.Map.Layer = ly.svg(bg.vb, "Communes dont le maire a parrainé ce candidat")
		out.Candidate[slug] = cs
	}
	return out, nil
}
