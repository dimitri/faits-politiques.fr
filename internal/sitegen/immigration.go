package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// chargerHistoriqueImmigration : la série longue (32 recensements/estimations,
// 1921-2025) était chargée mais réduite à huit lignes de tableau — courbePaliers
// (déjà utilisée ailleurs pour ce même besoin) montre la part d'immigrés avec
// ses trois changements de champ ou de protocole, cités depuis le commentaire
// de la table plutôt que reformulés à la main.
func chargerHistoriqueImmigration(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, immigres_pct FROM core.population_historique_nationalite ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []PointAnnee
	for rows.Next() {
		var p PointAnnee
		if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
			return "", err
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	paliers := []Palier{
		{De: 1921, A: 1990, Libelle: "Métropole"},
		{De: 1990, A: 2014, Libelle: "France, hors Mayotte"},
		{De: 2014, A: 2023, Libelle: "Mayotte incluse"},
		{De: 2024, A: 2025, Libelle: "Protocole de collecte revu"},
	}
	format := func(v float64) string { return Decimal(v, 1) + " %" }
	return courbePaliers(pts, paliers, format), nil
}
