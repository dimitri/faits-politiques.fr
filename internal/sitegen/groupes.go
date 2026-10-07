package sitegen

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MemberGroup struct {
	Slug, Name, Group                   string
	For, Against, Abstention, Expressed int
	Loyalty                             int // % de votes conformes à la position majoritaire du groupe
}

type ElectionGroup struct {
	Slug, Object, Date, Position string
	For, Against, Abstention     int
}

type Group struct {
	Slug, Name, NameShort, ANBodyUID, Period   string
	Members                                    []MemberGroup
	Elections                                  []ElectionGroup
	Headcount                                  int
	MajorityFor, MajorityAgainst, MajorityAbst int
	Parties                                    []*Organization
	PartiesDeclared                            []PartyDeclared
	Coalitions                                 []*Coalition
	For, Against, Abstention, Expressed        int
	ElectionsCovered                           int
	Cohesion                                   int
}

// loadGroups charge les groupes parlementaires et leurs positions agrégées.
//
// Tout est calculé à partir des VOTES INDIVIDUELS transcrits du relevé : la
// position d'un groupe n'est jamais une consigne, c'est la somme de ce que ses
// membres ont voté. La cohésion est la part des votes conformes à la position
// majoritaire du groupe ce jour-là — elle mesure une régularité observée, pas
// une discipline supposée.
func loadGroups(ctx context.Context, pool *pgxpool.Pool) (map[string]*Group, error) {
	out := map[string]*Group{}
	byID := map[int64]*Group{}

	rows, err := pool.Query(ctx, `
		SELECT o.id, o.slug, o.name, coalesce(o.short_name,''), i.value,
		       coalesce(lower(o.validity)::text,''), coalesce(upper(o.validity)::text,'')
		FROM core.organization o
		JOIN core.organization_identifier i
		  ON i.organization_id = o.id AND i.scheme = 'AN_ORGANE'
		WHERE o.kind = 'PARLIAMENTARY_GROUP'
		  AND EXISTS (SELECT 1 FROM mv.scrutin_groupe_vote v WHERE v.organization_id = o.id)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		g := &Group{}
		var id int64
		var start, end string
		if err := rows.Scan(&id, &g.Slug, &g.Name, &g.NameShort, &g.ANBodyUID, &start, &end); err != nil {
			return nil, err
		}
		g.Period = period(start, end)
		out[g.ANBodyUID] = g
		byID[id] = g
	}
	rows.Close()

	// Décompte agrégé et couverture — mv.scrutin_groupe_vote (internal/
	// matview) porte déjà un total par scrutin ; sommé sur tous les
	// scrutins, il donne le total par groupe sans rescanner core.ballot.
	rows, err = pool.Query(ctx, `
		SELECT organization_id, position, sum(nombre_votes)::int
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
			g.For = n
		case "AGAINST":
			g.Against = n
		case "ABSTAIN":
			g.Abstention = n
		}
	}
	rows.Close()

	for _, g := range out {
		g.Expressed = g.For + g.Against + g.Abstention
	}

	if err := loadMembers(ctx, pool, byID); err != nil {
		return nil, err
	}
	if err := loadElectionsGroup(ctx, pool, byID); err != nil {
		return nil, err
	}
	if err := loadPartiesDeclared(ctx, pool, byID); err != nil {
		return nil, err
	}
	return out, nil
}

// loadMembers lit mv.scrutin_vote_nominal (internal/matview) — plus le JOIN
// ballot/person et le GROUP BY sur la totalité de core.ballot que cette
// fonction refaisait à chaque construction.
func loadMembers(ctx context.Context, pool *pgxpool.Pool, byID map[int64]*Group) error {
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
		var m MemberGroup
		var givenName, familyName string
		if err := rows.Scan(&id, &m.Slug, &givenName, &familyName, &m.For, &m.Against, &m.Abstention); err != nil {
			return err
		}
		m.Name = givenName + " " + familyName
		g, ok := byID[id]
		if !ok {
			continue
		}
		m.Expressed = m.For + m.Against + m.Abstention
		g.Members = append(g.Members, m)
		g.Headcount++
	}
	return rows.Err()
}

