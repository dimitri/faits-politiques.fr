package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// loadSeriesDoctors : l'effectif total de médecins (Cnam, « Ensemble des
// médecins », toutes catégories confondues), 2010-2024, cité en prose dans le
// dossier santé (§ 2, « Évolution 2010-2024 ») mais jamais mis en graphique —
// la même série, en barres, sur le modèle de courbe() (cartepage.go).
func loadSeriesDoctors(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, sum(effectif)::float8 FROM core.medecin_secteur_effectif
		WHERE profession_sante = 'Ensemble des médecins' AND libelle_region = 'FRANCE'
		GROUP BY annee ORDER BY annee`)
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
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) < 2 {
		return "", nil
	}
	return curve(pts, func(v float64) string { return Count(int(v + 0.5)) }), nil
}
