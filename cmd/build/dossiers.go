package main

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
}

var kindFr = map[string]string{
	"PROJET_DE_LOI":             "Projet de loi",
	"PROPOSITION_DE_LOI":        "Proposition de loi",
	"PROPOSITION_DE_RESOLUTION": "Proposition de résolution",
}

// loadDossiers charge, pour chaque scrutin qui en référence un, le dossier
// législatif et les textes qui le composent.
//
// L'open data de l'Assemblée ne publie PAS les exposés des motifs : seuls les
// titres, les auteurs, les dates et les étapes y figurent. Ce bloc dit donc ce
// qui a été déposé et par qui — ce qui est un fait — et ne prétend pas résumer
// le contenu du texte, ce qui demanderait une rédaction humaine relue.
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
