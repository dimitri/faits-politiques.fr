package main

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cartes de situation : chaque région, département, intercommunalité et
// commune placé dans la France entière, outre-mer compris, le reste laissé
// sans information. Elles répondent à « où est-ce, et quelle place cela
// tient-il » avant tout chiffre ; la légende donne la population, la superficie,
// les communes et groupements, et les budgets.
//
// Le fond (les départements de métropole, les limites de régions) est le même
// pour 36 000 pages : il est écrit UNE fois dans media/situation-france.svg, et
// chaque page ne porte qu'un calque SVG de même boîte, superposé en CSS. Écrire
// le fond dans chaque page coûterait plus d'un gigaoctet. Les outre-mer, eux,
// sont de petits cartons en ligne, chacun dans sa projection légale ; celui du
// territoire affiché porte le calque.

const tolSituation = 0.005 // ≈ 500 m : à l'échelle de la France, un pixel couvre près de 2 km

type CartonSituation struct {
	Nom, Fond string
	URL       string        // page du département d'outre-mer, vide s'il n'en a pas
	SVG       template.HTML // calque du carton actif, vide sinon
	Actif     bool
}

type LigneLegende struct{ Libelle, Valeur, Detail, URL string }

// ligneLegende : une ligne de légende, avec un lien facultatif sur la valeur.
func ligneLegende(libelle, valeur, detail string, url ...string) LigneLegende {
	l := LigneLegende{Libelle: libelle, Valeur: valeur, Detail: detail}
	if len(url) > 0 {
		l.URL = url[0]
	}
	return l
}

type Situation struct {
	Fond, ViewBox    string
	Largeur, Hauteur string
	Calque           template.HTML // vide quand le territoire est en outre-mer
	Cartons          []CartonSituation
	Legende          []LigneLegende
	Note             string
}

type geomSituation struct {
	d, dept, region, nom string
	srid                 int
	superficieKm2        float64
	population           int
	x, y                 float64 // point intérieur, pour repérer une petite commune
	epcis                []string
}

type fondSituation struct {
	url, vb     string
	racine      string
	lienDept    func(code string) string
	deps, regs  *JeuContours
	communes    map[string]*geomSituation
	epci        map[string]*geomSituation
	circos      map[string]*geomSituation // circonscriptions législatives (circonscriptions.go)
	natureEPCI  map[string]string
	domDept     map[string]contourSeul // code département d'outre-mer → carton
	domOrdre    []string
	totalKm2    float64
	totalPop    int
	budgetCom   map[string][2]float64 // département → fonctionnement, investissement des communes
	budgetEPCI  map[string][2]float64 // département du siège → idem, groupements à fiscalité propre
	anneeBudget int
}

var regionDOM = map[string]string{"01": "971", "02": "972", "03": "973", "04": "974", "06": "976"}

