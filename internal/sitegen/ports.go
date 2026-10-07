package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// portsSuivis : les quatre grands ports maritimes du continent les plus
// commentés dans le débat sur la compétitivité portuaire française — pas
// les 42 ports de la table, qui produiraient un graphique illisible.
var portsTracked = []string{"HAROPA", "MARSEILLE", "DUNKERQUE", "NANTES SAINT-NAZAIRE"}

type pointPort struct {
	Year      int
	TonnageMt float64
}

type seriesPort struct {
	Name   string
	Points []pointPort
}

// loadPortsFrench : le trafic total (Entrée + Sortie, tous types de
// marchandises) des quatre grands ports suivis, 2000-2025 (SDES).
func loadPortsFrench(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT port, annee, sum(tonnage_tot)::float8 FROM core.trafic_portuaire
		WHERE port = ANY($1) GROUP BY port, annee ORDER BY port, annee`, portsTracked)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	series := map[string]*seriesPort{}
	for rows.Next() {
		var name string
		var year int
		var tonnage float64
		if err := rows.Scan(&name, &year, &tonnage); err != nil {
			return "", err
		}
		s, ok := series[name]
		if !ok {
			s = &seriesPort{Name: name}
			series[name] = s
		}
		s.Points = append(s.Points, pointPort{year, tonnage / 1e6})
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(series) == 0 {
		return "", nil
	}
	var all []*seriesPort
	for _, name := range portsTracked {
		if s, ok := series[name]; ok {
			all = append(all, s)
		}
	}
	return drawPortsFrench(all), nil
}

// labelPort : des noms courts pour l'étiquette du graphique (la
// clarification HAROPA/Le Havre-Rouen est déjà faite dans le texte du
// dossier, § 1) — un nom trop long recoupait la marge droite du graphique,
// repéré à l'écran avant publication.
func labelPort(name string) string {
	switch name {
	case "NANTES SAINT-NAZAIRE":
		return "Nantes-St-Nazaire"
	case "HAROPA":
		return "HAROPA"
	case "MARSEILLE":
		return "Marseille"
	case "DUNKERQUE":
		return "Dunkerque"
	default:
		return name
	}
}

var classPort = map[string]string{
	"HAROPA": "haropa", "MARSEILLE": "marseille",
	"DUNKERQUE": "dunkerque", "NANTES SAINT-NAZAIRE": "nantes",
}

func drawPortsFrench(series []*seriesPort) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 320.0, 34.0, 170.0, 14.0, 26.0
	const yearStart, yearEnd = 2000, 2025
	maxVal := 0.0
	for _, s := range series {
		for _, p := range s.Points {
			if p.TonnageMt > maxVal {
				maxVal = p.TonnageMt
			}
		}
	}
	maxVal *= 1.08
	x := func(year int) float64 { return ml + (w-ml-mr)*float64(year-yearStart)/float64(yearEnd-yearStart) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxVal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe ports-fr" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Trafic total, quatre grands ports français, %d-%d">`, w, h, yearStart, yearEnd)
	for _, bracket := range []float64{0, 25, 50, 75, 100} {
		if bracket > maxVal {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(bracket), w-mr, y(bracket))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d Mt</text>`, ml-6, y(bracket)+3, int(bracket))
	}
	for _, s := range series {
		cl := classPort[s.Name]
		var coords []string
		for _, p := range s.Points {
			coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Year), y(p.TonnageMt)))
		}
		fmt.Fprintf(&b, `<polyline class="ligne-%s" points="%s"/>`, cl, strings.Join(coords, " "))
		last := s.Points[len(s.Points)-1]
		fmt.Fprintf(&b, `<circle class="pt-%s" cx="%.1f" cy="%.1f" r="2.6"/>`, cl, x(last.Year), y(last.TonnageMt))
		fmt.Fprintf(&b, `<text class="lbl-%s" x="%.1f" y="%.1f">%s (%s Mt)</text>`,
			cl, x(last.Year)+6, y(last.TonnageMt)+3, template.HTMLEscapeString(labelPort(s.Name)), Decimal(last.TonnageMt, 0))
	}
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, yearStart)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, x(yearEnd), h-8, yearEnd)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointPortEurope struct {
	Label, Country string
	Year           int
	TonnageMt      float64
}

// loadPortsEurope : le dernier tonnage disponible par port (l'année
// diffère selon le port — Anvers s'arrête en 2021 dans Eurostat — chaque
// barre porte sa propre année plutôt qu'une année commune inventée).
func loadPortsEurope(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (code_port) port_label, pays_code, annee, tonnage_milliers
		FROM core.trafic_portuaire_europe
		WHERE code_port != 'FR_1FRLEH'
		ORDER BY code_port, annee DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []pointPortEurope
	for rows.Next() {
		var p pointPortEurope
		var thousands float64
		if err := rows.Scan(&p.Label, &p.Country, &p.Year, &thousands); err != nil {
			return "", err
		}
		p.TonnageMt = thousands / 1000
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].TonnageMt > pts[j].TonnageMt })
	return drawPortsEurope(pts), nil
}

func drawPortsEurope(pts []pointPortEurope) template.HTML {
	const w, mr, ml, widthBar, gap = 720.0, 90.0, 190.0, 26.0, 14.0
	h := float64(len(pts))*(widthBar+gap) + gap
	widthAxis := w - ml - mr
	max := pts[0].TonnageMt

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="barres-ports-europe" role="img" `+
		`aria-label="Trafic portuaire total, six ports, France et rang nord-européen">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(widthBar+gap)
		width := widthAxis * p.TonnageMt / max
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+widthBar/2+4, template.HTMLEscapeString(p.Label))
		class := "autre"
		if p.Country == "FR" {
			class = "fr"
		}
		fmt.Fprintf(&b, `<rect class="barre-%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f"><title>%s, %d : %s Mt</title></rect>`,
			class, ml, y, width, widthBar, template.HTMLEscapeString(p.Label), p.Year, Decimal(p.TonnageMt, 0))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">%s Mt (%d)</text>`,
			ml+width+8, y+widthBar/2+4, Decimal(p.TonnageMt, 0), p.Year)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
