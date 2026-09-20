package main

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
type StatsJeunesse struct {
	CourbeSVG       template.HTML
	PremierePromo   string
	DernierePromo   string
	TauxDebut       float64
	TauxFin         float64
	EnseignementSup float64
	VieEtudiante    float64
	CEJ2024         float64
	CEJ2025         float64
	NiveauxEmploi   []NiveauEmploi
	CarteInsertion  CarteTerritoire
}

type NiveauEmploi struct {
	Niveau        string
	NbCFA         int
	MedianeEmploi float64
}

// codeRegionInserJeunes : InserJeunes nomme ses régions en toutes lettres,
// majuscules et sans accents — pas le code INSEE qu'attend jeuContours(...,
// "REGION", ...). Vérifié sur l'export complet (dix-sept régions, DOM
// compris) avant d'écrire cette table plutôt que deviné depuis une
// translittération générique.
var codeRegionInserJeunes = map[string]string{
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

// cumulAnnee : "cumul 2023-2024" -> 2024. Le jeu InserJeunes nomme ses
// promotions par une plage de deux années scolaires ; la seconde année est
// celle où l'insertion à 6 mois est mesurée.
func cumulAnnee(s string) (int, bool) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, false
	}
	rng := strings.Split(parts[len(parts)-1], "-")
	if len(rng) != 2 {
		return 0, false
	}
	a, err := strconv.Atoi(rng[1])
	if err != nil {
		return 0, false
	}
	return a, true
}

