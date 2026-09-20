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
const debutDecoupage2012 = "2012-06-17"

// cleCirco : un nom de département réduit à ses lettres, pour rapprocher
// « Alpes de Haute-Provence » (Insee) et « Alpes-de-Haute-Provence »
// (Assemblée), « La Réunion » et « Réunion », « Côte-d’Or » et « Côte-d'Or ».
func cleCirco(s string) string {
	s = CleTri(s)
	s = strings.TrimPrefix(s, "la ")
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var reCodeRNECirco = regexp.MustCompile(`^(?:[0-9]{2}|2[AB])[0-9]{2}$|^9[78][0-9]{3}$`)

// codeCirco : le code Insee d'une circonscription, lu dans le libellé d'un
// mandat. « Dordogne, 1e circonscription » → 24001 ; « 0608 8Ème
// Circonscription » → 06008 ; « 97701 1Ère Circonscription » → 97801 (le
// répertoire des élus et l'Insee ne numérotent pas Saint-Barthélemy et
// Saint-Martin de la même façon). Vide pour les Français établis hors de
// France, que l'Insee ne décrit pas.
func (r *Resolveur) codeCirco(circo string) string {
	var code string
	if m := reCircoAN.FindStringSubmatch(circo); m != nil {
		dep := r.deptCirco[cleCirco(m[1])]
		n, err := strconv.Atoi(m[2])
		if dep == "" || err != nil {
			return ""
		}
		if len(dep) == 3 {
			code = fmt.Sprintf("%s%02d", dep, n)
		} else {
			code = fmt.Sprintf("%s%03d", dep, n)
		}
	} else if m := reCodeLibelle.FindStringSubmatch(circo); m != nil && reCodeRNECirco.MatchString(m[1]) {
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

// numeroCirco et ordinalCirco : « 24001 » → 1, « 1re » ; « 97302 » → 2, « 2e ».
func numeroCirco(code string) int {
	n, _ := strconv.Atoi(code[len(code)-2:])
	return n
}

func ordinalCirco(code string) string {
	if n := numeroCirco(code); n == 1 {
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

// PhraseCommunes : « 49 communes », « 12 communes, dont 2 en partie »,
// « une partie de la commune de Paris ».
func (p *PageCirco) PhraseCommunes() string {
	n := len(p.Communes)
	switch {
	case n == 1 && p.NbPartielles == 1:
		return "une partie de la commune de " + p.Communes[0].Nom
	case n == 1:
		return "une commune"
	case p.NbPartielles == n:
		return Nombre(n) + " communes, chacune en partie"
	case p.NbPartielles > 0:
		return Nombre(n) + " communes, dont " + Nombre(p.NbPartielles) + " en partie"
	}
	return Nombre(n) + " communes"
}

type CommuneCirco struct {
	Lieu
	Partielle bool
}

type DeputeCirco struct {
	Nom, Slug, Periode string
	EnCours            bool
	debut              string
}

type LignePortrait struct {
	Libelle, Valeur, France string
}

type PageCirco struct {
	Code, Titre, Ordinal     string
	Dept, Region             Lieu
	Population, Population13 int
	Evolution                string // taux annuel 2013-2019, déjà formaté
	Inscrits                 int
	Communes                 []CommuneCirco
	NbPartielles             int
	Deputes                  []DeputeCirco
	EnCours                  []DeputeCirco
	Portrait                 []LignePortrait
	AvecContour              bool
	Situation                *Situation
	superficieApprox         float64 // km², aire du contour Insee
}

// Le portrait : une sélection des variables Insee, libellées d'après leur
// définition. Attention aux dénominateurs : l'Insee publie « part de la
// population active au chômage », mais les six catégories d'activité (en
// emploi, chômeurs, retraités, élèves, moins de 14 ans, autres inactifs)
// somment à 100 % de la population totale — c'est donc elle le dénominateur,
// et le libellé le dit. Les catégories socioprofessionnelles, elles, somment à
// 100 % des actifs.
var portraitCirco = []struct {
	Var, Libelle, Unite string
	Dec                 int
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

func chargerCirconscriptions(ctx context.Context, pool *pgxpool.Pool, r *Resolveur,
	persons map[string]*Person) (map[string]*PageCirco, error) {

	pages := map[string]*PageCirco{}
	rows, err := pool.Query(ctx, `
		SELECT c.code, c.code_departement, g.code IS NOT NULL,
		       coalesce(ST_Area(g.geom::geography)/1e6, 0)
		  FROM ref.circonscription_legislative c
		  LEFT JOIN geo.contour_circonscription g USING (code)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := &PageCirco{}
		var dep string
		if err := rows.Scan(&p.Code, &dep, &p.AvecContour, &p.superficieApprox); err != nil {
			rows.Close()
			return nil, err
		}
		p.Ordinal = ordinalCirco(p.Code)
		if l, ok := r.lieuDept(dep); ok {
			p.Dept = l
		} else {
			p.Dept = Lieu{Type: "DEPARTEMENT", Code: dep}
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
		var code, nom string
		if err := nrows.Scan(&code, &nom); err != nil {
			nrows.Close()
			return nil, err
		}
		p := pages[code]
		if n := nomsCOM[p.Dept.Code]; n != "" {
			p.Dept.Nom = n
		} else if p.Dept.Nom == "" {
			d, _, _ := strings.Cut(nom, " - ")
			p.Dept.Nom = strings.TrimSpace(d)
		}
		p.Titre = p.Dept.Nom + ", " + p.Ordinal + " circonscription"
	}
	nrows.Close()

	regionDe := map[string]string{}
	for _, c := range r.communes {
		regionDe[c.Dept] = c.Region
	}
	for _, p := range pages {
		if l, ok := r.lieuRegion(regionDe[p.Dept.Code]); ok {
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
		var circo, code, nom string
		var entiere bool
		if err := crows.Scan(&circo, &code, &nom, &entiere); err != nil {
			crows.Close()
			return nil, err
		}
		p := pages[circo]
		if p == nil {
			continue
		}
		l := Lieu{Type: "COMMUNE", Code: code, Nom: nom, URL: r.urlCommune(code)}
		p.Communes = append(p.Communes, CommuneCirco{Lieu: l, Partielle: !entiere})
		if !entiere {
			p.NbPartielles++
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
		p.Inscrits = int(v["Inscrit_22"])
		if t, ok := v["tvar_pop"]; ok {
			p.Evolution = signe(t) + Decimal(t, 1) + " % par an"
		}
		for _, d := range portraitCirco {
			x, ok := v[d.Var]
			if !ok {
				continue
			}
			l := LignePortrait{Libelle: d.Libelle, Valeur: Decimal(x, d.Dec) + d.Unite}
			if f, ok := france[d.Var]; ok {
				l.France = Decimal(f, d.Dec) + d.Unite
			}
			p.Portrait = append(p.Portrait, l)
		}
	}

	// Les députés élus dans la circonscription depuis juin 2012. Un libellé
	// qui ne se rattache à rien est signalé au build, pas ignoré en silence.
	nonRattaches := map[string]int{}
	for _, pr := range persons {
		for _, m := range pr.Mandats {
			if m.Type != "DEPUTE" || m.DebutISO < debutDecoupage2012 {
				continue
			}
			p := pages[r.codeCirco(m.Circo)]
			if p == nil {
				if !strings.Contains(m.Circo, "hors de France") && !strings.HasPrefix(m.Circo, "ZZ") {
					nonRattaches[m.Circo]++
				}
				continue
			}
			d := DeputeCirco{Nom: strings.TrimSpace(pr.Prenom + " " + pr.Nom), Slug: pr.Slug,
				Periode: m.Periode, EnCours: m.FinISO == "", debut: m.DebutISO}
			p.Deputes = append(p.Deputes, d)
		}
	}
	if len(nonRattaches) > 0 {
		fmt.Printf("  circonscriptions : %d libellés de mandat de député non rattachés : %v\n", len(nonRattaches), nonRattaches)
	}
	for _, p := range pages {
		sort.SliceStable(p.Deputes, func(i, j int) bool { return p.Deputes[i].debut > p.Deputes[j].debut })
		// Deux sources publient le même mandat en cours (Assemblée et
		// répertoire des élus) : une personne n'apparaît qu'une fois en tête.
		vu := map[string]bool{}
		for _, d := range p.Deputes {
			if d.EnCours && !vu[d.Slug] {
				vu[d.Slug] = true
				p.EnCours = append(p.EnCours, d)
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

func (f *fondSituation) chargerCirconscriptions(ctx context.Context, pool *pgxpool.Pool) error {
	f.circos = map[string]*geomSituation{}
	rows, err := pool.Query(ctx, `
		SELECT g.code, c.code_departement, g.srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(g.geom,$1),g.srid_rendu),1,0),
		       st_x(st_transform(st_pointonsurface(g.geom),g.srid_rendu)), -st_y(st_transform(st_pointonsurface(g.geom),g.srid_rendu))
		  FROM geo.contour_circonscription g JOIN ref.circonscription_legislative c USING (code)`, tolSituation)
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

func (f *fondSituation) pourCirconscription(p *PageCirco, popFrance int) *Situation {
	g := f.circos[p.Code]
	if g == nil {
		return nil
	}
	s := &Situation{}
	f.envelopper(s, g.dept, func(c *calque, dom bool) {
		c.groupe("territoire", []string{g.d})
		c.groupe("contour", []string{g.d})
		if !dom {
			if t := f.deps.traces[g.dept]; t != "" {
				c.groupe("departements", []string{t})
			}
			c.repere(g.x, g.y, 16000)
		}
	}, "Situation de la "+p.Ordinal+" circonscription ("+p.Dept.Nom+") dans la France entière")

	if p.Population > 0 {
		det := "population légale 2019"
		if popFrance > 0 {
			det += " · " + partFrance(float64(p.Population), float64(popFrance))
		}
		if p.Evolution != "" {
			det += " · " + p.Evolution + " depuis 2013"
		}
		s.Legende = append(s.Legende, ligneLegende("Population", Nombre(p.Population)+" habitants", det))
	}

	// Superficie : cadastrale (IGN) quand la circonscription est faite de
	// communes entières toutes présentes au COG courant ; sinon l'aire du
	// contour Insee simplifié, annoncée comme approchée.
	km, exacte := 0.0, p.NbPartielles == 0
	for _, c := range p.Communes {
		cg := f.communes[c.Code]
		if cg == nil {
			exacte = false
			break
		}
		km += cg.superficieKm2
	}
	valeur := km2(km)
	det := partFrance(km, f.totalKm2)
	if !exacte {
		km = p.superficieApprox
		valeur = "≈ " + km2(km)
		det = partFrance(km, f.totalKm2) + " · aire du contour Insee simplifié"
	}
	if km > 0 && p.Population > 0 {
		det += " · " + Nombre(int(float64(p.Population)/km+0.5)) + " hab./km²"
	}
	s.Legende = append(s.Legende, ligneLegende("Superficie", valeur, det))

	com := Nombre(len(p.Communes))
	detCom := ""
	if len(p.Communes) == 1 && p.NbPartielles == 1 {
		detCom = p.Communes[0].Nom + ", en partie : la commune est partagée entre plusieurs circonscriptions"
	} else if p.NbPartielles > 0 {
		detCom = "dont " + Nombre(p.NbPartielles) + " en partie seulement, partagée"
		if p.NbPartielles > 1 {
			detCom += "s"
		}
		detCom += " avec une autre circonscription"
	}
	s.Legende = append(s.Legende, ligneLegende("Communes", com, detCom))
	if p.Inscrits > 0 {
		s.Legende = append(s.Legende, ligneLegende("Inscrits sur les listes électorales", Nombre(p.Inscrits), "au 11 avril 2022"))
	}
	if len(p.EnCours) > 0 {
		var noms []string
		for _, d := range p.EnCours {
			noms = append(noms, d.Nom)
		}
		s.Legende = append(s.Legende, ligneLegende("À l'Assemblée nationale", strings.Join(noms, ", "), p.EnCours[0].Periode))
	}
	s.Legende = append(s.Legende, ligneLegende("Budget", "aucun",
		"une circonscription élit un député ; elle n'a ni conseil ni budget"))
	s.Note = "La circonscription en évidence, son département en trait moyen ; le cercle aide à la trouver à l'échelle de la France. Contour et population : Insee, portraits des circonscriptions législatives (fond du 3 mai 2022)."
	return s
}
