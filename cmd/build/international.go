package main

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SIPRI publie la dépense militaire (% du PIB) pour dix pays depuis 1949 —
// chargée en entier (core.indicateur_mondial), mais le dossier n'en
// montrait qu'une seule année en tableau. L'Arabie saoudite dépasse les
// autres pays d'un facteur 2 à 4 sur une bonne partie de la série : la
// mettre sur la même échelle écraserait les huit autres courbes, d'où le
// choix de ne nommer que la France, la Russie et l'Arabie saoudite (les
// trois trajectoires les plus commentées) et de laisser les six autres en
// gris, individuellement survolables.
var paysColoresSIPRI = map[string]string{"FR": "fr", "RU": "ru", "SA": "sa"}

// libelleFrSIPRI : le classeur SIPRI nomme les pays en anglais ; le reste
// de ce dossier les nomme en français (tableaux voisins) — un même pays ne
// doit pas changer de langue d'une section à l'autre de la même page.
var libelleFrSIPRI = map[string]string{
	"FR": "France", "RU": "Russie", "SA": "Arabie saoudite", "US": "États-Unis",
	"GB": "Royaume-Uni", "DE": "Allemagne", "IT": "Italie", "CN": "Chine",
	"CA": "Canada", "JP": "Japon",
}

type pointSIPRI struct {
	Annee  int
	Valeur float64
}

type serieSIPRI struct {
	Code, Label string
	Points      []pointSIPRI
}

func chargerSIPRI(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT pays_code, pays_label, annee, valeur FROM core.indicateur_mondial
		WHERE indicateur='SIPRI_DEPENSE_MILITAIRE_PIB' ORDER BY pays_code, annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	series := map[string]*serieSIPRI{}
	var ordre []string
	for rows.Next() {
		var code, label string
		var annee int
		var valeur float64
		if err := rows.Scan(&code, &label, &annee, &valeur); err != nil {
			return "", err
		}
		s, ok := series[code]
		if !ok {
			if fr, ok := libelleFrSIPRI[code]; ok {
				label = fr
			}
			s = &serieSIPRI{Code: code, Label: label}
			series[code] = s
			ordre = append(ordre, code)
		}
		s.Points = append(s.Points, pointSIPRI{annee, valeur})
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(ordre) == 0 {
		return "", nil
	}
	sort.Strings(ordre)
	var toutes []*serieSIPRI
	for _, c := range ordre {
		toutes = append(toutes, series[c])
	}
	return dessinerSIPRI(toutes), nil
}

func dessinerSIPRI(series []*serieSIPRI) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 340.0, 34.0, 100.0, 14.0, 26.0
	const anneeDebut, anneeFin = 1949, 2025
	maxVal := 0.0
	for _, s := range series {
		for _, p := range s.Points {
			if p.Valeur > maxVal {
				maxVal = p.Valeur
			}
		}
	}
	maxVal = maxVal * 1.08
	x := func(annee int) float64 { return ml + (w-ml-mr)*float64(annee-anneeDebut)/float64(anneeFin-anneeDebut) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxVal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe sipri-milex" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Dépense militaire en %% du PIB, dix pays, %d à %d">`, w, h, anneeDebut, anneeFin)
	for _, palier := range []float64{0, 5, 10, 15, 20} {
		if palier > maxVal {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d%%</text>`, ml-6, y(palier)+3, int(palier))
	}

	// Les six pays non nommés d'abord (gris, sous les trois courbes vedettes).
	for _, s := range series {
		if _, nommee := paysColoresSIPRI[s.Code]; nommee {
			continue
		}
		tracerLigneSIPRI(&b, s, "autre", x, y, false)
	}
	for _, s := range series {
		cl, nommee := paysColoresSIPRI[s.Code]
		if !nommee {
			continue
		}
		tracerLigneSIPRI(&b, s, cl, x, y, true)
	}
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anneeDebut)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anneeFin)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func tracerLigneSIPRI(b *strings.Builder, s *serieSIPRI, classe string, x func(int) float64, y func(float64) float64, etiquette bool) {
	var coords []string
	for _, p := range s.Points {
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Annee), y(p.Valeur)))
	}
	fmt.Fprintf(b, `<polyline class="ligne-%s" points="%s"/>`, classe, strings.Join(coords, " "))
	dernier := s.Points[len(s.Points)-1]
	fmt.Fprintf(b, `<circle class="pt-%s" cx="%.1f" cy="%.1f" r="2.6"><title>%s, %d : %s %% du PIB</title></circle>`,
		classe, x(dernier.Annee), y(dernier.Valeur), template.HTMLEscapeString(s.Label), dernier.Annee, Decimal(dernier.Valeur, 1))
	if etiquette {
		fmt.Fprintf(b, `<text class="lbl-%s" x="%.1f" y="%.1f">%s</text>`,
			classe, x(dernier.Annee)+5, y(dernier.Valeur)+3, template.HTMLEscapeString(s.Label))
	}
}
