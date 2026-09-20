package sitegen

import (
	"context"
	"sort"

	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les thèmes : deux taxonomies officielles, posées sur deux objets différents.
//
//	Sénat    30 thèmes, posés sur la LOI      -> atteignent les scrutins de
//	                                             l'Assemblée par la navette
//	EuroVoc  1 808 concepts, posés sur le SCRUTIN du Parlement européen
//
// Aucune des deux n'est produite par ce site : les poser nous-mêmes serait un
// jugement éditorial, et la taxonomie maison v1 compte d'ailleurs 0 assignation.
//
// Le paradoxe à assumer tel quel : les thèmes du Sénat ne s'appliquent PAS aux
// scrutins du Sénat, faute de référence de texte dans le dump Dosleg.
type Theme struct {
	Code, Slug, Label string
	Scrutins          int
	Groupes           []GroupeLigne
	Derniers          []FluxLigne
	// Ce que le thème dit des candidats de 2027 : leur vote personnel sur les
	// scrutins qui s'y rattachent. C'est un décompte, pas une position — un
	// vote contre peut viser le véhicule, le calendrier ou un texte concurrent.
	Candidats []VoteThemeCandidat
}

type VoteThemeCandidat struct {
	Slug, Nom, Organisation          string
	Pour, Contre, Abstention, Absent int
	Exprimes                         int
}

type StatsThemes struct {
	Themes          []*Theme
	TotalSenat      int
	MaxScrutins     int
	ScrutinsAN      int
	ScrutinsPE      int
	TotalConcepts   int
	CHESPartis      int
	CHESGroupesLies int
	CHESJoignables  int
}

func loadThemes(ctx context.Context, pool *pgxpool.Pool, maxParTheme int,
	candidatSlugs []string, orgParSlug map[string]string) (*StatsThemes, error) {
	st := &StatsThemes{}
	byCode := map[string]*Theme{}

	rows, err := pool.Query(ctx, `
		SELECT r.code, r.label, count(DISTINCT t.scrutin_id)
		FROM derived.scrutin_topic t
		JOIN ref.topic r ON r.code = t.topic_code
		WHERE r.taxonomy_version = 'senat'
		GROUP BY 1,2 ORDER BY 3 DESC, 2`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t := &Theme{}
		if err := rows.Scan(&t.Code, &t.Label, &t.Scrutins); err != nil {
			rows.Close()
			return nil, err
		}
		t.Slug = partis.Slugify(t.Label)
		byCode[t.Code] = t
		st.Themes = append(st.Themes, t)
		if t.Scrutins > st.MaxScrutins {
			st.MaxScrutins = t.Scrutins
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Ventilation par groupe, thème par thème. Descriptif seulement : aucun
	// classement, aucune comparaison, aucun score de proximité.
	//
	// mv.scrutin_groupe_vote (internal/matview) remplace le JOIN sur la
	// totalité de core.ballot : déjà un total par (scrutin, groupe,
	// position), sommer par thème ne rescanne jamais le fait brut.
	grows, err := pool.Query(ctx, `
		SELECT t.topic_code, mv.organisation_nom, mv.organisation_slug,
		       -- coalesce(...,0), pas sum() nu : voir groupBreakdown ou
		       -- loadScrutinsGroupe (groupes.go) pour la raison — sum() sur
		       -- un FILTER sans ligne rend NULL, jamais 0.
		       coalesce(sum(mv.n) FILTER (WHERE mv.position='FOR'), 0),
		       coalesce(sum(mv.n) FILTER (WHERE mv.position='AGAINST'), 0),
		       coalesce(sum(mv.n) FILTER (WHERE mv.position='ABSTAIN'), 0),
		       coalesce(sum(mv.n) FILTER (WHERE mv.position NOT IN ('FOR','AGAINST','ABSTAIN')), 0)
		FROM derived.scrutin_topic t
		JOIN ref.topic r ON r.code = t.topic_code AND r.taxonomy_version = 'senat'
		JOIN mv.scrutin_groupe_vote mv ON mv.scrutin_id = t.scrutin_id
		GROUP BY 1,2,3`)
	if err != nil {
		return nil, err
	}
	for grows.Next() {
		var code string
		var g GroupeLigne
		if err := grows.Scan(&code, &g.Nom, &g.Slug, &g.Pour, &g.Contre, &g.Abstention, &g.Absent); err != nil {
			grows.Close()
			return nil, err
		}
		g.Total = g.Pour + g.Contre + g.Abstention + g.Absent
		if t := byCode[code]; t != nil {
			t.Groupes = append(t.Groupes, g)
		}
	}
	grows.Close()
	if err := grows.Err(); err != nil {
		return nil, err
	}

	srows, err := pool.Query(ctx, `
		SELECT t.topic_code, s.slug, s.objet, to_char(s.date_seance,'DD/MM/YYYY'),
		       coalesce(s.resultat,''), coalesce(s.nb_pour,0), coalesce(s.nb_contre,0),
		       coalesce(s.nb_abstentions,0)
		FROM derived.scrutin_topic t
		JOIN ref.topic r ON r.code = t.topic_code AND r.taxonomy_version = 'senat'
		JOIN mv.scrutin s ON s.id = t.scrutin_id
		ORDER BY s.date_seance DESC, s.numero DESC`)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var code string
		var f FluxLigne
		if err := srows.Scan(&code, &f.Slug, &f.Objet, &f.Date, &f.Resultat,
			&f.Pour, &f.Contre, &f.Abstentions); err != nil {
			return nil, err
		}
		t := byCode[code]
		if t == nil || len(t.Derniers) >= maxParTheme {
			continue
		}
		f.Objet, _ = TitreCourt(f.Objet)
		f.Exprimes = f.Pour + f.Contre + f.Abstentions
		f.Resultat = ResultatLong(
			map[string]string{"ADOPTE": "adopté", "REJETE": "rejeté", "": ""}[f.Resultat], "")
		t.Derniers = append(t.Derniers, f)
	}

	// Vote des candidats déclarés à 2027, thème par thème — mv.
	// scrutin_vote_nominal (internal/matview) remplace le JOIN
	// ballot/person sur la totalité de core.ballot.
	crows, err := pool.Query(ctx, `
		SELECT t.topic_code, mv.person_slug, mv.person_given_name || ' ' || mv.person_family_name,
		       count(*) FILTER (WHERE mv.position='FOR'),
		       count(*) FILTER (WHERE mv.position='AGAINST'),
		       count(*) FILTER (WHERE mv.position='ABSTAIN'),
		       count(*) FILTER (WHERE mv.position NOT IN ('FOR','AGAINST','ABSTAIN'))
		FROM derived.scrutin_topic t
		JOIN ref.topic r ON r.code=t.topic_code AND r.taxonomy_version='senat'
		JOIN mv.scrutin_vote_nominal mv ON mv.scrutin_id=t.scrutin_id
		WHERE mv.person_slug = ANY($1)
		GROUP BY 1,2,3`, candidatSlugs)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var code string
		var v VoteThemeCandidat
		if err := crows.Scan(&code, &v.Slug, &v.Nom, &v.Pour, &v.Contre,
			&v.Abstention, &v.Absent); err != nil {
			crows.Close()
			return nil, err
		}
		v.Exprimes = v.Pour + v.Contre + v.Abstention
		if t := byCode[code]; t != nil && v.Exprimes > 0 {
			v.Organisation = orgParSlug[v.Slug]
			t.Candidats = append(t.Candidats, v)
		}
	}
	crows.Close()

	for _, t := range st.Themes {
		sort.Slice(t.Candidats, func(i, j int) bool {
			return CleTri(t.Candidats[i].Nom) < CleTri(t.Candidats[j].Nom)
		})
		sort.Slice(t.Groupes, func(i, j int) bool {
			if t.Groupes[i].Total != t.Groupes[j].Total {
				return t.Groupes[i].Total > t.Groupes[j].Total
			}
			return t.Groupes[i].Nom < t.Groupes[j].Nom
		})
	}

	// Ce que le pont CHES permettrait, et ce qui manque pour le franchir.
	_ = pool.QueryRow(ctx, `
		SELECT (SELECT count(DISTINCT party_id) FROM core.party_classification),
		       (SELECT count(*) FROM core.party_group_link),
		       (SELECT count(*) FROM (SELECT party_id FROM core.party_classification
		          INTERSECT SELECT party_id FROM core.party_group_link) x)`).
		Scan(&st.CHESPartis, &st.CHESGroupesLies, &st.CHESJoignables)

	// 30 thèmes existent au Sénat ; tous n'atteignent pas un scrutin de
	// l'Assemblée. Afficher « les 28 thèmes du Sénat » serait faux.
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM ref.topic WHERE taxonomy_version='senat'`).
		Scan(&st.TotalSenat)

	_ = pool.QueryRow(ctx, `
		SELECT (SELECT count(DISTINCT t.scrutin_id) FROM derived.scrutin_topic t
		         JOIN mv.scrutin s ON s.id=t.scrutin_id
		         WHERE s.institution='ASSEMBLEE_NATIONALE'),
		       (SELECT count(DISTINCT t.scrutin_id) FROM derived.scrutin_topic t
		         JOIN mv.scrutin s ON s.id=t.scrutin_id
		         WHERE s.institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(*) FROM ref.topic WHERE taxonomy_version='eurovoc')`).
		Scan(&st.ScrutinsAN, &st.ScrutinsPE, &st.TotalConcepts)

	return st, nil
}
