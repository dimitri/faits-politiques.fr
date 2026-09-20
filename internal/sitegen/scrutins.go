package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

type Scrutin struct {
	ID        int64
	DossierID *int64
	Dossier   *Dossier
	TexteID   *int64
	// Expose : l'exposé des motifs du texte sur lequel porte ce scrutin, quand
	// l'Assemblée en publie un — voir internal/an/exposes.go. Ce n'est pas un
	// résumé neutre : l'auteur y défend son texte, et il est cité comme tel.
	Expose      *ExposeMotif
	Institution string
	EstEuropeen bool
	EstSenat    bool
	// InstitutionNom et Chambre évitent le piège du booléen binaire : tant que
	// le gabarit ne connaissait que « européen ou non », les 4 764 scrutins du
	// Sénat s'affichaient sous « Assemblée nationale, 17e législature ».
	InstitutionNom string
	Chambre        string

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

// ExposeMotif : l'exposé des motifs d'un texte, verbatim. Chapeau est un
// extrait (les premiers paragraphes jusqu'à une fin de phrase) ; Integral est
// le texte complet, affiché derrière un <details> pour ne pas noyer la page.
type ExposeMotif struct {
	Chapeau, Integral, URL string
	NCaracteres            int
}

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
		SELECT id, dossier_id, texte_id, slug, coalesce(numero,''), objet, to_char(date_seance,'DD/MM/YYYY'),
		       institution::text,
		       coalesce(type_vote,''), coalesce(resultat,''), source_uid,
		       coalesce(nb_pour,0), coalesce(nb_contre,0), coalesce(nb_abstentions,0)
		FROM mv.scrutin ORDER BY date_seance DESC, numero DESC LIMIT `+limit)
	if err != nil {
		return 0, err
	}
	var all []Scrutin
	for rows.Next() {
		var s Scrutin
		if err := rows.Scan(&s.ID, &s.DossierID, &s.TexteID, &s.Slug, &s.Numero, &s.Objet, &s.Date, &s.Institution, &s.TypeVote,
			&s.Resultat, &s.SourceUID, &s.Pour, &s.Contre, &s.Abstentions); err != nil {
			return 0, err
		}
		s.Resultat = map[string]string{
			"ADOPTE": "adopté", "REJETE": "rejeté", "": "non publié",
		}[s.Resultat]
		s.EstEuropeen = s.Institution == "PARLEMENT_EUROPEEN"
		s.EstSenat = s.Institution == "SENAT"
		switch s.Institution {
		case "PARLEMENT_EUROPEEN":
			s.InstitutionNom, s.Chambre = "Parlement européen", "europe"
		case "SENAT":
			s.InstitutionNom, s.Chambre = "Sénat", "senat"
		default:
			s.InstitutionNom, s.Chambre = "Assemblée nationale", "assemblee"
		}
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

	exposes, err := chargerExposes(ctx, pool)
	if err != nil {
		return 0, err
	}
	for i := range all {
		if all[i].DossierID != nil {
			all[i].Expose = exposes[*all[i].DossierID]
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
	//
	// Écrit en parallèle, borné au nombre de cœurs : à partir d'ici, chaque
	// itération ne lit plus que des données déjà rassemblées en mémoire
	// (all, groupes, votes) et n'écrit que son propre fichier — rien de
	// partagé entre deux scrutins. C'est le rendu du gabarit et la passe de
	// ponctuation française (write, dans main.go) qui dominent le temps de
	// cette section à 38 000 pages, pas une attente réseau ; les répartir sur
	// plusieurs cœurs raccourcit directement le mur d'horloge, là où une
	// requête SQL supplémentaire par page ne l'aurait fait qu'à condition
	// d'attendre le réseau plutôt que le CPU.
	g := new(errgroup.Group)
	g.SetLimit(runtime.NumCPU())
	for i, s := range all {
		g.Go(func() error {
			gs := groupes[s.ID]
			maxG := 0
			for _, gr := range gs {
				if gr.Total > maxG {
					maxG = gr.Total
				}
				s.NonVotants += gr.Absent
			}
			// « Sans position enregistrée » n'est calculé que lorsqu'une base
			// certaine existe (data/seuils.csv). Ailleurs, l'effectif de
			// référence n'est pas une donnée : on ne le devine pas.
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
			return write(tpl, filepath.Join(out, "scrutin", s.Slug, "index.html"), data)
		})
	}
	if err := g.Wait(); err != nil {
		return 0, err
	}
	return len(all), nil
}

// chargerExposes charge un exposé des motifs par DOSSIER, pas par texte : un
// scrutin porte le texte_id de la LECTURE sur laquelle il vote, alors que
// l'exposé n'est publié que sur le texte du DÉPÔT initial — deux texte_id
// différents du même dossier. Sans ce détour, aucun scrutin ne rejoint
// jamais son exposé (vérifié : 0 correspondance directe par texte_id, 1830
// par dossier_id). Quand un dossier a déposé plusieurs textes avec exposé
// (rare — texte retiré puis redéposé), le plus ancien est gardé : c'est la
// première intention déclarée, avant qu'un texte ne soit retravaillé.
func chargerExposes(ctx context.Context, pool *pgxpool.Pool) (map[int64]*ExposeMotif, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (t.dossier_id)
		       t.dossier_id, e.chapeau, e.integral, e.url, e.n_caracteres
		FROM core.texte_expose e
		JOIN core.texte t ON t.id = e.texte_id
		ORDER BY t.dossier_id, t.date_depot NULLS LAST, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*ExposeMotif{}
	for rows.Next() {
		var id int64
		var e ExposeMotif
		if err := rows.Scan(&id, &e.Chapeau, &e.Integral, &e.URL, &e.NCaracteres); err != nil {
			return nil, err
		}
		out[id] = &e
	}
	return out, rows.Err()
}

// groupBreakdown lit mv.scrutin_groupe_vote (internal/matview) — un SELECT
// à plat, plus le GROUP BY sur la totalité de core.ballot (4,9 millions de
// lignes) que cette fonction refaisait à chaque construction alors que
// core.ballot ne change qu'à l'ingestion. La matvue se rafraîchit via
// « fpctl ingest systeme matviews » (ou la chaîne complète, RunTout), pas
// ici : internal/sitegen ne fait jamais de REFRESH, seulement des SELECT — même
// principe que core.section_checksum pour le cache de construction.
func groupBreakdown(ctx context.Context, pool *pgxpool.Pool, wanted map[int64]bool) (map[int64][]GroupeLigne, error) {
	rows, err := pool.Query(ctx, `
		SELECT scrutin_id, organisation_nom, organisation_slug, position, n
		FROM mv.scrutin_groupe_vote`)
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

// nominalVotes lit mv.scrutin_vote_nominal (internal/matview) — plus le JOIN
// à trois tables et le tri sur la totalité de core.ballot que cette
// fonction refaisait à chaque construction. Voir groupBreakdown ci-dessus
// pour le principe (mv ne se rafraîchit jamais depuis internal/sitegen).
func nominalVotes(ctx context.Context, pool *pgxpool.Pool, wanted map[int64]bool) (map[int64][]VoteLigne, error) {
	rows, err := pool.Query(ctx, `
		SELECT scrutin_id, person_slug, person_family_name, person_given_name,
		       organisation_nom, organisation_slug, position, rectifiee
		FROM mv.scrutin_vote_nominal
		ORDER BY scrutin_id, person_family_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64][]VoteLigne{}
	for rows.Next() {
		var sid int64
		var v VoteLigne
		var familyName, givenName string
		if err := rows.Scan(&sid, &v.Slug, &familyName, &givenName, &v.Groupe, &v.GroupeSlug, &v.Position, &v.Rectifiee); err != nil {
			return nil, err
		}
		v.Nom = familyName + ", " + givenName
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
		FROM mv.scrutin
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
		FROM mv.scrutin
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
