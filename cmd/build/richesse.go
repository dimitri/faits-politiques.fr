package main

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Ce qu'il y a dans le dernier décile de niveau de vie (D10) : le dossier
// pauvreté (vieillesse... non, pauvrete-donnees.md) montre D1 à D9 avec un
// dixième décile délibérément « non borné ». Ce dossier ouvre ce dixième
// décile — 1 %, 0,1 %, 0,01 % les plus aisés — et lui adjoint le patrimoine,
// une notion distincte, jamais additionnée au revenu. Voir docs/
// repartition-richesse-donnees.md et la migration 0117.
type StatsRichesse struct {
	Seuils      []SeuilRevenu
	Courbe1Pct  template.HTML
	Courbe01Pct template.HTML
	AnneeDebut, AnneeFin int
	Part1PctDebut, Part1PctFin   float64
	Part01PctDebut, Part01PctFin float64
	Patrimoines []TranchePatrimoine
	AnneePatrimoine int

	// L'héritage comme facteur d'accès à la richesse (§ 4) et la
	// comparaison patrimoine/niveau de vie (§ 5) — même fiche Insee,
	// migration 0118.
	HeriteEnsemble, HeriteHaut     float64
	DonationEnsemble, DonationHaut float64
	GiniPatrimoine, GiniNiveauVie  float64
	Concentrations                 []Concentration
}

type Concentration struct {
	Position                 string
	MassePatrimoine, MasseNiveauVie float64
}

type SeuilRevenu struct {
	Libelle, Code                string
	RevenuAvant, NiveauDeVie     int
}

type TranchePatrimoine struct {
	Libelle                        string
	Seuil2015, Seuil2021           int
	Moyen2015, Moyen2021           int
	PartMasse2021                  float64
}

var libelleSeuil = map[string]string{
	"D5": "Médiane (50 %)", "D9": "10 % les plus aisés", "Q99": "1 % les plus aisés",
	"Q99_9": "0,1 % les plus aisés", "Q99_99": "0,01 % les plus aisés",
}
var ordreSeuil = []string{"D5", "D9", "Q99", "Q99_9", "Q99_99"}

var libellePatrimoine = map[string]string{
	"P90_P95": "Du 90ᵉ au 95ᵉ centile", "P95_P99": "Du 95ᵉ au 99ᵉ centile", "SUP_P99": "Au-delà du 99ᵉ centile",
}
var ordrePatrimoine = []string{"P90_P95", "P95_P99", "SUP_P99"}

