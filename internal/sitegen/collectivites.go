package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// L'étage intermédiaire : régions, départements, intercommunalités.
//
// C'est le niveau dont on parle le moins et qui dépense le plus après l'État :
// à eux trois, 137 Md€ de fonctionnement en 2025, contre 84 Md€ pour les
// 34 772 communes. Leurs exécutifs ne sont élus par personne directement — un
// président de conseil communautaire est désigné par des conseillers eux-mêmes
// élus sur des listes municipales — et le site les nomme sans commenter ce fait.
type ElectedLocal struct {
	Name, Slug, Role, Depuis string
	// Fiche : le site ne publie de page que pour les élus à mandat national.
	// Un vice-président de conseil départemental n'en a pas, et le lier
	// donnerait 404 — le nom s'affiche alors sans lien.
	Profile bool
}

// rankVice lit le nombre en tête d'un intitulé « 1er Vice-président… »,
// « 10ème Vice-président… » : le rôle vient du RNE sous forme de texte, et un
// tri alphabétique mettrait le 10ème avant le 2ème.
func rankVice(role string) int {
	i := 0
	for i < len(role) && role[i] >= '0' && role[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0
	}
	n, _ := strconv.Atoi(role[:i])
	return n
}

func sortVices(idx map[string]*Authority) {
	for _, c := range idx {
		sort.Slice(c.Vices, func(i, j int) bool {
			return rankVice(c.Vices[i].Role) < rankVice(c.Vices[j].Role)
		})
	}
}

type Authority struct {
	Level, Code, Name, Slug string
	Population              int
	President               *ElectedLocal
	Vices                   []ElectedLocal
	Councillors             int
	// Par code d'indicateur : le montant total, et le montant par habitant.
	Total, PerInhabitants map[string]float64
	// SansBudgetPropre : ce département a un territoire (un contour, une
	// préfecture) mais pas de budget départemental distinct — sa fiscalité a
	// été fusionnée dans une autre collectivité. FusionCode/FusionLibelle
	// disent laquelle.
	WithoutBudgetClean                  bool
	FusionLabel, FusionCode, FusionSlug string
	FusionLevel                         string // "REGION" ou "DEPARTEMENT"
}

type LevelWeight struct {
	Level, Label string
	Authorities  int
	Totals       map[string]float64
}

type NatureEPCI struct {
	Code, Label   string
	Count         int
	Population    int64
	WithPresident int
	Taxation      bool
}

type ResponsibilityEPCI struct {
	Code, Label string
	Count       int
}

type StatsAuthorities struct {
	FiscalYear           int
	MapRegions, MapDepts Map
	// CarteInteractive : la carte principale de la page — national par
	// défaut, recadrage sur une région au clic, puis sur ses départements
	// (site.js). Les cartes en onglets ci-dessus (CarteRegions/CarteDepts/
	// CarteEPCI) restent le détail exhaustif, avec classement et cartons.
	MapInteractive template.HTML
	MapEPCI        Map
	// ResumeRegions/Depts/EPCI : nombre d'unités, total et médiane de
	// l'indicateur cartographié — de quoi remplir la colonne de droite de
	// chaque onglet d'un vrai contenu (sur le modèle du panneau « situation »
	// d'une page de collectivité), plutôt que le seul titre et l'échelle de
	// couleur qui y suffisaient à peine.
	SummaryRegions, SummaryDepts, SummaryEPCI *SummaryMap
	// ResumeEPCIBudget : ce dont cette page parle vraiment — l'intercommunalité,
	// pas la population. Nombre de groupements, budget cumulé, élus indirects
	// cumulés : voir chargerResumeEPCIBudget.
	SummaryEPCIBudget    *SummaryEPCIBudget
	VintageEPCI          int
	CountEPCIOnMap       int
	Regions, Departments []*Authority
	Weight               []LevelWeight
	Indicators           []IndicatorAuthority
	Natures              []NatureEPCI
	Responsibilities     []ResponsibilityEPCI
	CountEPCI            int
	CountEPCIWithBudget  int
	CountMembersEPCI     int
	CountTaxationClean   int
	EPCIPerDept          map[string][]*Grouping
	OutsideMap           []string
	TaxationLocal        *StatsTaxationLocal
}

// StatsTaxationLocal : qui paie, via quel mécanisme fiscal nommé — pas
// seulement quel niveau de collectivité reçoit (voir Poids ci-dessus). Bloc
// communal seulement (commune + intercommunalité), quatre dispositifs
// seulement (foncier bâti, foncier non bâti, CFE, TASCOM) — voir le
// commentaire de internal/communes/fiscalite_locale.go pour ce qui est
// délibérément exclu.
type StatsTaxationLocal struct {
	Year                        int
	Households, Companies       float64 // Md€
	HouseholdsPct, CompaniesPct float64
	DetailHouseholds            []LineTaxationLocal
	DetailCompanies             []LineTaxationLocal
}

type LineTaxationLocal struct {
	Label  string
	Amount float64 // Md€
}

type Grouping struct {
	Siren, Name, Nature string
	Population          int
	Members             int
	President           string
	Responsibilities    int
	PerInhabitants      map[string]float64
}

type IndicatorAuthority struct{ Code, Label, Short string }

// Les cinq indicateurs de l'OFGL, dans l'ordre où ils se lisent : ce qu'on
// dépense pour faire tourner, ce qu'on investit, ce qu'on doit, ce qu'il reste,
// ce que coûte le personnel.
var indicsAuthority = []IndicatorAuthority{
	{"ofgl.fonctionnement_par_hab", "Dépenses de fonctionnement", "fonctionnement"},
	{"ofgl.investissement_par_hab", "Dépenses d'investissement", "investissement"},
	{"ofgl.dette_par_hab", "Encours de dette", "dette"},
	{"ofgl.epargne_brute_par_hab", "Épargne brute", "épargne"},
	{"ofgl.masse_salariale_par_hab", "Charges de personnel", "personnel"},
}

