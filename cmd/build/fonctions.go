package main

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Une page par fonction de la dépense publique (COFOG) : les dix lignes du
// tableau « sur 1 000 € de dépense publique » de l'accueil. Chaque ligne y
// mène désormais (D-073) : trente ans de la même dépense, sa place parmi les
// neuf autres, et les sujets de campagne qui en relèvent.

type LigneComparaison struct {
	Libelle     string
	Milliards   float64
	Largeur     float64
	ParMille    int
	URL         string
	EstCourante bool
}

type PageFonction struct {
	Code, Nom, Detail string
	Milliards         float64
	ParMille          int
	Rang              int
	Sur               int
	Annee             int
	Serie             []PointAnnee
	Courbe            template.HTML
	Famille           *Famille
	Comparaison       []LigneComparaison
	ComparaisonHTML   template.HTML
}

func chargerFonctions(ctx context.Context, pool *pgxpool.Pool, a *DonneesAccueil) (map[string]*PageFonction, error) {
	pages := map[string]*PageFonction{}

	rangs := make([]FonctionCofog, len(a.Fonctions))
	copy(rangs, a.Fonctions)
	sort.SliceStable(rangs, func(i, j int) bool { return rangs[i].Milliards > rangs[j].Milliards })

	for _, f := range a.Fonctions {
		p := &PageFonction{
			Code: f.Code, Nom: f.Libelle, Detail: f.Detail,
			Milliards: f.Milliards, ParMille: f.ParMille, Sur: len(a.Fonctions),
			Annee: a.Annee, Famille: f.Famille,
		}
		for r, g := range rangs {
			if g.Code == f.Code {
				p.Rang = r + 1
			}
		}
		for _, g := range a.Fonctions {
			p.Comparaison = append(p.Comparaison, LigneComparaison{
				Libelle: g.Libelle, Milliards: g.Milliards, Largeur: g.Largeur,
				ParMille: g.ParMille, URL: g.URL(), EstCourante: g.Code == f.Code,
			})
		}
		pages[f.Code] = p
	}

	rows, err := pool.Query(ctx, `
		SELECT serie_code, annee, valeur::float8 FROM core.macro_value
		WHERE serie_code ~ '^depense\.GF[0-9]{2}$' ORDER BY serie_code, annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var annee int
		var meur float64
		if err := rows.Scan(&code, &annee, &meur); err != nil {
			return nil, err
		}
		gf := code[len("depense."):]
		if p := pages[gf]; p != nil {
			p.Serie = append(p.Serie, PointAnnee{Annee: annee, Valeur: meur / 1000})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, p := range pages {
		p.Courbe = courbe(p.Serie, func(v float64) string { return Decimal(v, 1) + " Md€" })
		p.ComparaisonHTML = barresComparaison(p.Comparaison)
	}
	return pages, nil
}

// barresComparaison : les dix fonctions, une barre chacune, celle de la page
// courante mise en évidence — même principe que barresNiveaux (collectivites.go),
// à l'échelle d'une seule année plutôt que d'un empilement de niveaux.
func barresComparaison(lignes []LigneComparaison) template.HTML {
	var b strings.Builder
	for _, l := range lignes {
		tag, attrs := "a", ` href="`+template.HTMLEscapeString(l.URL)+`"`
		cl := "ligne-mille"
		if l.EstCourante {
			tag, attrs, cl = "span", ` aria-current="page"`, "ligne-mille en-evidence"
		}
		fmt.Fprintf(&b, `<%s class="%s"%s><span class="l">%s</span>`+
			`<span class="b" aria-hidden="true"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%d&#8239;€</span></%s>`,
			tag, cl, attrs, template.HTMLEscapeString(l.Libelle),
			l.Largeur, l.ParMille, tag)
	}
	return template.HTML(b.String())
}
