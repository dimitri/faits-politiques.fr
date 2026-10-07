package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupEP struct {
	Slug, Name, NameShort               string
	Headcount, For, Against, Abstention int
}

type ThemeEuroVocabulary struct {
	Code, Label string
	Elections   int
}

type StatsEurope struct {
	Elections, Votes, Eurodeputes int
	Groups                        []GroupEP
	Last                          []Vote
	Themes                        []ThemeEuroVocabulary
	ThemesSVG                     template.HTML
	Eurodep                       []*Person
}

// drawThemesEuroVocabulary : les vingt thématiques EuroVoc les plus fréquentes
// parmi les scrutins chargés — le classement complet (jusqu'à 60 thèmes,
// seuil à 20 scrutins) reste disponible dans le tableau qui suit le
// graphique, celui-ci n'en montre que la tête pour rester lisible.
func drawThemesEuroVocabulary(themes []ThemeEuroVocabulary) template.HTML {
	if len(themes) == 0 {
		return ""
	}
	n := len(themes)
	if n > 20 {
		n = 20
	}
	max := themes[0].Elections
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="barres">`)
	for _, t := range themes[:n] {
		fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s</span></div>`,
			template.HTMLEscapeString(t.Label), 100*float64(t.Elections)/float64(max), Count(t.Elections))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// loadEurope charge le volet européen. Les votes du Parlement européen vivent
// dans les mêmes tables que ceux de l'Assemblée, distingués par leur
// institution — mais ils ne sont JAMAIS agrégés avec eux : ce sont deux espaces
// de vote distincts, non superposables.
// loadEurope lit mv.scrutin_vote_nominal (internal/matview) au lieu de
// core.ballot directement — le JOIN sur core.scrutin (institution) reste
// applicatif, mais porte sur une table de quelques dizaines de milliers de
// lignes, pas sur le fait 4,9 millions de lignes.
func loadEurope(ctx context.Context, pool *pgxpool.Pool) (*StatsEurope, error) {
	e := &StatsEurope{}
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM core.scrutin WHERE institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(*) FROM mv.scrutin_vote_nominal mv JOIN core.scrutin s ON s.id=mv.scrutin_id
		         WHERE s.institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(*) FROM core.person_identifier WHERE scheme='EP_MEP')`).
		Scan(&e.Elections, &e.Votes, &e.Eurodeputes); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT o.slug, o.name, coalesce(o.short_name,''),
		       count(DISTINCT mv.person_slug),
		       count(*) FILTER (WHERE mv.position='FOR'),
		       count(*) FILTER (WHERE mv.position='AGAINST'),
		       count(*) FILTER (WHERE mv.position='ABSTAIN')
		FROM mv.scrutin_vote_nominal mv
		JOIN core.scrutin s ON s.id = mv.scrutin_id AND s.institution='PARLEMENT_EUROPEEN'
		JOIN core.organization o ON o.id = mv.organization_id
		GROUP BY o.slug, o.name, o.short_name
		ORDER BY 4 DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g GroupEP
		if err := rows.Scan(&g.Slug, &g.Name, &g.NameShort, &g.Headcount,
			&g.For, &g.Against, &g.Abstention); err != nil {
			return nil, err
		}
		e.Groups = append(e.Groups, g)
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
		if err := rows.Scan(&v.Slug, &v.Object, &v.Date, &v.Result); err != nil {
			return nil, err
		}
		v.Object, _ = TitleShort(v.Object)
		e.Last = append(e.Last, v)
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
		var t ThemeEuroVocabulary
		if err := rows.Scan(&t.Code, &t.Label, &t.Elections); err != nil {
			return nil, err
		}
		e.Themes = append(e.Themes, t)
	}
	rows.Close()
	e.ThemesSVG = drawThemesEuroVocabulary(e.Themes)

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
		if err := rows.Scan(&p.Slug, &p.FirstName, &p.Name, &p.Group); err != nil {
			return nil, err
		}
		e.Eurodep = append(e.Eurodep, p)
	}
	return e, rows.Err()
}