// lienDept donne, pour un code de département, le chemin de sa page depuis la
// racine du site (« collectivites/departement/29/ »), vide s'il n'en a pas.
func chargerFondSituation(ctx context.Context, pool *pgxpool.Pool, out, root string, exercice int,
	lienDept func(code string) string) (*fondSituation, error) {
	f := &fondSituation{url: root + "/media/situation-france.svg", racine: root, lienDept: lienDept, communes: map[string]*geomSituation{},
		epci: map[string]*geomSituation{}, natureEPCI: map[string]string{}, domDept: map[string]contourSeul{},
		budgetCom: map[string][2]float64{}, budgetEPCI: map[string][2]float64{}, anneeBudget: exercice}
	var err error
	if f.deps, err = jeuContours(ctx, pool, "DEPARTEMENT", tolPleine); err != nil {
		return nil, err
	}
	if f.regs, err = jeuContours(ctx, pool, "REGION", tolPleine); err != nil {
		return nil, err
	}
	f.vb = f.deps.ViewBox
	for _, o := range f.deps.outremer {
		f.domDept[o.Code] = o
		f.domOrdre = append(f.domOrdre, o.Code)
	}
	sort.Strings(f.domOrdre)

	// Le fond partagé. Couleurs fixes et translucides : le document ne lit pas
	// les variables CSS du thème, il doit tenir sur papier clair comme sombre.
	// Chaque département est un lien vers sa page : on voyage de carte en
	// carte. Les liens sont relatifs au fichier (media/…) et ouvrent la page
	// entière (target="_top") : ils tiennent quel que soit le préfixe sous
	// lequel le site est servi.
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s">`, f.vb)
	b.WriteString(`<style>a path{cursor:pointer}a:hover path,a:focus path{fill:#2A8A96;fill-opacity:.38}a:focus{outline:none}</style>`)
	b.WriteString(`<g fill="#8C877D" fill-opacity=".16" stroke="#8C877D" stroke-opacity=".45" stroke-width="700" stroke-linejoin="round">`)
	for _, c := range f.deps.Codes {
		if u := lienDept(c); u != "" {
			fmt.Fprintf(&b, `<a href="../%s" target="_top"><title>%s</title><path d="%s"/></a>`,
				u, template.HTMLEscapeString(f.deps.Noms[c]), f.deps.traces[c])
		} else {
			fmt.Fprintf(&b, `<path d="%s"/>`, f.deps.traces[c])
		}
	}
	b.WriteString(`</g><g fill="none" stroke="#8C877D" stroke-opacity=".75" stroke-width="1800" stroke-linejoin="round" pointer-events="none">`)
	for _, c := range f.regs.Codes {
		fmt.Fprintf(&b, `<path d="%s"/>`, f.regs.traces[c])
	}
	b.WriteString(`</g></svg>`)
	if err := os.MkdirAll(filepath.Join(out, "media"), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(out, "media", "situation-france.svg"), []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	for _, code := range f.domOrdre {
		o := f.domDept[code]
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + o.ViewBox + `"><path d="` + o.Trace +
			`" fill="#8C877D" fill-opacity=".16" stroke="#8C877D" stroke-opacity=".5" stroke-width=".8" vector-effect="non-scaling-stroke" stroke-linejoin="round"/></svg>`
		if err := os.WriteFile(filepath.Join(out, "media", "situation-"+code+".svg"), []byte(svg), 0o644); err != nil {
			return nil, err
		}
	}

	var mill int
	if err := pool.QueryRow(ctx, `SELECT max(cog_millesime) FROM geo.contour_cog WHERE niveau='COMMUNE'`).Scan(&mill); err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT niveau, code, nom, coalesce(code_departement,''), coalesce(code_region,''), srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(geom,$2),srid_rendu),1,0),
		       coalesce(superficie_cadastrale_ha,0)::float8/100, coalesce(population,0),
		       st_x(st_transform(st_pointonsurface(geom),srid_rendu)), -st_y(st_transform(st_pointonsurface(geom),srid_rendu)),
		       coalesce(codes_siren_epci, '{}')
		FROM geo.contour_cog WHERE niveau IN ('COMMUNE','EPCI') AND cog_millesime=$1`, mill, tolSituation)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var niveau, code string
		g := &geomSituation{}
		if err := rows.Scan(&niveau, &code, &g.nom, &g.dept, &g.region, &g.srid, &g.d,
			&g.superficieKm2, &g.population, &g.x, &g.y, &g.epcis); err != nil {
			rows.Close()
			return nil, err
		}
		if niveau == "COMMUNE" {
			f.communes[code] = g
			f.totalKm2 += g.superficieKm2
			f.totalPop += g.population
		} else {
			f.epci[code] = g
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Superficie et population d'un groupement : celles de ses communes membres.
	for _, c := range f.communes {
		for _, s := range c.epcis {
			if e := f.epci[s]; e != nil {
				e.superficieKm2 += c.superficieKm2
			}
		}
	}

	nrows, err := pool.Query(ctx, `SELECT siren, nature_juridique FROM core.epci`)
	if err != nil {
		return nil, err
	}
	for nrows.Next() {
		var s, n string
		if err := nrows.Scan(&s, &n); err == nil {
			f.natureEPCI[s] = n
		}
	}
	nrows.Close()

	// Budgets agrégés par département : communes (montant par habitant ×
	// population) et groupements à fiscalité propre (montant publié).
	crows, err := pool.Query(ctx, `
		SELECT rc.code_departement,
		       coalesce(sum(f.value*p.value) FILTER (WHERE f.indicator_code='ofgl.fonctionnement_par_hab'),0)::float8,
		       coalesce(sum(f.value*p.value) FILTER (WHERE f.indicator_code='ofgl.investissement_par_hab'),0)::float8
		FROM core.commune_indicator f
		JOIN core.commune_indicator p ON p.commune_code=f.commune_code AND p.period_year=f.period_year
		 AND p.indicator_code='ofgl.population_totale'
		JOIN ref.commune rc ON rc.code_insee=f.commune_code AND rc.cog_millesime=f.cog_millesime
		WHERE f.period_year=$1 AND f.indicator_code IN ('ofgl.fonctionnement_par_hab','ofgl.investissement_par_hab')
		GROUP BY 1`, exercice)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var d string
		var fo, in float64
		if err := crows.Scan(&d, &fo, &in); err == nil {
			f.budgetCom[d] = [2]float64{fo, in}
		}
	}
	crows.Close()
	erows, err := pool.Query(ctx, `
		SELECT e.code_departement,
		       coalesce(sum(b.montant) FILTER (WHERE b.indicator_code='ofgl.fonctionnement_par_hab'),0)::float8,
		       coalesce(sum(b.montant) FILTER (WHERE b.indicator_code='ofgl.investissement_par_hab'),0)::float8
		FROM core.collectivite_budget b JOIN core.epci e ON e.siren=b.code
		WHERE b.niveau='GROUPEMENT' AND b.exercice=$1 AND e.code_departement IS NOT NULL
		GROUP BY 1`, exercice)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var d string
		var fo, in float64
		if err := erows.Scan(&d, &fo, &in); err == nil {
			f.budgetEPCI[d] = [2]float64{fo, in}
		}
	}
	erows.Close()
	return f, nil
}

// ── Dessin ──────────────────────────────────────────────────────────────

// calque : le territoire mis en évidence, dans une boîte donnée. Les traits
// ne changent pas d'épaisseur avec l'échelle (vector-effect).
type calque struct {
	b strings.Builder
}

func (c *calque) groupe(classe string, traces []string) {
	if len(traces) == 0 {
		return
	}
	fmt.Fprintf(&c.b, `<g class="%s">`, classe)
	for _, d := range traces {
		fmt.Fprintf(&c.b, `<path d="%s"/>`, d)
	}
	c.b.WriteString(`</g>`)
}

func (c *calque) repere(x, y float64, rayon float64) {
	fmt.Fprintf(&c.b, `<circle class="repere" cx="%.0f" cy="%.0f" r="%.0f"/>`, x, y, rayon)
}

func (c *calque) svg(vb, aria string) template.HTML {
	return template.HTML(`<svg viewBox="` + vb + `" class="calque" role="img" aria-label="` +
		template.HTMLEscapeString(aria) + `">` + c.b.String() + `</svg>`)
}

