package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// loadExpenseEnvironmental : 1503 lignes chargées (Eurostat env_epea_neep,
// toutes combinaisons de secteur et de finalité), mais le dossier n'en
// montrait que deux années (2020, 2023) en tableau — la ligne agrégée
// (TOT_CEP_EP, secteur S1 « ensemble de l'économie », en millions d'euros
// courants) donne la série complète.
func loadExpenseEnvironmental(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, valeur FROM core.depense_environnementale
		WHERE purpose_code = 'TOT_CEP_EP' AND secteur_code = 'S1' AND unite = 'MIO_EUR'
		ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []PointYear
	for rows.Next() {
		var p PointYear
		if err := rows.Scan(&p.Year, &p.Value); err != nil {
			return "", err
		}
		p.Value /= 1000 // millions -> milliards
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	format := func(v float64) string { return Decimal(v, 1) + " Md€" }
	return curve(pts, format), nil
}
