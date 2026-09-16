package main

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/jackc/pgx/v5/pgxpool"
)

// L'étage intermédiaire : régions, départements, intercommunalités.
//
// C'est le niveau dont on parle le moins et qui dépense le plus après l'État :
// à eux trois, 137 Md€ de fonctionnement en 2025, contre 84 Md€ pour les
// 34 772 communes. Leurs exécutifs ne sont élus par personne directement — un
// président de conseil communautaire est désigné par des conseillers eux-mêmes
// élus sur des listes municipales — et le site les nomme sans commenter ce fait.
type EluLocal struct {
	Nom, Slug, Role, Depuis string
	// Fiche : le site ne publie de page que pour les élus à mandat national.
	// Un vice-président de conseil départemental n'en a pas, et le lier
	// donnerait 404 — le nom s'affiche alors sans lien.
	Fiche bool
}

// rangVice lit le nombre en tête d'un intitulé « 1er Vice-président… »,
// « 10ème Vice-président… » : le rôle vient du RNE sous forme de texte, et un
// tri alphabétique mettrait le 10ème avant le 2ème.
func rangVice(role string) int {
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

func trierVices(idx map[string]*Collectivite) {
	for _, c := range idx {
		sort.Slice(c.Vices, func(i, j int) bool {
			return rangVice(c.Vices[i].Role) < rangVice(c.Vices[j].Role)
		})
	}
}

type Collectivite struct {
	Niveau, Code, Nom, Slug string
	Population              int
	President               *EluLocal
	Vices                   []EluLocal
	Conseillers             int
	// Par code d'indicateur : le montant total, et le montant par habitant.
	Total, ParHab map[string]float64
	// SansBudgetPropre : ce département a un territoire (un contour, une
	// préfecture) mais pas de budget départemental distinct — sa fiscalité a
	// été fusionnée dans une autre collectivité. FusionCode/FusionLibelle
	// disent laquelle.
	SansBudgetPropre                      bool
	FusionLibelle, FusionCode, FusionSlug string
	FusionNiveau                          string // "REGION" ou "DEPARTEMENT"
}

type NiveauPoids struct {
	Niveau, Libelle string
	Collectivites   int
	Totaux          map[string]float64
}

type NatureEPCI struct {
	Code, Libelle string
	Nombre        int
	Population    int64
	AvecPresident int
	Fiscalite     bool
}

type CompetenceEPCI struct {
	Code, Libelle string
	Nombre        int
}

type StatsCollectivites struct {
	Exercice                 int
	CarteRegions, CarteDepts Carte
	CarteEPCI                Carte
	MillesimeEPCI            int
	NbEPCISurCarte           int
	Regions, Departements    []*Collectivite
	Poids                    []NiveauPoids
	Indicateurs              []IndicCollectivite
	Natures                  []NatureEPCI
	Competences              []CompetenceEPCI
	NbEPCI                   int
	NbEPCIAvecBudget         int
	NbMembresEPCI            int
	NbFiscalitePropre        int
	EPCIParDept              map[string][]*Groupement
	HorsCarte                []string
}

type Groupement struct {
	Siren, Nom, Nature string
	Population         int
	Membres            int
	President          string
	Competences        int
	ParHab             map[string]float64
}

type IndicCollectivite struct{ Code, Libelle, Court string }

// Les cinq indicateurs de l'OFGL, dans l'ordre où ils se lisent : ce qu'on
// dépense pour faire tourner, ce qu'on investit, ce qu'on doit, ce qu'il reste,
// ce que coûte le personnel.
var indicsCollectivite = []IndicCollectivite{
	{"ofgl.fonctionnement_par_hab", "Dépenses de fonctionnement", "fonctionnement"},
	{"ofgl.investissement_par_hab", "Dépenses d'investissement", "investissement"},
	{"ofgl.dette_par_hab", "Encours de dette", "dette"},
	{"ofgl.epargne_brute_par_hab", "Épargne brute", "épargne"},
	{"ofgl.masse_salariale_par_hab", "Charges de personnel", "personnel"},
}

// PartRecette : la part de la DGF et celle des impôts et taxes dans les
// recettes totales, par niveau — ce qui distingue « financé par l'État » de
// « financé par la fiscalité que la collectivité vote elle-même ». Calculé ici
// plutôt qu'écrit en dur dans le modèle de page, pour rester exact quand
// l'exercice change.
type PartRecette struct {
	Niveau, Libelle             string
	Recettes, DGF, Impots       float64 // milliards d'euros, pour l'ordre de grandeur
	DGFPct, ImpotsPct, AutrePct float64
}

func partsRecettes(poids []NiveauPoids) []PartRecette {
	var out []PartRecette
	for _, p := range poids {
		rec := p.Totaux["ofgl.recettes_totales_par_hab"]
		if rec <= 0 {
			continue
		}
		dgf := p.Totaux["ofgl.dgf_par_hab"]
		imp := p.Totaux["ofgl.impots_taxes_par_hab"]
		out = append(out, PartRecette{
			Niveau: p.Niveau, Libelle: p.Libelle,
			Recettes: rec / 1e9, DGF: dgf / 1e9, Impots: imp / 1e9,
			DGFPct: 100 * dgf / rec, ImpotsPct: 100 * imp / rec,
			AutrePct: 100 * (rec - dgf - imp) / rec,
		})
	}
	return out
}

var libelleNature = map[string]string{
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
var natureAFiscalite = map[string]bool{
	"CC": true, "CA": true, "CU": true, "METRO": true, "MET69": true, "EPT": true,
}

// fusionConnue : les départements géographiques dont le budget n'est plus
// tenu séparément, et où il est allé. Fait de droit — la collectivité
// territoriale unique de Corse (2018), celles de Martinique et de Guyane
// (2015), et la Collectivité européenne d'Alsace (2021) — pas une déduction
// depuis les données, qui ne portent aucune trace de la fusion elle-même.
var fusionConnue = map[string]struct{ code, niveau, libelle string }{
	"2A":  {"94", "REGION", "Collectivité de Corse"},
	"2B":  {"94", "REGION", "Collectivité de Corse"},
	"67":  {"67A", "DEPARTEMENT", "Collectivité européenne d'Alsace"},
	"68":  {"67A", "DEPARTEMENT", "Collectivité européenne d'Alsace"},
	"972": {"02", "REGION", "Collectivité territoriale de Martinique"},
	"973": {"03", "REGION", "Collectivité territoriale de Guyane"},
}

func loadCollectivites(ctx context.Context, pool *pgxpool.Pool) (*StatsCollectivites, error) {
	st := &StatsCollectivites{EPCIParDept: map[string][]*Groupement{}}
	if err := pool.QueryRow(ctx, `SELECT max(exercice) FROM core.collectivite_budget`).
		Scan(&st.Exercice); err != nil {
		return nil, err
	}
	st.Indicateurs = indicsCollectivite

	// --- poids relatif des quatre niveaux
	//
	// La vue expose le LIBELLÉ de l'indicateur, pas son code ; on le retraduit
	// pour que tout le reste du fichier ne manipule que des codes.
	prows, err := pool.Query(ctx, `
		SELECT p.niveau, i.code, p.collectivites, p.total::float8
		FROM derived.poids_des_niveaux p
		JOIN ref.indicator i ON i.label = p.indicateur
		WHERE p.exercice=$1`, st.Exercice)
	if err != nil {
		return nil, err
	}
	parNiveau := map[string]*NiveauPoids{}
	libNiveau := map[string]string{"COMMUNE": "Communes", "GROUPEMENT": "Intercommunalités",
		"DEPARTEMENT": "Départements", "REGION": "Régions"}
	for prows.Next() {
		var niv, ind string
		var nb int
		var tot float64
		if err := prows.Scan(&niv, &ind, &nb, &tot); err != nil {
			break
		}
		n := parNiveau[niv]
		if n == nil {
			n = &NiveauPoids{Niveau: niv, Libelle: libNiveau[niv], Collectivites: nb,
				Totaux: map[string]float64{}}
			parNiveau[niv] = n
		}
		n.Totaux[ind] = tot
	}
	prows.Close()
	for _, niv := range []string{"COMMUNE", "GROUPEMENT", "DEPARTEMENT", "REGION"} {
		if n := parNiveau[niv]; n != nil {
			st.Poids = append(st.Poids, *n)
		}
	}

	// --- régions et départements, avec leurs cinq indicateurs
	charger := func(niveau string) ([]*Collectivite, map[string]*Collectivite, error) {
		rows, err := pool.Query(ctx, `
			SELECT code, max(nom), max(population),
			       jsonb_object_agg(indicator_code, montant),
			       jsonb_object_agg(indicator_code, euros_par_hab)
			FROM core.collectivite_budget
			WHERE niveau=$1 AND exercice=$2
			GROUP BY code ORDER BY max(nom)`, niveau, st.Exercice)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		var out []*Collectivite
		idx := map[string]*Collectivite{}
		for rows.Next() {
			c := &Collectivite{Niveau: niveau, Total: map[string]float64{},
				ParHab: map[string]float64{}}
			var tot, hab map[string]*float64
			if err := rows.Scan(&c.Code, &c.Nom, &c.Population, &tot, &hab); err != nil {
				return nil, nil, err
			}
			for k, v := range tot {
				if v != nil {
					c.Total[k] = *v
				}
			}
			for k, v := range hab {
				if v != nil {
					c.ParHab[k] = *v
				}
			}
			c.Slug = partis.Slugify(c.Nom)
			out = append(out, c)
			idx[c.Code] = c
		}
		return out, idx, rows.Err()
	}
	var idxReg, idxDep map[string]*Collectivite
	st.Regions, idxReg, err = charger("REGION")
	if err != nil {
		return nil, err
	}
	st.Departements, idxDep, err = charger("DEPARTEMENT")
	if err != nil {
		return nil, err
	}
	// Les départements se lisent par numéro — c'est ainsi qu'on les cherche.
	sort.Slice(st.Departements, func(i, j int) bool {
		return st.Departements[i].Code < st.Departements[j].Code
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
		var code, nom string
		if err := grows2.Scan(&code, &nom); err != nil {
			grows2.Close()
			return nil, err
		}
		c := &Collectivite{Niveau: "DEPARTEMENT", Code: code, Nom: nom,
			Slug: partis.Slugify(nom), SansBudgetPropre: true}
		if f, ok := fusionConnue[code]; ok {
			c.FusionCode, c.FusionNiveau, c.FusionLibelle = f.code, f.niveau, f.libelle
			switch f.niveau {
			case "REGION":
				if cible := idxReg[f.code]; cible != nil {
					c.FusionLibelle, c.FusionSlug = cible.Nom, cible.Slug
				}
			case "DEPARTEMENT":
				if cible := idxDep[f.code]; cible != nil {
					c.FusionLibelle = cible.Nom
				}
			}
		} else {
			c.FusionLibelle = "aucune ligne budgétaire publiée à ce code"
		}
		st.Departements = append(st.Departements, c)
	}
	grows2.Close()
	if err := grows2.Err(); err != nil {
		return nil, err
	}
	sort.Slice(st.Departements, func(i, j int) bool {
		return st.Departements[i].Code < st.Departements[j].Code
	})

	// --- exécutifs. Le code de la collectivité se lit dans la circonscription
	// du mandat : « 44 Grand Est » pour une région, « 0114 Nantua » — le canton
	// — pour un département, dont les deux premiers caractères sont le numéro.
	elus := func(role string, longueur int, idx map[string]*Collectivite, vice bool) error {
		rows, err := pool.Query(ctx, `
			SELECT left(m.constituency,$2), p.given_name||' '||p.family_name, p.slug,
			       m.role, to_char(lower(m.validity),'DD/MM/YYYY')
			FROM core.mandate m JOIN core.person p ON p.id=m.person_id
			WHERE m.role LIKE $1 AND m.constituency IS NOT NULL
			  AND upper(m.validity) IS NULL`, role, longueur)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			var e EluLocal
			if err := rows.Scan(&code, &e.Nom, &e.Slug, &e.Role, &e.Depuis); err != nil {
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
	if err := elus("Président du conseil régional", 2, idxReg, false); err != nil {
		return nil, err
	}
	if err := elus("%Vice-président du conseil régional", 2, idxReg, true); err != nil {
		return nil, err
	}
	if err := elus("Président du conseil départemental", 2, idxDep, false); err != nil {
		return nil, err
	}
	if err := elus("%Vice-président du conseil départemental", 2, idxDep, true); err != nil {
		return nil, err
	}
	trierVices(idxReg)
	trierVices(idxDep)

	compte := func(mandat string, longueur int, idx map[string]*Collectivite) error {
		rows, err := pool.Query(ctx, `
			SELECT left(constituency,$2), count(*) FROM core.mandate
			WHERE mandate_type::text=$1 AND constituency IS NOT NULL
			  AND upper(validity) IS NULL GROUP BY 1`, mandat, longueur)
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
				c.Conseillers = n
			}
		}
		return rows.Err()
	}
	if err := compte("CONSEILLER_REGIONAL", 2, idxReg); err != nil {
		return nil, err
	}
	if err := compte("CONSEILLER_DEPARTEMENTAL", 2, idxDep); err != nil {
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
		if err := nrows.Scan(&n.Code, &n.Nombre, &n.Population, &n.AvecPresident); err != nil {
			break
		}
		n.Libelle = libelleNature[n.Code]
		if n.Libelle == "" {
			n.Libelle = n.Code
		}
		n.Fiscalite = natureAFiscalite[n.Code]
		st.NbEPCI += n.Nombre
		st.Natures = append(st.Natures, n)
	}
	nrows.Close()

	_ = pool.QueryRow(ctx, `SELECT count(*) FROM core.epci_membre`).Scan(&st.NbMembresEPCI)
	for _, n := range st.Natures {
		if n.Fiscalite {
			st.NbFiscalitePropre += n.Nombre
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
		var c CompetenceEPCI
		if err := crows.Scan(&c.Code, &c.Libelle, &c.Nombre); err != nil {
			break
		}
		st.Competences = append(st.Competences, c)
	}
	crows.Close()

	// Les groupements à fiscalité propre, rangés par département : c'est
	// l'échelon que le lecteur habite, et il n'a pas de page à lui ailleurs.
	grows, err := pool.Query(ctx, `
		SELECT e.siren, e.nom, e.nature_juridique, coalesce(e.code_departement,''),
		       coalesce(e.population_totale,0), coalesce(e.nb_membres,0),
		       trim(coalesce(e.president_prenom,'')||' '||coalesce(e.president_nom,'')),
		       (SELECT count(*) FROM core.epci_competence x WHERE x.epci_siren=e.siren),
		       coalesce(jsonb_object_agg(b.indicator_code, b.euros_par_hab)
		                FILTER (WHERE b.indicator_code IS NOT NULL), '{}'::jsonb)
		FROM core.epci e
		LEFT JOIN core.collectivite_budget b
		  ON b.niveau='GROUPEMENT' AND b.code=e.siren AND b.exercice=$1
		WHERE e.nature_juridique = ANY($2)
		GROUP BY e.siren
		ORDER BY e.code_departement, e.nom`, st.Exercice,
		[]string{"CC", "CA", "CU", "METRO", "MET69", "EPT"})
	if err != nil {
		return nil, err
	}
	for grows.Next() {
		g := &Groupement{ParHab: map[string]float64{}}
		var dep string
		var hab map[string]*float64
		if err := grows.Scan(&g.Siren, &g.Nom, &g.Nature, &dep, &g.Population,
			&g.Membres, &g.President, &g.Competences, &hab); err != nil {
			break
		}
		for k, v := range hab {
			if v != nil {
				g.ParHab[k] = *v
			}
		}
		if len(g.ParHab) > 0 {
			st.NbEPCIAvecBudget++
		}
		st.EPCIParDept[dep] = append(st.EPCIParDept[dep], g)
	}
	grows.Close()
	return st, nil
}

// relierFiches marque les élus qui ont une page sur ce site.
func relierFiches(st *StatsCollectivites, avecFiche map[string]bool) {
	marquer := func(e *EluLocal) {
		if e != nil {
			e.Fiche = avecFiche[e.Slug]
		}
	}
	for _, c := range append(append([]*Collectivite{}, st.Regions...), st.Departements...) {
		marquer(c.President)
		for i := range c.Vices {
			marquer(&c.Vices[i])
		}
	}
}

// cartesCollectivites dessine les deux cartes de l'index : une par niveau, sur
// la même boîte, donc superposables.
func cartesCollectivites(ctx context.Context, pool *pgxpool.Pool, st *StatsCollectivites,
	indic string) error {

	eur := func(v float64) string { return Nombre(int(v+0.5)) + " €" }

	reg, err := jeuContours(ctx, pool, "REGION", tolApercu)
	if err != nil {
		return err
	}
	var casesR []CaseCarte
	for _, c := range st.Regions {
		if v, ok := c.ParHab[indic]; ok {
			casesR = append(casesR, CaseCarte{Code: c.Code, Nom: c.Nom, Valeur: v})
		}
	}
	st.CarteRegions = pleine(reg, casesR, "€ par habitant", eur)

	dep, err := jeuContours(ctx, pool, "DEPARTEMENT", tolApercu)
	if err != nil {
		return err
	}
	var casesD []CaseCarte
	// Un code est « sur la carte » s'il a un contour, en métropole OU en
	// carton : sans la seconde moitié, la Guadeloupe, La Réunion et Mayotte
	// passaient pour absentes alors que leurs budgets sont en base.
	dansCarte := map[string]bool{}
	for _, code := range dep.Codes {
		dansCarte[code] = true
	}
	for _, o := range dep.outremer {
		dansCarte[o.Code] = true
	}
	for _, c := range st.Departements {
		if !dansCarte[c.Code] {
			st.HorsCarte = append(st.HorsCarte, c.Nom+" ("+c.Code+")")
			continue
		}
		if v, ok := c.ParHab[indic]; ok {
			casesD = append(casesD, CaseCarte{Code: c.Code, Nom: c.Nom, Valeur: v})
		}
	}
	sort.Strings(st.HorsCarte)
	st.CarteDepts = pleine(dep, casesD, "€ par habitant", eur)

	// La carte des intercommunalités : rendue possible par geo.contour_cog
	// (IGN Admin Express COG CARTO), qui donne enfin un tracé à chaque EPCI.
	_ = pool.QueryRow(ctx, `
		SELECT max(cog_millesime) FROM geo.contour_cog WHERE niveau='EPCI'`).Scan(&st.MillesimeEPCI)
	if st.MillesimeEPCI > 0 {
		epci, err := jeuContoursEPCI(ctx, pool, st.MillesimeEPCI, tolApercu)
		if err != nil {
			return err
		}
		vrows, err := pool.Query(ctx, `
			SELECT code, euros_par_hab::float8 FROM core.collectivite_budget
			WHERE niveau='GROUPEMENT' AND indicator_code=$1 AND exercice=$2
			  AND euros_par_hab IS NOT NULL`, indic, st.Exercice)
		if err != nil {
			return err
		}
		var casesE []CaseCarte
		for vrows.Next() {
			var code string
			var v float64
			if err := vrows.Scan(&code, &v); err != nil {
				vrows.Close()
				return err
			}
			casesE = append(casesE, CaseCarte{Code: code, Nom: epci.Noms[code], Valeur: v})
		}
		vrows.Close()
		if err := vrows.Err(); err != nil {
			return err
		}
		st.NbEPCISurCarte = len(epci.Codes) + len(epci.outremer)
		st.CarteEPCI = pleine(epci, casesE, "€ par habitant", eur)
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
		for _, c := range reg.Codes {
			fmt.Fprintf(&fr, `<path class="reg" data-code="%s" data-nom="%s" d="%s"/>`,
				c, template.HTMLEscapeString(reg.Noms[c]), reg.traces[c])
		}
		fr.WriteString(`</g>`)
		st.CarteEPCI.SVG = template.HTML(strings.Replace(string(st.CarteEPCI.SVG), "</svg>", fr.String()+"</svg>", 1))
	}
	return nil
}

// CelluleDepense : un montant (Md€) et sa part du maximum de SA COLONNE
// (pas de sa ligne) — un repère de fond ténu dans le tableau, sans dupliquer
// une barre par cellule.
type CelluleDepense struct {
	Valeur, Pct float64
}

type LigneDepense struct {
	Niveau        string
	Collectivites int
	Cellules      []CelluleDepense // même ordre que TableDepenses.Indicateurs
}

type TableDepenses struct {
	Indicateurs []IndicCollectivite
	Lignes      []LigneDepense
}

// tableDepenses construit un tableau niveau × indicateur plutôt qu'une suite
// de mini-graphiques à barres presque identiques (un par indicateur) : à 4
// niveaux et 5 indicateurs, un tableau se lit à la fois en ligne (le profil
// d'un niveau) et en colonne (qui dépense le plus pour un poste donné), ce
// qu'un mur de petits graphiques empêche de voir d'un coup d'œil.
func tableDepenses(poids []NiveauPoids, indics []IndicCollectivite) TableDepenses {
	t := TableDepenses{Indicateurs: indics}
	max := make([]float64, len(indics))
	for _, p := range poids {
		for i, ind := range indics {
			if v := p.Totaux[ind.Code]; v > max[i] {
				max[i] = v
			}
		}
	}
	for _, p := range poids {
		l := LigneDepense{Niveau: p.Libelle, Collectivites: p.Collectivites}
		for i, ind := range indics {
			v := p.Totaux[ind.Code]
			var pct float64
			if max[i] > 0 {
				pct = 100 * v / max[i]
			}
			l.Cellules = append(l.Cellules, CelluleDepense{Valeur: v / 1e9, Pct: pct})
		}
		t.Lignes = append(t.Lignes, l)
	}
	return t
}

// ── Page d'une collectivité ───────────────────────────────────────────

type LigneFinance struct {
	Libelle       string
	Total, ParHab float64
	Mediane       float64
	Rang, Sur     int
}

type PageCollectivite struct {
	Nom, Code, Slug                 string
	TypeLabel, TypePluriel, TypeURL string
	MotConseiller                   string
	Exercice, Population            int
	Conseillers, NbEPCI             int
	President                       *EluLocal
	Vices                           []EluLocal
	Lignes                          []LigneFinance
	Groupements                     []*Groupement
	Voisines                        []LienCarte
	// Pour une région, ses départements ; pour un département, sa région.
	Departements []Lieu
	Region       *Lieu
	Communes     []Lieu
	// Carte maillée des communes du département, avec le contour de ses
	// intercommunalités — absente pour une région, où le luxe de détail
	// noierait la lecture.
	CarteCommunes   template.HTML
	NbCommunesCarte int
	// Carte de situation dans la France entière (carte_situation.go).
	Situation *Situation
	// Pour un département, ses circonscriptions législatives (circonscriptions.go).
	Circonscriptions []Lieu
	// D'où viennent les recettes de la collectivité, face à son niveau.
	Recettes *OrigineRecettes
}

// OrigineRecettes : les recettes totales d'UNE collectivité, lues dans ses
// propres comptes (agrégats OFGL), découpées en impôts et taxes, DGF et reste.
// La référence est la même découpe pour l'ensemble du niveau.
type OrigineRecettes struct {
	Total, Impots, DGF, Autres           float64 // euros
	ImpotsPct, DGFPct, AutresPct         float64
	RefImpotsPct, RefDGFPct, RefAutrePct float64
	RefLibelle                           string
}

func origineRecettes(c *Collectivite, ref PartRecette) *OrigineRecettes {
	rec := c.Total["ofgl.recettes_totales_par_hab"]
	if rec <= 0 {
		return nil
	}
	imp, dgf := c.Total["ofgl.impots_taxes_par_hab"], c.Total["ofgl.dgf_par_hab"]
	o := &OrigineRecettes{Total: rec, Impots: imp, DGF: dgf, Autres: rec - imp - dgf,
		RefImpotsPct: ref.ImpotsPct, RefDGFPct: ref.DGFPct, RefAutrePct: ref.AutrePct,
		RefLibelle: strings.ToLower(ref.Libelle)}
	o.ImpotsPct, o.DGFPct, o.AutresPct = 100*imp/rec, 100*dgf/rec, 100*o.Autres/rec
	return o
}

// pagesCollectivites fabrique une page par région et par département. Le rang
// et la médiane sont calculés SUR LE NIVEAU, jamais entre niveaux : comparer le
// budget par habitant d'une région à celui d'un département n'a pas de sens,
// ils ne gèrent pas les mêmes compétences.
func pagesCollectivites(ctx context.Context, pool *pgxpool.Pool, st *StatsCollectivites, r *Resolveur) ([]PageCollectivite, error) {
	// La référence de chaque niveau pour « d'où vient l'argent » : la part de
	// la DGF et des impôts et taxes dans les recettes de tous ses membres.
	refRecettes := map[string]PartRecette{}
	for _, pr := range partsRecettes(st.Poids) {
		refRecettes[pr.Niveau] = pr
	}

	// région → départements, et département → région, lus dans le code
	// officiel géographique à travers les communes.
	depsDeReg := map[string]map[string]bool{}
	regDeDep := map[string]string{}
	comDeDep := map[string][]Lieu{}
	for _, c := range r.communes {
		comDeDep[c.Dept] = append(comDeDep[c.Dept], r.lieuCommune(c.Code))
		if depsDeReg[c.Region] == nil {
			depsDeReg[c.Region] = map[string]bool{}
		}
		depsDeReg[c.Region][c.Dept] = true
		regDeDep[c.Dept] = c.Region
	}
	var out []PageCollectivite
	for _, jeu := range []struct {
		liste                           []*Collectivite
		typeLabel, typePluriel, typeURL string
		motConseiller                   string
		parCode                         bool
	}{
		{st.Regions, "Conseil régional", "régions", "region", "régionaux", false},
		{st.Departements, "Conseil départemental", "départements", "departement", "départementaux", true},
	} {
		// Médianes du niveau, indicateur par indicateur.
		med := map[string]float64{}
		classe := map[string][]float64{}
		for _, c := range jeu.liste {
			for k, v := range c.ParHab {
				classe[k] = append(classe[k], v)
			}
		}
		for k, vs := range classe {
			sort.Float64s(vs)
			med[k] = vs[len(vs)/2]
		}
		for _, c := range jeu.liste {
			// Pas de page de détail pour un département sans budget propre :
			// il n'y a rien à y montrer que le tableau d'index ne dise déjà,
			// et une page presque vide serait pire qu'une absence de page.
			if c.SansBudgetPropre {
				continue
			}
			p := PageCollectivite{
				Nom: c.Nom, Code: c.Code, Slug: c.Slug,
				TypeLabel: jeu.typeLabel, TypePluriel: jeu.typePluriel, TypeURL: jeu.typeURL,
				MotConseiller: jeu.motConseiller,
				Exercice:      st.Exercice, Population: c.Population,
				Conseillers: c.Conseillers, President: c.President, Vices: c.Vices,
			}
			if jeu.parCode {
				p.Slug = c.Code
				p.Groupements = st.EPCIParDept[c.Code]
				p.NbEPCI = len(p.Groupements)
				if l, ok := r.lieuRegion(regDeDep[c.Code]); ok {
					p.Region = &l
				}
				p.Communes = comDeDep[c.Code]
				// Alsace (67A) et la Métropole de Lyon (691) portent un code
				// budgétaire qui n'est pas un code de département du COG.
				if len(p.Communes) == 0 {
					switch c.Code {
					case "67A":
						p.Communes = append(append([]Lieu{}, comDeDep["67"]...), comDeDep["68"]...)
					case "691", "69":
						p.Communes = comDeDep["69"]
					}
				}
				sort.Slice(p.Communes, func(i, j int) bool {
					return CleTri(p.Communes[i].Nom) < CleTri(p.Communes[j].Nom)
				})
			} else {
				for d := range depsDeReg[c.Code] {
					if l, ok := r.lieuDept(d); ok {
						p.Departements = append(p.Departements, l)
					}
				}
				sort.Slice(p.Departements, func(i, j int) bool {
					return p.Departements[i].Code < p.Departements[j].Code
				})
			}
			p.Recettes = origineRecettes(c, refRecettes[c.Niveau])
			for _, ind := range indicsCollectivite {
				v, ok := c.ParHab[ind.Code]
				if !ok {
					continue
				}
				l := LigneFinance{Libelle: ind.Libelle, Total: c.Total[ind.Code],
					ParHab: v, Mediane: med[ind.Code], Sur: len(classe[ind.Code])}
				for _, autre := range classe[ind.Code] {
					if autre > v {
						l.Rang++
					}
				}
				l.Rang++
				p.Lignes = append(p.Lignes, l)
			}
			for _, autre := range jeu.liste {
				if autre.Code == c.Code {
					continue
				}
				s := autre.Slug
				if jeu.parCode {
					s = autre.Code
				}
				p.Voisines = append(p.Voisines, LienCarte{Slug: s, Titre: autre.Nom})
			}
			out = append(out, p)
		}
	}
	return out, nil
}