// ShareRevenue : la part de la DGF et celle des impôts et taxes dans les
// recettes totales, par niveau — ce qui distingue « financé par l'État » de
// « financé par la fiscalité que la collectivité vote elle-même ». Calculé ici
// plutôt qu'écrit en dur dans le modèle de page, pour rester exact quand
// l'exercice change.
type ShareRevenue struct {
	Level, Label               string
	Revenues, DGF, Taxes       float64 // milliards d'euros, pour l'ordre de grandeur
	DGFPct, TaxesPct, OtherPct float64
}

func sharesRevenues(weight []LevelWeight) []ShareRevenue {
	var out []ShareRevenue
	for _, p := range weight {
		rec := p.Totals["ofgl.recettes_totales_par_hab"]
		if rec <= 0 {
			continue
		}
		dgf := p.Totals["ofgl.dgf_par_hab"]
		imp := p.Totals["ofgl.impots_taxes_par_hab"]
		out = append(out, ShareRevenue{
			Level: p.Level, Label: p.Label,
			Revenues: rec / 1e9, DGF: dgf / 1e9, Taxes: imp / 1e9,
			DGFPct: 100 * dgf / rec, TaxesPct: 100 * imp / rec,
			OtherPct: 100 * (rec - dgf - imp) / rec,
		})
	}
	return out
}

var labelNature = map[string]string{
	"CC": "Communauté de communes", "CA": "Communauté d'agglomération",
	"CU": "Communauté urbaine", "METRO": "Métropole",
	"MET69": "Métropole de Lyon", "EPT": "Établissement public territorial",
	"SIVU":  "Syndicat intercommunal à vocation unique",
	"SIVOM": "Syndicat intercommunal à vocations multiples",
	"SMF":   "Syndicat mixte fermé", "SMO": "Syndicat mixte ouvert",
	"PETR":  "Pôle d'équilibre territorial et rural",
	"POLEM": "Pôle métropolitain",
}

// natureAFiscalite : les six formes qui lèvent l'impôt et exercent des
// compétences en propre. Les syndicats, eux, sont des outils techniques
// financés par leurs membres — les mettre dans le même tableau ferait croire à
// 9 282 échelons de décision.
var natureATaxation = map[string]bool{
	"CC": true, "CA": true, "CU": true, "METRO": true, "MET69": true, "EPT": true,
}

var labelSchemeFiscal = map[string]string{
	"FB": "Foncier bâti", "FNB": "Foncier non bâti",
	"CFE": "Cotisation foncière des entreprises (CFE)", "TASCOM": "Taxe sur les surfaces commerciales (TASCOM)",
}
var orderSchemeFiscal = []string{"FB", "FNB", "CFE", "TASCOM"}

// categoriePayeurDispositif : même classification que
// internal/communes/fiscalite_locale.go (categoriePayeur, non exportée) —
// dupliquée ici plutôt qu'importée, internal/sitegen ne dépendant d'aucun paquet
// internal/communes pour l'instant.
var categoryPayerScheme = map[string]string{
	"FB": "MENAGES", "FNB": "MENAGES",
	"CFE": "ENTREPRISES", "TASCOM": "ENTREPRISES",
}

