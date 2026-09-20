package sitegen

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TexteLigne struct {
	Kind, Titre, DateDepot string
	Auteurs                []string
	Gouvernement           bool
}

type Initiateur struct{ Nom, Slug string }

type Dossier struct {
	Slug, Titre     string
	URLAN, URLSenat string
	Textes          []TexteLigne
	Initiateurs     []Initiateur
	Gouvernement    bool
	// Promulgation : la loi qui est sortie de ce dossier, par égalité EXACTE de
	// référence NOR entre le flux de l'Assemblée et le Journal officiel — voir
	// internal/an/promulgation.go. Nil si ce dossier n'a jamais été promulgué
	// (rejeté, retiré, encore en discussion).
	Promulgation *Promulgation
}

// Promulgation : ce qu'une loi devient au Journal officiel. TexteURL est
// renseigné seulement quand le texte a été retrouvé dans le corpus JORF
// chargé — voir la colonne jo_texte_id, nullable par construction.
type Promulgation struct {
	CodeLoi, DateJO, DatePromulgation, URLLegifrance string
	TexteURL                                         string
}

var kindFr = map[string]string{
	"PROJET_DE_LOI":             "Projet de loi",
	"PROPOSITION_DE_LOI":        "Proposition de loi",
	"PROPOSITION_DE_RESOLUTION": "Proposition de résolution",
}

// loadDossiers charge, pour chaque scrutin qui en référence un, le dossier
// législatif et les textes qui le composent.
//
// L'open data de l'Assemblée ne publie pas nativement les exposés des motifs
// dans ce flux — ils viennent d'un point d'accès séparé (internal/an/exposes.go,
// core.texte_expose, chargé sur internal/sitegen/scrutins.go). Ce bloc-ci dit ce qui
// a été déposé, par qui, et — via Promulgation — ce qu'il est devenu au
// Journal officiel : trois faits, jamais un résumé du contenu rédigé ici.
func loadDossiers(ctx context.Context, pool *pgxpool.Pool) (map[int64]*Dossier, error) {
	dossiers := map[int64]*Dossier{} // indexé par core.dossier.id
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT d.id, d.slug, d.titre,
		       coalesce(d.titre_chemin,''), coalesce(d.senat_chemin,'')
		FROM core.dossier d
		WHERE EXISTS (SELECT 1 FROM core.scrutin s WHERE s.dossier_id = d.id)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		d := &Dossier{}
		var id int64
		var cheminAN, cheminSenat string
		if err := rows.Scan(&id, &d.Slug, &d.Titre, &cheminAN, &cheminSenat); err != nil {
			return nil, err
		}
		if cheminAN != "" {
			d.URLAN = "https://www.assemblee-nationale.fr/dyn/17/dossiers/" + cheminAN
		}
		// senatChemin est tantôt un segment, tantôt une URL complète : la
		// source mélange les deux formes pour le même champ.
		switch {
		case strings.HasPrefix(cheminSenat, "http"):
			d.URLSenat = cheminSenat
		case cheminSenat != "":
			d.URLSenat = "https://www.senat.fr/dossier-legislatif/" + cheminSenat + ".html"
		}
		dossiers[id] = d
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT t.dossier_id, t.id, t.kind, t.titre,
		       coalesce(to_char(t.date_depot,'DD/MM/YYYY'),'')
		FROM core.texte t
		WHERE t.dossier_id = ANY($1)
		ORDER BY t.dossier_id, t.date_depot NULLS LAST`, keys(dossiers))
	if err != nil {
		return nil, err
	}
	texteIdx := map[int64]*TexteLigne{}
	for rows.Next() {
		var did, tid int64
		var t TexteLigne
		if err := rows.Scan(&did, &tid, &t.Kind, &t.Titre, &t.DateDepot); err != nil {
			return nil, err
		}
		if d, ok := dossiers[did]; ok {
			if fr, ok := kindFr[t.Kind]; ok {
				t.Kind = fr
			}
			d.Textes = append(d.Textes, t)
			texteIdx[tid] = &d.Textes[len(d.Textes)-1]
		}
	}
	rows.Close()

	// L'initiateur est publié au niveau du DOSSIER, pas du document : le champ
	// « auteurs » d'une proposition de loi ne contient que l'organe
	// « Assemblée nationale ». On lit donc la source là où elle met l'information.
	rows, err = pool.Query(ctx, `
		SELECT a.dossier_id, a.role,
		       coalesce(p.given_name || ' ' || p.family_name, o.name, ''),
		       coalesce(p.slug, '')
		FROM core.dossier_author a
		LEFT JOIN core.person p ON p.id = a.person_id
		LEFT JOIN core.organization o ON o.id = a.organization_id
		WHERE a.dossier_id = ANY($1)
		ORDER BY a.dossier_id, a.rang NULLS LAST, a.id`, keys(dossiers))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var did int64
		var role, nom, slug string
		if err := rows.Scan(&did, &role, &nom, &slug); err != nil {
			return nil, err
		}
		d, ok := dossiers[did]
		if !ok || nom == "" {
			continue
		}
		if role == "GOUVERNEMENT" {
			d.Gouvernement = true
			continue
		}
		if len(d.Initiateurs) < 12 {
			d.Initiateurs = append(d.Initiateurs, Initiateur{Nom: nom, Slug: slug})
		}
	}
	rows.Close()

	// « Qui propose » est une dimension distincte de « qui vote » : c'est
	// l'origine politique documentée du texte, transcrite telle que publiée.
	rows, err = pool.Query(ctx, `
		SELECT a.texte_id, a.role,
		       coalesce(p.given_name || ' ' || p.family_name, o.name, '')
		FROM core.texte_author a
		LEFT JOIN core.person p ON p.id = a.person_id
		LEFT JOIN core.organization o ON o.id = a.organization_id
		WHERE a.texte_id = ANY($1)
		ORDER BY a.texte_id, a.rang NULLS LAST, a.id`, keysOf(texteIdx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tid int64
		var role, nom string
		if err := rows.Scan(&tid, &role, &nom); err != nil {
			return nil, err
		}
		t, ok := texteIdx[tid]
		if !ok || nom == "" {
			continue
		}
		if role == "GOUVERNEMENT" {
			t.Gouvernement = true
			continue
		}
		if len(t.Auteurs) < 8 {
			t.Auteurs = append(t.Auteurs, nom)
		}
	}

	// La loi promulguée, quand ce dossier en a produit une — par égalité EXACTE
	// de référence NOR (core.dossier_promulgation), jamais un rapprochement de
	// titres. jo_texte_id peut être NULL : le dossier porte sa propre référence
	// même quand le texte promulgué n'est pas (encore) dans le corpus JORF chargé.
	prows, err := pool.Query(ctx, `
		SELECT dp.dossier_id, dp.code_loi,
		       coalesce(to_char(dp.date_jo,'DD/MM/YYYY'),''),
		       coalesce(to_char(dp.date_promulgation,'DD/MM/YYYY'),''),
		       dp.jo_texte_id
		FROM core.dossier_promulgation dp WHERE dp.dossier_id = ANY($1)`, keys(dossiers))
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var did int64
		var p Promulgation
		var joID *string
		if err := prows.Scan(&did, &p.CodeLoi, &p.DateJO, &p.DatePromulgation, &joID); err != nil {
			return nil, err
		}
		if joID != nil {
			p.TexteURL = "https://www.legifrance.gouv.fr/jorf/id/" + *joID
		}
		if d, ok := dossiers[did]; ok {
			d.Promulgation = &p
		}
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}
	return dossiers, nil
}

func keys(m map[int64]*Dossier) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOf(m map[int64]*TexteLigne) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
