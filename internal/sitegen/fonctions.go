package sitegen

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

type LineComparison struct {
	Label       string
	Billions    float64
	Width       float64
	PerThousand int
	URL         string
	EstCurrent  bool
}

type PageFunction struct {
	Code, Name, Detail string
	Billions           float64
	PerThousand        int
	Rank               int
	On                 int
	Year               int
	Series             []PointYear
	Curve              template.HTML
	Family             *Family
	Comparison         []LineComparison
	ComparisonHTML     template.HTML
}

func loadFunctions(ctx context.Context, pool *pgxpool.Pool, a *DataHome) (map[string]*PageFunction, error) {
	pages := map[string]*PageFunction{}

	ranks := make([]FunctionCofog, len(a.Functions))
	copy(ranks, a.Functions)
	sort.SliceStable(ranks, func(i, j int) bool { return ranks[i].Billions > ranks[j].Billions })

	for _, f := range a.Functions {
		p := &PageFunction{
			Code: f.Code, Name: f.Label, Detail: f.Detail,
			Billions: f.Billions, PerThousand: f.PerThousand, On: len(a.Functions),
			Year: a.Year, Family: f.Family,
		}
		for r, g := range ranks {
			if g.Code == f.Code {
				p.Rank = r + 1
			}
		}
		for _, g := range a.Functions {
			p.Comparison = append(p.Comparison, LineComparison{
				Label: g.Label, Billions: g.Billions, Width: g.Width,
				PerThousand: g.PerThousand, URL: g.URL(), EstCurrent: g.Code == f.Code,
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
		var year int
		var meur float64
		if err := rows.Scan(&code, &year, &meur); err != nil {
			return nil, err
		}
		gf := code[len("depense."):]
		if p := pages[gf]; p != nil {
			p.Series = append(p.Series, PointYear{Year: year, Value: meur / 1000})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, p := range pages {
		p.Curve = curve(p.Series, func(v float64) string { return Decimal(v, 1) + " Md€" })
		p.ComparisonHTML = barsComparison(p.Comparison)
	}
	return pages, nil
}

// barsComparison : les dix fonctions, une barre chacune, celle de la page
// courante mise en évidence — même principe que barresNiveaux (collectivites.go),
// à l'échelle d'une seule année plutôt que d'un empilement de niveaux.
func barsComparison(lines []LineComparison) template.HTML {
	var b strings.Builder
	for _, l := range lines {
		tag, attrs := "a", ` href="`+template.HTMLEscapeString(l.URL)+`"`
		cl := "ligne-mille"
		if l.EstCurrent {
			tag, attrs, cl = "span", ` aria-current="page"`, "ligne-mille en-evidence"
		}
		fmt.Fprintf(&b, `<%s class="%s"%s><span class="l">%s</span>`+
			`<span class="b" aria-hidden="true"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%d&#8239;€</span></%s>`,
			tag, cl, attrs, template.HTMLEscapeString(l.Label),
			l.Width, l.PerThousand, tag)
	}
	return template.HTML(b.String())
}
