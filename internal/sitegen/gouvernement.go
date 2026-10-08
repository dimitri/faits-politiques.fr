package sitegen

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// L'exécutif : présidents, Premiers ministres, et les décrets de composition
// tels qu'ils sont parus au Journal officiel.
//
// Ce que cette page ne fait PAS, et c'est le point important : elle n'affiche
// pas « le gouvernement actuel ». Un décret nomme, il ne dit pas jusqu'à quand ;
// reconstituer une composition à une date donnée demande une règle de clôture
// des mandats qui n'est pas encore écrite dans `derived`. Publier une liste
// « au 13 septembre 2026 » reviendrait à inventer les dates de fin.
//
// Une seule déduction est faite, parce que le droit la porte : un Premier
// ministre reste en fonction jusqu'à la nomination du suivant.
type MemberDecree struct {
	Function, FunctionFr, Name, Portfolio string
	Attachment                            string
	Status, Slug                          string
	Rank                                  int
	// Fiche : la personne rapprochée a une page sur ce site. Le lien n'est
	// posé que pour un rapprochement à homonyme unique (CANDIDAT) — jamais pour
	// AMBIGU — et la cellule le dit.
	Profile bool
}

type DecreeGovernment struct {
	ActID, Date, DateISO string
	Title                string
	Nominations          []MemberDecree
	Cessations           []MemberDecree
}

type FirstMinister struct {
	Name, Start, End string
	Slug             string
	Profile          bool
	Ongoing          bool
	// Reconductions : un remaniement republie un décret nommant le même
	// Premier ministre. Trois lignes « François Fillon » à trois dates ne
	// décrivent pas trois Premiers ministres, et « Villepin, du 31/05/2005 au
	// 31/05/2005 » ne décrit rien du tout.
	Renewals []string
}

// TermPresidential : une ligne par ÉLECTION, pas par personne. Charles de
// Gaulle, François Mitterrand, Jacques Chirac et Emmanuel Macron ont chacun été
// élus deux fois ; les fusionner en une ligne effaçait la seconde élection et
// son résultat, qui ne ressemble pas toujours à la première.
type TermPresidential struct {
	Year                        int
	Elected, Opponent           string
	Slug                        string
	Profile                     bool
	Start, End                  string
	VotesElected, VotesOpponent int64
	PctElected, PctOpponent     float64
	PctElectedRegistered        float64
	Registered, Expressed       int64
	Abstention                  float64
	Decision, DecisionURL       string
	SuffrageUniversal           bool
}

type StatsGovernment struct {
	Terms         []TermPresidential
	Presidents    []President
	FirstMin      []FirstMinister
	Decrees       []DecreeGovernment
	All           []DecreeGovernment
	CountDecrees  int
	CountActs     int
	CountQuotes   int
	FirstDecree   string
	LastDecree    string
	Candidates    int
	Ambiguous     int
	Absents       int
	TermsAMO      int
	TermsInMissio int
}

var functionFr = map[string]string{
	"PREMIER_MINISTRE": "Premier ministre",
	"MINISTRE_ETAT":    "Ministre d'État",
	"MINISTRE":         "Ministre",
	"MINISTRE_DELEGUE": "Ministre délégué",
	"SECRETAIRE_ETAT":  "Secrétaire d'État",
	"HAUT_COMMISSAIRE": "Haut-commissaire",
}

