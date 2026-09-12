package main

import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Scrutin struct {
	ID          int64
	DossierID   *int64
	Dossier     *Dossier
	Institution string
	EstEuropeen bool

	Slug, Numero, Objet, Date, TypeVote string
	Resultat, SourceUID                 string
	Pour, Contre, Abstentions           int

	// TitreCourt est le libellé source rendu lisible comme un titre, sans
	// réécriture : coupé avant les signataires, première lettre en capitale.
	// Tronque dit si l'opération a eu lieu, auquel cas la page affiche le
	// libellé officiel intégral juste en dessous.
	TitreCourt   string
	Tronque      bool
	ResultatLong string

	// Seuil : le nombre de voix requis, quand une règle s'applique à ce type
	// de scrutin (data/seuils.csv). Zéro signifie « pas de seuil à base fixe »,
	// et la page retombe sur une barre proportionnelle aux exprimés.
	Seuil                              int
	Base                               int
	SeuilRegle, SeuilNote, SeuilSource string

	Exprimes     int
	NonVotants   int
	SansPosition int
}

// ScrutinLien : de quoi naviguer de proche en proche. Une fiche isolée oblige
// à repasser par une liste pour lire le scrutin suivant.
type ScrutinLien struct{ Slug, Objet string }

type GroupeLigne struct {
	Nom, Slug                        string
	Pour, Contre, Abstention, Absent int
	PctPour, PctContre, PctAbst      int
	// Total : l'effectif recensé du groupe sur CE scrutin. Il sert d'échelle
	// absolue aux barres — une barre en pourcentage des exprimés occupe toute
	// la largeur pour tous les groupes, et fait lire 4 voix comme 72.
	Total int
}

type VoteLigne struct {
	Slug, Nom, Groupe, GroupeSlug, Position, PositionFr string
	Rectifiee                                           bool
}

// buildScrutins génère une fiche par scrutin. Le groupe de chaque votant est
// celui que la SOURCE a publié avec ce scrutin : c'est une transcription du
// relevé, pas une reconstitution à partir des mandats — les fichiers de mandats
// publiés par l'Assemblée ne portent pas les groupes de la 17e législature.
func buildScrutins(ctx context.Context, pool *pgxpool.Pool, tpl *template.Template,
	layout Layout, out string, max int, seuils map[string]Seuil, src SourceInfo) (int, error) {

	limit := "ALL"
	if max > 0 {
		limit = fmt.Sprint(max)
	}
	rows, err := pool.Query(ctx, `
		SELECT id, dossier_id, slug, coalesce(numero,''), objet, to_char(date_seance,'DD/MM/YYYY'),
		       institution::text,
		       coalesce(type_vote,''), coalesce(resultat,''), source_uid,
		       coalesce(nb_pour,0), coalesce(nb_contre,0), coalesce(nb_abstentions,0)
		FROM core.scrutin ORDER BY date_seance DESC, numero DESC LIMIT `+limit)
	if err != nil {
		return 0, err
	}
	var all []Scrutin
	for rows.Next() {
		var s Scrutin
		if err := rows.Scan(&s.ID, &s.DossierID, &s.Slug, &s.Numero, &s.Objet, &s.Date, &s.Institution, &s.TypeVote,
			&s.Resultat, &s.SourceUID, &s.Pour, &s.Contre, &s.Abstentions); err != nil {
			return 0, err
		}
		s.Resultat = map[string]string{
			"ADOPTE": "adopté", "REJETE": "rejeté", "": "non publié",
		}[s.Resultat]
		s.EstEuropeen = s.Institution == "PARLEMENT_EUROPEEN"
		s.TitreCourt, s.Tronque = TitreCourt(s.Objet)
		s.ResultatLong = ResultatLong(s.Resultat, s.TypeVote)
		s.Exprimes = s.Pour + s.Contre + s.Abstentions
		if sl, ok := seuils[s.TypeVote]; ok {
			s.Seuil, s.Base = sl.Voix, sl.Base
			s.SeuilRegle, s.SeuilNote, s.SeuilSource = sl.Regle, sl.Note, sl.Source
		}
		all = append(all, s)
	}
	rows.Close()

	wanted := map[int64]bool{}
	for _, s := range all {
		wanted[s.ID] = true
	}

	dossiers, err := loadDossiers(ctx, pool)
	if err != nil {
		return 0, err
	}
	for i := range all {
		if all[i].DossierID != nil {
			all[i].Dossier = dossiers[*all[i].DossierID]
		}
	}

	groupes, err := groupBreakdown(ctx, pool, wanted)
	if err != nil {
		return 0, err
	}
	votes, err := nominalVotes(ctx, pool, wanted)
	if err != nil {
		return 0, err
	}

	// all est trié par date décroissante : le « précédent » chronologique est
	// donc l'élément suivant dans la tranche.
	for i, s := range all {
		gs := groupes[s.ID]
		maxG := 0
		for _, g := range gs {
			if g.Total > maxG {
				maxG = g.Total
			}
			s.NonVotants += g.Absent
		}
		// « Sans position enregistrée » n'est calculé que lorsqu'une base
		// certaine existe (data/seuils.csv). Ailleurs, l'effectif de référence
		// n'est pas une donnée : on ne le devine pas.
		if s.Base > 0 {
			if n := s.Base - len(votes[s.ID]); n > 0 {
				s.SansPosition = n
			}
		}

		var prec, suiv *ScrutinLien
		if i+1 < len(all) {
			prec = &ScrutinLien{all[i+1].Slug, all[i+1].TitreCourt}
		}
		if i > 0 {
			suiv = &ScrutinLien{all[i-1].Slug, all[i-1].TitreCourt}
		}

		l := layout
		l.Title = "Scrutin n° " + s.Numero
		data := struct {
			Layout
			S          Scrutin
			Groupes    []GroupeLigne
			Votes      []VoteLigne
			MaxGroupe  int
			Prec, Suiv *ScrutinLien
			Src        SourceInfo
		}{l, s, gs, votes[s.ID], maxG, prec, suiv, src}
		if err := write(tpl, filepath.Join(out, "scrutin", s.Slug, "index.html"), data); err != nil {
			return 0, err
		}
	}
	return len(all), nil
}