// cartons : les cinq départements d'outre-mer. Leur fond est une image
// partagée (media/situation-<code>.svg) ; seul le carton du territoire porte
// un calque, ce qui garde les 35 000 pages de communes légères.
func (f *fondSituation) cartons(actif string, calqueDOM func(*calque)) []CartonSituation {
	var out []CartonSituation
	for _, code := range f.domOrdre {
		o := f.domDept[code]
		k := CartonSituation{Nom: o.Nom, Fond: f.urlDOM(code), Actif: code == actif}
		if u := f.lienDept(code); u != "" {
			k.URL = f.racine + "/" + u
		}
		if k.Actif && calqueDOM != nil {
			var c calque
			calqueDOM(&c)
			k.SVG = template.HTML(`<svg viewBox="` + o.ViewBox + `" class="calque" aria-hidden="true">` + c.b.String() + `</svg>`)
		}
		out = append(out, k)
	}
	return out
}

func (f *fondSituation) urlDOM(code string) string {
	return f.racine + "/media/situation-" + code + ".svg"
}

func (f *fondSituation) envelopper(s *Situation, deptTerritoire string, dessiner func(*calque, bool), aria string) {
	s.Fond, s.ViewBox = f.url, f.vb
	if v := strings.Fields(f.vb); len(v) == 4 {
		s.Largeur, s.Hauteur = v[2], v[3]
	}
	_, dom := f.domDept[deptTerritoire]
	if !dom {
		var c calque
		dessiner(&c, false)
		s.Calque = c.svg(f.vb, aria)
	}
	s.Cartons = f.cartons(deptTerritoire, func(c *calque) { dessiner(c, true) })
}

// ── Légende ─────────────────────────────────────────────────────────────

func km2(v float64) string { return Nombre(int(v+0.5)) + " km²" }

func partFrance(v, total float64) string {
	if total <= 0 {
		return ""
	}
	p := 100 * v / total
	if p < 0.1 {
		return "moins de 0,1 % de la France"
	}
	return Decimal(p, 1) + " % de la France"
}

func meur(v float64) string {
	if v >= 1e9 {
		return Decimal(v/1e9, 2) + " Md€"
	}
	return Nombre(int(v/1e6+0.5)) + " M€"
}

func (f *fondSituation) legendeTerritoire(pop int, km float64) []LigneLegende {
	var l []LigneLegende
	if pop > 0 {
		l = append(l, ligneLegende("Population", Nombre(pop)+" habitants", "population municipale · "+partFrance(float64(pop), float64(f.totalPop))))
	}
	if km > 0 {
		det := partFrance(km, f.totalKm2)
		if pop > 0 {
			det += " · " + Nombre(int(float64(pop)/km+0.5)) + " hab./km²"
		}
		l = append(l, ligneLegende("Superficie", km2(km), det))
	}
	return l
}