// loadTaxationLocal : le dernier millésime disponible de
// core.fiscalite_directe_locale (internal/communes/fiscalite_locale.go),
// sommé sur les deux destinataires chargés (commune, intercommunalité) et
// catégorisé ménages/entreprises.
func loadTaxationLocal(ctx context.Context, pool *pgxpool.Pool) (*StatsTaxationLocal, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, dispositif, categorie_payeur, sum(montant_eur)
		FROM core.fiscalite_directe_locale
		WHERE annee = (SELECT max(annee) FROM core.fiscalite_directe_locale)
		GROUP BY annee, dispositif, categorie_payeur`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	amounts := map[string]float64{} // code dispositif -> € (avant conversion en Md€)
	f := &StatsTaxationLocal{}
	for rows.Next() {
		var scheme, cat string
		var amount float64
		if err := rows.Scan(&f.Year, &scheme, &cat, &amount); err != nil {
			return nil, err
		}
		amounts[scheme] = amount
		if cat == "MENAGES" {
			f.Households += amount
		} else {
			f.Companies += amount
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if f.Households+f.Companies == 0 {
		return nil, nil // table absente ou vide : la section est simplement omise
	}
	total := f.Households + f.Companies
	f.HouseholdsPct = 100 * f.Households / total
	f.CompaniesPct = 100 * f.Companies / total
	for _, code := range orderSchemeFiscal {
		m, ok := amounts[code]
		if !ok {
			continue
		}
		l := LineTaxationLocal{Label: labelSchemeFiscal[code], Amount: m / 1e9}
		if categoryPayerScheme[code] == "MENAGES" {
			f.DetailHouseholds = append(f.DetailHouseholds, l)
		} else {
			f.DetailCompanies = append(f.DetailCompanies, l)
		}
	}
	f.Households /= 1e9
	f.Companies /= 1e9
	return f, nil
}

// fusionConnue : les départements géographiques dont le budget n'est plus
// tenu séparément, et où il est allé. Fait de droit — la collectivité
// territoriale unique de Corse (2018), celles de Martinique et de Guyane
// (2015), et la Collectivité européenne d'Alsace (2021) — pas une déduction
// depuis les données, qui ne portent aucune trace de la fusion elle-même.
var fusionKnown = map[string]struct{ code, level, label string }{
	"2A":  {"94", "REGION", "Collectivité de Corse"},
	"2B":  {"94", "REGION", "Collectivité de Corse"},
	"67":  {"67A", "DEPARTEMENT", "Collectivité européenne d'Alsace"},
	"68":  {"67A", "DEPARTEMENT", "Collectivité européenne d'Alsace"},
	"972": {"02", "REGION", "Collectivité territoriale de Martinique"},
	"973": {"03", "REGION", "Collectivité territoriale de Guyane"},
}

func loadAuthorities(ctx context.Context, pool *pgxpool.Pool) (*StatsAuthorities, error) {
	st := &StatsAuthorities{EPCIPerDept: map[string][]*Grouping{}}
	// max(...) est une agrégation : la ligne existe même sans budget de
	// collectivité encore ingéré, avec un exercice NULL.
	var fiscalYearN sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT max(exercice) FROM core.collectivite_budget`).
		Scan(&fiscalYearN); err != nil {
		return nil, err
	}
	st.FiscalYear = int(fiscalYearN.Int64)
	st.Indicators = indicsAuthority

	// --- poids relatif des quatre niveaux
	//
	// La vue expose le LIBELLÉ de l'indicateur, pas son code ; on le retraduit
	// pour que tout le reste du fichier ne manipule que des codes.
	prows, err := pool.Query(ctx, `
		SELECT p.niveau, i.code, p.collectivites, p.total::float8
		FROM derived.poids_des_niveaux p
		JOIN ref.indicator i ON i.label = p.indicateur
		WHERE p.exercice=$1`, st.FiscalYear)
	if err != nil {
		return nil, err
	}
	perLevel := map[string]*LevelWeight{}
	libLevel := map[string]string{"COMMUNE": "Communes", "GROUPEMENT": "Intercommunalités",
		"DEPARTEMENT": "Départements", "REGION": "Régions"}
	for prows.Next() {
		var niv, ind string
		var count int
		var total float64
		if err := prows.Scan(&niv, &ind, &count, &total); err != nil {
			break
		}
		n := perLevel[niv]
		if n == nil {
			n = &LevelWeight{Level: niv, Label: libLevel[niv], Authorities: count,
				Totals: map[string]float64{}}
			perLevel[niv] = n
		}
		n.Totals[ind] = total
	}
	prows.Close()
	for _, niv := range []string{"COMMUNE", "GROUPEMENT", "DEPARTEMENT", "REGION"} {
		if n := perLevel[niv]; n != nil {
			st.Weight = append(st.Weight, *n)
		}
	}

	// --- régions et départements, avec leurs cinq indicateurs — mv.
	// collectivite_budget_pivot (internal/matview) remplace le pivot
	// jsonb_object_agg que cette fermeture refaisait une fois par niveau à
	// chaque construction.
	load := func(level string) ([]*Authority, map[string]*Authority, error) {
		rows, err := pool.Query(ctx, `
			SELECT code, nom, population, totaux, par_hab
			FROM mv.collectivite_budget_pivot
			WHERE niveau=$1 AND exercice=$2
			ORDER BY nom`, level, st.FiscalYear)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		var out []*Authority
		idx := map[string]*Authority{}
		for rows.Next() {
			c := &Authority{Level: level, Total: map[string]float64{},
				PerInhabitants: map[string]float64{}}
			var total, inhabitants map[string]*float64
			if err := rows.Scan(&c.Code, &c.Name, &c.Population, &total, &inhabitants); err != nil {
				return nil, nil, err
			}
			for k, v := range total {
				if v != nil {
					c.Total[k] = *v
				}
			}
			for k, v := range inhabitants {
				if v != nil {
					c.PerInhabitants[k] = *v
				}
			}
			c.Slug = partis.Slugify(c.Name)
			out = append(out, c)
			idx[c.Code] = c
		}
		return out, idx, rows.Err()
	}
	var idxRegion, idxDep map[string]*Authority
	st.Regions, idxRegion, err = load("REGION")
	if err != nil {
		return nil, err
	}
	st.Departments, idxDep, err = load("DEPARTEMENT")
	if err != nil {
		return nil, err
	}
	// Les départements se lisent par numéro — c'est ainsi qu'on les cherche.
	sort.Slice(st.Departments, func(i, j int) bool {
		return st.Departments[i].Code < st.Departments[j].Code
	})

	// Les départements SANS budget propre : un territoire réel, un contour, une
	// préfecture — mais leur fiscalité a été fusionnée dans une autre
	// collectivité, et core.collectivite_budget ne porte donc aucune ligne à
	// leur code. Les omettre du tableau les ferait passer pour oubliés plutôt
	// que fusionnés ; ils y figurent avec la raison et un lien vers où
	// regarder à la place.
	grows2, err := pool.Query(ctx, `
		SELECT code_insee, nom FROM geo.contour WHERE niveau='DEPARTEMENT'
		  AND code_insee <> ALL($1)`,
		func() []string {
			codes := make([]string, 0, len(idxDep))
			for c := range idxDep {
				codes = append(codes, c)
			}
			return codes
		}())
	if err != nil {
		return nil, err
	}
	for grows2.Next() {
		var code, name string
		if err := grows2.Scan(&code, &name); err != nil {
			grows2.Close()
			return nil, err
		}
		c := &Authority{Level: "DEPARTEMENT", Code: code, Name: name,
			Slug: partis.Slugify(name), WithoutBudgetClean: true}
		if f, ok := fusionKnown[code]; ok {
			c.FusionCode, c.FusionLevel, c.FusionLabel = f.code, f.level, f.label
			switch f.level {
			case "REGION":
				if target := idxRegion[f.code]; target != nil {
					c.FusionLabel, c.FusionSlug = target.Name, target.Slug
				}
			case "DEPARTEMENT":
				if target := idxDep[f.code]; target != nil {
					c.FusionLabel = target.Name
				}
			}
		} else {
			c.FusionLabel = "aucune ligne budgétaire publiée à ce code"
		}
		st.Departments = append(st.Departments, c)
	}
	grows2.Close()
	if err := grows2.Err(); err != nil {
		return nil, err
	}
	sort.Slice(st.Departments, func(i, j int) bool {
		return st.Departments[i].Code < st.Departments[j].Code
	})

	// --- exécutifs. Le code de la collectivité se lit dans la circonscription
	// du mandat : « 44 Grand Est » pour une région, « 0114 Nantua » — le canton
	// — pour un département, dont les deux premiers caractères sont le numéro.
	// mv.mandat_executif_local (internal/matview) — 13 044 lignes en poste sur
	// 617 196 mandats, plus le GROUP BY sur la totalité de core.mandate que
	// cette fonction refaisait à chaque construction.
	elected := func(role string, length int, idx map[string]*Authority, vice bool) error {
		rows, err := pool.Query(ctx, `
			SELECT left(constituency,$2), person_given_name||' '||person_family_name, person_slug,
			       role, to_char(depuis,'DD/MM/YYYY')
			FROM mv.mandat_executif_local
			WHERE role LIKE $1`, role, length)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			var e ElectedLocal
			if err := rows.Scan(&code, &e.Name, &e.Slug, &e.Role, &e.Depuis); err != nil {
				return err
			}
			c := idx[code]
			if c == nil {
				continue
			}
			if vice {
				c.Vices = append(c.Vices, e)
			} else if c.President == nil {
				c.President = &e
			}
		}
		return rows.Err()
	}
	if err := elected("Président du conseil régional", 2, idxRegion, false); err != nil {
		return nil, err
	}
	if err := elected("%Vice-président du conseil régional", 2, idxRegion, true); err != nil {
		return nil, err
	}
	if err := elected("Président du conseil départemental", 2, idxDep, false); err != nil {
		return nil, err
	}
	if err := elected("%Vice-président du conseil départemental", 2, idxDep, true); err != nil {
		return nil, err
	}
	sortVices(idxRegion)
	sortVices(idxDep)

	// mv.mandat_local_compte (internal/matview) porte déjà ce décompte par
	// (type de mandat, territoire).
	account := func(term string, length int, idx map[string]*Authority) error {
		rows, err := pool.Query(ctx, `
			SELECT left(code_territoire,$2), nombre_elus FROM mv.mandat_local_compte
			WHERE mandate_type=$1`, term, length)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			var n int
			if err := rows.Scan(&code, &n); err != nil {
				return err
			}
			if c := idx[code]; c != nil {
				c.Councillors = n
			}
		}
		return rows.Err()
	}
	if err := account("CONSEILLER_REGIONAL", 2, idxRegion); err != nil {
		return nil, err
	}
	if err := account("CONSEILLER_DEPARTEMENTAL", 2, idxDep); err != nil {
		return nil, err
	}
	// --- intercommunalités
	nrows, err := pool.Query(ctx, `
		SELECT nature_juridique, count(*), sum(population_totale)::bigint, count(president_nom)
		FROM core.epci GROUP BY 1 ORDER BY 2 DESC`)
	if err != nil {
		return nil, err
	}
	for nrows.Next() {
		var n NatureEPCI
		if err := nrows.Scan(&n.Code, &n.Count, &n.Population, &n.WithPresident); err != nil {
			break
		}
		n.Label = labelNature[n.Code]
		if n.Label == "" {
			n.Label = n.Code
		}
		n.Taxation = natureATaxation[n.Code]
		st.CountEPCI += n.Count
		st.Natures = append(st.Natures, n)
	}
	nrows.Close()

	_ = pool.QueryRow(ctx, `SELECT count(*) FROM core.epci_membre`).Scan(&st.CountMembersEPCI)
	for _, n := range st.Natures {
		if n.Taxation {
			st.CountTaxationClean += n.Count
		}
	}

	crows, err := pool.Query(ctx, `
		SELECT c.code, c.libelle, count(*)
		FROM core.epci_competence ec JOIN ref.competence c ON c.code=ec.competence_code
		GROUP BY 1,2 ORDER BY 3 DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var c ResponsibilityEPCI
		if err := crows.Scan(&c.Code, &c.Label, &c.Count); err != nil {
			break
		}
		st.Responsibilities = append(st.Responsibilities, c)
	}
	crows.Close()

	// Les groupements à fiscalité propre, rangés par département : c'est
	// l'échelon que le lecteur habite, et il n'a pas de page à lui ailleurs.
	// mv.epci / mv.epci_budget_exercice (internal/matview) remplacent le
	// pivot jsonb_object_agg (et sa sous-requête de comptage de
	// compétences) que cette requête refaisait sur les ~9 000 EPCI à
	// chaque construction. LEFT JOIN, pas INNER : un EPCI sans budget pour
	// CET exercice doit rester dans la liste, par_hab vide — comme avant.
	grows, err := pool.Query(ctx, `
		SELECT e.siren, e.nom, e.nature_juridique, e.code_departement,
		       e.population, e.nb_membres, e.president, e.nb_competences,
		       coalesce(b.par_hab, '{}'::jsonb)
		FROM mv.epci e
		LEFT JOIN mv.epci_budget_exercice b ON b.siren = e.siren AND b.exercice = $1
		ORDER BY e.code_departement, e.nom`, st.FiscalYear)
	if err != nil {
		return nil, err
	}
	for grows.Next() {
		g := &Grouping{PerInhabitants: map[string]float64{}}
		var dep string
		var inhabitants map[string]*float64
		if err := grows.Scan(&g.Siren, &g.Name, &g.Nature, &dep, &g.Population,
			&g.Members, &g.President, &g.Responsibilities, &inhabitants); err != nil {
			break
		}
		for k, v := range inhabitants {
			if v != nil {
				g.PerInhabitants[k] = *v
			}
		}
		if len(g.PerInhabitants) > 0 {
			st.CountEPCIWithBudget++
		}
		st.EPCIPerDept[dep] = append(st.EPCIPerDept[dep], g)
	}
	grows.Close()

	fisc, err := loadTaxationLocal(ctx, pool)
	if err != nil {
		return nil, err
	}
	st.TaxationLocal = fisc

	return st, nil
}

// linkProfiles marque les élus qui ont une page sur ce site.
func linkProfiles(st *StatsAuthorities, withProfile map[string]bool) {
	mark := func(e *ElectedLocal) {
		if e != nil {
			e.Profile = withProfile[e.Slug]
		}
	}
	for _, c := range append(append([]*Authority{}, st.Regions...), st.Departments...) {
		mark(c.President)
		for i := range c.Vices {
			mark(&c.Vices[i])
		}
	}
}

// mapInteractive : la carte principale de la page. Deux calques dans le
// même repère Lambert-93 (régions, départements) — les coordonnées des
// tracés sont des mètres absolus, valables sous n'importe quel viewBox, donc
// superposables sans recalcul. Régions visibles par défaut ; site.js recadre
// sur la région cliquée et bascule vers le calque départements, sur le
// modèle déjà en place pour #carte-epci (« ?departement=»/« ?region= »),
// généralisé ici au clic direct plutôt qu'à un paramètre d'URL. Cliquer un
// département renvoie vers sa page complète — la réutilisation la plus
// fidèle de /collectivites/departement/<code>/ est d'y renvoyer au bon
// moment, pas de dupliquer son contenu ici.
func mapInteractive(region, dep *SetOutlines, cellsR, cellsD []CellMap, unit string, format func(float64) string) template.HTML {
	cR := prepare(cellsR, unit, format)
	cD := prepare(cellsD, unit, format)
	if cR.Empty || cD.Empty {
		return ""
	}
	byR, byD := index(cellsR), index(cellsD)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo carte-interactive" role="img" `+
		`aria-label="Carte interactive : la France, une région, puis un département">`, dep.ViewBox)
	b.WriteString(`<g class="calque-regions">`)
	for _, code := range region.Codes {
		cc := byR[code]
		title := region.Noms[code]
		if !cc.Absent && cc.Code != "" {
			title += " — " + format(cc.Value)
		}
		fmt.Fprintf(&b, `<path class="cliquable" data-niveau="region" data-code="%s" data-nom="%s" `+
			`d="%s" fill="%s"><title>%s</title></path>`,
			code, template.HTMLEscapeString(region.Noms[code]), region.traces[code], cR.fill(cc),
			template.HTMLEscapeString(title))
	}
	b.WriteString(`</g><g class="calque-departements" hidden>`)
	for _, code := range dep.Codes {
		cc := byD[code]
		title := dep.Noms[code]
		if !cc.Absent && cc.Code != "" {
			title += " — " + format(cc.Value)
		}
		fmt.Fprintf(&b, `<path class="cliquable" data-niveau="departement" data-code="%s" data-nom="%s" `+
			`d="%s" fill="%s"><title>%s</title></path>`,
			code, template.HTMLEscapeString(dep.Noms[code]), dep.traces[code], cD.fill(cc),
			template.HTMLEscapeString(title))
	}
	b.WriteString(`</g></svg>`)
	return template.HTML(b.String())
}