func loadGovernment(ctx context.Context, pool *pgxpool.Pool, dataDir string,
	withProfile map[string]bool) (*StatsGovernment, error) {

	presidents, err := loadPresidents(dataDir + "/presidents.csv")
	if err != nil {
		return nil, err
	}
	st := &StatsGovernment{Presidents: presidents}

	// Les mandats présidentiels, élection par élection, depuis les décisions
	// de proclamation du Conseil constitutionnel.
	mrows, err := pool.Query(ctx, `
		SELECT p.annee, e.candidat, coalesce(a.candidat,''),
		       e.voix, coalesce(a.voix,0), r.inscrits, r.exprimes,
		       p.decision_numero, p.decision_url,
		       to_char(p.decision_date,'DD/MM/YYYY')
		FROM ref.pdr_proclamation p
		JOIN core.pdr_voix e ON e.annee=p.annee AND e.tour=p.tour AND e.elu
		LEFT JOIN core.pdr_voix a ON a.annee=p.annee AND a.tour=p.tour AND NOT a.elu
		JOIN core.pdr_resultat r ON r.annee=p.annee AND r.tour=p.tour
		ORDER BY p.annee DESC`)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		m := TermPresidential{SuffrageUniversal: true}
		if err := mrows.Scan(&m.Year, &m.Elected, &m.Opponent, &m.VotesElected, &m.VotesOpponent,
			&m.Registered, &m.Expressed, &m.Decision, &m.DecisionURL, &m.Start); err != nil {
			mrows.Close()
			return nil, err
		}
		m.Elected, m.Opponent = NameClean(m.Elected), NameClean(m.Opponent)
		if m.Expressed > 0 {
			m.PctElected = 100 * float64(m.VotesElected) / float64(m.Expressed)
			m.PctOpponent = 100 * float64(m.VotesOpponent) / float64(m.Expressed)
		}
		if m.Registered > 0 {
			m.PctElectedRegistered = 100 * float64(m.VotesElected) / float64(m.Registered)
		}
		st.Terms = append(st.Terms, m)
	}
	mrows.Close()
	// Le premier mandat de Charles de Gaulle ne vient pas du suffrage
	// universel : il a été élu le 21 décembre 1958 par un collège de quelque
	// 80 000 grands électeurs. Il n'a donc pas de proclamation du Conseil
	// constitutionnel au sens des autres, et la ligne le dit.
	st.Terms = append(st.Terms, TermPresidential{
		Year: 1958, Elected: "Charles de Gaulle", Start: "21/12/1958",
		SuffrageUniversal: false,
	})

	// Le lien vers la fiche : le nom publié par le Conseil constitutionnel
	// (« Georges POMPIDOU ») est comparé, casse et accents ôtés, au titulaire du
	// mandat PRESIDENT_REPUBLIQUE. Neuf personnes, aucun homonyme possible.
	presSlug := map[string]string{}
	prows0, err := pool.Query(ctx, `
		SELECT DISTINCT p.given_name||' '||p.family_name, p.slug
		FROM core.mandate m JOIN core.person p ON p.id=m.person_id
		WHERE m.mandate_type::text='PRESIDENT_REPUBLIQUE'`)
	if err != nil {
		return nil, err
	}
	for prows0.Next() {
		var name, slug string
		if err := prows0.Scan(&name, &slug); err != nil {
			break
		}
		presSlug[KeySort(name)] = slug
	}
	prows0.Close()
	for i := range st.Terms {
		m := &st.Terms[i]
		m.Slug = presSlug[KeySort(m.Elected)]
		m.Profile = m.Slug != "" && withProfile[m.Slug]
	}

	// Les dates de fin et les liens se déduisent de la succession.
	for i := range st.Terms {
		if i > 0 {
			st.Terms[i].End = st.Terms[i-1].Start
		}
	}

	_ = pool.QueryRow(ctx, `SELECT count(*) FROM core.acte_jo`).Scan(&st.CountActs)
	_ = pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE statut='CANDIDAT'),
		       count(*) FILTER (WHERE statut='AMBIGU'),
		       count(*) FILTER (WHERE statut='ABSENT'),
		       to_char(min(date_effet),'DD/MM/YYYY'), to_char(max(date_effet),'DD/MM/YYYY')
		FROM core.gouvernement_membre`).
		Scan(&st.CountQuotes, &st.Candidates, &st.Ambiguous, &st.Absents,
			&st.FirstDecree, &st.LastDecree)
	_ = pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE role='en mission')
		FROM core.mandate WHERE mandate_type::text='MINISTRE'`).
		Scan(&st.TermsAMO, &st.TermsInMissio)

	// La succession des Premiers ministres. Un PM cesse quand le suivant est
	// nommé : c'est la seule règle que le droit rende évidente, et elle évite
	// le mandat de quarante-huit heures que donnerait la règle des ministres.
	prows, qerr := pool.Query(ctx, `
		SELECT to_char(g.date_effet,'DD/MM/YYYY'), g.prenom||' '||g.nom,
		       CASE WHEN g.statut='CANDIDAT' THEN coalesce(p.slug,'') ELSE '' END
		FROM core.gouvernement_membre g LEFT JOIN core.person p ON p.id=g.person_id
		WHERE g.fonction='PREMIER_MINISTRE' AND g.sens='NOMINATION'
		ORDER BY g.date_effet DESC, g.rang`)
	if qerr != nil {
		return nil, qerr
	}
	type pmGross struct{ date, name, slug string }
	var gross []pmGross
	for prows.Next() {
		var b pmGross
		if err := prows.Scan(&b.date, &b.name, &b.slug); err != nil {
			prows.Close()
			return nil, err
		}
		gross = append(gross, b)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return nil, err
	}
	// bruts est trié du plus récent au plus ancien ; on fusionne les
	// nominations consécutives d'une même personne.
	keyName := func(s string) string { return KeySort(s) }
	for i := 0; i < len(gross); {
		j := i
		for j+1 < len(gross) && keyName(gross[j+1].name) == keyName(gross[i].name) {
			j++
		}
		pm := FirstMinister{Name: NameClean(gross[i].name), Start: gross[j].date}
		for k := i; k <= j; k++ {
			if gross[k].slug != "" {
				pm.Slug = gross[k].slug
			}
		}
		pm.Profile = pm.Slug != "" && withProfile[pm.Slug]
		for k := j - 1; k >= i; k-- {
			pm.Renewals = append(pm.Renewals, gross[k].date)
		}
		if i == 0 {
			pm.Ongoing = true
		} else {
			pm.End = gross[i-1].date
		}
		st.FirstMin = append(st.FirstMin, pm)
		i = j + 1
	}

	// Les décrets, tels quels. Aucune fusion, aucune reconstitution : un décret
	// de remaniement partiel affiche ses huit lignes et rien de plus.
	drows, err := pool.Query(ctx, `
		SELECT g.acte_id, to_char(g.date_effet,'DD/MM/YYYY'),
		       to_char(g.date_effet,'YYYY-MM-DD'),
		       coalesce(a.titre, a.titre_complet, ''),
		       g.sens, g.fonction, g.rang,
		       g.civilite||' '||g.prenom||' '||g.nom,
		       coalesce(g.portefeuille,''), coalesce(g.rattachement,''),
		       g.statut, coalesce(p.slug,'')
		FROM core.gouvernement_membre g
		LEFT JOIN core.acte_jo a ON a.id = g.acte_id
		LEFT JOIN core.person p ON p.id = g.person_id
		ORDER BY g.date_effet DESC, g.sens, g.rang`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	perAct := map[string]*DecreeGovernment{}
	var order []string
	for drows.Next() {
		var act, date, iso, title, direction string
		var m MemberDecree
		if err := drows.Scan(&act, &date, &iso, &title, &direction, &m.Function, &m.Rank,
			&m.Name, &m.Portfolio, &m.Attachment, &m.Status, &m.Slug); err != nil {
			return nil, err
		}
		m.Name = NameClean(m.Name)
		m.Profile = m.Status == "CANDIDAT" && m.Slug != "" && withProfile[m.Slug]
		m.FunctionFr = functionFr[m.Function]
		if m.FunctionFr == "" {
			m.FunctionFr = m.Function
		}
		d := perAct[act]
		if d == nil {
			d = &DecreeGovernment{ActID: act, Date: date, DateISO: iso, Title: title}
			perAct[act] = d
			order = append(order, act)
		}
		if direction == "CESSATION" {
			d.Cessations = append(d.Cessations, m)
		} else {
			d.Nominations = append(d.Nominations, m)
		}
	}
	for _, a := range order {
		st.Decrees = append(st.Decrees, *perAct[a])
	}
	sort.SliceStable(st.Decrees, func(i, j int) bool {
		return st.Decrees[i].DateISO > st.Decrees[j].DateISO
	})
	// La page d'entrée n'en montre que les plus récents : les 160 décrets
	// tenaient en une page de 520 Ko, dont personne ne lit le bas.
	st.All = st.Decrees
	st.CountDecrees = len(st.Decrees)
	const onLIndex = 20
	if len(st.Decrees) > onLIndex {
		st.Decrees = st.Decrees[:onLIndex]
	}
	return st, drows.Err()
}
