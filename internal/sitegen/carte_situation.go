package sitegen

import (
	"context"
	"database/sql"
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

const toleranceSituation = 0.005 // ≈ 500 m : à l'échelle de la France, un pixel couvre près de 2 km

type TileSituation struct {
	Name, Background string
	URL              string        // page du département d'outre-mer, vide s'il n'en a pas
	SVG              template.HTML // calque du carton actif, vide sinon
	Actif            bool
}

type LineLegend struct{ Label, Value, Detail, URL string }

// lineLegend : une ligne de légende, avec un lien facultatif sur la valeur.
func lineLegend(label, value, detail string, url ...string) LineLegend {
	l := LineLegend{Label: label, Value: value, Detail: detail}
	if len(url) > 0 {
		l.URL = url[0]
	}
	return l
}

type Situation struct {
	Background, ViewBox string
	Width, Height       string
	Layer               template.HTML // vide quand le territoire est en outre-mer
	Tiles               []TileSituation
	Legend              []LineLegend
	Note                string
}

type geomSituation struct {
	d, dept, region, name string
	srid                  int
	areaKm2               float64
	population            int
	x, y                  float64 // point intérieur, pour repérer une petite commune
	epcis                 []string
}

type backgroundSituation struct {
	url, vb        string
	root           string
	linkDept       func(code string) string
	deps, regs     *SetOutlines
	municipalities map[string]*geomSituation
	epci           map[string]*geomSituation
	circos         map[string]*geomSituation // circonscriptions législatives (circonscriptions.go)
	natureEPCI     map[string]string
	domDept        map[string]outlineOnly // code département d'outre-mer → carton
	domOrder       []string
	totalKm2       float64
	totalPop       int
	budgetCom      map[string][2]float64 // département → fonctionnement, investissement des communes
	budgetEPCI     map[string][2]float64 // département du siège → idem, groupements à fiscalité propre
	yearBudget     int
}

var regionDOM = map[string]string{"01": "971", "02": "972", "03": "973", "04": "974", "06": "976"}

// lienDept donne, pour un code de département, le chemin de sa page depuis la
// racine du site (« collectivites/departement/29/ »), vide s'il n'en a pas.
func loadBackgroundSituation(ctx context.Context, pool *pgxpool.Pool, out, root string, fiscalYear int,
	linkDept func(code string) string) (*backgroundSituation, error) {
	f := &backgroundSituation{url: root + "/media/situation-france.svg", root: root, linkDept: linkDept, municipalities: map[string]*geomSituation{},
		epci: map[string]*geomSituation{}, natureEPCI: map[string]string{}, domDept: map[string]outlineOnly{},
		budgetCom: map[string][2]float64{}, budgetEPCI: map[string][2]float64{}, yearBudget: fiscalYear}
	var err error
	if f.deps, err = setOutlines(ctx, pool, "DEPARTEMENT", toleranceFull); err != nil {
		return nil, err
	}
	if f.regs, err = setOutlines(ctx, pool, "REGION", toleranceFull); err != nil {
		return nil, err
	}
	f.vb = f.deps.ViewBox
	for _, o := range f.deps.overseas {
		f.domDept[o.Code] = o
		f.domOrder = append(f.domOrder, o.Code)
	}
	sort.Strings(f.domOrder)

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
		if u := linkDept(c); u != "" {
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
	// Les fleuves en dernier, par-dessus départements et régions — ce fichier
	// est un <img> statique, il ne lit pas les variables CSS du thème
	// (comme le reste de ce fond) : couleur fixe plutôt que var(--eau).
	// Un bleu franc et un trait aussi épais que la limite de région (1800,
	// juste au-dessus) : à la largeur d'affichage réelle de ce fond (~600 px
	// pour toute la France), le bleu discret de var(--eau) à 700 devenait
	// invisible — repéré sur les pages région et département.
	b.WriteString(`</g><g fill="none" stroke="#2C74A6" stroke-opacity=".85" stroke-width="1600" stroke-linejoin="round" pointer-events="none">`)
	b.WriteString(f.deps.rivers)
	b.WriteString(`</g></svg>`)
	if err := os.MkdirAll(filepath.Join(out, "media"), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(out, "media", "situation-france.svg"), []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	for _, code := range f.domOrder {
		o := f.domDept[code]
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + o.ViewBox + `"><path d="` + o.Trace +
			`" fill="#8C877D" fill-opacity=".16" stroke="#8C877D" stroke-opacity=".5" stroke-width=".8" vector-effect="non-scaling-stroke" stroke-linejoin="round"/></svg>`
		if err := os.WriteFile(filepath.Join(out, "media", "situation-"+code+".svg"), []byte(svg), 0o644); err != nil {
			return nil, err
		}
	}

	// max(...) est une agrégation : la ligne existe même sans contour encore
	// ingéré, avec un millésime NULL.
	var millN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT max(cog_millesime) FROM geo.contour_cog WHERE niveau='COMMUNE'`).Scan(&millN); err != nil {
		return nil, err
	}
	mill := int(millN.Int64)
	rows, err := pool.Query(ctx, `
		SELECT niveau, code, nom, coalesce(code_departement,''), coalesce(code_region,''), srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(geom,$2),srid_rendu),1,0),
		       coalesce(superficie_cadastrale_ha,0)::float8/100, coalesce(population,0),
		       st_x(st_transform(st_pointonsurface(geom),srid_rendu)), -st_y(st_transform(st_pointonsurface(geom),srid_rendu)),
		       coalesce(codes_siren_epci, '{}')
		FROM geo.contour_cog WHERE niveau IN ('COMMUNE','EPCI') AND cog_millesime=$1`, mill, toleranceSituation)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var level, code string
		g := &geomSituation{}
		if err := rows.Scan(&level, &code, &g.name, &g.dept, &g.region, &g.srid, &g.d,
			&g.areaKm2, &g.population, &g.x, &g.y, &g.epcis); err != nil {
			rows.Close()
			return nil, err
		}
		if level == "COMMUNE" {
			f.municipalities[code] = g
			f.totalKm2 += g.areaKm2
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
	for _, c := range f.municipalities {
		for _, s := range c.epcis {
			if e := f.epci[s]; e != nil {
				e.areaKm2 += c.areaKm2
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
	// population) et groupements à fiscalité propre (montant publié) — mv.
	// dept_budget_commune/mv.dept_budget_epci (internal/matview) remplacent
	// le calcul.
	crows, err := pool.Query(ctx, `
		SELECT code_departement, fonctionnement, investissement
		FROM mv.dept_budget_commune WHERE period_year=$1`, fiscalYear)
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
		SELECT code_departement, fonctionnement, investissement
		FROM mv.dept_budget_epci WHERE exercice=$1`, fiscalYear)
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

// layer : le territoire mis en évidence, dans une boîte donnée. Les traits
// ne changent pas d'épaisseur avec l'échelle (vector-effect).
type layer struct {
	b strings.Builder
}

func (c *layer) group(class string, traces []string) {
	if len(traces) == 0 {
		return
	}
	fmt.Fprintf(&c.b, `<g class="%s">`, class)
	for _, d := range traces {
		fmt.Fprintf(&c.b, `<path d="%s"/>`, d)
	}
	c.b.WriteString(`</g>`)
}

func (c *layer) marker(x, y float64, radius float64) {
	fmt.Fprintf(&c.b, `<circle class="repere" cx="%.0f" cy="%.0f" r="%.0f"/>`, x, y, radius)
}

func (c *layer) svg(vb, aria string) template.HTML {
	return template.HTML(`<svg viewBox="` + vb + `" class="calque" role="img" aria-label="` +
		template.HTMLEscapeString(aria) + `">` + c.b.String() + `</svg>`)
}

// tiles : les cinq départements d'outre-mer. Leur fond est une image
// partagée (media/situation-<code>.svg) ; seul le carton du territoire porte
// un calque, ce qui garde les 35 000 pages de communes légères.
func (f *backgroundSituation) tiles(actif string, layerDOM func(*layer)) []TileSituation {
	var out []TileSituation
	for _, code := range f.domOrder {
		o := f.domDept[code]
		k := TileSituation{Name: o.Name, Background: f.urlDOM(code), Actif: code == actif}
		if u := f.linkDept(code); u != "" {
			k.URL = f.root + "/" + u
		}
		if k.Actif && layerDOM != nil {
			var c layer
			layerDOM(&c)
			k.SVG = template.HTML(`<svg viewBox="` + o.ViewBox + `" class="calque" aria-hidden="true">` + c.b.String() + `</svg>`)
		}
		out = append(out, k)
	}
	return out
}

func (f *backgroundSituation) urlDOM(code string) string {
	return f.root + "/media/situation-" + code + ".svg"
}

func (f *backgroundSituation) envelopper(s *Situation, deptTerritory string, draw func(*layer, bool), aria string) {
	s.Background, s.ViewBox = f.url, f.vb
	if v := strings.Fields(f.vb); len(v) == 4 {
		s.Width, s.Height = v[2], v[3]
	}
	_, dom := f.domDept[deptTerritory]
	if !dom {
		var c layer
		draw(&c, false)
		s.Layer = c.svg(f.vb, aria)
	}
	s.Tiles = f.tiles(deptTerritory, func(c *layer) { draw(c, true) })
}

// ── Légende ─────────────────────────────────────────────────────────────

func km2(v float64) string { return Count(int(v+0.5)) + " km²" }

func shareFrance(v, total float64) string {
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
	return Count(int(v/1e6+0.5)) + " M€"
}

func (f *backgroundSituation) legendTerritory(pop int, km float64) []LineLegend {
	var l []LineLegend
	if pop > 0 {
		l = append(l, lineLegend("Population", Count(pop)+" habitants", "population municipale · "+shareFrance(float64(pop), float64(f.totalPop))))
	}
	if km > 0 {
		det := shareFrance(km, f.totalKm2)
		if pop > 0 {
			det += " · " + Count(int(float64(pop)/km+0.5)) + " hab./km²"
		}
		l = append(l, lineLegend("Superficie", km2(km), det))
	}
	return l
}

// groupingsPerNature : « 12 communautés de communes · 2 d'agglomération ».
func (f *backgroundSituation) groupingsPerNature(sirens map[string]bool) string {
	n := map[string]int{}
	for s := range sirens {
		n[f.natureEPCI[s]]++
	}
	var shares []string
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
			shares = append(shares, Count(v)+" "+lib)
		}
	}
	return strings.Join(shares, " · ")
}

