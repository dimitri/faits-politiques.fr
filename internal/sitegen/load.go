package sitegen

import (
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// loadPersons charge les personnes, leurs mandats, leurs appartenances et le
// décompte de leurs positions. ABSENT et NON_VOTING sont comptés à part : ce
// sont des données manquantes, pas des positions.
func loadPersons(ctx context.Context, pool *pgxpool.Pool, totalScrutins int) (map[string]*Person, error) {
	persons := map[string]*Person{}
	byID := map[int64]*Person{}

	// Seules les personnes ayant un mandat ou un vote DANS LE PÉRIMÈTRE COUVERT
	// reçoivent une fiche. Le fichier « tous acteurs » de l'Assemblée remonte à
	// plusieurs législatures : publier des milliers de fiches vides donnerait
	// l'illusion d'une couverture qui n'existe pas.
	//
	// Le filtre porte sur les mandats NATIONAUX. Depuis l'ingestion du RNE,
	// core.mandate compte 613 256 lignes, dont 508 788 conseillers municipaux :
	// un simple « a un mandat » produisait 514 410 fiches, presque toutes vides
	// de tout vote, et un site de 500 000 pages. Les élus locaux sont documentés
	// par la fiche de leur COMMUNE (docs/mairies-conception.md), pas par une
	// fiche personnelle sans contenu.
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT p.id, p.slug, p.given_name, p.family_name
		FROM core.person p
		WHERE EXISTS (SELECT 1 FROM core.ballot b WHERE b.person_id = p.id)
		   OR EXISTS (SELECT 1 FROM core.mandate m WHERE m.person_id = p.id
		               AND m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN',
		                                      'MINISTRE','PRESIDENT_REPUBLIQUE'))`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := &Person{}
		if err := rows.Scan(&p.ID, &p.Slug, &p.Prenom, &p.Nom); err != nil {
			return nil, err
		}
		persons[p.Slug] = p
		byID[p.ID] = p
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT person_id, mandate_type::text, coalesce(constituency,''),
		       coalesce(role,''), coalesce(portefeuille,''),
		       lower(validity)::text, coalesce(upper(validity)::text,''),
		       coalesce(commune_code,'')
		FROM core.mandate ORDER BY lower(validity) DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var m Mandat
		var debut, fin string
		if err := rows.Scan(&id, &m.Type, &m.Circo, &m.Role, &m.Portefeuille, &debut, &fin,
			&m.CommuneCode); err != nil {
			return nil, err
		}
		m.Periode = periode(debut, fin)
		m.DebutISO, m.FinISO = debut, fin
		if p, ok := byID[id]; ok {
			p.Mandats = append(p.Mandats, m)
			if p.Mandat == "" {
				p.Mandat = strings.ToLower(m.Type)
				if m.Type == "MINISTRE" && m.Role != "" {
					p.Mandat = m.Role
				}
				if m.Circo != "" {
					p.Mandat += " — " + m.Circo
				}
			}
		}
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT a.person_id, o.name, a.organization_kind::text, a.declared_via::text,
		       lower(a.validity)::text, coalesce(upper(a.validity)::text,''),
		       upper_inf(a.validity)
		FROM core.affiliation a JOIN core.organization o ON o.id = a.organization_id
		ORDER BY a.organization_kind, lower(a.validity) DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var a Affil
		var debut, fin string
		var courant bool
		if err := rows.Scan(&id, &a.Nom, &a.Kind, &a.Via, &debut, &fin, &courant); err != nil {
			return nil, err
		}
		a.Periode = periode(debut, fin)
		a.Kind = map[string]string{
			"PARLIAMENTARY_GROUP": "groupe parlementaire",
			"PARTY":               "parti",
		}[a.Kind]
		a.Via = map[string]string{
			"INSTITUTION":       "publié par l'Assemblée",
			"PARTY_DECLARATION": "déclaratif",
			"JO_RATTACHEMENT":   "rattachement publié au JO",
			"EP_DECLARATION":    "publié par le Parlement européen",
		}[a.Via]
		if p, ok := byID[id]; ok {
			p.Affiliations = append(p.Affiliations, a)
			_ = courant
		}
	}
	rows.Close()

	// Le groupe courant est celui du DERNIER vote enregistré : c'est une donnée
	// de relevé, datée et sourcée, là où les fichiers de mandats publiés ne
	// portent pas les groupes de la 17e législature. mv.scrutin_vote_nominal
	// (internal/matview) remplace le JOIN sur la totalité de core.ballot.
	rows, err = pool.Query(ctx, `
		SELECT DISTINCT ON (mv.person_id) mv.person_id, mv.organisation_nom
		FROM mv.scrutin_vote_nominal mv
		JOIN mv.scrutin s ON s.id = mv.scrutin_id
		WHERE mv.organization_id IS NOT NULL
		ORDER BY mv.person_id, s.date_seance DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var g string
		if err := rows.Scan(&id, &g); err != nil {
			return nil, err
		}
		if p, ok := byID[id]; ok {
			p.Groupe = g
		}
	}
	rows.Close()

	// PAS mv.scrutin_vote_nominal ici, à la différence des autres requêtes de
	// ce fichier : ce décompte veut la position D'ORIGINE (position, sans
	// coalesce avec position_rectifiee) — la matvue, elle, porte déjà la
	// correction (comme groupBreakdown/nominalVotes le veulent, eux). Les
	// deux sont des choix légitimes mais différents ; les confondre
	// changerait silencieusement le décompte de 1 612 bulletins rectifiés.
	rows, err = pool.Query(ctx, `
		SELECT person_id, position::text, count(*)
		FROM core.ballot GROUP BY 1,2`)
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
		p, ok := byID[id]
		if !ok {
			continue
		}
		switch pos {
		case "FOR":
			p.Pour = n
		case "AGAINST":
			p.Contre = n
		case "ABSTAIN":
			p.Abstention = n
		default:
			p.NonVotant += n
		}
	}
	rows.Close()

	for _, p := range persons {
		p.Exprimes = p.Pour + p.Contre + p.Abstention
		p.HasVotes = p.Exprimes+p.NonVotant > 0
		if totalScrutins > 0 {
			p.PctExprimes = p.Exprimes * 100 / totalScrutins
		}
		// Répartition des positions EXPRIMÉES : les absents en sont exclus,
		// faute de quoi la barre mesurerait l'assiduité.
		if p.Exprimes > 0 {
			p.PctPour = p.Pour * 100 / p.Exprimes
			p.PctContre = p.Contre * 100 / p.Exprimes
			p.PctAbst = 100 - p.PctPour - p.PctContre
		}
	}
	return persons, nil
}

