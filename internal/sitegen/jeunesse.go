package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La jeunesse : études supérieures, apprentissage, premiers emplois — voir
// docs/jeunesse-donnees.md. En miroir de vieillesse.go : une courbe (le taux
// d'emploi médian après apprentissage, promotion par promotion), une carte
// (par région cette fois — InserJeunes ne publie aucun découpage
// départemental), des chiffres-clés budgétaires.
type StatsYouth struct {
	CurveSVG         template.HTML
	FirstPromo       string
	LastPromo        string
	RateStart        float64
	RateEnd          float64
	EducationHigher  float64
	LifeStudent      float64
	CEJ2024          float64
	CEJ2025          float64
	LevelsEmployment []LevelEmployment
	MapInsertion     MapTerritory
}

type LevelEmployment struct {
	Level            string
	CountCFA         int
	MedianEmployment float64
}

// codeRegionInserJeunes : InserJeunes nomme ses régions en toutes lettres,
// majuscules et sans accents — pas le code INSEE qu'attend jeuContours(...,
// "REGION", ...). Vérifié sur l'export complet (dix-sept régions, DOM
// compris) avant d'écrire cette table plutôt que deviné depuis une
// translittération générique.
var codeRegionInsertYoung = map[string]string{
	"AUVERGNE-RHONE-ALPES":       "84",
	"BOURGOGNE-FRANCHE-COMTE":    "27",
	"BRETAGNE":                   "53",
	"CENTRE-VAL DE LOIRE":        "24",
	"CORSE":                      "94",
	"GRAND EST":                  "44",
	"GUADELOUPE":                 "01",
	"GUYANE":                     "03",
	"HAUTS-DE-FRANCE":            "32",
	"ILE-DE-FRANCE":              "11",
	"LA REUNION":                 "04",
	"MARTINIQUE":                 "02",
	"NORMANDIE":                  "28",
	"NOUVELLE-AQUITAINE":         "75",
	"OCCITANIE":                  "76",
	"PAYS DE LA LOIRE":           "52",
	"PROVENCE-ALPES-COTE D'AZUR": "93",
}

// cumulativeYear : "cumul 2023-2024" -> 2024. Le jeu InserJeunes nomme ses
// promotions par une plage de deux années scolaires ; la seconde année est
// celle où l'insertion à 6 mois est mesurée.
func cumulativeYear(s string) (int, bool) {
	shares := strings.Fields(s)
	if len(shares) == 0 {
		return 0, false
	}
	rng := strings.Split(shares[len(shares)-1], "-")
	if len(rng) != 2 {
		return 0, false
	}
	a, err := strconv.Atoi(rng[1])
	if err != nil {
		return 0, false
	}
	return a, true
}

