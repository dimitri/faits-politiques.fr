package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
	"os"
	"sort"
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
	BasLabel, HautLabel      string
	// Spark : une forme au « min → max » déjà écrit sur la carte — le texte dit
	// où ça commence et où ça finit, la mini-courbe dit COMMENT on y est allé
	// (une marche brutale, une pente régulière, un plateau).
	Spark template.HTML
}

type President struct {
	Nom              string
	Debut, Fin       int
	Y0, Y1           int
	SansAucunChiffre bool
}

// ComparaisonChomage : la question « la baisse du chômage est-elle vraie ? »
// ne se tranche pas, mais elle se documente avec la mesure la plus large
// qu'Eurostat publie du manque de travail — les « capacités excédentaires sur
// le marché du travail » (labour market slack), quatre catégories qui se
// somment exactement, sur le même champ, la même enquête, la même année que
// le chômage BIT. C'est la comparaison qu'aucune des sources n'a besoin d'être
// recalculée pour tenir : Eurostat publie déjà la somme.
type ComparaisonChomage struct {
	BIT2015, BIT2022, BITDernier float64
	AnDernierBIT                 int
	RSA2016, RSA2020, RSADernier float64
	AnDernierRSA                 int
	// Le halo, année par année, depuis que la série existe (2014).
	AnneesSlack                    []int
	SlackDebut                     int
	SlackFin                       int
	UNEDebut, UNEFin               float64
	SlackTotalDebut, SlackTotalFin float64
	BarresSlack                    template.HTML
}

type StatsFrise struct {
	Chomage            ComparaisonChomage
	Presidents         []President
	Series             []SerieFrise
	An0, An1           int
	Hauteur            int
	Decennies          []int
	DelinquanceGroupes []GroupeDelinquance
}

// MiniDelinquance : un indicateur de délinquance, en petit — sa propre
// échelle, jamais celle des quatorze autres, pour qu'un indicateur qui recule
// ne s'écrase pas visuellement contre un autre qui explose.
type MiniDelinquance struct {
	Libelle              string
	Debut, Fin           int
	DebutLabel, FinLabel string
	Spark                template.HTML
}

