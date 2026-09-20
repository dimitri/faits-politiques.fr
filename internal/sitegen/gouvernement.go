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
type MembreDecret struct {
	Fonction, FonctionFr, Nom, Portefeuille string
	Rattachement                            string
	Statut, Slug                            string
	Rang                                    int
	// Fiche : la personne rapprochée a une page sur ce site. Le lien n'est
	// posé que pour un rapprochement à homonyme unique (CANDIDAT) — jamais pour
	// AMBIGU — et la cellule le dit.
	Fiche bool
}

type DecretGouvernement struct {
	ActeID, Date, DateISO string
	Titre                 string
	Nominations           []MembreDecret
	Cessations            []MembreDecret
}

type PremierMinistre struct {
	Nom, Debut, Fin string
	Slug            string
	Fiche           bool
	EnCours         bool
	// Reconductions : un remaniement republie un décret nommant le même
	// Premier ministre. Trois lignes « François Fillon » à trois dates ne
	// décrivent pas trois Premiers ministres, et « Villepin, du 31/05/2005 au
	// 31/05/2005 » ne décrit rien du tout.
	Reconductions []string
}

// MandatPresidentiel : une ligne par ÉLECTION, pas par personne. Charles de
// Gaulle, François Mitterrand, Jacques Chirac et Emmanuel Macron ont chacun été
// élus deux fois ; les fusionner en une ligne effaçait la seconde élection et
// son résultat, qui ne ressemble pas toujours à la première.
type MandatPresidentiel struct {
	Annee                   int
	Elu, Adversaire         string
	Slug                    string
	Fiche                   bool
	Debut, Fin              string
	VoixElu, VoixAdversaire int64
	PctElu, PctAdversaire   float64
	PctEluInscrits          float64
	Inscrits, Exprimes      int64
	Abstention              float64
	Decision, DecisionURL   string
	SuffrageUniversel       bool
}

type StatsGouvernement struct {
	Mandats         []MandatPresidentiel
	Presidents      []President
	PremiersMin     []PremierMinistre
	Decrets         []DecretGouvernement
	Tous            []DecretGouvernement
	NbDecrets       int
	NbActes         int
	NbCitations     int
	PremierDecret   string
	DernierDecret   string
	Candidats       int
	Ambigus         int
	Absents         int
	MandatsAMO      int
	MandatsEnMissio int
}

var fonctionFr = map[string]string{
	"PREMIER_MINISTRE": "Premier ministre",
	"MINISTRE_ETAT":    "Ministre d'État",
	"MINISTRE":         "Ministre",
	"MINISTRE_DELEGUE": "Ministre délégué",
	"SECRETAIRE_ETAT":  "Secrétaire d'État",
	"HAUT_COMMISSAIRE": "Haut-commissaire",
}

