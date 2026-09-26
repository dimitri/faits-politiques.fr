package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsClimatInternational struct {
	CourbeRatificationSVG template.HTML
	NbPays                int
	NbJamaisRatifie       int
}

// chargerClimatInternational : le nombre CUMULÉ de pays ayant ratifié
// l'Accord de Paris, par année — la quasi-totalité en 2016-2017, ce qui
// disqualifie une carte du monde (voir D-079) : une courbe cumulative
// montre la vitesse d'adoption, qu'une carte presque entièrement d'une
// seule couleur ne montrerait pas.
func chargerClimatInternational(ctx context.Context, pool *pgxpool.Pool) (*StatsClimatInternational, error) {
	rows, err := pool.Query(ctx, `
		SELECT extract(year FROM date_ratification)::int AS annee, count(*)
		FROM core.ratification_accord_paris WHERE date_ratification IS NOT NULL
		GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []PointAnnee
	cumul := 0
	for rows.Next() {
		var annee, n int
		if err := rows.Scan(&annee, &n); err != nil {
			return nil, err
		}
		cumul += n
		pts = append(pts, PointAnnee{Annee: annee, Valeur: float64(cumul)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, nil
	}

	st := &StatsClimatInternational{}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.ratification_accord_paris`).Scan(&st.NbPays); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM core.ratification_accord_paris WHERE date_ratification IS NULL`).
		Scan(&st.NbJamaisRatifie); err != nil {
		return nil, err
	}
	format := func(v float64) string { return Nombre(int(v)) + " pays" }
	st.CourbeRatificationSVG = courbe(pts, format)
	return st, nil
}