func loadYouth(ctx context.Context, pool *pgxpool.Pool) (*StatsYouth, error) {
	st := &StatsYouth{}

	// Courbe : taux d'emploi médian à 6 mois, toutes filières confondues,
	// par promotion — une médiane, pas une moyenne pondérée (voir le
	// commentaire de la migration 0113 : ce jeu ne publie aucun effectif).
	rows, err := pool.Query(ctx, `
		SELECT annee_cumul, percentile_cont(0.5) WITHIN GROUP (ORDER BY taux_emploi_6_mois)
		FROM core.insertion_apprentissage
		WHERE taux_emploi_6_mois IS NOT NULL
		GROUP BY annee_cumul`)
	if err != nil {
		return nil, err
	}
	labelPerYear := map[int]string{}
	var pts []PointYear
	for rows.Next() {
		var cumulative string
		var rate float64
		if err := rows.Scan(&cumulative, &rate); err != nil {
			rows.Close()
			return nil, err
		}
		year, ok := cumulativeYear(cumulative)
		if !ok {
			rows.Close()
			return nil, fmt.Errorf("jeunesse : promotion %q illisible", cumulative)
		}
		labelPerYear[year] = cumulative
		pts = append(pts, PointYear{Year: year, Value: rate})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) > 1 {
		// tri par année : GROUP BY ne garantit pas l'ordre
		for i := 1; i < len(pts); i++ {
			for j := i; j > 0 && pts[j].Year < pts[j-1].Year; j-- {
				pts[j], pts[j-1] = pts[j-1], pts[j]
			}
		}
		st.FirstPromo = labelPerYear[pts[0].Year]
		st.LastPromo = labelPerYear[pts[len(pts)-1].Year]
		st.RateStart, st.RateEnd = pts[0].Value, pts[len(pts)-1].Value
		st.CurveSVG = curve(pts, func(v float64) string { return Decimal(v, 0) + " %" })
	}

	// Trois niveaux de diplôme, promotion la plus récente.
	levelsRows, err := pool.Query(ctx, `
		SELECT niveau_formation, count(*), percentile_cont(0.5) WITHIN GROUP (ORDER BY taux_emploi_6_mois)
		FROM core.insertion_apprentissage
		WHERE annee_cumul = (SELECT max(annee_cumul) FROM core.insertion_apprentissage)
		  AND niveau_formation IN ('CAP','BAC PRO','BTS') AND taux_emploi_6_mois IS NOT NULL
		GROUP BY niveau_formation ORDER BY 3`)
	if err != nil {
		return nil, err
	}
	for levelsRows.Next() {
		var n LevelEmployment
		if err := levelsRows.Scan(&n.Level, &n.CountCFA, &n.MedianEmployment); err != nil {
			levelsRows.Close()
			return nil, err
		}
		st.LevelsEmployment = append(st.LevelsEmployment, n)
	}
	levelsRows.Close()
	if err := levelsRows.Err(); err != nil {
		return nil, err
	}

	// sum(...) FILTER(...) est une agrégation : la ligne existe même sans
	// budget encore ingéré, avec des sommes NULL.
	var educationHigher, lifeStudent, cej2024, cej2025 sql.NullFloat64
	if err := pool.QueryRow(ctx, `
		SELECT sum(credit_paiement) FILTER (WHERE programme_libelle = 'Formations supérieures et recherche universitaire' AND exercice=2025),
		       sum(credit_paiement) FILTER (WHERE programme_libelle = 'Vie étudiante' AND exercice=2025),
		       sum(credit_paiement) FILTER (WHERE action_libelle ILIKE '%Contrat d''engagement jeunes%' AND exercice=2024),
		       sum(credit_paiement) FILTER (WHERE action_libelle ILIKE '%Contrat d''engagement jeunes%' AND exercice=2025)
		FROM core.budget_programme`).
		Scan(&educationHigher, &lifeStudent, &cej2024, &cej2025); err != nil {
		return nil, err
	}
	st.EducationHigher = educationHigher.Float64 / 1e9
	st.LifeStudent = lifeStudent.Float64 / 1e9
	st.CEJ2024 = cej2024.Float64 / 1e9
	st.CEJ2025 = cej2025.Float64 / 1e9

	// Carte : taux d'emploi médian par région, promotion la plus récente —
	// une médiane des CFA de la région, jamais une moyenne pondérée par
	// effectif (le jeu source n'en publie aucun).
	mapRows, err := pool.Query(ctx, `
		SELECT region, percentile_cont(0.5) WITHIN GROUP (ORDER BY taux_emploi_6_mois)
		FROM core.insertion_apprentissage
		WHERE annee_cumul = (SELECT max(annee_cumul) FROM core.insertion_apprentissage)
		  AND taux_emploi_6_mois IS NOT NULL
		GROUP BY region`)
	if err != nil {
		return nil, err
	}
	end, err := setOutlines(ctx, pool, "REGION", toleranceFull)
	if err != nil {
		mapRows.Close()
		return nil, err
	}
	var cells []CellMap
	for mapRows.Next() {
		var nameSource string
		var v *float64
		if err := mapRows.Scan(&nameSource, &v); err != nil {
			mapRows.Close()
			return nil, err
		}
		code, ok := codeRegionInsertYoung[nameSource]
		if !ok {
			mapRows.Close()
			return nil, fmt.Errorf("jeunesse : région InserJeunes inconnue %q — table codeRegionInserJeunes à compléter", nameSource)
		}
		cc := CellMap{Code: code, Name: end.Noms[code]}
		if v == nil {
			cc.Absent = true
		} else {
			cc.Value = *v
		}
		cells = append(cells, cc)
	}
	mapRows.Close()
	if err := mapRows.Err(); err != nil {
		return nil, err
	}
	if len(cells) > 0 {
		format := func(v float64) string { return Decimal(v, 0) + " %" }
		ranks := ranking(cells, end.Noms, format)
		st.MapInsertion = MapTerritory{
			Slug: "insertion-apprentissage", Title: "Taux d'emploi médian à 6 mois après un contrat d'apprentissage",
			Question: "Où l'insertion après l'apprentissage est-elle la meilleure ?",
			Note: "Médiane des CFA de la région, jamais une moyenne pondérée : InserJeunes ne publie " +
				"aucun effectif par CFA, une pondération par la taille réelle des établissements est " +
				"donc hors de portée de cette source.",
			Source: "DEPP, enquête InserJeunes, promotion la plus récente",
			Page: PageMap{
				Slug: "insertion-apprentissage", Title: "Taux d'emploi médian à 6 mois après un contrat d'apprentissage",
				Question: "Où l'insertion après l'apprentissage est-elle la meilleure ?",
				Source:   "DEPP, enquête InserJeunes, promotion la plus récente",
				Section:  "Jeunesse", SectionURL: "jeunesse", SectionIndexURL: "jeunesse",
				Map:     full(end, cells, "% médian", format),
				Summary: summarizeRanking(ranks),
				Ranking: ranks,
			},
		}
	}

	return st, nil
}