// groupementsParNature : « 12 communautés de communes · 2 d'agglomération ».
func (f *fondSituation) groupementsParNature(sirens map[string]bool) string {
	n := map[string]int{}
	for s := range sirens {
		n[f.natureEPCI[s]]++
	}
	var parts []string
	for _, k := range []struct{ code, sing, plur string }{
		{"CC", "communauté de communes", "communautés de communes"},
		{"CA", "communauté d'agglomération", "communautés d'agglomération"},
		{"CU", "communauté urbaine", "communautés urbaines"},
		{"METRO", "métropole", "métropoles"},
		{"MET69", "Métropole de Lyon", "Métropole de Lyon"},
		{"EPT", "établissement public territorial", "établissements publics territoriaux"},
	} {
		if v := n[k.code]; v > 0 {
			lib := k.plur
			if v == 1 {
				lib = k.sing
			}
			parts = append(parts, Nombre(v)+" "+lib)
		}
	}
	return strings.Join(parts, " · ")
}

func (f *fondSituation) budgets(l []LigneLegende, conseil string, lignes []LigneFinance, depts map[string]bool) []LigneLegende {
	var fo, in float64
	for _, lf := range lignes {
		switch lf.Libelle {
		case "Dépenses de fonctionnement":
			fo = lf.Total
		case "Dépenses d'investissement":
			in = lf.Total
		}
	}
	if fo+in > 0 {
		l = append(l, ligneLegende("Budget du "+conseil, meur(fo+in)+" dépensés",
			meur(fo)+" de fonctionnement, "+meur(in)+" d'investissement ("+fmt.Sprint(f.anneeBudget)+")"))
	}
	var cf, ci, ef, ei float64
	for d := range depts {
		cf += f.budgetCom[d][0]
		ci += f.budgetCom[d][1]
		ef += f.budgetEPCI[d][0]
		ei += f.budgetEPCI[d][1]
	}
	if cf+ci > 0 {
		l = append(l, ligneLegende("Budget des communes", meur(cf+ci)+" dépensés", meur(cf)+" de fonctionnement, "+meur(ci)+" d'investissement"))
	}
	if ef+ei > 0 {
		l = append(l, ligneLegende("Budget des intercommunalités", meur(ef+ei)+" dépensés", meur(ef)+" de fonctionnement, "+meur(ei)+" d'investissement"))
	}
	return l
}

// ── Les quatre niveaux ──────────────────────────────────────────────────

func (f *fondSituation) pourDepartement(codeBudget, nom string, lignes []LigneFinance) *Situation {
	codes := codesCOGDe(codeBudget)
	depts := map[string]bool{}
	for _, c := range codes {
		depts[c] = true
	}
	var comTraces []string
	epcis := map[string]bool{}
	pop, km := 0, 0.0
	nCom := 0
	for _, c := range f.communes {
		if !depts[c.dept] {
			continue
		}
		comTraces = append(comTraces, c.d)
		pop += c.population
		km += c.superficieKm2
		nCom++
		for _, s := range c.epcis {
			epcis[s] = true
		}
	}
	var epciTraces []string
	for s := range epcis {
		if e := f.epci[s]; e != nil {
			epciTraces = append(epciTraces, e.d)
		}
	}
	var contour []string
	for _, c := range codes {
		if d := f.deps.traces[c]; d != "" {
			contour = append(contour, d)
		}
	}
	s := &Situation{}
	f.envelopper(s, codes[0], func(c *calque, dom bool) {
		c.groupe("communes", comTraces)
		c.groupe("groupements", epciTraces)
		if !dom {
			c.groupe("contour", contour)
		}
	}, "Situation de "+nom+" dans la France entière")
	s.Legende = f.legendeTerritoire(pop, km)
	s.Legende = append(s.Legende, ligneLegende("Communes", Nombre(nCom), ""))
	if g := f.groupementsParNature(epcis); g != "" {
		s.Legende = append(s.Legende, ligneLegende("Intercommunalités", Nombre(len(epcis)), g,
			f.racine+"/collectivites/?departement="+strings.Join(codes, ",")+"#carte-epci"))
	}
	s.Legende = f.budgets(s.Legende, "conseil départemental", lignes, depts)
	s.Note = "Communes en clair, intercommunalités en trait moyen, limite du département en trait épais. Les budgets ne s'additionnent pas : les transferts entre collectivités sont comptés chez chacune."
	return s
}

