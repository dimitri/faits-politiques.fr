package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupeEP struct {
	Slug, Nom, NomCourt                string
	Effectif, Pour, Contre, Abstention int
}

type ThemeEuroVoc struct {
	Code, Label string
	Scrutins    int
}

type StatsEurope struct {
	Scrutins, Votes, Eurodeputes int
	Groupes                      []GroupeEP
	Derniers                     []Vote
	Themes                       []ThemeEuroVoc
	Eurodep                      []*Person
}

// loadEurope charge le volet européen. Les votes du Parlement européen vivent
// dans les mêmes tables que ceux de l'Assemblée, distingués par leur
// institution — mais ils ne sont JAMAIS agrégés avec eux : ce sont deux espaces
// de vote distincts, non superposables.
func loadEurope(ctx context.Context, pool *pgxpool.Pool) (*StatsEurope, error) {
	e := &StatsEurope{}
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM core.scrutin WHERE institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id=b.scrutin_id
		         WHERE s.institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(*) FROM core.person_identifier WHERE scheme='EP_MEP')`).
		Scan(&e.Scrutins, &e.Votes, &e.Eurodeputes); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT o.slug, o.name, coalesce(o.short_name,''),
		       count(DISTINCT b.person_id),
		       count(*) FILTER (WHERE b.position='FOR'),
		       count(*) FILTER (WHERE b.position='AGAINST'),
		       count(*) FILTER (WHERE b.position='ABSTAIN')
		FROM core.ballot b
		JOIN core.scrutin s ON s.id = b.scrutin_id AND s.institution='PARLEMENT_EUROPEEN'
		JOIN core.organization o ON o.id = b.organization_id
		GROUP BY o.slug, o.name, o.short_name
		ORDER BY 4 DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g GroupeEP
		if err := rows.Scan(&g.Slug, &g.Nom, &g.NomCourt, &g.Effectif,
			&g.Pour, &g.Contre, &g.Abstention); err != nil {
			return nil, err
		}
		e.Groupes = append(e.Groupes, g)
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT slug, objet, to_char(date_seance,'DD/MM/YYYY'), coalesce(type_vote,'')
		FROM core.scrutin WHERE institution='PARLEMENT_EUROPEEN'
		ORDER BY date_seance DESC, numero DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v Vote
		if err := rows.Scan(&v.Slug, &v.Objet, &v.Date, &v.Resultat); err != nil {
			return nil, err
		}
		v.Objet = tronque(v.Objet, 130)
		e.Derniers = append(e.Derniers, v)
	}
	rows.Close()

	// Les thèmes viennent d'EuroVoc, le thésaurus officiel de l'Union :
	// l'affectation est transcrite du producteur, pas produite par ce site.
	rows, err = pool.Query(ctx, `
		SELECT t.code, t.label, count(DISTINCT a.scrutin_id)
		FROM core.topic_assignment a
		JOIN ref.topic t ON t.code = a.topic_code AND t.taxonomy_version = 'eurovoc'
		GROUP BY t.code, t.label
		HAVING count(DISTINCT a.scrutin_id) >= 20
		ORDER BY 3 DESC LIMIT 60`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t ThemeEuroVoc
		if err := rows.Scan(&t.Code, &t.Label, &t.Scrutins); err != nil {
			return nil, err
		}
		e.Themes = append(e.Themes, t)
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT DISTINCT p.slug, p.given_name, p.family_name,
		       coalesce(o.short_name, o.name, '')
		FROM core.person p
		JOIN core.person_identifier i ON i.person_id = p.id AND i.scheme = 'EP_MEP'
		LEFT JOIN core.affiliation a ON a.person_id = p.id
		 AND a.organization_kind = 'EP_GROUP' AND upper_inf(a.validity)
		LEFT JOIN core.organization o ON o.id = a.organization_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		p := &Person{}
		if err := rows.Scan(&p.Slug, &p.Prenom, &p.Nom, &p.Groupe); err != nil {
			return nil, err
		}
		e.Eurodep = append(e.Eurodep, p)
	}
	return e, rows.Err()
}
