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
	"golang.org/x/text/unicode/norm"
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

// DeptShare : un département dans la carte d'un scrutin — le chiffre brut (N sur
// Total) et la part qui en est tirée.
type DeptShare struct {
	Name, URL string
	N, Total  int
	Part      string // « 12,1 », vide si la base est trop faible
	Class     int    // 0 à 4 ; -1 : base trop faible, aucune part n'est calculée
}

// ClassLegend : une classe de la légende.
type ClassLegend struct {
	Class int
	Label string
}

// SponsorshipElection : les maires d'un scrutin qui ont parrainé ce candidat.
type SponsorshipElection struct {
	Election int
	Mayors   int    // tous les maires qui l'ont parrainé
	Total    int    // tous les maires qui ont parrainé, tous candidats confondus
	Part     string // Mayors / Total, en pour cent
	OffMap   int    // dont communes d'outre-mer ou introuvables, non dessinées
	List     []SponsoringMayor
	Map      *SponsorMap // nil quand ils sont trop peu pour une carte
	Legend   []ClassLegend
	Depts    []DeptShare // ceux où il compte au moins un parrain, par part décroissante
}

// SponsorMap : la carte de France des départements, colorés par la part de leurs
// maires parrains qui ont choisi ce candidat.
type SponsorMap struct {
	Background, ViewBox, Width, Height string
	Layer                              template.HTML
}

// CandidateSponsorships : ce que la fiche d'un candidat montre.
type CandidateSponsorships struct {
	Elections []SponsorshipElection // du plus récent au plus ancien
}

const (
	minBaseDept    = 15 // en dessous, la part d'un département ne veut rien dire
	minMayorsOnMap = 30 // en dessous, une carte de parts n'est que du bruit
)

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

