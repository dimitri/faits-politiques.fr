package sitegen

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MembreGroupe struct {
	Slug, Nom, Groupe                  string
	Pour, Contre, Abstention, Exprimes int
	Loyaute                            int // % de votes conformes à la position majoritaire du groupe
}

type ScrutinGroupe struct {
	Slug, Objet, Date, Position string
	Pour, Contre, Abstention    int
}

type Groupe struct {
	Slug, Nom, NomCourt, ANOrganeUID, Periode string
	Membres                                   []MembreGroupe
	Scrutins                                  []ScrutinGroupe
	Effectif                                  int
	MajPour, MajContre, MajAbst               int
	Partis                                    []*Organisation
	PartisDeclares                            []PartiDeclare
	Coalitions                                []*Coalition
	Pour, Contre, Abstention, Exprimes        int
	ScrutinsCouverts                          int
	Cohesion                                  int
}

// loadGroupes charge les groupes parlementaires et leurs positions agrégées.
//
// Tout est calculé à partir des VOTES INDIVIDUELS transcrits du relevé : la
// position d'un groupe n'est jamais une consigne, c'est la somme de ce que ses
// membres ont voté. La cohésion est la part des votes conformes à la position
// majoritaire du groupe ce jour-là — elle mesure une régularité observée, pas
// une discipline supposée.
func loadGroupes(ctx context.Context, pool *pgxpool.Pool) (map[string]*Groupe, error) {
	out := map[string]*Groupe{}
	byID := map[int64]*Groupe{}

	rows, err := pool.Query(ctx, `
		SELECT o.id, o.slug, o.name, coalesce(o.short_name,''), i.value,
		       coalesce(lower(o.validity)::text,''), coalesce(upper(o.validity)::text,'')
		FROM core.organization o
		JOIN core.organization_identifier i
		  ON i.organization_id = o.id AND i.scheme = 'AN_ORGANE'
		WHERE o.kind = 'PARLIAMENTARY_GROUP'
		  AND EXISTS (SELECT 1 FROM core.ballot b WHERE b.organization_id = o.id)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		g := &Groupe{}
		var id int64
		var debut, fin string
		if err := rows.Scan(&id, &g.Slug, &g.Nom, &g.NomCourt, &g.ANOrganeUID, &debut, &fin); err != nil {
			return nil, err
		}
		g.Periode = periode(debut, fin)
		out[g.ANOrganeUID] = g
		byID[id] = g
	}
	rows.Close()

	// Décompte agrégé et couverture — mv.scrutin_groupe_vote (internal/
	// matview) porte déjà un total par scrutin ; sommé sur tous les
	// scrutins, il donne le total par groupe sans rescanner core.ballot.
	rows, err = pool.Query(ctx, `
		SELECT organization_id, position, sum(n)::int
		FROM mv.scrutin_groupe_vote
		GROUP BY 1,2`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var pos string
		var n int
		if err := rows.Scan(&id, &pos, &n); err != nil {
			return nil, err
		}
		g, ok := byID[id]
		if !ok {
			continue
		}
		switch pos {
		case "FOR":
			g.Pour = n
		case "AGAINST":
			g.Contre = n
		case "ABSTAIN":
			g.Abstention = n
		}
	}
	rows.Close()

	for _, g := range out {
		g.Exprimes = g.Pour + g.Contre + g.Abstention
	}

	if err := loadMembres(ctx, pool, byID); err != nil {
		return nil, err
	}
	if err := loadScrutinsGroupe(ctx, pool, byID); err != nil {
		return nil, err
	}
	if err := loadPartisDeclares(ctx, pool, byID); err != nil {
		return nil, err
	}
	return out, nil
}

// loadMembres lit mv.scrutin_vote_nominal (internal/matview) — plus le JOIN
// ballot/person et le GROUP BY sur la totalité de core.ballot que cette
// fonction refaisait à chaque construction.
func loadMembres(ctx context.Context, pool *pgxpool.Pool, byID map[int64]*Groupe) error {
	rows, err := pool.Query(ctx, `
		SELECT organization_id, person_slug, person_given_name, person_family_name,
		       count(*) FILTER (WHERE position = 'FOR'),
		       count(*) FILTER (WHERE position = 'AGAINST'),
		       count(*) FILTER (WHERE position = 'ABSTAIN')
		FROM mv.scrutin_vote_nominal
		WHERE organization_id IS NOT NULL
		GROUP BY 1,2,3,4
		ORDER BY 1, person_given_name || ' ' || person_family_name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var m MembreGroupe
		var givenName, familyName string
		if err := rows.Scan(&id, &m.Slug, &givenName, &familyName, &m.Pour, &m.Contre, &m.Abstention); err != nil {
			return err
		}
		m.Nom = givenName + " " + familyName
		g, ok := byID[id]
		if !ok {
			continue
		}
		m.Exprimes = m.Pour + m.Contre + m.Abstention
		g.Membres = append(g.Membres, m)
		g.Effectif++
	}
	return rows.Err()
}