// loadVotesBulk lit mv.person_dernier_vote (internal/matview) — plus le
// fenêtrage SQL sur la totalité de core.ballot/mv.scrutin que cette
// fonction refaisait à chaque construction (déjà en un seul aller-retour,
// pas une requête par personne, mais toujours un passage complet sur le
// fait brut). La matvue plafonne à 100 rangs ; limit (60 aujourd'hui) reste
// le tri final affiché.
func loadVotesBulk(ctx context.Context, pool *pgxpool.Pool, persons map[string]*Person, limit int) error {
	if limit > 100 {
		// mv.person_dernier_vote (db/migrations/0156) ne garde que les 100
		// premiers rangs par personne : au-delà, silencieusement tronquer
		// serait servir une page fausse. Une vraie limite plus haute
		// demande d'abord d'élargir la matvue (une nouvelle migration).
		return fmt.Errorf("loadVotesBulk : limite %d > 100, la matvue mv.person_dernier_vote n'en garde pas plus", limit)
	}
	var ids []int64
	byID := map[int64]*Person{}
	for _, p := range persons {
		if p.HasVotes {
			ids = append(ids, p.ID)
			byID[p.ID] = p
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := pool.Query(ctx, `
		SELECT person_id, scrutin_slug, objet, date_txt, position, rectifiee, resultat
		FROM mv.person_dernier_vote
		WHERE person_id = ANY($1) AND rang <= $2
		ORDER BY person_id, rang`, ids, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var v Vote
		if err := rows.Scan(&pid, &v.Slug, &v.Objet, &v.Date, &v.Position, &v.Rectifiee, &v.Resultat); err != nil {
			return err
		}
		v.PositionFr = positionFr[v.Position]
		// Même traitement que les titres de fiches : coupé avant les
		// signataires, première lettre en capitale. Aucun mot ajouté.
		v.Objet, _ = TitreCourt(v.Objet)
		v.Resultat = map[string]string{
			"ADOPTE": "adopté", "REJETE": "rejeté", "": "non publié",
		}[v.Resultat]
		byID[pid].Votes = append(byID[pid].Votes, v)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range byID {
		p.VotesShown = len(p.Votes)
	}
	return nil
}

// loadCandidats lit la décision éditoriale et la rapproche des personnes
// connues. Un candidat sans correspondance n'est pas une erreur : c'est le cas
// courant d'un maire, d'un sénateur ou d'un eurodéputé, et la fiche doit le dire.
func loadCandidats(path string, persons map[string]*Person) ([]*Candidat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : aucun candidat", path)
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		idx[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, k string) string {
		if i, ok := idx[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var out []*Candidat
	for _, rec := range recs[1:] {
		c := &Candidat{
			Slug: get(rec, "slug"), Nom: get(rec, "nom"), Prenom: get(rec, "prenom"),
			Organisation: get(rec, "organisation"), Statut: get(rec, "statut"),
			OrganisationSlug: get(rec, "organisation_slug"),
			DateDeclaration:  get(rec, "date_declaration"), SourceURL: get(rec, "source_url"),
			SourceConsultee: get(rec, "source_consultee"), SiteCampagne: get(rec, "site_campagne"),
		}
		if c.Slug == "" {
			continue
		}
		c.Person = persons[c.Slug] // rapprochement déterministe par slug
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return CleTri(out[i].Nom+" "+out[i].Prenom) < CleTri(out[j].Nom+" "+out[j].Prenom)
	})
	return out, nil
}

func periode(debut, fin string) string {
	if fin == "" {
		return "depuis le " + fr(debut)
	}
	return "du " + fr(debut) + " au " + fr(fin)
}

func fr(iso string) string {
	if len(iso) < 10 {
		return iso
	}
	return iso[8:10] + "/" + iso[5:7] + "/" + iso[0:4]
}

var _ = template.HTMLEscapeString