// indicPopulation : un repère démographique, pas un chiffre budgétaire —
// la carte d'ouverture montre où vivent les gens avant les cartes de
// dépenses, dette et recettes plus bas, plutôt que de présenter un seul
// indicateur budgétaire (au hasard, le fonctionnement) comme LE chiffre par
// défaut.
const indicatorPopulation = "population"

type SummaryMap struct {
	Count              int
	Total, Median      float64
	MaxName, MinName   string
	MaxValue, MinValue float64
}

// summarizeMap : nombre d'unités, total, médiane, maximum et minimum de
// l'indicateur cartographié — le contenu de la colonne de droite de chaque
// onglet « Trois niveaux, trois cartes », sur le modèle du panneau
// « situation » d'une page de collectivité (un vrai résumé chiffré, pas
// seulement un titre et l'échelle de couleur).
func summarizeMap(cells []CellMap) *SummaryMap {
	if len(cells) == 0 {
		return nil
	}
	r := &SummaryMap{Count: len(cells)}
	max, min := cells[0], cells[0]
	vals := make([]float64, len(cells))
	for i, c := range cells {
		vals[i] = c.Value
		r.Total += c.Value
		if c.Value > max.Value {
			max = c
		}
		if c.Value < min.Value {
			min = c
		}
	}
	sort.Float64s(vals)
	r.Median = vals[len(vals)/2]
	r.MaxName, r.MaxValue = max.Name, max.Value
	r.MinName, r.MinValue = min.Name, min.Value
	return r
}