// GroupeDelinquance : les indicateurs qui comptent la MÊME chose — des
// victimes, des mis en cause, des véhicules, des infractions — jamais
// mélangés dans un total, contrairement à la ‰ unique de la carte
// « Délinquance enregistrée » ci-dessus, dont c'est justement la limite.
type GroupeDelinquance struct {
	UniteDeCompte string
	Couleur       string
	Indicateurs   []MiniDelinquance
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
	_ = pool.QueryRow(ctx, `
		SELECT max(valeur) FILTER (WHERE serie_code='chomeurs.nombre' AND annee=2015)::float8,
		       max(valeur) FILTER (WHERE serie_code='chomeurs.nombre' AND annee=2022)::float8,
		       max(annee) FILTER (WHERE serie_code='chomeurs.nombre'),
		       max(valeur) FILTER (WHERE serie_code='rsa.foyers' AND annee=2016)::float8,
		       max(valeur) FILTER (WHERE serie_code='rsa.foyers' AND annee=2020)::float8,
		       max(annee) FILTER (WHERE serie_code='rsa.foyers')
		FROM core.macro_value WHERE serie_code IN ('chomeurs.nombre','rsa.foyers')`).
		Scan(&st.Chomage.BIT2015, &st.Chomage.BIT2022, &st.Chomage.AnDernierBIT,
			&st.Chomage.RSA2016, &st.Chomage.RSA2020, &st.Chomage.AnDernierRSA)
	_ = pool.QueryRow(ctx, `
		SELECT max(valeur) FILTER (WHERE serie_code='chomeurs.nombre' AND annee=$1)::float8,
		       max(valeur) FILTER (WHERE serie_code='rsa.foyers' AND annee=$2)::float8
		FROM core.macro_value`, st.Chomage.AnDernierBIT, st.Chomage.AnDernierRSA).
		Scan(&st.Chomage.BITDernier, &st.Chomage.RSADernier)

	// Les quatre composantes du « labour market slack », année par année.
	srows, err := pool.Query(ctx, `
		SELECT annee, serie_code, valeur::float8 FROM core.macro_value
		WHERE serie_code IN ('chomeurs.nombre','chomage.sous_emploi_temps_partiel',
		                      'chomage.cherchent_indisponibles','chomage.disponibles_sans_recherche')
		ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	brut := map[int]map[string]float64{}
	for srows.Next() {
		var an int
		var code string
		var v float64
		if err := srows.Scan(&an, &code, &v); err != nil {
			srows.Close()
			return nil, err
		}
		if brut[an] == nil {
			brut[an] = map[string]float64{}
		}
		brut[an][code] = v
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}
	// Seules les années où les QUATRE composantes existent entrent dans le
	// graphique empilé : une barre qui n'additionnerait que trois catégories
	// sur quatre sous-estimerait le total sans le dire.
	for an := range brut {
		c := brut[an]
		if _, ok := c["chomeurs.nombre"]; !ok {
			continue
		}
		if _, ok := c["chomage.sous_emploi_temps_partiel"]; !ok {
			continue
		}
		if _, ok := c["chomage.cherchent_indisponibles"]; !ok {
			continue
		}
		if _, ok := c["chomage.disponibles_sans_recherche"]; !ok {
			continue
		}
		st.Chomage.AnneesSlack = append(st.Chomage.AnneesSlack, an)
	}
	sort.Ints(st.Chomage.AnneesSlack)
	if n := len(st.Chomage.AnneesSlack); n > 0 {
		st.Chomage.SlackDebut, st.Chomage.SlackFin = st.Chomage.AnneesSlack[0], st.Chomage.AnneesSlack[n-1]
		d, f := brut[st.Chomage.SlackDebut], brut[st.Chomage.SlackFin]
		st.Chomage.UNEDebut, st.Chomage.UNEFin = d["chomeurs.nombre"], f["chomeurs.nombre"]
		sommeAnnee := func(m map[string]float64) float64 {
			return m["chomeurs.nombre"] + m["chomage.sous_emploi_temps_partiel"] +
				m["chomage.cherchent_indisponibles"] + m["chomage.disponibles_sans_recherche"]
		}
		st.Chomage.SlackTotalDebut, st.Chomage.SlackTotalFin = sommeAnnee(d), sommeAnnee(f)

		milliers := func(v float64) string { return Decimal(v/1000, 1) + "\u202fM" }
		serieVal := func(code string) map[int]float64 {
			m := map[int]float64{}
			for _, an := range st.Chomage.AnneesSlack {
				m[an] = brut[an][code]
			}
			return m
		}
		// Ordre FIXE, du plus proche du marché du travail au plus éloigné :
		// chômage BIT d'abord (déjà connu du lecteur), puis les trois
		// catégories qu'il ne voit jamais nommées ailleurs.
		st.Chomage.BarresSlack = barresEmpileesAnnuelles(st.Chomage.AnneesSlack, []SerieEmpilee{
			{Libelle: "Chômeurs au sens du BIT", Couleur: "#1E5C69", Valeurs: serieVal("chomeurs.nombre")},
			{Libelle: "Sous-employés à temps partiel", Couleur: "#4A8894", Valeurs: serieVal("chomage.sous_emploi_temps_partiel")},
			{Libelle: "Cherchent un emploi, indisponibles", Couleur: "#8AB6BD", Valeurs: serieVal("chomage.cherchent_indisponibles")},
			{Libelle: "Disponibles, ne cherchent pas (halo)", Couleur: "#C9DDE0", Valeurs: serieVal("chomage.disponibles_sans_recherche")},
		}, milliers)
	}
	pres, err := loadPresidents(dataDir + "/presidents.csv")
	if err != nil {
		return nil, err
	}
	st.Presidents = pres

	def := []struct{ code, titre, unite, note string }{
		{"dette.publique.pib", "Dette publique", "% du PIB", ""},
		{"solde.public.pib", "Solde public", "% du PIB", "Négatif = déficit."},
		{"chomeurs.nombre", "Chômeurs", "milliers", "Au sens du BIT (Eurostat) — ni les inscrits à France Travail, ni les allocataires du RSA."},
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
		// L'échelle est celle des valeurs de la série, PAS zéro-max.
		//
		// Le nombre de personnes sous le seuil de pauvreté oscille entre 8,2 et
		// 9,8 millions : cadré sur zéro, cela donnait un trait vertical où l'on
		// ne voyait rigoureusement rien bouger. Un axe qui ne part pas de zéro
		// exagère les variations si on le tait ; les deux bornes sont donc
		// écrites en tête de colonne, et la note le dit.
		if s.Max == s.Min {
			s.Max = s.Min + 1
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
			s.Dernier = valeurFrise(v)
		}
		s.BasLabel, s.HautLabel = valeurFrise(s.Min), valeurFrise(s.Max)
		var pts []PointAnnee
		for a := s.Debut; a <= s.Fin; a++ {
			if v, ok := s.Points[a]; ok {
				pts = append(pts, PointAnnee{Annee: a, Valeur: v})
			}
		}
		s.Spark = sparkline(pts, "")
	}
	for a := 1960; a <= 2020; a += 10 {
		st.Decennies = append(st.Decennies, a)
	}

	st.DelinquanceGroupes, err = delinquanceParGroupe(ctx, pool)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func valeurFrise(v float64) string {
	if v >= 1000 || v <= -1000 {
		return Nombre(int(v))
	}
	return Decimal(v, 1)
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

// couleurDelinquance : une teinte fixe par unité de compte — jamais par
// indicateur (quinze, ce serait une rampe recalculée) ni par valeur. Cinq
// unités seulement dans ref.indicateur_delinquance : la couleur est un fait
// du référentiel, pas un choix de rendu.
var couleurDelinquance = map[string]string{
	"Victime":          "#1E5C69",
	"Victime entendue": "#3D6FA0",
	"Mis en cause":     "#A34F86",
	"Véhicule":         "#B0763A",
	"Infraction":       "#6B5CA5",
}

// delinquanceParGroupe : quinze indicateurs, chacun sur SA propre échelle,
// groupés par ce qu'ils comptent réellement — jamais mélangés dans un total
// comme le fait la carte « Délinquance enregistrée » ci-dessus, dont c'est
// justement la limite (voir docs/budget-donnees.md et la note de la carte).
func delinquanceParGroupe(ctx context.Context, pool *pgxpool.Pool) ([]GroupeDelinquance, error) {
	rows, err := pool.Query(ctx, `
		SELECT i.unite_de_compte, i.libelle, d.annee,
		       1000.0*sum(d.nombre)/nullif(sum(d.population),0)
		FROM core.commune_delinquance d
		JOIN ref.indicateur_delinquance i ON i.code=d.indicateur_code
		WHERE d.diffuse
		GROUP BY i.unite_de_compte, i.libelle, d.annee
		ORDER BY i.unite_de_compte, i.libelle, d.annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type cle struct{ unite, libelle string }
	pts := map[cle][]PointAnnee{}
	var ordre []cle
	vu := map[cle]bool{}
	for rows.Next() {
		var unite, libelle string
		var annee int
		var v *float64
		if err := rows.Scan(&unite, &libelle, &annee, &v); err != nil {
			return nil, err
		}
		if v == nil {
			continue
		}
		c := cle{unite, libelle}
		if !vu[c] {
			vu[c] = true
			ordre = append(ordre, c)
		}
		pts[c] = append(pts[c], PointAnnee{Annee: annee, Valeur: *v})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groupes := map[string]*GroupeDelinquance{}
	var ordreGroupes []string
	for _, c := range ordre {
		p := pts[c]
		if len(p) < 2 {
			continue
		}
		g := groupes[c.unite]
		if g == nil {
			g = &GroupeDelinquance{UniteDeCompte: c.unite, Couleur: couleurDelinquance[c.unite]}
			groupes[c.unite] = g
			ordreGroupes = append(ordreGroupes, c.unite)
		}
		min, max := p[0].Valeur, p[0].Valeur
		for _, pt := range p {
			if pt.Valeur < min {
				min = pt.Valeur
			}
			if pt.Valeur > max {
				max = pt.Valeur
			}
		}
		g.Indicateurs = append(g.Indicateurs, MiniDelinquance{
			Libelle: c.libelle, Debut: p[0].Annee, Fin: p[len(p)-1].Annee,
			DebutLabel: Decimal(min, 1), FinLabel: Decimal(max, 1),
			Spark: sparkline(p, g.Couleur),
		})
	}
	sort.Strings(ordreGroupes)
	out := make([]GroupeDelinquance, 0, len(ordreGroupes))
	for _, nom := range ordreGroupes {
		out = append(out, *groupes[nom])
	}
	return out, nil
}