// loadElectionsGroup lit mv.scrutin_groupe_vote (internal/matview), pivotée
// par position — plus le GROUP BY sur la totalité de core.ballot que cette
// fonction refaisait à chaque construction ; le JOIN sur core.scrutin reste
// applicatif, mais porte sur une table de quelques dizaines de milliers de
// lignes, pas sur le fait 4,9 millions de lignes.
func loadElectionsGroup(ctx context.Context, pool *pgxpool.Pool, byID map[int64]*Group) error {
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
		         coalesce(sum(nombre_votes) FILTER (WHERE position='FOR'), 0)     AS pour,
		         coalesce(sum(nombre_votes) FILTER (WHERE position='AGAINST'), 0) AS contre,
		         coalesce(sum(nombre_votes) FILTER (WHERE position='ABSTAIN'), 0) AS abst
		  FROM mv.scrutin_groupe_vote
		  GROUP BY 1,2) x
		JOIN core.scrutin s ON s.id = x.scrutin_id
		ORDER BY x.organization_id, s.date_seance DESC, s.numero DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	compliant := map[int64]int{}
	total := map[int64]int{}
	for rows.Next() {
		var id int64
		var sg ElectionGroup
		if err := rows.Scan(&id, &sg.Slug, &sg.Object, &sg.Date, &sg.For, &sg.Against, &sg.Abstention); err != nil {
			return err
		}
		g, ok := byID[id]
		if !ok {
			continue
		}
		// Position majoritaire du groupe sur ce scrutin, absents exclus.
		majority, n := "FOR", sg.For
		if sg.Against > n {
			majority, n = "AGAINST", sg.Against
		}
		if sg.Abstention > n {
			majority, n = "ABSTAIN", sg.Abstention
		}
		sg.Position = positionFr[majority]
		// Ce qui caractérise un groupe n'est pas le cumul de ses votes — qui
		// mesure surtout sa taille — mais la répartition de ses positions
		// MAJORITAIRES scrutin par scrutin.
		switch majority {
		case "FOR":
			g.MajorityFor++
		case "AGAINST":
			g.MajorityAgainst++
		case "ABSTAIN":
			g.MajorityAbst++
		}
		expressed := sg.For + sg.Against + sg.Abstention
		if expressed > 0 {
			compliant[id] += n
			total[id] += expressed
			g.ElectionsCovered++
			if len(g.Elections) < 40 {
				sg.Object, _ = TitleShort(sg.Object)
				g.Elections = append(g.Elections, sg)
			}
		}
	}
	for id, g := range byID {
		if total[id] > 0 {
			g.Cohesion = compliant[id] * 100 / total[id]
		}
		for i := range g.Members {
			if g.Members[i].Expressed > 0 {
				g.Members[i].Group = g.NameShort
			}
		}
	}
	return rows.Err()
}

var _ = fmt.Sprint

type PartyDeclared struct {
	Name     string
	Deputies int
	Period   string
}

// loadPartiesDeclared établit la composition OBSERVÉE d'un groupe à partir des
// mandats de parti que la source publie pour ses membres.
//
// Limite majeure, affichée sur la page : l'Assemblée n'a publié aucune
// appartenance partisane pour la 17e législature — tous les mandats de parti
// s'arrêtent au 10 juin 2024, jour de la dissolution. Ce que montre cette
// composition est donc l'appartenance déclarée LORS DE LA LÉGISLATURE
// PRÉCÉDENTE par des députés qui siègent aujourd'hui dans ce groupe. C'est une
// indication, pas la composition actuelle, et le dire est la seule façon de ne
// pas induire en erreur.
// loadPartiesDeclared part de la liste (organisation, personne) de mv.
// scrutin_vote_nominal, DISTINCT — plus le JOIN sur la totalité de
// core.ballot que cette fonction refaisait à chaque construction pour
// obtenir seulement cette liste d'appartenances, avant de la croiser avec
// core.affiliation (une table de quelques milliers de lignes).
func loadPartiesDeclared(ctx context.Context, pool *pgxpool.Pool, byID map[int64]*Group) error {
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
		var p PartyDeclared
		var start, end string
		if err := rows.Scan(&id, &p.Name, &p.Deputies, &start, &end); err != nil {
			return err
		}
		g, ok := byID[id]
		if !ok || p.Deputies < 2 {
			continue // un député isolé ne caractérise pas un groupe
		}
		p.Period = period(start, end)
		if len(g.PartiesDeclared) < 10 {
			g.PartiesDeclared = append(g.PartiesDeclared, p)
		}
	}
	return rows.Err()
}