func (f *backgroundSituation) budgets(l []LineLegend, conseil string, lines []LineFinance, depts map[string]bool) []LineLegend {
	var fo, in float64
	for _, lf := range lines {
		switch lf.Label {
		case "Dépenses de fonctionnement":
			fo = lf.Total
		case "Dépenses d'investissement":
			in = lf.Total
		}
	}
	if fo+in > 0 {
		l = append(l, lineLegend("Budget du "+conseil, meur(fo+in)+" dépensés",
			meur(fo)+" de fonctionnement, "+meur(in)+" d'investissement ("+fmt.Sprint(f.yearBudget)+")"))
	}
	var cf, ci, ef, ei float64
	for d := range depts {
		cf += f.budgetCom[d][0]
		ci += f.budgetCom[d][1]
		ef += f.budgetEPCI[d][0]
		ei += f.budgetEPCI[d][1]
	}
	if cf+ci > 0 {
		l = append(l, lineLegend("Budget des communes", meur(cf+ci)+" dépensés", meur(cf)+" de fonctionnement, "+meur(ci)+" d'investissement"))
	}
	if ef+ei > 0 {
		l = append(l, lineLegend("Budget des intercommunalités", meur(ef+ei)+" dépensés", meur(ef)+" de fonctionnement, "+meur(ei)+" d'investissement"))
	}
	return l
}