func loadRichesse(ctx context.Context, pool *pgxpool.Pool) (*StatsRichesse, error) {
	st := &StatsRichesse{}

	seuilRows, err := pool.Query(ctx, `
		SELECT seuil, revenu_avant_redistribution_eur, niveau_de_vie_eur
		FROM core.filosofi_haut_revenu`)
	if err != nil {
		return nil, err
	}
	parSeuil := map[string]SeuilRevenu{}
	for seuilRows.Next() {
		var code string
		var avant, niveau int
		if err := seuilRows.Scan(&code, &avant, &niveau); err != nil {
			seuilRows.Close()
			return nil, err
		}
		parSeuil[code] = SeuilRevenu{Libelle: libelleSeuil[code], Code: code, RevenuAvant: avant, NiveauDeVie: niveau}
	}
	seuilRows.Close()
	if err := seuilRows.Err(); err != nil {
		return nil, err
	}
	for _, code := range ordreSeuil {
		if s, ok := parSeuil[code]; ok {
			st.Seuils = append(st.Seuils, s)
		}
	}

	chargerSerie := func(groupe string) ([]PointAnnee, error) {
		rows, err := pool.Query(ctx, `
			SELECT annee, part_pct FROM core.revenu_part_groupe
			WHERE groupe = $1 ORDER BY annee`, groupe)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var pts []PointAnnee
		for rows.Next() {
			var p PointAnnee
			if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
				return nil, err
			}
			pts = append(pts, p)
		}
		return pts, rows.Err()
	}
	pct1, err := chargerSerie("1_PLUS_AISES")
	if err != nil {
		return nil, err
	}
	pct01, err := chargerSerie("0_1_PLUS_AISES")
	if err != nil {
		return nil, err
	}
	if len(pct1) > 0 {
		st.AnneeDebut, st.AnneeFin = pct1[0].Annee, pct1[len(pct1)-1].Annee
		st.Part1PctDebut, st.Part1PctFin = pct1[0].Valeur, pct1[len(pct1)-1].Valeur
		st.Courbe1Pct = courbe(pct1, func(v float64) string { return Decimal(v, 1) + " %" })
	}
	if len(pct01) > 0 {
		st.Part01PctDebut, st.Part01PctFin = pct01[0].Valeur, pct01[len(pct01)-1].Valeur
		st.Courbe01Pct = courbe(pct01, func(v float64) string { return Decimal(v, 1) + " %" })
	}

	patRows, err := pool.Query(ctx, `
		SELECT tranche, annee, seuil_bas_eur, patrimoine_moyen_eur, part_masse_pct
		FROM core.patrimoine_haut ORDER BY tranche, annee`)
	if err != nil {
		return nil, err
	}
	parTranche := map[string]*TranchePatrimoine{}
	for patRows.Next() {
		var tranche string
		var annee, seuil, moyen int
		var part *float64
		if err := patRows.Scan(&tranche, &annee, &seuil, &moyen, &part); err != nil {
			patRows.Close()
			return nil, err
		}
		t, ok := parTranche[tranche]
		if !ok {
			t = &TranchePatrimoine{Libelle: libellePatrimoine[tranche]}
			parTranche[tranche] = t
		}
		if annee == 2015 {
			t.Seuil2015, t.Moyen2015 = seuil, moyen
		} else {
			t.Seuil2021, t.Moyen2021 = seuil, moyen
			if part != nil {
				t.PartMasse2021 = *part
			}
			st.AnneePatrimoine = annee
		}
	}
	patRows.Close()
	if err := patRows.Err(); err != nil {
		return nil, err
	}
	for _, code := range ordrePatrimoine {
		if t, ok := parTranche[code]; ok {
			st.Patrimoines = append(st.Patrimoines, *t)
		}
	}

	if err := pool.QueryRow(ctx, `
		SELECT max(part_herite_pct) FILTER (WHERE categorie='ENSEMBLE'),
		       max(part_herite_pct) FILTER (WHERE categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE'),
		       max(part_donation_pct) FILTER (WHERE categorie='ENSEMBLE'),
		       max(part_donation_pct) FILTER (WHERE categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE')
		FROM core.menage_heritage WHERE tranche_age = 'TOUS_AGES'`).
		Scan(&st.HeriteEnsemble, &st.HeriteHaut, &st.DonationEnsemble, &st.DonationHaut); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT indice_patrimoine, indice_niveau_vie FROM core.gini_patrimoine_niveau_vie
		ORDER BY annee DESC LIMIT 1`).
		Scan(&st.GiniPatrimoine, &st.GiniNiveauVie); err != nil {
		return nil, err
	}

	concRows, err := pool.Query(ctx, `
		SELECT position_distribution, masse_patrimoine_pct, masse_niveau_vie_pct
		FROM core.concentration_patrimoine_niveau_vie
		ORDER BY array_position(ARRAY[
			'Inférieure au 2e décile', 'Inférieure au 4e décile',
			'Inférieure au 5e décile (médiane)', 'Supérieure au 5e décile (médiane)',
			'Supérieure au 8e décile', 'Supérieure au 9e décile'
		], position_distribution)`)
	if err != nil {
		return nil, err
	}
	for concRows.Next() {
		var c Concentration
		if err := concRows.Scan(&c.Position, &c.MassePatrimoine, &c.MasseNiveauVie); err != nil {
			concRows.Close()
			return nil, err
		}
		st.Concentrations = append(st.Concentrations, c)
	}
	concRows.Close()
	if err := concRows.Err(); err != nil {
		return nil, err
	}

	return st, nil
}
