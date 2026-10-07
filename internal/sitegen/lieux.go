package sitegen

import (
	"context"
	"html/template"
	"regexp"
	"sort"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les lieux : où s'exerce un mandat, et la page qui en dit plus.
//
// Un mandat sans lieu ne se lit pas. « Conseiller municipal » ne dit rien tant
// qu'on ignore de quelle commune ; « 3e vice-président du conseil départemental »
// ne dit rien sans le département. Chaque mandat affiché reçoit donc sa chaîne
// de lieux — commune, intercommunalité, département, région — et chaque maillon
// qui a une page sur ce site devient un lien.
//
// Le lieu est LU dans la source, jamais deviné : le code commune du répertoire
// national des élus, le SIREN de l'intercommunalité, le code du canton dont les
// premiers caractères sont le département. Quand la source ne publie rien, le
// mandat s'affiche sans lieu, et c'est visible.
type Place struct {
	Type, Code, Name string
	URL              string // vide : pas de page pour ce lieu
}

// Les échelons, avec le libellé qui s'affiche devant le nom.
var labelTier = map[string]string{
	"COMMUNE": "commune", "EPCI": "intercommunalité", "CANTON": "canton",
	"CIRCONSCRIPTION": "circonscription", "DEPARTEMENT": "département",
	"REGION": "région",
}

type municipalityRef struct {
	Code, Name, Dept, Region string
	Population               int
}

type epciRef struct {
	Siren, Name, Nature, Dept string
	Page                      bool
}

type Resolver struct {
	root           string
	municipalities map[string]municipalityRef
	depts          map[string]string // code → nom
	deptPage       map[string]bool
	regions        map[string]string // code → nom
	regionSlug     map[string]string
	epci           map[string]epciRef
	epciOfCom      map[string]string // commune → SIREN du groupement à fiscalité propre
	nomsDept       map[string]string // nom normalisé → code, pour « Gers, 1e circonscription »
	circos         map[string]bool   // circonscriptions législatives connues de l'Insee (code)
	deptDistrict   map[string]string // clé cleCirco du département → code, outre-mer compris
}

func loadResolver(ctx context.Context, pool *pgxpool.Pool, root string,
	col *StatsAuthorities) (*Resolver, error) {

	r := &Resolver{root: root,
		municipalities: map[string]municipalityRef{}, depts: map[string]string{},
		deptPage: map[string]bool{}, regions: map[string]string{},
		regionSlug: map[string]string{}, epci: map[string]epciRef{},
		epciOfCom: map[string]string{}, nomsDept: map[string]string{},
		circos: map[string]bool{}, deptDistrict: map[string]string{},
	}

	rows, err := pool.Query(ctx, `
		-- La population du COG 2026 n'est pas renseignée (colonne vide pour les
		-- 34 875 communes) : on prend la population totale retenue par l'OFGL,
		-- dernière année publiée. Sans elle, Metz tombait dans la strate « moins
		-- de 500 habitants » et sa dette était comparée à celle des villages.
		SELECT c.code_insee, c.nom, c.code_departement, c.code_region,
		       coalesce(c.population_municipale, p.value::int, 0)
		FROM ref.commune c
		LEFT JOIN mv.commune_indicator_dernier p
		  ON p.commune_code=c.code_insee AND p.indicator_code='ofgl.population_totale'
		WHERE c.cog_millesime=(SELECT max(cog_millesime) FROM ref.commune)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c municipalityRef
		if err := rows.Scan(&c.Code, &c.Name, &c.Dept, &c.Region, &c.Population); err != nil {
			rows.Close()
			return nil, err
		}
		r.municipalities[c.Code] = c
	}
	rows.Close()

	// Noms des départements et des régions : ceux du référentiel géographique,
	// qui couvre aussi Corse et outre-mer, complétés par les pages existantes.
	grows, err := pool.Query(ctx, `
		SELECT niveau, code_insee, nom FROM geo.contour WHERE niveau IN ('DEPARTEMENT','REGION')`)
	if err != nil {
		return nil, err
	}
	for grows.Next() {
		var niv, code, name string
		if err := grows.Scan(&niv, &code, &name); err != nil {
			grows.Close()
			return nil, err
		}
		if niv == "DEPARTEMENT" {
			r.depts[code] = name
			r.nomsDept[KeySort(name)] = code
		} else {
			r.regions[code] = name
		}
	}
	grows.Close()
	for name, code := range r.nomsDept {
		r.deptDistrict[keyDistrict(name)] = code
	}
	// Les circonscriptions : leur libellé Insee donne aussi le nom des
	// collectivités d'outre-mer, absentes des contours départementaux.
	crows, err := pool.Query(ctx, `SELECT code, nom, code_departement FROM ref.circonscription_legislative`)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var code, name, dep string
		if err := crows.Scan(&code, &name, &dep); err != nil {
			crows.Close()
			return nil, err
		}
		r.circos[code] = true
		d, _, _ := strings.Cut(name, " - ")
		if k := keyDistrict(d); r.deptDistrict[k] == "" {
			r.deptDistrict[k] = dep
		}
	}
	crows.Close()
	for _, d := range col.Departments {
		// Un département fusionné (Alsace 67/68, Corse 2A/2B, Martinique
		// 972, Guyane 973 — voir fusionConnue) reste dans col.Departements
		// pour être listé comme tel, mais n'a pas sa propre page écrite sur
		// le disque : le marquer ici comme "page existante" produirait un
		// lien mort (repéré sur les pages de député d'Alsace, qui pointaient
		// vers /collectivites/departement/67/, jamais générée).
		if !d.WithoutBudgetClean {
			r.deptPage[d.Code] = true
		}
		if r.depts[d.Code] == "" {
			r.depts[d.Code] = d.Name
		}
	}
	for _, g := range col.Regions {
		r.regionSlug[g.Code] = g.Slug
		if r.regions[g.Code] == "" {
			r.regions[g.Code] = g.Name
		}
	}

	erows, err := pool.Query(ctx, `
		SELECT siren, nom, nature_juridique, coalesce(code_departement,'') FROM core.epci`)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var e epciRef
		if err := erows.Scan(&e.Siren, &e.Name, &e.Nature, &e.Dept); err != nil {
			erows.Close()
			return nil, err
		}
		e.Page = natureATaxation[e.Nature]
		r.epci[e.Siren] = e
	}
	erows.Close()

	mrows, err := pool.Query(ctx, `
		SELECT m.commune_code, m.epci_siren FROM core.epci_membre m
		JOIN core.epci e ON e.siren=m.epci_siren
		WHERE e.nature_juridique = ANY($1)`,
		[]string{"CC", "CA", "CU", "METRO", "MET69", "EPT"})
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var com, siren string
		if err := mrows.Scan(&com, &siren); err != nil {
			mrows.Close()
			return nil, err
		}
		r.epciOfCom[com] = siren
	}
	mrows.Close()
	return r, nil
}

// Les URL de chaque échelon.
func (r *Resolver) urlMunicipality(code string) string {
	if _, ok := r.municipalities[code]; !ok {
		return ""
	}
	return r.root + "/collectivites/commune/" + code + "/"
}
func (r *Resolver) urlEPCI(siren string) string {
	if e, ok := r.epci[siren]; ok && e.Page {
		return r.root + "/collectivites/epci/" + siren + "/"
	}
	return ""
}
func (r *Resolver) urlDept(code string) string {
	if r.deptPage[code] {
		return r.root + "/collectivites/departement/" + code + "/"
	}
	// Un département fusionné (voir fusionConnue, internal/sitegen/collectivites.go)
	// n'a pas sa propre page : renvoyer vers la collectivité qui tient
	// désormais son budget plutôt qu'un lien mort ou une absence de lien.
	if f, ok := fusionKnown[code]; ok {
		if f.level == "REGION" {
			return r.urlRegion(f.code)
		}
		return r.urlDept(f.code)
	}
	return ""
}
func (r *Resolver) urlRegion(code string) string {
	if s := r.regionSlug[code]; s != "" {
		return r.root + "/collectivites/region/" + s + "/"
	}
	return ""
}

func (r *Resolver) placeMunicipality(code string) Place {
	c := r.municipalities[code]
	return Place{Type: "COMMUNE", Code: code, Name: c.Name, URL: r.urlMunicipality(code)}
}
func (r *Resolver) placeDept(code string) (Place, bool) {
	name := r.depts[code]
	if name == "" {
		return Place{}, false
	}
	return Place{Type: "DEPARTEMENT", Code: code, Name: name, URL: r.urlDept(code)}, true
}
func (r *Resolver) placeRegion(code string) (Place, bool) {
	name := r.regions[code]
	if name == "" {
		return Place{}, false
	}
	return Place{Type: "REGION", Code: code, Name: name, URL: r.urlRegion(code)}, true
}

var (
	reCodeLabel  = regexp.MustCompile(`^([0-9]{2,3}[0-9AB]*|2[AB][0-9]*|ZZ[0-9]*)\s+(.+)$`)
	reDistrictAN = regexp.MustCompile(`^(.+?),\s*(\d+)(?:e|er|ème)\s+circonscription$`)
)

// deptOfCode : le département contenu dans un code de canton ou de
// circonscription. « 0608 » → 06, « 97613 » → 976, « 2A04 » → 2A.
func deptOfCode(code string) string {
	switch {
	case strings.HasPrefix(code, "97") && len(code) >= 3:
		return code[:3]
	case len(code) >= 2:
		return code[:2]
	}
	return ""
}

// Term : la chaîne de lieux d'un mandat, du plus local au plus large.
func (r *Resolver) Term(typeTerm, municipalityCode, district string) []Place {
	var out []Place
	addDept := func(code string) {
		if l, ok := r.placeDept(code); ok {
			out = append(out, l)
		}
	}
	switch typeTerm {
	case "MAIRE", "CONSEILLER_MUNICIPAL":
		c, ok := r.municipalities[municipalityCode]
		if !ok {
			return nil
		}
		out = append(out, r.placeMunicipality(municipalityCode))
		if s := r.epciOfCom[municipalityCode]; s != "" {
			e := r.epci[s]
			out = append(out, Place{Type: "EPCI", Code: s, Name: e.Name, URL: r.urlEPCI(s)})
		}
		addDept(c.Dept)

	case "CONSEILLER_COMMUNAUTAIRE":
		// « 200029999 Cc Rives De L'Ain-Pays Du Cerdon »
		siren, _, _ := strings.Cut(district, " ")
		if e, ok := r.epci[siren]; ok {
			out = append(out, Place{Type: "EPCI", Code: siren, Name: e.Name, URL: r.urlEPCI(siren)})
			addDept(e.Dept)
		}

	case "CONSEILLER_DEPARTEMENTAL":
		// « 0608 Cannes-2 » : canton, puis département
		if m := reCodeLabel.FindStringSubmatch(district); m != nil {
			out = append(out, Place{Type: "CANTON", Code: m[1], Name: m[2]})
			addDept(deptOfCode(m[1]))
		}

	case "CONSEILLER_REGIONAL":
		if m := reCodeLabel.FindStringSubmatch(district); m != nil {
			if l, ok := r.placeRegion(m[1]); ok {
				out = append(out, l)
			}
		}

	case "DEPUTE":
		// Deux formats coexistent : l'Assemblée publie « Gers, 1e circonscription »,
		// le RNE « 3701 1Ère Circonscription ».
		// Une circonscription connue de l'Insee a sa page ; situerMandats retire
		// le lien des mandats antérieurs au découpage de 2012.
		code := r.codeDistrict(district)
		if code != "" {
			out = append(out, Place{Type: "CIRCONSCRIPTION", Code: code, Name: ordinalDistrict(code) + " circonscription",
				URL: r.root + "/circonscription/" + code + "/"})
		}
		if m := reDistrictAN.FindStringSubmatch(district); m != nil {
			if code == "" {
				out = append(out, Place{Type: "CIRCONSCRIPTION", Name: m[2] + "e circonscription"})
			}
			if dep := r.deptDistrict[keyDistrict(m[1])]; dep != "" {
				addDept(dep)
			}
		} else if m := reCodeLabel.FindStringSubmatch(district); m != nil {
			if code == "" {
				out = append(out, Place{Type: "CIRCONSCRIPTION", Name: strings.ToLower(m[2])})
			}
			if !strings.HasPrefix(m[1], "ZZ") {
				addDept(deptOfCode(m[1]))
			}
		}

	case "SENATEUR":
		if m := reCodeLabel.FindStringSubmatch(district); m != nil && !strings.HasPrefix(m[1], "ZZ") {
			addDept(m[1])
		} else if code := r.nomsDept[KeySort(district)]; code != "" {
			addDept(code)
		}
	}
	return out
}

// ── Pages de commune et d'intercommunalité ────────────────────────────

type ElectedMunicipality struct {
	Name, Function, Slug, Depuis string
	Profile                      bool
	Order                        int
}

type IndicatorMunicipality struct {
	Label, Value, Median string
	Gross, GrossMedian   float64
}

type SecurityMunicipality struct {
	Label, Rate, Count string
	Mask               bool
}

type ListMunicipal struct {
	Label, Nuance string
	Votes, Seats  int
	Pct           float64
}

type PageMunicipality struct {
	RoundFinal       int
	ListsT1          []ListMunicipal
	Code, Name       string
	Population       int
	Dept, Region     Place
	EPCI             *Place
	NatureEPCI       string
	Mayor            *ElectedMunicipality
	Elected          []ElectedMunicipality
	CountCouncillors int
	Finances         []IndicatorMunicipality
	YearFinances     int
	Security         []SecurityMunicipality
	YearSecurity     int
	Lists            []ListMunicipal
	Associations     int
	Neighboring      []LinkMap
	Situation        *Situation
}

type PageEPCI struct {
	Siren, Name, Nature, NatureLib  string
	Population, CountMunicipalities int
	President                       string
	Dept, Region                    Place
	Municipalities                  []Place
	Elected                         []ElectedMunicipality
	CountCouncillors                int
	Responsibilities                []string
	Finances                        []LineFinance
	FiscalYear                      int
	MapMunicipalities               template.HTML
	CountMunicipalitiesMap          int
	Situation                       *Situation
}

var orderFunction = regexp.MustCompile(`(\d+)`)

// rankFunction : le maire d'abord, puis les adjoints ou vice-présidents dans
// leur ordre, puis les conseillers — l'ordre du tableau du conseil.
func rankFunction(f string) int {
	lf := strings.ToLower(f)
	switch {
	case lf == "maire" || strings.HasPrefix(lf, "président"):
		return 0
	case strings.Contains(lf, "adjoint") || strings.Contains(lf, "vice-président"):
		if m := orderFunction.FindString(lf); m != "" {
			n := 0
			for _, c := range m {
				n = n*10 + int(c-'0')
			}
			return n
		}
		return 50
	case lf == "":
		return 1000
	}
	return 500
}

func sortElected(e []ElectedMunicipality) {
	sort.SliceStable(e, func(i, j int) bool {
		if e[i].Order != e[j].Order {
			return e[i].Order < e[j].Order
		}
		return KeySort(e[i].Name) < KeySort(e[j].Name)
	})
}

// slugNature : le slug du libellé long d'une nature juridique.
func slugNature(n string) string { return partis.Slugify(labelNature[n]) }

// locateTerms pose la chaîne de lieux sur les mandats d'une personne, écarte
// le doublon « conseiller municipal, fonction : maire » quand le mandat de
// maire de la même commune est déjà listé — le répertoire publie les deux, et
// les afficher côte à côte fait croire à deux mandats — et refait le libellé
// court de la personne à partir du mandat le plus récent.
func (r *Resolver) locateTerms(p *Person) {
	mayors := map[string]bool{}
	for _, m := range p.Terms {
		if m.Type == "MAIRE" && m.MunicipalityCode != "" {
			mayors[m.MunicipalityCode] = true
		}
	}
	gardes := p.Terms[:0]
	for _, m := range p.Terms {
		if m.Type == "CONSEILLER_MUNICIPAL" && strings.EqualFold(m.Role, "maire") && mayors[m.MunicipalityCode] {
			continue
		}
		m.Places = r.Term(m.Type, m.MunicipalityCode, m.District)
		if m.Type == "DEPUTE" && m.StartISO < startBreakdown2012 {
			for i := range m.Places {
				if m.Places[i].Type == "CIRCONSCRIPTION" {
					m.Places[i].URL = ""
				}
			}
		}
		gardes = append(gardes, m)
	}
	p.Terms = gardes
	if len(p.Terms) == 0 {
		return
	}
	m := p.Terms[0]
	p.Term = labelTerm(m.Type)
	if m.Type == "MINISTRE" && m.Role != "" {
		p.Term = m.Role
	}
	if n := len(m.Places); n > 0 {
		p.Term += " — " + m.Places[0].Name
	}
}