// ── Les quatre niveaux ──────────────────────────────────────────────────

func (f *backgroundSituation) forDepartment(codeBudget, name string, lines []LineFinance) *Situation {
	codes := codesCOGOf(codeBudget)
	depts := map[string]bool{}
	for _, c := range codes {
		depts[c] = true
	}
	var comTraces []string
	epcis := map[string]bool{}
	pop, km := 0, 0.0
	nCom := 0
	for _, c := range f.municipalities {
		if !depts[c.dept] {
			continue
		}
		comTraces = append(comTraces, c.d)
		pop += c.population
		km += c.areaKm2
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
	var outline []string
	for _, c := range codes {
		if d := f.deps.traces[c]; d != "" {
			outline = append(outline, d)
		}
	}
	s := &Situation{}
	f.envelopper(s, codes[0], func(c *layer, dom bool) {
		c.group("communes", comTraces)
		c.group("groupements", epciTraces)
		if !dom {
			c.group("contour", outline)
		}
	}, "Situation de "+name+" dans la France entière")
	s.Legend = f.legendTerritory(pop, km)
	s.Legend = append(s.Legend, lineLegend("Communes", Count(nCom), ""))
	if g := f.groupingsPerNature(epcis); g != "" {
		s.Legend = append(s.Legend, lineLegend("Intercommunalités", Count(len(epcis)), g))
	}
	s.Legend = f.budgets(s.Legend, "conseil départemental", lines, depts)
	s.Note = "Communes en clair, intercommunalités en trait moyen, limite du département en trait épais. Les budgets ne s'additionnent pas : les transferts entre collectivités sont comptés chez chacune."
	return s
}

func (f *backgroundSituation) forRegion(code, name string, lines []LineFinance) *Situation {
	depts := map[string]bool{}
	epcis := map[string]bool{}
	pop, km, nCom := 0, 0.0, 0
	for _, c := range f.municipalities {
		if c.region != code {
			continue
		}
		depts[c.dept] = true
		pop += c.population
		km += c.areaKm2
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
	f.envelopper(s, dom, func(c *layer, inDOM bool) {
		if inDOM {
			var com []string
			for _, cm := range f.municipalities {
				if cm.region == code {
					com = append(com, cm.d)
				}
			}
			c.group("communes", com)
			c.group("groupements", epciTraces)
			return
		}
		c.group("territoire", depTraces)
		c.group("groupements", epciTraces)
		c.group("departements", depTraces)
		if t := f.regs.traces[code]; t != "" {
			c.group("contour", []string{t})
		}
	}, "Situation de la région "+name+" dans la France entière")
	s.Legend = f.legendTerritory(pop, km)
	s.Legend = append(s.Legend, lineLegend("Départements", Count(len(depts)), ""),
		lineLegend("Communes", Count(nCom), ""))
	if g := f.groupingsPerNature(epcis); g != "" {
		s.Legend = append(s.Legend, lineLegend("Intercommunalités", Count(len(epcis)), g))
	}
	s.Legend = f.budgets(s.Legend, "conseil régional", lines, depts)
	s.Note = "Départements de la région en clair, intercommunalités en trait fin, limite de la région en trait épais. Les budgets ne s'additionnent pas : les transferts entre collectivités sont comptés chez chacune."
	return s
}

func (f *backgroundSituation) forEPCI(siren, name string, lines []LineFinance) *Situation {
	e := f.epci[siren]
	if e == nil {
		return nil
	}
	var com []string
	pop, nCom := 0, 0
	for _, c := range f.municipalities {
		for _, s := range c.epcis {
			if s == siren {
				com = append(com, c.d)
				pop += c.population
				nCom++
			}
		}
	}
	s := &Situation{}
	f.envelopper(s, e.dept, func(c *layer, dom bool) {
		c.group("communes", com)
		c.group("contour", []string{e.d})
		if !dom {
			if t := f.deps.traces[e.dept]; t != "" {
				c.group("departements", []string{t})
			}
			c.marker(e.x, e.y, 16000)
		}
	}, "Situation de "+name+" dans la France entière")
	s.Legend = f.legendTerritory(pop, e.areaKm2)
	s.Legend = append(s.Legend, lineLegend("Communes membres", Count(nCom), ""))
	var fo, in float64
	for _, lf := range lines {
		switch lf.Label {
		case "Dépenses de fonctionnement":
			fo = lf.Total
		case "Dépenses d'investissement":
			in = lf.Total
		}
	}
	if fo+in > 0 {
		s.Legend = append(s.Legend, lineLegend("Budget du groupement", meur(fo+in)+" dépensés", meur(fo)+" de fonctionnement, "+meur(in)+" d'investissement"))
	}
	s.Note = "Le groupement en évidence, ses communes en trait fin, son département de siège en trait moyen ; le cercle aide à le trouver à l'échelle de la France."
	return s
}

func (f *backgroundSituation) forMunicipality(code, name string) *Situation {
	c := f.municipalities[code]
	if c == nil {
		return nil
	}
	s := &Situation{}
	f.envelopper(s, c.dept, func(k *layer, dom bool) {
		k.group("commune", []string{c.d})
		if !dom {
			if t := f.deps.traces[c.dept]; t != "" {
				k.group("departements", []string{t})
			}
			k.marker(c.x, c.y, 14000)
		}
	}, "Situation de "+name+" dans la France entière")
	s.Legend = f.legendTerritory(c.population, c.areaKm2)
	s.Note = "La commune en évidence, son département en trait moyen ; le cercle aide à la trouver à l'échelle de la France."
	return s
}
