package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La frise de la Ve République : des présidents, et des chiffres datés.
//
// Un repère chronologique n'est PAS une affirmation de causalité. Dire « en
// 2010, la dette valait tant » sous une bande « Nicolas Sarkozy » est de
// l'histoire avec des nombres ; dire « Sarkozy a fait la dette » serait une
// opinion. Trois choix de dessin tiennent la distinction :
//
//   - les bandes ne sont jamais colorées par parti ;
//   - aucun titre de la forme « bilan de X » ;
//   - les courbes traversent les alternances SANS rupture — ce qui se voit à
//     l'œil est justement qu'il ne se passe rien de particulier au changement.
//
// La contrainte dure est ailleurs : les séries commencent bien après 1958.
// Les comptes publics démarrent en 1995, le chômage BIT en 2003, la sécurité en
// 2016. Seules les entreprises remontent à 1971. La frise part quand même de
// 1958 et MONTRE le bord : la République est plus vieille que ses statistiques.
type SerieFrise struct {
	Code, Titre, Unite, Note string
	Debut, Fin               int
	Points                   map[int]float64
	Min, Max                 float64
	Path                     template.HTML
	Dernier                  string
}

type President struct {
	Nom              string
	Debut, Fin       int
	Y0, Y1           int
	SansAucunChiffre bool
}

type StatsFrise struct {
	Presidents []President
	Series     []SerieFrise
	An0, An1   int
	Hauteur    int
	Decennies  []int
}

func loadPresidents(path string) ([]President, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []President
	for i, c := range rows {
		if i == 0 || len(c) < 4 {
			continue
		}
		// Les intérims de quelques semaines ne portent pas de bande : ils
		// seraient illisibles et ne situent rien.
		if !strings.HasPrefix(c[3], "Président de la République") {
			continue
		}
		d, _ := strconv.Atoi(c[0][:4])
		fin := 2026
		if len(c[1]) >= 4 {
			fin, _ = strconv.Atoi(c[1][:4])
		}
		out = append(out, President{Nom: c[2], Debut: d, Fin: fin})
	}
	return out, nil
}

func loadFrise(ctx context.Context, pool *pgxpool.Pool, dataDir string) (*StatsFrise, error) {
	st := &StatsFrise{An0: 1958, An1: 2026}
	pres, err := loadPresidents(dataDir + "/presidents.csv")
	if err != nil {
		return nil, err
	}
	st.Presidents = pres

	def := []struct{ code, titre, unite, note string }{
		{"dette.publique.pib", "Dette publique", "% du PIB", ""},
		{"solde.public.pib", "Solde public", "% du PIB", "Négatif = déficit."},
		{"chomeurs.nombre", "Chômeurs", "milliers", "Au sens du BIT, pas les inscrits à France Travail."},
		{"pauvrete.nombre", "Sous le seuil de pauvreté", "milliers", "Seuil relatif : il bouge avec le niveau de vie médian."},
		{"__ratio_div", "Dividendes versés", "% de l'EBE", "Rapportés au profit brut, faute de déflateur pour comparer des euros de 1971 et de 2024."},
		{"__securite", "Délinquance enregistrée", "faits ‰ hab.", "Faits ENREGISTRÉS, pas commis : une hausse peut venir des faits, des plaintes, ou de l'enregistrement."},
	}

	for _, d := range def {
		s := SerieFrise{Code: d.code, Titre: d.titre, Unite: d.unite, Note: d.note,
			Points: map[int]float64{}}
		var rows interface {
			Next() bool
			Scan(...any) error
			Close()
			Err() error
		}
		var qerr error
		switch d.code {
		case "__ratio_div":
			rows, qerr = pool.Query(ctx, `
				SELECT d.annee, 100.0*d.valeur/nullif(e.valeur,0)
				FROM core.macro_value d
				JOIN core.macro_value e ON e.annee=d.annee AND e.serie_code='ebe.snf'
				WHERE d.serie_code='dividendes.verses.snf' ORDER BY 1`)
		case "__securite":
			rows, qerr = pool.Query(ctx, `
				SELECT annee, 1000.0*sum(nombre)/nullif(sum(population),0)
				FROM core.commune_delinquance WHERE diffuse GROUP BY 1 ORDER BY 1`)
		default:
			rows, qerr = pool.Query(ctx,
				`SELECT annee, valeur FROM core.macro_value WHERE serie_code=$1 ORDER BY 1`, d.code)
		}
		if qerr != nil {
			return nil, qerr
		}
		first := true
		for rows.Next() {
			var a int
			var v *float64
			if err := rows.Scan(&a, &v); err != nil {
				rows.Close()
				return nil, err
			}
			if v == nil {
				continue
			}
			s.Points[a] = *v
			if first || *v < s.Min {
				s.Min = *v
			}
			if first || *v > s.Max {
				s.Max = *v
			}
			if first {
				s.Debut = a
				first = false
			}
			s.Fin = a
		}
		rows.Close()
		if len(s.Points) == 0 {
			continue
		}
		if s.Min > 0 {
			s.Min = 0
		}
		st.Series = append(st.Series, s)
	}

	// Géométrie : une année vaut 9 pixels, la colonne 120.
	const py, cw = 9.0, 120.0
	st.Hauteur = (st.An1-st.An0)*9 + 8
	y := func(a int) float64 { return float64(a-st.An0) * py }
	yi := func(a int) int { return (a - st.An0) * 9 }
	for i := range st.Presidents {
		p := &st.Presidents[i]
		p.Y0, p.Y1 = yi(max(p.Debut, st.An0)), yi(min(p.Fin, st.An1))
		p.SansAucunChiffre = true
		for _, s := range st.Series {
			if s.Debut <= p.Fin && s.Fin >= p.Debut {
				p.SansAucunChiffre = false
			}
		}
	}
	for i := range st.Series {
		s := &st.Series[i]
		var b strings.Builder
		n := 0
		for a := s.Debut; a <= s.Fin; a++ {
			v, ok := s.Points[a]
			if !ok {
				continue
			}
			x := (v - s.Min) / (s.Max - s.Min) * cw
			if n == 0 {
				fmt.Fprintf(&b, "M%.1f,%.1f", x, y(a))
			} else {
				fmt.Fprintf(&b, "L%.1f,%.1f", x, y(a))
			}
			n++
		}
		s.Path = template.HTML(b.String())
		if v, ok := s.Points[s.Fin]; ok {
			if v >= 1000 {
				s.Dernier = Nombre(int(v))
			} else {
				s.Dernier = fmt.Sprintf("%.1f", v)
			}
		}
	}
	for a := 1960; a <= 2020; a += 10 {
		st.Decennies = append(st.Decennies, a)
	}
	return st, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
