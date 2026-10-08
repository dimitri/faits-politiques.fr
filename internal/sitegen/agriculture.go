package sitegen

import (
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Agriculture et alimentation. Aucune carte : ce n'est pas un oubli.
//
// Les deux séries ingérées — usage des sols et bilan alimentaire — sont
// NATIONALES. Les poser sur une carte départementale supposerait de répartir un
// total national entre 96 départements, c'est-à-dire d'inventer la donnée
// manquante. La page le dit et montre les séries telles qu'elles existent.
type SeriesAgri struct {
	Code, Title, Unit, Note string
	Start, End              int
	Points                  []PointYear
	Curve                   template.HTML
	First, Last             string
	Variation               float64
}

type SharedProduct struct {
	Code, Label      string
	Human, Livestock float64
	ShareLivestock   float64
	Interior         float64
	ShareOutsideFood float64
	WithInterior     bool
}

type StatsAgri struct {
	Series         []SeriesAgri
	Shared         []SharedProduct
	YearSummary    int
	Kcal           float64
	KcalPlant      float64
	KcalAnimal     float64
	YearKcal       int
	CountProducts  int
	TotalHuman     float64
	TotalLivestock float64
	Yield          []PointYear
	CurveYield     template.HTML
	KcalHectare    float64
	FedPer1k       float64
	SurfaceHa      float64
	Autonomy       []LineAutonomy

	IncomeFrance []PointYear
	CurveIncome  template.HTML
	IncomeLastFR float64
	IncomeLastEu float64
	YearIncome   int
}

// Le taux d'auto-approvisionnement : production ÷ disponibilité intérieure.
// C'est une division entre deux colonnes publiées, pas un modèle. Elle ne dit
// PAS que le pays se nourrit lui-même : un taux de 176 % sur les céréales
// coexiste avec des importations, parce qu'on n'exporte pas et n'importe pas
// les mêmes qualités ni aux mêmes saisons.
type LineAutonomy struct {
	Label                string
	Production, Interior float64
	Rate                 float64
}

// thousands : les séries agricoles sont publiées en « 1000 ha » et « 1000 No ».
// On garde l'unité de la source plutôt que de multiplier par mille pour le
// plaisir d'aligner des zéros.
func thousands(v float64) string { return Count(int(v + 0.5)) }

func loadProductsFood(path string) ([][3]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out [][3]string
	for i, c := range rows {
		if i == 0 || len(c) < 3 {
			continue
		}
		out = append(out, [3]string{strings.TrimSpace(c[0]), strings.TrimSpace(c[1]), c[2]})
	}
	return out, nil
}

func loadAgriculture(ctx context.Context, pool *pgxpool.Pool, dataDir string) (*StatsAgri, error) {
	st := &StatsAgri{}

	def := []struct{ code, title, note string }{
		{"terres.agricoles", "Terres agricoles",
			"Surface agricole utilisée : cultures, prairies permanentes et cultures permanentes réunies."},
		{"terres.arables", "Terres arables",
			"Les terres labourées. Une baisse peut venir d'une urbanisation, d'un boisement ou d'un passage en prairie : la série ne dit pas lequel."},
		{"terres.prairies", "Prairies permanentes", ""},
		{"emploi.agricole", "Emploi agricole", "Personnes, pas équivalents temps plein."},
		{"emploi.agroalimentaire", "Emploi agroalimentaire",
			"Industrie de transformation, pas les exploitations."},
	}
	for _, d := range def {
		rows, err := pool.Query(ctx, `
			SELECT annee, valeur::float8, unite FROM core.agriculture_indicateur
			WHERE code=$1 ORDER BY annee`, d.code)
		if err != nil {
			return nil, err
		}
		s := SeriesAgri{Code: d.code, Title: d.title, Note: d.note}
		for rows.Next() {
			var p PointYear
			if err := rows.Scan(&p.Year, &p.Value, &s.Unit); err != nil {
				break
			}
			s.Points = append(s.Points, p)
		}
		rows.Close()
		if len(s.Points) < 2 {
			continue
		}
		s.Start, s.End = s.Points[0].Year, s.Points[len(s.Points)-1].Year
		s.First = thousands(s.Points[0].Value)
		s.Last = thousands(s.Points[len(s.Points)-1].Value)
		if s.Points[0].Value != 0 {
			s.Variation = 100 * (s.Points[len(s.Points)-1].Value - s.Points[0].Value) / s.Points[0].Value
		}
		s.Curve = curve(s.Points, thousands)
		st.Series = append(st.Series, s)
	}

	_ = pool.QueryRow(ctx, `SELECT max(annee) FROM core.bilan_alimentaire`).Scan(&st.YearSummary)
	_ = pool.QueryRow(ctx, `
		SELECT max(valeur) FILTER (WHERE produit_code='grand_total')::float8,
		       max(valeur) FILTER (WHERE produit_code='vegetal_products')::float8,
		       max(valeur) FILTER (WHERE produit_code='animal_products')::float8
		FROM core.bilan_alimentaire
		WHERE element='Food supply (kcal/capita/day)' AND annee=$1`,
		st.YearSummary).Scan(&st.Kcal, &st.KcalPlant, &st.KcalAnimal)
	st.YearKcal = st.YearSummary

	// Calories nourries à l'hectare : le seul indicateur de cette page qui
	// rapporte la production à la surface. Il vient d'une vue de la base, pas
	// d'un calcul fait ici.
	rrows, err := pool.Query(ctx, `
		SELECT annee, kcal_par_hectare_an::float8, surface_agricole_ha::float8,
		       habitants_nourris_par_10000_ha::float8
		FROM derived.calories_par_hectare ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for rrows.Next() {
		var p PointYear
		var surf, fed float64
		if err := rrows.Scan(&p.Year, &p.Value, &surf, &fed); err != nil {
			break
		}
		st.Yield = append(st.Yield, p)
		st.KcalHectare, st.SurfaceHa, st.FedPer1k = p.Value, surf, fed
	}
	rrows.Close()
	if len(st.Yield) > 1 {
		st.CurveYield = curve(st.Yield, func(v float64) string {
			return Decimal(v/1e6, 2) + " Mkcal"
		})
	}

	arows, err := pool.Query(ctx, `
		SELECT libelle, production::float8, disponibilite_interieure::float8, taux_pct::float8
		FROM derived.autonomie_alimentaire
		WHERE annee=$1 AND production>2000 ORDER BY production DESC`, st.YearSummary)
	if err != nil {
		return nil, err
	}
	for arows.Next() {
		var a LineAutonomy
		if err := arows.Scan(&a.Label, &a.Production, &a.Interior, &a.Rate); err != nil {
			break
		}
		st.Autonomy = append(st.Autonomy, a)
	}
	arows.Close()

	rrows2, err := pool.Query(ctx, `
		SELECT annee, euro_par_uta FROM core.revenu_agricole_reel
		WHERE geo_code='FR' ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for rrows2.Next() {
		var p PointYear
		if err := rrows2.Scan(&p.Year, &p.Value); err != nil {
			break
		}
		st.IncomeFrance = append(st.IncomeFrance, p)
	}
	rrows2.Close()
	if len(st.IncomeFrance) > 1 {
		st.CurveIncome = curve(st.IncomeFrance, func(v float64) string { return Count(int(v)) + " €" })
		last := st.IncomeFrance[len(st.IncomeFrance)-1]
		st.YearIncome = last.Year
		st.IncomeLastFR = last.Value
		_ = pool.QueryRow(ctx, `
			SELECT euro_par_uta FROM core.revenu_agricole_reel WHERE geo_code='EU27_2020' AND annee=$1`,
			st.YearIncome).Scan(&st.IncomeLastEu)
	}

	products, err := loadProductsFood(dataDir + "/produits-alimentaires.csv")
	if err != nil {
		return nil, err
	}
	st.CountProducts = len(products)
	for _, p := range products {
		q := SharedProduct{Code: p[0], Label: p[1]}
		var f, fe, di *float64
		_ = pool.QueryRow(ctx, `
			SELECT max(valeur) FILTER (WHERE element='Food')::float8,
			       max(valeur) FILTER (WHERE element='Feed')::float8,
			       max(valeur) FILTER (WHERE element='Domestic supply quantity')::float8
			FROM core.bilan_alimentaire WHERE produit_code=$1 AND annee=$2`,
			p[0], st.YearSummary).Scan(&f, &fe, &di)
		if f != nil {
			q.Human = *f
		}
		if fe != nil {
			q.Livestock = *fe
		}
		if q.Human+q.Livestock == 0 {
			continue
		}
		q.ShareLivestock = 100 * q.Livestock / (q.Human + q.Livestock)
		if di != nil && *di > 0 {
			q.Interior, q.WithInterior = *di, true
			q.ShareOutsideFood = 100 * (*di - q.Human - q.Livestock) / *di
		}
		st.TotalHuman += q.Human
		st.TotalLivestock += q.Livestock
		st.Shared = append(st.Shared, q)
	}
	return st, nil
}

func pctAgri(v float64) string { return strings.ReplaceAll(fmt.Sprintf("%.0f", v), ".", ",") + " %" }