// SummaryEPCIBudget : ce dont la page parle réellement — l'échelon
// intercommunal, ses moyens et ses élus indirects — plutôt qu'un repère
// démographique générique. Card dédiée à l'onglet Intercommunalités, la
// carte que la page ouvre désormais en premier.
type SummaryEPCIBudget struct {
	Count                 int
	Operating, Investment float64 // Md€, cumulés, tous les groupements
	Elected               int     // conseillers communautaires en mandat, cumulés
	Population            float64
	// PartBlocCommunal : le fonctionnement des intercommunalités rapporté à
	// celui des intercommunalités PLUS des communes — la part du bloc
	// communal qui passe déjà par l'échelon indirect, pas par la commune.
	ShareBlockCommunal float64
}

// loadSummaryEPCIBudget : budget cumulé (derived.poids_des_niveaux, déjà
// chargé dans st.Poids) et nombre d'élus communautaires en mandat
// (mv.mandat_local_compte, internal/matview) — deux chiffres qu'aucune des
// cartes de dépenses plus bas ne met en avant à ce niveau de la page.
func loadSummaryEPCIBudget(ctx context.Context, pool *pgxpool.Pool, st *StatsAuthorities, count int, population float64) (*SummaryEPCIBudget, error) {
	r := &SummaryEPCIBudget{Count: count, Population: population}
	var functionMunicipalities float64
	for _, p := range st.Weight {
		switch p.Level {
		case "GROUPEMENT":
			r.Operating = p.Totals["ofgl.fonctionnement_par_hab"] / 1e9
			r.Investment = p.Totals["ofgl.investissement_par_hab"] / 1e9
		case "COMMUNE":
			functionMunicipalities = p.Totals["ofgl.fonctionnement_par_hab"] / 1e9
		}
	}
	if total := r.Operating + functionMunicipalities; total > 0 {
		r.ShareBlockCommunal = 100 * r.Operating / total
	}
	if err := pool.QueryRow(ctx, `
		SELECT nombre_elus FROM mv.mandat_local_compte
		WHERE mandate_type='CONSEILLER_COMMUNAUTAIRE'`).Scan(&r.Elected); err != nil {
		return nil, err
	}
	return r, nil
}

