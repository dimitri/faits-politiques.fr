package main

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
type Lieu struct {
	Type, Code, Nom string
	URL             string // vide : pas de page pour ce lieu
}

// Les échelons, avec le libellé qui s'affiche devant le nom.
var libelleEchelon = map[string]string{
	"COMMUNE": "commune", "EPCI": "intercommunalité", "CANTON": "canton",
	"CIRCONSCRIPTION": "circonscription", "DEPARTEMENT": "département",
	"REGION": "région",
}

type communeRef struct {
	Code, Nom, Dept, Region string
	Population              int
}

type epciRef struct {
	Siren, Nom, Nature, Dept string
	Page                     bool
}

type Resolveur struct {
	root      string
	communes  map[string]communeRef
	depts     map[string]string // code → nom
	deptPage  map[string]bool
	regions   map[string]string // code → nom
	regSlug   map[string]string
	epci      map[string]epciRef
	epciDeCom map[string]string // commune → SIREN du groupement à fiscalité propre
	nomsDept  map[string]string // nom normalisé → code, pour « Gers, 1e circonscription »
	circos    map[string]bool   // circonscriptions législatives connues de l'Insee (code)
	deptCirco map[string]string // clé cleCirco du département → code, outre-mer compris
}

func chargerResolveur(ctx context.Context, pool *pgxpool.Pool, root string,
	col *StatsCollectivites) (*Resolveur, error) {

	r := &Resolveur{root: root,
		communes: map[string]communeRef{}, depts: map[string]string{},
		deptPage: map[string]bool{}, regions: map[string]string{},
		regSlug: map[string]string{}, epci: map[string]epciRef{},
		epciDeCom: map[string]string{}, nomsDept: map[string]string{},
		circos: map[string]bool{}, deptCirco: map[string]string{},
	}

	rows, err := pool.Query(ctx, `
		-- La population du COG 2026 n'est pas renseignée (colonne vide pour les
		-- 34 875 communes) : on prend la population totale retenue par l'OFGL,
		-- dernière année publiée. Sans elle, Metz tombait dans la strate « moins
		-- de 500 habitants » et sa dette était comparée à celle des villages.
		SELECT c.code_insee, c.nom, c.code_departement, c.code_region,
		       coalesce(c.population_municipale, p.value::int, 0)
		FROM ref.commune c
		LEFT JOIN LATERAL (
		  SELECT value FROM core.commune_indicator i
		  WHERE i.commune_code=c.code_insee AND i.indicator_code='ofgl.population_totale'
		  ORDER BY period_year DESC LIMIT 1) p ON true
		WHERE c.cog_millesime=(SELECT max(cog_millesime) FROM ref.commune)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c communeRef
		if err := rows.Scan(&c.Code, &c.Nom, &c.Dept, &c.Region, &c.Population); err != nil {
			rows.Close()
			return nil, err
		}
		r.communes[c.Code] = c
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
		var niv, code, nom string
		if err := grows.Scan(&niv, &code, &nom); err != nil {
			grows.Close()
			return nil, err
		}
		if niv == "DEPARTEMENT" {
			r.depts[code] = nom
			r.nomsDept[CleTri(nom)] = code
		} else {
			r.regions[code] = nom
		}
	}
	grows.Close()
	for nom, code := range r.nomsDept {
		r.deptCirco[cleCirco(nom)] = code
	}
	// Les circonscriptions : leur libellé Insee donne aussi le nom des
	// collectivités d'outre-mer, absentes des contours départementaux.
	crows, err := pool.Query(ctx, `SELECT code, nom, code_departement FROM ref.circonscription_legislative`)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var code, nom, dep string
		if err := crows.Scan(&code, &nom, &dep); err != nil {
			crows.Close()
			return nil, err
		}
		r.circos[code] = true
		d, _, _ := strings.Cut(nom, " - ")
		if k := cleCirco(d); r.deptCirco[k] == "" {
			r.deptCirco[k] = dep
		}
	}
	crows.Close()
	for _, d := range col.Departements {
		r.deptPage[d.Code] = true
		if r.depts[d.Code] == "" {
			r.depts[d.Code] = d.Nom
		}
	}
	for _, g := range col.Regions {
		r.regSlug[g.Code] = g.Slug
		if r.regions[g.Code] == "" {
			r.regions[g.Code] = g.Nom
		}
	}

	erows, err := pool.Query(ctx, `
		SELECT siren, nom, nature_juridique, coalesce(code_departement,'') FROM core.epci`)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var e epciRef
		if err := erows.Scan(&e.Siren, &e.Nom, &e.Nature, &e.Dept); err != nil {
			erows.Close()
			return nil, err
		}
		e.Page = natureAFiscalite[e.Nature]
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
		r.epciDeCom[com] = siren
	}
	mrows.Close()
	return r, nil
}

// Les URL de chaque échelon.
func (r *Resolveur) urlCommune(code string) string {
	if _, ok := r.communes[code]; !ok {
		return ""
	}
	return r.root + "/collectivites/commune/" + code + "/"
}
func (r *Resolveur) urlEPCI(siren string) string {
	if e, ok := r.epci[siren]; ok && e.Page {
		return r.root + "/collectivites/epci/" + siren + "/"
	}
	return ""
}
func (r *Resolveur) urlDept(code string) string {
	if r.deptPage[code] {
		return r.root + "/collectivites/departement/" + code + "/"
	}
	return ""
}
func (r *Resolveur) urlRegion(code string) string {
	if s := r.regSlug[code]; s != "" {
		return r.root + "/collectivites/region/" + s + "/"
	}
	return ""
}

func (r *Resolveur) lieuCommune(code string) Lieu {
	c := r.communes[code]
	return Lieu{Type: "COMMUNE", Code: code, Nom: c.Nom, URL: r.urlCommune(code)}
}
func (r *Resolveur) lieuDept(code string) (Lieu, bool) {
	nom := r.depts[code]
	if nom == "" {
		return Lieu{}, false
	}
	return Lieu{Type: "DEPARTEMENT", Code: code, Nom: nom, URL: r.urlDept(code)}, true
}
func (r *Resolveur) lieuRegion(code string) (Lieu, bool) {
	nom := r.regions[code]
	if nom == "" {
		return Lieu{}, false
	}
	return Lieu{Type: "REGION", Code: code, Nom: nom, URL: r.urlRegion(code)}, true
}

var (
	reCodeLibelle = regexp.MustCompile(`^([0-9]{2,3}[0-9AB]*|2[AB][0-9]*|ZZ[0-9]*)\s+(.+)$`)
	reCircoAN     = regexp.MustCompile(`^(.+?),\s*(\d+)(?:e|er|ème)\s+circonscription$`)
)

// deptDeCode : le département contenu dans un code de canton ou de
// circonscription. « 0608 » → 06, « 97613 » → 976, « 2A04 » → 2A.
func deptDeCode(code string) string {
	switch {
	case strings.HasPrefix(code, "97") && len(code) >= 3:
		return code[:3]
	case len(code) >= 2:
		return code[:2]
	}
	return ""
}

// Mandat : la chaîne de lieux d'un mandat, du plus local au plus large.
func (r *Resolveur) Mandat(typeMandat, communeCode, circo string) []Lieu {
	var out []Lieu
	ajouterDept := func(code string) {
		if l, ok := r.lieuDept(code); ok {
			out = append(out, l)
		}
	}
	switch typeMandat {
	case "MAIRE", "CONSEILLER_MUNICIPAL":
		c, ok := r.communes[communeCode]
		if !ok {
			return nil
		}
		out = append(out, r.lieuCommune(communeCode))
		if s := r.epciDeCom[communeCode]; s != "" {
			e := r.epci[s]
			out = append(out, Lieu{Type: "EPCI", Code: s, Nom: e.Nom, URL: r.urlEPCI(s)})
		}
		ajouterDept(c.Dept)

	case "CONSEILLER_COMMUNAUTAIRE":
		// « 200029999 Cc Rives De L'Ain-Pays Du Cerdon »
		siren, _, _ := strings.Cut(circo, " ")
		if e, ok := r.epci[siren]; ok {
			out = append(out, Lieu{Type: "EPCI", Code: siren, Nom: e.Nom, URL: r.urlEPCI(siren)})
			ajouterDept(e.Dept)
		}

	case "CONSEILLER_DEPARTEMENTAL":
		// « 0608 Cannes-2 » : canton, puis département
		if m := reCodeLibelle.FindStringSubmatch(circo); m != nil {
			out = append(out, Lieu{Type: "CANTON", Code: m[1], Nom: m[2]})
			ajouterDept(deptDeCode(m[1]))
		}

	case "CONSEILLER_REGIONAL":
		if m := reCodeLibelle.FindStringSubmatch(circo); m != nil {
			if l, ok := r.lieuRegion(m[1]); ok {
				out = append(out, l)
			}
		}

	case "DEPUTE":
		// Deux formats coexistent : l'Assemblée publie « Gers, 1e circonscription »,
		// le RNE « 3701 1Ère Circonscription ».
		// Une circonscription connue de l'Insee a sa page ; situerMandats retire
		// le lien des mandats antérieurs au découpage de 2012.
		code := r.codeCirco(circo)
		if code != "" {
			out = append(out, Lieu{Type: "CIRCONSCRIPTION", Code: code, Nom: ordinalCirco(code) + " circonscription",
				URL: r.root + "/circonscription/" + code + "/"})
		}
		if m := reCircoAN.FindStringSubmatch(circo); m != nil {
			if code == "" {
				out = append(out, Lieu{Type: "CIRCONSCRIPTION", Nom: m[2] + "e circonscription"})
			}
			if dep := r.deptCirco[cleCirco(m[1])]; dep != "" {
				ajouterDept(dep)
			}
		} else if m := reCodeLibelle.FindStringSubmatch(circo); m != nil {
			if code == "" {
				out = append(out, Lieu{Type: "CIRCONSCRIPTION", Nom: strings.ToLower(m[2])})
			}
			if !strings.HasPrefix(m[1], "ZZ") {
				ajouterDept(deptDeCode(m[1]))
			}
		}

	case "SENATEUR":
		if m := reCodeLibelle.FindStringSubmatch(circo); m != nil && !strings.HasPrefix(m[1], "ZZ") {
			ajouterDept(m[1])
		} else if code := r.nomsDept[CleTri(circo)]; code != "" {
			ajouterDept(code)
		}
	}
	return out
}

// ── Pages de commune et d'intercommunalité ────────────────────────────

type EluCommune struct {
	Nom, Fonction, Slug, Depuis string
	Fiche                       bool
	Ordre                       int
}

type IndicCommune struct {
	Libelle, Valeur, Mediane string
	Brut, BrutMediane        float64
}

type SecuriteCommune struct {
	Libelle, Taux, Nombre string
	Masque                bool
}

type ListeMunicipale struct {
	Libelle, Nuance string
	Voix, Sieges    int
	Pct             float64
}

type PageCommune struct {
	TourFinal     int
	ListesT1      []ListeMunicipale
	Code, Nom     string
	Population    int
	Dept, Region  Lieu
	EPCI          *Lieu
	NatureEPCI    string
	Maire         *EluCommune
	Elus          []EluCommune
	NbConseillers int
	Finances      []IndicCommune
	AnneeFinances int
	Securite      []SecuriteCommune
	AnneeSecurite int
	Listes        []ListeMunicipale
	Associations  int
	Voisines      []LienCarte
	Situation     *Situation
}

type PageEPCI struct {
	Siren, Nom, Nature, NatureLib string
	Population, NbCommunes        int
	President                     string
	Dept, Region                  Lieu
	Communes                      []Lieu
	Elus                          []EluCommune
	NbConseillers                 int
	Competences                   []string
	Finances                      []LigneFinance
	Exercice                      int
	CarteCommunes                 template.HTML
	NbCommunesCarte               int
	Situation                     *Situation
}

var ordreFonction = regexp.MustCompile(`(\d+)`)

// rangFonction : le maire d'abord, puis les adjoints ou vice-présidents dans
// leur ordre, puis les conseillers — l'ordre du tableau du conseil.
func rangFonction(f string) int {
	lf := strings.ToLower(f)
	switch {
	case lf == "maire" || strings.HasPrefix(lf, "président"):
		return 0
	case strings.Contains(lf, "adjoint") || strings.Contains(lf, "vice-président"):
		if m := ordreFonction.FindString(lf); m != "" {
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

func trierElus(e []EluCommune) {
	sort.SliceStable(e, func(i, j int) bool {
		if e[i].Ordre != e[j].Ordre {
			return e[i].Ordre < e[j].Ordre
		}
		return CleTri(e[i].Nom) < CleTri(e[j].Nom)
	})
}

// slugNature : le slug du libellé long d'une nature juridique.
func slugNature(n string) string { return partis.Slugify(libelleNature[n]) }

// situerMandats pose la chaîne de lieux sur les mandats d'une personne, écarte
// le doublon « conseiller municipal, fonction : maire » quand le mandat de
// maire de la même commune est déjà listé — le répertoire publie les deux, et
// les afficher côte à côte fait croire à deux mandats — et refait le libellé
// court de la personne à partir du mandat le plus récent.
func (r *Resolveur) situerMandats(p *Person) {
	maires := map[string]bool{}
	for _, m := range p.Mandats {
		if m.Type == "MAIRE" && m.CommuneCode != "" {
			maires[m.CommuneCode] = true
		}
	}
	gardes := p.Mandats[:0]
	for _, m := range p.Mandats {
		if m.Type == "CONSEILLER_MUNICIPAL" && strings.EqualFold(m.Role, "maire") && maires[m.CommuneCode] {
			continue
		}
		m.Lieux = r.Mandat(m.Type, m.CommuneCode, m.Circo)
		if m.Type == "DEPUTE" && m.DebutISO < debutDecoupage2012 {
			for i := range m.Lieux {
				if m.Lieux[i].Type == "CIRCONSCRIPTION" {
					m.Lieux[i].URL = ""
				}
			}
		}
		gardes = append(gardes, m)
	}
	p.Mandats = gardes
	if len(p.Mandats) == 0 {
		return
	}
	m := p.Mandats[0]
	p.Mandat = libelleMandat(m.Type)
	if m.Type == "MINISTRE" && m.Role != "" {
		p.Mandat = m.Role
	}
	if n := len(m.Lieux); n > 0 {
		p.Mandat += " — " + m.Lieux[0].Nom
	}
}