func loadJeunesse(ctx context.Context, pool *pgxpool.Pool) (*StatsJeunesse, error) {
	st := &StatsJeunesse{}

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
	labelParAnnee := map[int]string{}
	var pts []PointAnnee
	for rows.Next() {
		var cumul string
		var taux float64
		if err := rows.Scan(&cumul, &taux); err != nil {
			rows.Close()
			return nil, err
		}
		annee, ok := cumulAnnee(cumul)
		if !ok {
			rows.Close()
			return nil, fmt.Errorf("jeunesse : promotion %q illisible", cumul)
		}
		labelParAnnee[annee] = cumul
		pts = append(pts, PointAnnee{Annee: annee, Valeur: taux})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) > 1 {
		// tri par année : GROUP BY ne garantit pas l'ordre
		for i := 1; i < len(pts); i++ {
			for j := i; j > 0 && pts[j].Annee < pts[j-1].Annee; j-- {
				pts[j], pts[j-1] = pts[j-1], pts[j]
			}
		}
		st.PremierePromo = labelParAnnee[pts[0].Annee]
		st.DernierePromo = labelParAnnee[pts[len(pts)-1].Annee]
		st.TauxDebut, st.TauxFin = pts[0].Valeur, pts[len(pts)-1].Valeur
		st.CourbeSVG = courbe(pts, func(v float64) string { return Decimal(v, 0) + " %" })
	}

	// Trois niveaux de diplôme, promotion la plus récente.
	niveauxRows, err := pool.Query(ctx, `
		SELECT niveau_formation, count(*), percentile_cont(0.5) WITHIN GROUP (ORDER BY taux_emploi_6_mois)
		FROM core.insertion_apprentissage
		WHERE annee_cumul = (SELECT max(annee_cumul) FROM core.insertion_apprentissage)
		  AND niveau_formation IN ('CAP','BAC PRO','BTS') AND taux_emploi_6_mois IS NOT NULL
		GROUP BY niveau_formation ORDER BY 3`)
	if err != nil {
		return nil, err
	}
	for niveauxRows.Next() {
		var n NiveauEmploi
		if err := niveauxRows.Scan(&n.Niveau, &n.NbCFA, &n.MedianeEmploi); err != nil {
			niveauxRows.Close()
			return nil, err
		}
		st.NiveauxEmploi = append(st.NiveauxEmploi, n)
	}
	niveauxRows.Close()
	if err := niveauxRows.Err(); err != nil {
		return nil, err
	}

	// sum(...) FILTER(...) est une agrégation : la ligne existe même sans
	// budget encore ingéré, avec des sommes NULL.
	var enseignementSup, vieEtudiante, cej2024, cej2025 sql.NullFloat64
	if err := pool.QueryRow(ctx, `
		SELECT sum(credit_paiement) FILTER (WHERE programme_libelle = 'Formations supérieures et recherche universitaire' AND exercice=2025),
		       sum(credit_paiement) FILTER (WHERE programme_libelle = 'Vie étudiante' AND exercice=2025),
		       sum(credit_paiement) FILTER (WHERE action_libelle ILIKE '%Contrat d''engagement jeunes%' AND exercice=2024),
		       sum(credit_paiement) FILTER (WHERE action_libelle ILIKE '%Contrat d''engagement jeunes%' AND exercice=2025)
		FROM core.budget_programme`).
		Scan(&enseignementSup, &vieEtudiante, &cej2024, &cej2025); err != nil {
		return nil, err
	}
	st.EnseignementSup = enseignementSup.Float64 / 1e9
	st.VieEtudiante = vieEtudiante.Float64 / 1e9
	st.CEJ2024 = cej2024.Float64 / 1e9
	st.CEJ2025 = cej2025.Float64 / 1e9

	// Carte : taux d'emploi médian par région, promotion la plus récente —
	// une médiane des CFA de la région, jamais une moyenne pondérée par
	// effectif (le jeu source n'en publie aucun).
	carteRows, err := pool.Query(ctx, `
		SELECT region, percentile_cont(0.5) WITHIN GROUP (ORDER BY taux_emploi_6_mois)
		FROM core.insertion_apprentissage
		WHERE annee_cumul = (SELECT max(annee_cumul) FROM core.insertion_apprentissage)
		  AND taux_emploi_6_mois IS NOT NULL
		GROUP BY region`)
	if err != nil {
		return nil, err
	}
	fin, err := jeuContours(ctx, pool, "REGION", tolPleine)
	if err != nil {
		carteRows.Close()
		return nil, err
	}
	var cases []CaseCarte
	for carteRows.Next() {
		var nomSource string
		var v *float64
		if err := carteRows.Scan(&nomSource, &v); err != nil {
			carteRows.Close()
			return nil, err
		}
		code, ok := codeRegionInserJeunes[nomSource]
		if !ok {
			carteRows.Close()
			return nil, fmt.Errorf("jeunesse : région InserJeunes inconnue %q — table codeRegionInserJeunes à compléter", nomSource)
		}
		cc := CaseCarte{Code: code, Nom: fin.Noms[code]}
		if v == nil {
			cc.Absent = true
		} else {
			cc.Valeur = *v
		}
		cases = append(cases, cc)
	}
	carteRows.Close()
	if err := carteRows.Err(); err != nil {
		return nil, err
	}
	if len(cases) > 0 {
		format := func(v float64) string { return Decimal(v, 0) + " %" }
		rangs := classement(cases, fin.Noms, format)
		st.CarteInsertion = CarteTerritoire{
			Slug: "insertion-apprentissage", Titre: "Taux d'emploi médian à 6 mois après un contrat d'apprentissage",
			Question: "Où l'insertion après l'apprentissage est-elle la meilleure ?",
			Note: "Médiane des CFA de la région, jamais une moyenne pondérée : InserJeunes ne publie " +
				"aucun effectif par CFA, une pondération par la taille réelle des établissements est " +
				"donc hors de portée de cette source.",
			Source: "DEPP, enquête InserJeunes, promotion la plus récente",
			Page: PageCarte{
				Slug: "insertion-apprentissage", Titre: "Taux d'emploi médian à 6 mois après un contrat d'apprentissage",
				Question: "Où l'insertion après l'apprentissage est-elle la meilleure ?",
				Source:   "DEPP, enquête InserJeunes, promotion la plus récente",
				Section:  "Jeunesse", SectionURL: "jeunesse", SectionIndexURL: "jeunesse",
				Carte:      pleine(fin, cases, "% médian", format),
				Resume:     resumerClassement(rangs),
				Classement: rangs,
			},
		}
	}

	return st, nil
}