// cartesCollectivites dessine les deux cartes de l'index : une par niveau, sur
// la même boîte, donc superposables.

func mapsAuthorities(ctx context.Context, pool *pgxpool.Pool, st *StatsAuthorities,
	indicator string) error {

	eur := func(v float64) string { return Count(int(v+0.5)) + " €" }
	inhabitants := func(v float64) string { return Count(int(v+0.5)) + " habitants" }
	unit, format := "€ par habitant", eur
	if indicator == indicatorPopulation {
		unit, format = "habitants", inhabitants
	}

	region, err := setOutlines(ctx, pool, "REGION", toleranceOverview)
	if err != nil {
		return err
	}
	var cellsR []CellMap
	for _, c := range st.Regions {
		if indicator == indicatorPopulation {
			if c.Population > 0 {
				cellsR = append(cellsR, CellMap{Code: c.Code, Name: c.Name, Value: float64(c.Population)})
			}
		} else if v, ok := c.PerInhabitants[indicator]; ok {
			cellsR = append(cellsR, CellMap{Code: c.Code, Name: c.Name, Value: v})
		}
	}
	st.MapRegions = full(region, cellsR, unit, format)
	st.SummaryRegions = summarizeMap(cellsR)

	dep, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceOverview)
	if err != nil {
		return err
	}
	var cellsD []CellMap
	// Un code est « sur la carte » s'il a un contour, en métropole OU en
	// carton : sans la seconde moitié, la Guadeloupe, La Réunion et Mayotte
	// passaient pour absentes alors que leurs budgets sont en base.
	inMap := map[string]bool{}
	for _, code := range dep.Codes {
		inMap[code] = true
	}
	for _, o := range dep.overseas {
		inMap[o.Code] = true
	}
	for _, c := range st.Departments {
		if !inMap[c.Code] {
			st.OutsideMap = append(st.OutsideMap, c.Name+" ("+c.Code+")")
			continue
		}
		if indicator == indicatorPopulation {
			// Les départements sans budget propre (Corse-du-Sud, Haute-Corse...,
			// fusionnés dans une collectivité territoriale unique) n'ont pas de
			// population chargée ici (Population reste à zéro) : les exclure de
			// la carte plutôt que de fausser le minimum avec un zéro qui ne veut
			// rien dire.
			if c.Population > 0 {
				cellsD = append(cellsD, CellMap{Code: c.Code, Name: c.Name, Value: float64(c.Population)})
			}
		} else if v, ok := c.PerInhabitants[indicator]; ok {
			cellsD = append(cellsD, CellMap{Code: c.Code, Name: c.Name, Value: v})
		}
	}
	sort.Strings(st.OutsideMap)
	st.MapDepts = full(dep, cellsD, unit, format)
	st.SummaryDepts = summarizeMap(cellsD)
	st.MapInteractive = mapInteractive(region, dep, cellsR, cellsD, unit, format)

	// La carte des intercommunalités : rendue possible par geo.contour_cog
	// (IGN Admin Express COG CARTO), qui donne enfin un tracé à chaque EPCI.
	_ = pool.QueryRow(ctx, `
		SELECT max(cog_millesime) FROM geo.contour_cog WHERE niveau='EPCI'`).Scan(&st.VintageEPCI)
	if st.VintageEPCI > 0 {
		epci, err := setOutlinesEPCI(ctx, pool, st.VintageEPCI, toleranceOverview)
		if err != nil {
			return err
		}
		var vrows pgx.Rows
		if indicator == indicatorPopulation {
			vrows, err = pool.Query(ctx, `
				SELECT siren, population_totale::float8 FROM core.epci
				WHERE nature_juridique = ANY($1) AND population_totale > 0`,
				[]string{"CC", "CA", "CU", "METRO", "MET69", "EPT"})
		} else {
			vrows, err = pool.Query(ctx, `
				SELECT code, euros_par_hab::float8 FROM core.collectivite_budget
				WHERE niveau='GROUPEMENT' AND indicator_code=$1 AND exercice=$2
				  AND euros_par_hab IS NOT NULL`, indicator, st.FiscalYear)
		}
		if err != nil {
			return err
		}
		var cellsE []CellMap
		for vrows.Next() {
			var code string
			var v float64
			if err := vrows.Scan(&code, &v); err != nil {
				vrows.Close()
				return err
			}
			cellsE = append(cellsE, CellMap{Code: code, Name: epci.Noms[code], Value: v})
		}
		vrows.Close()
		if err := vrows.Err(); err != nil {
			return err
		}
		st.CountEPCIOnMap = len(epci.Codes) + len(epci.overseas)
		st.MapEPCI = full(epci, cellsE, unit, format)
		st.SummaryEPCI = summarizeMap(cellsE)
		if st.SummaryEPCI != nil {
			reb, err := loadSummaryEPCIBudget(ctx, pool, st, st.SummaryEPCI.Count, st.SummaryEPCI.Total)
			if err != nil {
				return err
			}
			st.SummaryEPCIBudget = reb
		}
		// Les frontières des départements et des régions, tracées par-dessus
		// les intercommunalités : la même boîte Lambert-93, donc le même
		// repère. Un trait fin pour le département, épais pour la région —
		// on lit d'un coup où un groupement franchit (ou non) une limite.
		var fr strings.Builder
		fr.WriteString(`<g class="frontieres" aria-hidden="true">`)
		// data-code et data-nom : la page de département ou de région ouvre
		// cette carte cadrée sur son territoire (site.js, « ?departement= »).
		for _, c := range dep.Codes {
			fmt.Fprintf(&fr, `<path class="dep" data-code="%s" data-nom="%s" d="%s"/>`,
				c, template.HTMLEscapeString(dep.Noms[c]), dep.traces[c])
		}
		for _, c := range region.Codes {
			fmt.Fprintf(&fr, `<path class="reg" data-code="%s" data-nom="%s" d="%s"/>`,
				c, template.HTMLEscapeString(region.Noms[c]), region.traces[c])
		}
		fr.WriteString(`</g>`)
		st.MapEPCI.SVG = template.HTML(strings.Replace(string(st.MapEPCI.SVG), "</svg>", fr.String()+"</svg>", 1))
	}
	return nil
}