// cleNom : la clé de comparaison d'un nom propre, la même que core.f_cle_nom
// (migration 0208) : minuscules, sans accent, sans espace ni ponctuation.
func cleNom(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'œ':
			b.WriteString("oe")
		case r == 'æ':
			b.WriteString("ae")
		case r == 'ß':
			b.WriteString("ss")
		case r == 'ø':
			b.WriteByte('o')
		case r == 'ł':
			b.WriteByte('l')
		case r == 'đ' || r == 'ð':
			b.WriteByte('d')
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
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

	// Les clés se calculent ici, pas en SQL : la base du périmètre CI n'a pas les
	// fonctions de la base complète (core.f_cle_nom), seulement des tables.
	depByKey := map[string]string{}
	for code, name := range r.depts {
		// « Rhône (avec Métropole de Lyon) » : le Conseil écrit « Rhône ».
		name, _, _ = strings.Cut(name, " (")
		depByKey[cleNom(name)] = code
	}
	communesByKey := map[[2]string][]string{} // département, nom → codes INSEE
	for code, c := range r.municipalities {
		k := [2]string{c.Dept, cleNom(c.Name)}
		communesByKey[k] = append(communesByKey[k], code)
	}
	slugByKey := map[string]string{}
	bySlug := map[string]*Candidate{}
	for _, c := range cands {
		slugByKey[cleNom(c.Name+" "+c.FirstName)] = c.Slug
		bySlug[c.Slug] = c
	}

	rows, err := pool.Query(ctx, `
		SELECT election, candidat, nom, prenom, commune, departement, date_publication, nom_cle, prenom_cle, commune_cle
		FROM core.parrainage_maire`)
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
		var date time.Time
		var communeKey string
		if err := rows.Scan(&p.election, &p.candidat, &p.nom, &p.prenom, &p.commune, &p.dept,
			&date, &p.nomKey, &p.preK, &communeKey); err != nil {
			return nil, err
		}
		p.date = date.Format("02/01/2006")
		p.slug = slugByKey[cleNom(p.candidat)]
		if codes := communesByKey[[2]string{depByKey[cleNom(p.dept)], communeKey}]; len(codes) == 1 {
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
		SELECT m.commune_code, p.family_name, p.given_name
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
		mayors[code] = cle{cleNom(k.nom), cleNom(k.prenom)}
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

	// Les maires parrains de chaque département, tous candidats confondus : le
	// dénominateur de la carte.
	totalDept := map[int]map[string]int{}
	for _, p := range all {
		if p.code == "" {
			continue
		}
		c := r.municipalities[p.code]
		if c.Dept == "" {
			continue
		}
		if totalDept[p.election] == nil {
			totalDept[p.election] = map[string]int{}
		}
		totalDept[p.election][c.Dept]++
	}

	for slug, g := range grouped {
		cs := &CandidateSponsorships{}
		var years []int
		for y := range g.byElection {
			years = append(years, y)
		}
		sort.Ints(years)
		for _, y := range years {
			ps := g.byElection[y]
			el := SponsorshipElection{Election: y, Mayors: len(ps), Total: totals[y]}
			part := 100 * float64(len(ps)) / float64(max(totals[y], 1))
			if part < 0.1 { // un seul parrain sur près de 9 000 : « 0,0 % » dirait zéro
				el.Part = Decimal(part, 2)
			} else {
				el.Part = Decimal(part, 1)
			}
			mine := map[string]int{}
			for _, p := range ps {
				url := ""
				if p.code != "" {
					url = r.urlMunicipality(p.code)
					mine[r.municipalities[p.code].Dept]++
				}
				el.List = append(el.List, SponsoringMayor{
					Name: NameClean(p.prenom + " " + p.nom), Commune: p.commune, Dept: p.dept, URL: url, Date: p.date})
			}
			sort.Slice(el.List, func(i, j int) bool {
				a, b := el.List[i], el.List[j]
				if a.Dept != b.Dept {
					return KeySort(a.Dept) < KeySort(b.Dept)
				}
				return KeySort(a.Commune) < KeySort(b.Commune)
			})
			el.Depts, el.Legend, el.Map, el.OffMap = departmentShares(r, bg, mine, totalDept[y], len(ps))
			cs.Elections = append([]SponsorshipElection{el}, cs.Elections...)
		}
		out.Candidate[slug] = cs
	}
	return out, nil
}

// departmentShares calcule, pour chaque département de métropole, la part des
// maires qui y ont parrainé un candidat (tous candidats confondus) qui l'ont
// choisi, range les départements en cinq classes de même effectif, et dessine
// la carte. Un département de moins de minBaseDept maires parrains n'a pas de
// part : le chiffre brut reste affiché, la couleur est grise.
func departmentShares(r *Resolver, bg *backgroundSituation, mine, total map[string]int, mayors int) (
	[]DeptShare, []ClassLegend, *SponsorMap, int) {

	type row struct {
		code  string
		share float64
		ok    bool
	}
	var rows []row
	var shares []float64
	onMap := 0
	for _, code := range bg.deps.Codes {
		if _, overseas := bg.domDept[code]; overseas {
			continue
		}
		onMap += mine[code]
		ok := total[code] >= minBaseDept
		sh := 0.0
		if ok {
			sh = 100 * float64(mine[code]) / float64(total[code])
			shares = append(shares, sh)
		}
		rows = append(rows, row{code, sh, ok})
	}
	offMap := mayors - onMap
	if mayors < minMayorsOnMap || len(shares) == 0 {
		return nil, nil, nil, offMap
	}
	breaks := quantileBreaks(shares)

	var legend []ClassLegend
	f := func(v float64) string { return Decimal(v, 1) + "\u00a0%" }
	if len(breaks) == 0 {
		legend = []ClassLegend{{0, "partout pareil"}}
	} else {
		legend = append(legend, ClassLegend{0, "moins de " + f(breaks[0])})
		for i := 1; i < len(breaks); i++ {
			legend = append(legend, ClassLegend{i, f(breaks[i-1]) + " à " + f(breaks[i])})
		}
		legend = append(legend, ClassLegend{len(breaks), f(breaks[len(breaks)-1]) + " et plus"})
	}

	var depts []DeptShare
	var ly layer
	for _, rw := range rows {
		name, _, _ := strings.Cut(r.depts[rw.code], " (")
		d := DeptShare{Name: name, URL: r.urlDept(rw.code), N: mine[rw.code], Total: total[rw.code], Class: -1}
		cls, title := "kfaible", fmt.Sprintf("%s : %d des %d maires parrains — base trop faible pour une part", name, d.N, d.Total)
		if rw.ok {
			d.Class = 0
			for _, b := range breaks {
				if rw.share >= b {
					d.Class++
				}
			}
			d.Part = Decimal(rw.share, 1)
			cls = fmt.Sprintf("k%d", d.Class)
			title = fmt.Sprintf("%s : %d des %d maires parrains, soit %s\u00a0%%", name, d.N, d.Total, d.Part)
		}
		if d.N > 0 {
			depts = append(depts, d)
		}
		trace := bg.deps.traces[rw.code]
		if trace == "" {
			continue
		}
		if d.URL != "" {
			fmt.Fprintf(&ly.b, `<a href="%s">`, template.HTMLEscapeString(d.URL))
		}
		fmt.Fprintf(&ly.b, `<path class="%s" d="%s"><title>%s</title></path>`, cls, trace, template.HTMLEscapeString(title))
		if d.URL != "" {
			ly.b.WriteString(`</a>`)
		}
	}
	sort.Slice(depts, func(i, j int) bool {
		a, b := depts[i], depts[j]
		if (a.Class < 0) != (b.Class < 0) {
			return a.Class >= 0 // les parts d'abord
		}
		if a.Class >= 0 {
			return float64(a.N)/float64(a.Total) > float64(b.N)/float64(b.Total)
		}
		return a.N > b.N
	})
	m := &SponsorMap{Background: bg.url, ViewBox: bg.vb}
	if v := strings.Fields(bg.vb); len(v) == 4 {
		m.Width, m.Height = v[2], v[3]
	}
	m.Layer = ly.svg(bg.vb, "Carte de France : part des maires parrains de chaque département qui ont choisi ce candidat")
	return depts, legend, m, offMap
}

// quantileBreaks : les bornes de cinq classes de même effectif, sans doublon.
func quantileBreaks(values []float64) []float64 {
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	var out []float64
	for i := 1; i < 5; i++ {
		b := v[min(len(v)-1, len(v)*i/5)]
		if len(out) == 0 || b > out[len(out)-1] {
			out = append(out, b)
		}
	}
	return out
}