func (f *fondSituation) pourRegion(code, nom string, lignes []LigneFinance) *Situation {
	depts := map[string]bool{}
	epcis := map[string]bool{}
	pop, km, nCom := 0, 0.0, 0
	for _, c := range f.communes {
		if c.region != code {
			continue
		}
		depts[c.dept] = true
		pop += c.population
		km += c.superficieKm2
		nCom++
		for _, s := range c.epcis {
			epcis[s] = true
		}
	}
	var depTraces, epciTraces []string
	for d := range depts {
		if t := f.deps.traces[d]; t != "" {
			depTraces = append(depTraces, t)
		}
	}
	for s := range epcis {
		if e := f.epci[s]; e != nil {
			epciTraces = append(epciTraces, e.d)
		}
	}
	dom := regionDOM[code]
	s := &Situation{}
	f.envelopper(s, dom, func(c *calque, enDOM bool) {
		if enDOM {
			var com []string
			for _, cm := range f.communes {
				if cm.region == code {
					com = append(com, cm.d)
				}
			}
			c.groupe("communes", com)
			c.groupe("groupements", epciTraces)
			return
		}
		c.groupe("territoire", depTraces)
		c.groupe("groupements", epciTraces)
		c.groupe("departements", depTraces)
		if t := f.regs.traces[code]; t != "" {
			c.groupe("contour", []string{t})
		}
	}, "Situation de la région "+nom+" dans la France entière")
	s.Legende = f.legendeTerritoire(pop, km)
	s.Legende = append(s.Legende, ligneLegende("Départements", Nombre(len(depts)), ""),
		ligneLegende("Communes", Nombre(nCom), ""))
	if g := f.groupementsParNature(epcis); g != "" {
		s.Legende = append(s.Legende, ligneLegende("Intercommunalités", Nombre(len(epcis)), g,
			f.racine+"/collectivites/?region="+code+"#carte-epci"))
	}
	s.Legende = f.budgets(s.Legende, "conseil régional", lignes, depts)
	s.Note = "Départements de la région en clair, intercommunalités en trait fin, limite de la région en trait épais. Les budgets ne s'additionnent pas : les transferts entre collectivités sont comptés chez chacune."
	return s
}

func (f *fondSituation) pourEPCI(siren, nom string, lignes []LigneFinance) *Situation {
	e := f.epci[siren]
	if e == nil {
		return nil
	}
	var com []string
	pop, nCom := 0, 0
	for _, c := range f.communes {
		for _, s := range c.epcis {
			if s == siren {
				com = append(com, c.d)
				pop += c.population
				nCom++
			}
		}
	}
	s := &Situation{}
	f.envelopper(s, e.dept, func(c *calque, dom bool) {
		c.groupe("communes", com)
		c.groupe("contour", []string{e.d})
		if !dom {
			if t := f.deps.traces[e.dept]; t != "" {
				c.groupe("departements", []string{t})
			}
			c.repere(e.x, e.y, 16000)
		}
	}, "Situation de "+nom+" dans la France entière")
	s.Legende = f.legendeTerritoire(pop, e.superficieKm2)
	s.Legende = append(s.Legende, ligneLegende("Communes membres", Nombre(nCom), ""))
	var fo, in float64
	for _, lf := range lignes {
		switch lf.Libelle {
		case "Dépenses de fonctionnement":
			fo = lf.Total
		case "Dépenses d'investissement":
			in = lf.Total
		}
	}
	if fo+in > 0 {
		s.Legende = append(s.Legende, ligneLegende("Budget du groupement", meur(fo+in)+" dépensés", meur(fo)+" de fonctionnement, "+meur(in)+" d'investissement"))
	}
	s.Note = "Le groupement en évidence, ses communes en trait fin, son département de siège en trait moyen ; le cercle aide à le trouver à l'échelle de la France."
	return s
}

func (f *fondSituation) pourCommune(code, nom string) *Situation {
	c := f.communes[code]
	if c == nil {
		return nil
	}
	s := &Situation{}
	f.envelopper(s, c.dept, func(k *calque, dom bool) {
		k.groupe("commune", []string{c.d})
		if !dom {
			if t := f.deps.traces[c.dept]; t != "" {
				k.groupe("departements", []string{t})
			}
			k.repere(c.x, c.y, 14000)
		}
	}, "Situation de "+nom+" dans la France entière")
	s.Legende = f.legendeTerritoire(c.population, c.superficieKm2)
	s.Note = "La commune en évidence, son département en trait moyen ; le cercle aide à la trouver à l'échelle de la France."
	return s
}