// CellExpense : un montant (Md€) et sa part du maximum de SA COLONNE
// (pas de sa ligne) — un repère de fond ténu dans le tableau, sans dupliquer
// une barre par cellule.
type CellExpense struct {
	Value, Pct float64
}

type LineExpense struct {
	Level       string
	Authorities int
	Cells       []CellExpense // même ordre que TableDepenses.Indicateurs
}

type TableExpenses struct {
	Indicators []IndicatorAuthority
	Lines      []LineExpense
}

// tableExpenses construit un tableau niveau × indicateur plutôt qu'une suite
// de mini-graphiques à barres presque identiques (un par indicateur) : à 4
// niveaux et 5 indicateurs, un tableau se lit à la fois en ligne (le profil
// d'un niveau) et en colonne (qui dépense le plus pour un poste donné), ce
// qu'un mur de petits graphiques empêche de voir d'un coup d'œil.
func tableExpenses(weight []LevelWeight, indics []IndicatorAuthority) TableExpenses {
	t := TableExpenses{Indicators: indics}
	max := make([]float64, len(indics))
	for _, p := range weight {
		for i, ind := range indics {
			if v := p.Totals[ind.Code]; v > max[i] {
				max[i] = v
			}
		}
	}
	for _, p := range weight {
		l := LineExpense{Level: p.Label, Authorities: p.Authorities}
		for i, ind := range indics {
			v := p.Totals[ind.Code]
			var pct float64
			if max[i] > 0 {
				pct = 100 * v / max[i]
			}
			l.Cells = append(l.Cells, CellExpense{Value: v / 1e9, Pct: pct})
		}
		t.Lines = append(t.Lines, l)
	}
	return t
}

// ── Page d'une collectivité ───────────────────────────────────────────

type LineFinance struct {
	Label                 string
	Total, PerInhabitants float64
	Median                float64
	Rank, On              int
}

type PageAuthority struct {
	Name, Code, Slug               string
	TypeLabel, TypePlural, TypeURL string
	WordCouncillor                 string
	FiscalYear, Population         int
	Councillors, CountEPCI         int
	President                      *ElectedLocal
	Vices                          []ElectedLocal
	Lines                          []LineFinance
	Groupings                      []*Grouping
	Neighboring                    []LinkMap
	// Pour une région, ses départements ; pour un département, sa région.
	Departments    []Place
	Region         *Place
	Municipalities []Place
	// Carte maillée des communes du département, avec le contour de ses
	// intercommunalités — absente pour une région, où le luxe de détail
	// noierait la lecture.
	MapMunicipalities      template.HTML
	CountMunicipalitiesMap int
	// Carte de situation dans la France entière (carte_situation.go).
	Situation *Situation
	// Pour un département, ses circonscriptions législatives (circonscriptions.go).
	Districts []Place
	// D'où viennent les recettes de la collectivité, face à son niveau.
	Revenues *OriginRevenues
}

// OriginRevenues : les recettes totales d'UNE collectivité, lues dans ses
// propres comptes (agrégats OFGL), découpées en impôts et taxes, DGF et reste.
// La référence est la même découpe pour l'ensemble du niveau.
type OriginRevenues struct {
	Total, Taxes, DGF, Others           float64 // euros
	TaxesPct, DGFPct, OthersPct         float64
	RefTaxesPct, RefDGFPct, RefOtherPct float64
	RefLabel                            string
}