// loadScrutinsGroupe lit mv.scrutin_groupe_vote (internal/matview), pivotée
// par position — plus le GROUP BY sur la totalité de core.ballot que cette
// fonction refaisait à chaque construction ; le JOIN sur core.scrutin reste
// applicatif, mais porte sur une table de quelques dizaines de milliers de
// lignes, pas sur le fait 4,9 millions de lignes.
func loadScrutinsGroupe(ctx context.Context, pool *pgxpool.Pool, byID map[int64]*Groupe) error {
	rows, err := pool.Query(ctx, `
		SELECT x.organization_id, s.slug, s.objet, to_char(s.date_seance,'DD/MM/YYYY'),
		       x.pour, x.contre, x.abst
		FROM (
		  SELECT organization_id, scrutin_id,
		         -- coalesce(...,0), pas sum() nu : mv.scrutin_groupe_vote
		         -- n'a de ligne QUE pour les positions réellement observées
		         -- — un scrutin sans abstention dans ce groupe n'a aucune
		         -- ligne 'ABSTAIN' du tout, et sum() sur un ensemble vide
		         -- rend NULL, jamais 0 (contrairement à count()).
		         coalesce(sum(n) FILTER (WHERE position='FOR'), 0)     AS pour,
		         coalesce(sum(n) FILTER (WHERE position='AGAINST'), 0) AS contre,
		         coalesce(sum(n) FILTER (WHERE position='ABSTAIN'), 0) AS abst
		  FROM mv.scrutin_groupe_vote
		  GROUP BY 1,2) x
		JOIN core.scrutin s ON s.id = x.scrutin_id
		ORDER BY x.organization_id, s.date_seance DESC, s.numero DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	conformes := map[int64]int{}
	total := map[int64]int{}
	for rows.Next() {
		var id int64
		var sg ScrutinGroupe
		if err := rows.Scan(&id, &sg.Slug, &sg.Objet, &sg.Date, &sg.Pour, &sg.Contre, &sg.Abstention); err != nil {
			return err
		}
		g, ok := byID[id]
		if !ok {
			continue
		}
		// Position majoritaire du groupe sur ce scrutin, absents exclus.
		maj, n := "FOR", sg.Pour
		if sg.Contre > n {
			maj, n = "AGAINST", sg.Contre
		}
		if sg.Abstention > n {
			maj, n = "ABSTAIN", sg.Abstention
		}
		sg.Position = positionFr[maj]
		// Ce qui caractérise un groupe n'est pas le cumul de ses votes — qui
		// mesure surtout sa taille — mais la répartition de ses positions
		// MAJORITAIRES scrutin par scrutin.
		switch maj {
		case "FOR":
			g.MajPour++
		case "AGAINST":
			g.MajContre++
		case "ABSTAIN":
			g.MajAbst++
		}
		exprimes := sg.Pour + sg.Contre + sg.Abstention
		if exprimes > 0 {
			conformes[id] += n
			total[id] += exprimes
			g.ScrutinsCouverts++
			if len(g.Scrutins) < 40 {
				sg.Objet, _ = TitreCourt(sg.Objet)
				g.Scrutins = append(g.Scrutins, sg)
			}
		}
	}
	for id, g := range byID {
		if total[id] > 0 {
			g.Cohesion = conformes[id] * 100 / total[id]
		}
		for i := range g.Membres {
			if g.Membres[i].Exprimes > 0 {
				g.Membres[i].Groupe = g.NomCourt
			}
		}
	}
	return rows.Err()
}

var _ = fmt.Sprint

type PartiDeclare struct {
	Nom     string
	Deputes int
	Periode string
}

// loadPartisDeclares établit la composition OBSERVÉE d'un groupe à partir des
// mandats de parti que la source publie pour ses membres.
//
// Limite majeure, affichée sur la page : l'Assemblée n'a publié aucune
// appartenance partisane pour la 17e législature — tous les mandats de parti
// s'arrêtent au 10 juin 2024, jour de la dissolution. Ce que montre cette
// composition est donc l'appartenance déclarée LORS DE LA LÉGISLATURE
// PRÉCÉDENTE par des députés qui siègent aujourd'hui dans ce groupe. C'est une
// indication, pas la composition actuelle, et le dire est la seule façon de ne
// pas induire en erreur.
// loadPartisDeclares part de la liste (organisation, personne) de mv.
// scrutin_vote_nominal, DISTINCT — plus le JOIN sur la totalité de
// core.ballot que cette fonction refaisait à chaque construction pour
// obtenir seulement cette liste d'appartenances, avant de la croiser avec
// core.affiliation (une table de quelques milliers de lignes).
func loadPartisDeclares(ctx context.Context, pool *pgxpool.Pool, byID map[int64]*Groupe) error {
	rows, err := pool.Query(ctx, `
		SELECT g.organization_id, o.name, count(DISTINCT a.person_id),
		       min(lower(a.validity))::text, max(coalesce(upper(a.validity)::text,''))
		FROM (SELECT DISTINCT organization_id, person_slug FROM mv.scrutin_vote_nominal
		      WHERE organization_id IS NOT NULL) g
		JOIN core.person p ON p.slug = g.person_slug
		JOIN core.affiliation a ON a.person_id = p.id AND a.organization_kind = 'PARTY'
		JOIN core.organization o ON o.id = a.organization_id
		GROUP BY 1,2
		HAVING count(DISTINCT a.person_id) > 0
		ORDER BY 1, 3 DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p PartiDeclare
		var debut, fin string
		if err := rows.Scan(&id, &p.Nom, &p.Deputes, &debut, &fin); err != nil {
			return err
		}
		g, ok := byID[id]
		if !ok || p.Deputes < 2 {
			continue // un député isolé ne caractérise pas un groupe
		}
		p.Periode = periode(debut, fin)
		if len(g.PartisDeclares) < 10 {
			g.PartisDeclares = append(g.PartisDeclares, p)
		}
	}
	return rows.Err()
}
