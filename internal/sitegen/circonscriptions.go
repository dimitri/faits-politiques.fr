package sitegen

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les pages de circonscription législative : où elle se trouve, ce qu'elle
// pèse, qui l'a représentée depuis 2012, et le portrait qu'en publie l'Insee.
// Données : db/migrations/0110, internal/geo/circonscriptions.go.

// Le découpage de 2010 s'applique depuis les législatives des 10 et 17 juin
// 2012. Un mandat commencé avant représentait un autre territoire sous le même
// numéro : il n'est ni listé ni relié.
const startBreakdown2012 = "2012-06-17"

// keyDistrict : un nom de département réduit à ses lettres, pour rapprocher
// « Alpes de Haute-Provence » (Insee) et « Alpes-de-Haute-Provence »
// (Assemblée), « La Réunion » et « Réunion », « Côte-d’Or » et « Côte-d'Or ».
func keyDistrict(s string) string {
	s = KeySort(s)
	s = strings.TrimPrefix(s, "la ")
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var reCodeRNEDistrict = regexp.MustCompile(`^(?:[0-9]{2}|2[AB])[0-9]{2}$|^9[78][0-9]{3}$`)

// codeDistrict : le code Insee d'une circonscription, lu dans le libellé d'un
// mandat. « Dordogne, 1e circonscription » → 24001 ; « 0608 8Ème
// Circonscription » → 06008 ; « 97701 1Ère Circonscription » → 97801 (le
// répertoire des élus et l'Insee ne numérotent pas Saint-Barthélemy et
// Saint-Martin de la même façon). Vide pour les Français établis hors de
// France, que l'Insee ne décrit pas.
func (r *Resolver) codeDistrict(district string) string {
	var code string
	if m := reDistrictAN.FindStringSubmatch(district); m != nil {
		dep := r.deptDistrict[keyDistrict(m[1])]
		n, err := strconv.Atoi(m[2])
		if dep == "" || err != nil {
			return ""
		}
		if len(dep) == 3 {
			code = fmt.Sprintf("%s%02d", dep, n)
		} else {
			code = fmt.Sprintf("%s%03d", dep, n)
		}
	} else if m := reCodeLabel.FindStringSubmatch(district); m != nil && reCodeRNEDistrict.MatchString(m[1]) {
		code = m[1]
		if len(code) == 4 {
			code = code[:2] + "0" + code[2:]
		}
		if code == "97701" {
			code = "97801"
		}
	}
	if !r.circos[code] {
		return ""
	}
	return code
}

// numberDistrict et ordinalCirco : « 24001 » → 1, « 1re » ; « 97302 » → 2, « 2e ».
func numberDistrict(code string) int {
	n, _ := strconv.Atoi(code[len(code)-2:])
	return n
}

func ordinalDistrict(code string) string {
	if n := numberDistrict(code); n == 1 {
		return "1re"
	} else {
		return strconv.Itoa(n) + "e"
	}
}

// Les collectivités d'outre-mer n'ont pas de contour départemental, donc pas
// de nom au référentiel géographique ; le libellé Insee les orthographie à sa
// façon (« Polynésie-Française », « Nouvelle-Caledonie »).
var nomsCOM = map[string]string{
	"975": "Saint-Pierre-et-Miquelon", "978": "Saint-Barthélemy et Saint-Martin",
	"986": "Wallis-et-Futuna", "987": "Polynésie française", "988": "Nouvelle-Calédonie",
}

// PhraseMunicipalities : « 49 communes », « 12 communes, dont 2 en partie »,
// « une partie de la commune de Paris ».
func (p *PageDistrict) PhraseMunicipalities() string {
	n := len(p.Municipalities)
	switch {
	case n == 1 && p.CountPartial == 1:
		return "une partie de la commune de " + p.Municipalities[0].Name
	case n == 1:
		return "une commune"
	case p.CountPartial == n:
		return Count(n) + " communes, chacune en partie"
	case p.CountPartial > 0:
		return Count(n) + " communes, dont " + Count(p.CountPartial) + " en partie"
	}
	return Count(n) + " communes"
}

type MunicipalityDistrict struct {
	Place
	Partial bool
}

type DeputyDistrict struct {
	Name, Slug, Period string
	Ongoing            bool
	start              string
}

type LinePortrait struct {
	Label, Value, France string
}

type PageDistrict struct {
	Code, Title, Ordinal     string
	Dept, Region             Place
	Population, Population13 int
	Evolution                string // taux annuel 2013-2019, déjà formaté
	Registered               int
	Municipalities           []MunicipalityDistrict
	CountPartial             int
	Deputies                 []DeputyDistrict
	Ongoing                  []DeputyDistrict
	Portrait                 []LinePortrait
	WithOutline              bool
	Situation                *Situation
	areaApprox               float64 // km², aire du contour Insee
}

// Le portrait : une sélection des variables Insee, libellées d'après leur
// définition. Attention aux dénominateurs : l'Insee publie « part de la
// population active au chômage », mais les six catégories d'activité (en
// emploi, chômeurs, retraités, élèves, moins de 14 ans, autres inactifs)
// somment à 100 % de la population totale — c'est donc elle le dénominateur,
// et le libellé le dit. Les catégories socioprofessionnelles, elles, somment à
// 100 % des actifs.
var portraitDistrict = []struct {
	Var, Label, Unit string
	Dec              int
}{
	{"age_moyen", "Âge moyen de la population", " ans", 1},
	{"actemp", "Actifs en emploi, en part de la population totale", " %", 1},
	{"actcho", "Chômeurs, en part de la population totale", " %", 1},
	{"inactret", "Retraités, en part de la population totale", " %", 1},
	{"act_cad", "Cadres et professions intellectuelles supérieures, en part des actifs", " %", 1},
	{"act_ouv", "Ouvriers, en part des actifs", " %", 1},
	{"actdip_BAC3P", "Actifs diplômés de niveau bac + 3 ou plus, en part des actifs", " %", 1},
	{"nivvie_median_diff", "Niveau de vie médian", " € par an", 0},
	{"tx_pauvrete60_diff", "Taux de pauvreté au seuil de 60 % du niveau de vie médian", " %", 1},
	{"rpt_D9_D1_diff", "Rapport entre le 9e et le 1er décile de niveau de vie", "", 1},
	{"maison", "Maisons, en part des résidences principales", " %", 1},
	{"log_vac", "Logements vacants, en part des logements", " %", 1},
	{"modtrans_voit", "Actifs en emploi allant travailler surtout en voiture", " %", 1},
	{"modtrans_commun", "Actifs en emploi allant travailler surtout en transport en commun", " %", 1},
	{"acc_medecin", "Population ayant un médecin dans sa commune de résidence", " %", 1},
}

func loadDistricts(ctx context.Context, pool *pgxpool.Pool, r *Resolver,
	persons map[string]*Person) (map[string]*PageDistrict, error) {

	pages := map[string]*PageDistrict{}
	rows, err := pool.Query(ctx, `
		SELECT c.code, c.code_departement, g.code IS NOT NULL,
		       coalesce(ST_Area(g.geom::geography)/1e6, 0)
		  FROM ref.circonscription_legislative c
		  LEFT JOIN geo.contour_circonscription g USING (code)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := &PageDistrict{}
		var dep string
		if err := rows.Scan(&p.Code, &dep, &p.WithOutline, &p.areaApprox); err != nil {
			rows.Close()
			return nil, err
		}
		p.Ordinal = ordinalDistrict(p.Code)
		if l, ok := r.placeDept(dep); ok {
			p.Dept = l
		} else {
			p.Dept = Place{Type: "DEPARTEMENT", Code: dep}
		}
		pages[p.Code] = p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Noms : ceux des départements du site ; pour les collectivités d'outre-mer,
	// le libellé Insee.
	nrows, err := pool.Query(ctx, `SELECT code, nom FROM ref.circonscription_legislative`)
	if err != nil {
		return nil, err
	}
	for nrows.Next() {
		var code, name string
		if err := nrows.Scan(&code, &name); err != nil {
			nrows.Close()
			return nil, err
		}
		p := pages[code]
		if n := nomsCOM[p.Dept.Code]; n != "" {
			p.Dept.Name = n
		} else if p.Dept.Name == "" {
			d, _, _ := strings.Cut(name, " - ")
			p.Dept.Name = strings.TrimSpace(d)
		}
		p.Title = p.Dept.Name + ", " + p.Ordinal + " circonscription"
	}
	nrows.Close()

	regionOf := map[string]string{}
	for _, c := range r.municipalities {
		regionOf[c.Dept] = c.Region
	}
	for _, p := range pages {
		if l, ok := r.placeRegion(regionOf[p.Dept.Code]); ok {
			p.Region = l
		}
	}

	crows, err := pool.Query(ctx, `
		SELECT circonscription, commune_code, commune_nom, entiere
		  FROM ref.circonscription_commune ORDER BY commune_nom`)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var district, code, name string
		var whole bool
		if err := crows.Scan(&district, &code, &name, &whole); err != nil {
			crows.Close()
			return nil, err
		}
		p := pages[district]
		if p == nil {
			continue
		}
		l := Place{Type: "COMMUNE", Code: code, Name: name, URL: r.urlMunicipality(code)}
		p.Municipalities = append(p.Municipalities, MunicipalityDistrict{Place: l, Partial: !whole})
		if !whole {
			p.CountPartial++
		}
	}
	crows.Close()

	vals := map[string]map[string]float64{}
	irows, err := pool.Query(ctx, `SELECT circonscription, variable, valeur::float8 FROM core.circonscription_indicateur`)
	if err != nil {
		return nil, err
	}
	for irows.Next() {
		var c, v string
		var x float64
		if err := irows.Scan(&c, &v, &x); err != nil {
			irows.Close()
			return nil, err
		}
		if vals[c] == nil {
			vals[c] = map[string]float64{}
		}
		vals[c][v] = x
	}
	irows.Close()
	france := vals["00000"]
	for code, p := range pages {
		v := vals[code]
		p.Population = int(v["pop_légal_19"])
		p.Population13 = int(v["pop_légal_13"])
		p.Registered = int(v["Inscrit_22"])
		if t, ok := v["tvar_pop"]; ok {
			p.Evolution = signe(t) + Decimal(t, 1) + " % par an"
		}
		for _, d := range portraitDistrict {
			x, ok := v[d.Var]
			if !ok {
				continue
			}
			l := LinePortrait{Label: d.Label, Value: Decimal(x, d.Dec) + d.Unit}
			if f, ok := france[d.Var]; ok {
				l.France = Decimal(f, d.Dec) + d.Unit
			}
			p.Portrait = append(p.Portrait, l)
		}
	}

	// Les députés élus dans la circonscription depuis juin 2012. Un libellé
	// qui ne se rattache à rien est signalé au build, pas ignoré en silence.
	unattached := map[string]int{}
	for _, pr := range persons {
		for _, m := range pr.Terms {
			if m.Type != "DEPUTE" || m.StartISO < startBreakdown2012 {
				continue
			}
			p := pages[r.codeDistrict(m.District)]
			if p == nil {
				if !strings.Contains(m.District, "hors de France") && !strings.HasPrefix(m.District, "ZZ") {
					unattached[m.District]++
				}
				continue
			}
			d := DeputyDistrict{Name: strings.TrimSpace(pr.FirstName + " " + pr.Name), Slug: pr.Slug,
				Period: m.Period, Ongoing: m.EndISO == "", start: m.StartISO}
			p.Deputies = append(p.Deputies, d)
		}
	}
	if len(unattached) > 0 {
		fmt.Printf("  circonscriptions : %d libellés de mandat de député non rattachés : %v\n", len(unattached), unattached)
	}
	for _, p := range pages {
		sort.SliceStable(p.Deputies, func(i, j int) bool { return p.Deputies[i].start > p.Deputies[j].start })
		// Deux sources publient le même mandat en cours (Assemblée et
		// répertoire des élus) : une personne n'apparaît qu'une fois en tête.
		seen := map[string]bool{}
		for _, d := range p.Deputies {
			if d.Ongoing && !seen[d.Slug] {
				seen[d.Slug] = true
				p.Ongoing = append(p.Ongoing, d)
			}
		}
	}
	return pages, nil
}

func signe(x float64) string {
	if x > 0 {
		return "+"
	}
	return ""
}

// ── Carte de situation ──────────────────────────────────────────────────

func (f *backgroundSituation) loadDistricts(ctx context.Context, pool *pgxpool.Pool) error {
	f.circos = map[string]*geomSituation{}
	rows, err := pool.Query(ctx, `
		SELECT g.code, c.code_departement, g.srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(g.geom,$1),g.srid_rendu),1,0),
		       st_x(st_transform(st_pointonsurface(g.geom),g.srid_rendu)), -st_y(st_transform(st_pointonsurface(g.geom),g.srid_rendu))
		  FROM geo.contour_circonscription g JOIN ref.circonscription_legislative c USING (code)`, toleranceSituation)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		g := &geomSituation{}
		var code string
		if err := rows.Scan(&code, &g.dept, &g.srid, &g.d, &g.x, &g.y); err != nil {
			return err
		}
		f.circos[code] = g
	}
	return rows.Err()
}

func (f *backgroundSituation) forDistrict(p *PageDistrict, popFrance int) *Situation {
	g := f.circos[p.Code]
	if g == nil {
		return nil
	}
	s := &Situation{}
	f.envelopper(s, g.dept, func(c *layer, dom bool) {
		c.group("territoire", []string{g.d})
		c.group("contour", []string{g.d})
		if !dom {
			if t := f.deps.traces[g.dept]; t != "" {
				c.group("departements", []string{t})
			}
			c.marker(g.x, g.y, 16000)
		}
	}, "Situation de la "+p.Ordinal+" circonscription ("+p.Dept.Name+") dans la France entière")

	if p.Population > 0 {
		det := "population légale 2019"
		if popFrance > 0 {
			det += " · " + shareFrance(float64(p.Population), float64(popFrance))
		}
		if p.Evolution != "" {
			det += " · " + p.Evolution + " depuis 2013"
		}
		s.Legend = append(s.Legend, lineLegend("Population", Count(p.Population)+" habitants", det))
	}

	// Superficie : cadastrale (IGN) quand la circonscription est faite de
	// communes entières toutes présentes au COG courant ; sinon l'aire du
	// contour Insee simplifié, annoncée comme approchée.
	km, exact := 0.0, p.CountPartial == 0
	for _, c := range p.Municipalities {
		cg := f.municipalities[c.Code]
		if cg == nil {
			exact = false
			break
		}
		km += cg.areaKm2
	}
	value := km2(km)
	det := shareFrance(km, f.totalKm2)
	if !exact {
		km = p.areaApprox
		value = "≈ " + km2(km)
		det = shareFrance(km, f.totalKm2) + " · aire du contour Insee simplifié"
	}
	if km > 0 && p.Population > 0 {
		det += " · " + Count(int(float64(p.Population)/km+0.5)) + " hab./km²"
	}
	s.Legend = append(s.Legend, lineLegend("Superficie", value, det))

	com := Count(len(p.Municipalities))
	detCom := ""
	if len(p.Municipalities) == 1 && p.CountPartial == 1 {
		detCom = p.Municipalities[0].Name + ", en partie : la commune est partagée entre plusieurs circonscriptions"
	} else if p.CountPartial > 0 {
		detCom = "dont " + Count(p.CountPartial) + " en partie seulement, partagée"
		if p.CountPartial > 1 {
			detCom += "s"
		}
		detCom += " avec une autre circonscription"
	}
	s.Legend = append(s.Legend, lineLegend("Communes", com, detCom))
	if p.Registered > 0 {
		s.Legend = append(s.Legend, lineLegend("Inscrits sur les listes électorales", Count(p.Registered), "au 11 avril 2022"))
	}
	if len(p.Ongoing) > 0 {
		var noms []string
		for _, d := range p.Ongoing {
			noms = append(noms, d.Name)
		}
		s.Legend = append(s.Legend, lineLegend("À l'Assemblée nationale", strings.Join(noms, ", "), p.Ongoing[0].Period))
	}
	s.Legend = append(s.Legend, lineLegend("Budget", "aucun",
		"une circonscription élit un député ; elle n'a ni conseil ni budget"))
	s.Note = "La circonscription en évidence, son département en trait moyen ; le cercle aide à la trouver à l'échelle de la France. Contour et population : Insee, portraits des circonscriptions législatives (fond du 3 mai 2022)."
	return s
}