func originRevenues(c *Authority, ref ShareRevenue) *OriginRevenues {
	rec := c.Total["ofgl.recettes_totales_par_hab"]
	if rec <= 0 {
		return nil
	}
	imp, dgf := c.Total["ofgl.impots_taxes_par_hab"], c.Total["ofgl.dgf_par_hab"]
	o := &OriginRevenues{Total: rec, Taxes: imp, DGF: dgf, Others: rec - imp - dgf,
		RefTaxesPct: ref.TaxesPct, RefDGFPct: ref.DGFPct, RefOtherPct: ref.OtherPct,
		RefLabel: strings.ToLower(ref.Label)}
	o.TaxesPct, o.DGFPct, o.OthersPct = 100*imp/rec, 100*dgf/rec, 100*o.Others/rec
	return o
}

// pagesAuthorities fabrique une page par région et par département. Le rang
// et la médiane sont calculés SUR LE NIVEAU, jamais entre niveaux : comparer le
// budget par habitant d'une région à celui d'un département n'a pas de sens,
// ils ne gèrent pas les mêmes compétences.
func pagesAuthorities(ctx context.Context, pool *pgxpool.Pool, st *StatsAuthorities, r *Resolver) ([]PageAuthority, error) {
	// La référence de chaque niveau pour « d'où vient l'argent » : la part de
	// la DGF et des impôts et taxes dans les recettes de tous ses membres.
	refRevenues := map[string]ShareRevenue{}
	for _, pr := range sharesRevenues(st.Weight) {
		refRevenues[pr.Level] = pr
	}

	// région → départements, et département → région, lus dans le code
	// officiel géographique à travers les communes.
	depsOfRegion := map[string]map[string]bool{}
	regionOfDep := map[string]string{}
	comOfDep := map[string][]Place{}
	for _, c := range r.municipalities {
		comOfDep[c.Dept] = append(comOfDep[c.Dept], r.placeMunicipality(c.Code))
		if depsOfRegion[c.Region] == nil {
			depsOfRegion[c.Region] = map[string]bool{}
		}
		depsOfRegion[c.Region][c.Dept] = true
		regionOfDep[c.Dept] = c.Region
	}
	var out []PageAuthority
	for _, set := range []struct {
		list                           []*Authority
		typeLabel, typePlural, typeURL string
		wordCouncillor                 string
		perCode                        bool
	}{
		{st.Regions, "Conseil régional", "régions", "region", "régionaux", false},
		{st.Departments, "Conseil départemental", "départements", "departement", "départementaux", true},
	} {
		// Médianes du niveau, indicateur par indicateur.
		med := map[string]float64{}
		class := map[string][]float64{}
		for _, c := range set.list {
			for k, v := range c.PerInhabitants {
				class[k] = append(class[k], v)
			}
		}
		for k, vs := range class {
			sort.Float64s(vs)
			med[k] = vs[len(vs)/2]
		}
		for _, c := range set.list {
			// Pas de page de détail pour un département sans budget propre :
			// il n'y a rien à y montrer que le tableau d'index ne dise déjà,
			// et une page presque vide serait pire qu'une absence de page.
			if c.WithoutBudgetClean {
				continue
			}
			p := PageAuthority{
				Name: c.Name, Code: c.Code, Slug: c.Slug,
				TypeLabel: set.typeLabel, TypePlural: set.typePlural, TypeURL: set.typeURL,
				WordCouncillor: set.wordCouncillor,
				FiscalYear:     st.FiscalYear, Population: c.Population,
				Councillors: c.Councillors, President: c.President, Vices: c.Vices,
			}
			if set.perCode {
				p.Slug = c.Code
				p.Groupings = st.EPCIPerDept[c.Code]
				p.CountEPCI = len(p.Groupings)
				if l, ok := r.placeRegion(regionOfDep[c.Code]); ok {
					p.Region = &l
				}
				p.Municipalities = comOfDep[c.Code]
				// Alsace (67A) et la Métropole de Lyon (691) portent un code
				// budgétaire qui n'est pas un code de département du COG.
				if len(p.Municipalities) == 0 {
					switch c.Code {
					case "67A":
						p.Municipalities = append(append([]Place{}, comOfDep["67"]...), comOfDep["68"]...)
					case "691", "69":
						p.Municipalities = comOfDep["69"]
					}
				}
				sort.Slice(p.Municipalities, func(i, j int) bool {
					return KeySort(p.Municipalities[i].Name) < KeySort(p.Municipalities[j].Name)
				})
			} else {
				for d := range depsOfRegion[c.Code] {
					if l, ok := r.placeDept(d); ok {
						p.Departments = append(p.Departments, l)
					}
				}
				sort.Slice(p.Departments, func(i, j int) bool {
					return p.Departments[i].Code < p.Departments[j].Code
				})
			}
			p.Revenues = originRevenues(c, refRevenues[c.Level])
			for _, ind := range indicsAuthority {
				v, ok := c.PerInhabitants[ind.Code]
				if !ok {
					continue
				}
				l := LineFinance{Label: ind.Label, Total: c.Total[ind.Code],
					PerInhabitants: v, Median: med[ind.Code], On: len(class[ind.Code])}
				for _, other := range class[ind.Code] {
					if other > v {
						l.Rank++
					}
				}
				l.Rank++
				p.Lines = append(p.Lines, l)
			}
			for _, other := range set.list {
				// Un département fusionné (SansBudgetPropre) n'a pas sa
				// propre page : l'omettre ici plutôt que produire un lien
				// mort — la collectivité qui tient désormais son budget
				// (ex. "67A" pour le Bas-Rhin) apparaît de toute façon comme
				// sa propre entrée dans jeu.liste, à sa place.
				if other.Code == c.Code || other.WithoutBudgetClean {
					continue
				}
				s := other.Slug
				if set.perCode {
					s = other.Code
				}
				p.Neighboring = append(p.Neighboring, LinkMap{Slug: s, Title: other.Name})
			}
			out = append(out, p)
		}
	}
	return out, nil
}