func groupBreakdown(ctx context.Context, pool *pgxpool.Pool, wanted map[int64]bool) (map[int64][]GroupeLigne, error) {
	rows, err := pool.Query(ctx, `
		SELECT b.scrutin_id, coalesce(o.short_name, o.name), o.slug,
		       coalesce(b.position_rectifiee, b.position)::text, count(*)
		FROM core.ballot b
		JOIN core.organization o ON o.id = b.organization_id
		GROUP BY 1,2,3,4`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	acc := map[int64]map[string]*GroupeLigne{}
	for rows.Next() {
		var sid int64
		var nom, slug, pos string
		var n int
		if err := rows.Scan(&sid, &nom, &slug, &pos, &n); err != nil {
			return nil, err
		}
		if !wanted[sid] {
			continue
		}
		if acc[sid] == nil {
			acc[sid] = map[string]*GroupeLigne{}
		}
		g := acc[sid][nom]
		if g == nil {
			g = &GroupeLigne{Nom: nom, Slug: slug}
			acc[sid][nom] = g
		}
		switch pos {
		case "FOR":
			g.Pour += n
		case "AGAINST":
			g.Contre += n
		case "ABSTAIN":
			g.Abstention += n
		default:
			g.Absent += n
		}
	}

	out := map[int64][]GroupeLigne{}
	for sid, m := range acc {
		var list []GroupeLigne
		for _, g := range m {
			// Le dénominateur est le nombre de POSITIONS EXPRIMÉES : les absents
			// sont exclus. Les inclure classerait les groupes par assiduité.
			if e := g.Pour + g.Contre + g.Abstention; e > 0 {
				g.PctPour = g.Pour * 100 / e
				g.PctContre = g.Contre * 100 / e
				g.PctAbst = 100 - g.PctPour - g.PctContre
			}
			g.Total = g.Pour + g.Contre + g.Abstention + g.Absent
			list = append(list, *g)
		}
		sort.Slice(list, func(i, j int) bool {
			a := list[i].Pour + list[i].Contre + list[i].Abstention + list[i].Absent
			b := list[j].Pour + list[j].Contre + list[j].Abstention + list[j].Absent
			if a != b {
				return a > b
			}
			return list[i].Nom < list[j].Nom
		})
		out[sid] = list
	}
	return out, nil
}

func nominalVotes(ctx context.Context, pool *pgxpool.Pool, wanted map[int64]bool) (map[int64][]VoteLigne, error) {
	rows, err := pool.Query(ctx, `
		SELECT b.scrutin_id, p.slug, p.family_name || ', ' || p.given_name,
		       coalesce(o.short_name, o.name, ''), coalesce(o.slug,''),
		       coalesce(b.position_rectifiee, b.position)::text,
		       b.position_rectifiee IS NOT NULL
		FROM core.ballot b
		JOIN core.scrutin s ON s.id = b.scrutin_id
		JOIN core.person p ON p.id = b.person_id
		LEFT JOIN core.organization o ON o.id = b.organization_id
		ORDER BY b.scrutin_id, p.family_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64][]VoteLigne{}
	for rows.Next() {
		var sid int64
		var v VoteLigne
		if err := rows.Scan(&sid, &v.Slug, &v.Nom, &v.Groupe, &v.GroupeSlug, &v.Position, &v.Rectifiee); err != nil {
			return nil, err
		}
		if !wanted[sid] {
			continue
		}
		v.PositionFr = positionFr[v.Position]
		out[sid] = append(out[sid], v)
	}
	return out, rows.Err()
}

var _ = strings.TrimSpace

// derniersScrutins alimente la page Assemblée. La liste est bornée et le total
// affiché à côté : une troncature invisible laisserait croire à une sélection.
func derniersScrutins(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Vote, error) {
	rows, err := pool.Query(ctx, `
		SELECT slug, objet, to_char(date_seance,'DD/MM/YYYY'), coalesce(resultat,'')
		FROM core.scrutin
		ORDER BY date_seance DESC, numero DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Vote
	for rows.Next() {
		var v Vote
		if err := rows.Scan(&v.Slug, &v.Objet, &v.Date, &v.Resultat); err != nil {
			return nil, err
		}
		v.Objet, _ = TitreCourt(v.Objet)
		v.Resultat = map[string]string{"ADOPTE": "adopté", "REJETE": "rejeté", "": "non publié"}[v.Resultat]
		out = append(out, v)
	}
	return out, rows.Err()
}

// FluxLigne alimente le flux d'accueil. « Ce qui a été voté cette semaine » est
// la première question d'un soir de débat, et le site n'y répondait nulle part.
type FluxLigne struct {
	Slug, Objet, Date, TypeVote, Resultat string
	Pour, Contre, Abstentions, Exprimes   int
}

func derniersFlux(ctx context.Context, pool *pgxpool.Pool, limit int) ([]FluxLigne, error) {
	rows, err := pool.Query(ctx, `
		SELECT slug, objet, to_char(date_seance,'DD/MM/YYYY'), coalesce(type_vote,''),
		       coalesce(resultat,''), coalesce(nb_pour,0), coalesce(nb_contre,0),
		       coalesce(nb_abstentions,0)
		FROM core.scrutin
		WHERE institution = 'ASSEMBLEE_NATIONALE'
		ORDER BY date_seance DESC, numero DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FluxLigne
	for rows.Next() {
		var f FluxLigne
		if err := rows.Scan(&f.Slug, &f.Objet, &f.Date, &f.TypeVote, &f.Resultat,
			&f.Pour, &f.Contre, &f.Abstentions); err != nil {
			return nil, err
		}
		f.Objet, _ = TitreCourt(f.Objet)
		f.Resultat = ResultatLong(
			map[string]string{"ADOPTE": "adopté", "REJETE": "rejeté", "": "non publié"}[f.Resultat],
			f.TypeVote)
		f.Exprimes = f.Pour + f.Contre + f.Abstentions
		out = append(out, f)
	}
	return out, rows.Err()
}
