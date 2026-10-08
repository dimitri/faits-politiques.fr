package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsClimateInternational struct {
	CurveRatificationSVG template.HTML
	CountCountry         int
	CountNeverRatified   int
}

// loadClimateInternational : le nombre CUMULÉ de pays ayant ratifié
// l'Accord de Paris, par année — la quasi-totalité en 2016-2017, ce qui
// disqualifie une carte du monde (voir D-079) : une courbe cumulative
// montre la vitesse d'adoption, qu'une carte presque entièrement d'une
// seule couleur ne montrerait pas.
func loadClimateInternational(ctx context.Context, pool *pgxpool.Pool) (*StatsClimateInternational, error) {
	rows, err := pool.Query(ctx, `
		SELECT extract(year FROM date_ratification)::int AS annee, count(*)
		FROM core.ratification_accord_paris WHERE date_ratification IS NOT NULL
		GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []PointYear
	cumulative := 0
	for rows.Next() {
		var year, n int
		if err := rows.Scan(&year, &n); err != nil {
			return nil, err
		}
		cumulative += n
		pts = append(pts, PointYear{Year: year, Value: float64(cumulative)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, nil
	}

	st := &StatsClimateInternational{}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.ratification_accord_paris`).Scan(&st.CountCountry); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM core.ratification_accord_paris WHERE date_ratification IS NULL`).
		Scan(&st.CountNeverRatified); err != nil {
		return nil, err
	}
	format := func(v float64) string { return Count(int(v)) + " pays" }
	st.CurveRatificationSVG = curve(pts, format)
	return st, nil
}