func loadGouvernement(ctx context.Context, pool *pgxpool.Pool, dataDir string,
	avecFiche map[string]bool) (*StatsGouvernement, error) {

	presidents, err := loadPresidents(dataDir + "/presidents.csv")
	if err != nil {
		return nil, err
	}
	st := &StatsGouvernement{Presidents: presidents}

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
		m := MandatPresidentiel{SuffrageUniversel: true}
		if err := mrows.Scan(&m.Annee, &m.Elu, &m.Adversaire, &m.VoixElu, &m.VoixAdversaire,
			&m.Inscrits, &m.Exprimes, &m.Decision, &m.DecisionURL, &m.Debut); err != nil {
			mrows.Close()
			return nil, err
		}
		m.Elu, m.Adversaire = NomPropre(m.Elu), NomPropre(m.Adversaire)
		if m.Exprimes > 0 {
			m.PctElu = 100 * float64(m.VoixElu) / float64(m.Exprimes)
			m.PctAdversaire = 100 * float64(m.VoixAdversaire) / float64(m.Exprimes)
		}
		if m.Inscrits > 0 {
			m.PctEluInscrits = 100 * float64(m.VoixElu) / float64(m.Inscrits)
		}
		st.Mandats = append(st.Mandats, m)
	}
	mrows.Close()
	// Le premier mandat de Charles de Gaulle ne vient pas du suffrage
	// universel : il a été élu le 21 décembre 1958 par un collège de quelque
	// 80 000 grands électeurs. Il n'a donc pas de proclamation du Conseil
	// constitutionnel au sens des autres, et la ligne le dit.
	st.Mandats = append(st.Mandats, MandatPresidentiel{
		Annee: 1958, Elu: "Charles de Gaulle", Debut: "21/12/1958",
		SuffrageUniversel: false,
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
		var nom, slug string
		if err := prows0.Scan(&nom, &slug); err != nil {
			break
		}
		presSlug[CleTri(nom)] = slug
	}
	prows0.Close()
	for i := range st.Mandats {
		m := &st.Mandats[i]
		m.Slug = presSlug[CleTri(m.Elu)]
		m.Fiche = m.Slug != "" && avecFiche[m.Slug]
	}

	// Les dates de fin et les liens se déduisent de la succession.
	for i := range st.Mandats {
		if i > 0 {
			st.Mandats[i].Fin = st.Mandats[i-1].Debut
		}
	}

	_ = pool.QueryRow(ctx, `SELECT count(*) FROM core.acte_jo`).Scan(&st.NbActes)
	_ = pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE statut='CANDIDAT'),
		       count(*) FILTER (WHERE statut='AMBIGU'),
		       count(*) FILTER (WHERE statut='ABSENT'),
		       to_char(min(date_effet),'DD/MM/YYYY'), to_char(max(date_effet),'DD/MM/YYYY')
		FROM core.gouvernement_membre`).
		Scan(&st.NbCitations, &st.Candidats, &st.Ambigus, &st.Absents,
			&st.PremierDecret, &st.DernierDecret)
	_ = pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE role='en mission')
		FROM core.mandate WHERE mandate_type::text='MINISTRE'`).
		Scan(&st.MandatsAMO, &st.MandatsEnMissio)

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
	type pmBrut struct{ date, nom, slug string }
	var bruts []pmBrut
	for prows.Next() {
		var b pmBrut
		if err := prows.Scan(&b.date, &b.nom, &b.slug); err != nil {
			prows.Close()
			return nil, err
		}
		bruts = append(bruts, b)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return nil, err
	}
	// bruts est trié du plus récent au plus ancien ; on fusionne les
	// nominations consécutives d'une même personne.
	cleNom := func(s string) string { return CleTri(s) }
	for i := 0; i < len(bruts); {
		j := i
		for j+1 < len(bruts) && cleNom(bruts[j+1].nom) == cleNom(bruts[i].nom) {
			j++
		}
		pm := PremierMinistre{Nom: NomPropre(bruts[i].nom), Debut: bruts[j].date}
		for k := i; k <= j; k++ {
			if bruts[k].slug != "" {
				pm.Slug = bruts[k].slug
			}
		}
		pm.Fiche = pm.Slug != "" && avecFiche[pm.Slug]
		for k := j - 1; k >= i; k-- {
			pm.Reconductions = append(pm.Reconductions, bruts[k].date)
		}
		if i == 0 {
			pm.EnCours = true
		} else {
			pm.Fin = bruts[i-1].date
		}
		st.PremiersMin = append(st.PremiersMin, pm)
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
	parActe := map[string]*DecretGouvernement{}
	var ordre []string
	for drows.Next() {
		var acte, date, iso, titre, sens string
		var m MembreDecret
		if err := drows.Scan(&acte, &date, &iso, &titre, &sens, &m.Fonction, &m.Rang,
			&m.Nom, &m.Portefeuille, &m.Rattachement, &m.Statut, &m.Slug); err != nil {
			return nil, err
		}
		m.Nom = NomPropre(m.Nom)
		m.Fiche = m.Statut == "CANDIDAT" && m.Slug != "" && avecFiche[m.Slug]
		m.FonctionFr = fonctionFr[m.Fonction]
		if m.FonctionFr == "" {
			m.FonctionFr = m.Fonction
		}
		d := parActe[acte]
		if d == nil {
			d = &DecretGouvernement{ActeID: acte, Date: date, DateISO: iso, Titre: titre}
			parActe[acte] = d
			ordre = append(ordre, acte)
		}
		if sens == "CESSATION" {
			d.Cessations = append(d.Cessations, m)
		} else {
			d.Nominations = append(d.Nominations, m)
		}
	}
	for _, a := range ordre {
		st.Decrets = append(st.Decrets, *parActe[a])
	}
	sort.SliceStable(st.Decrets, func(i, j int) bool {
		return st.Decrets[i].DateISO > st.Decrets[j].DateISO
	})
	// La page d'entrée n'en montre que les plus récents : les 160 décrets
	// tenaient en une page de 520 Ko, dont personne ne lit le bas.
	st.Tous = st.Decrets
	st.NbDecrets = len(st.Decrets)
	const surLIndex = 20
	if len(st.Decrets) > surLIndex {
		st.Decrets = st.Decrets[:surLIndex]
	}
	return st, drows.Err()
}
