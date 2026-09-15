package main

import (
	"context"
	"fmt"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La vieillesse : combien pèse-t-elle dans la dépense publique, et la
// dépendance au-delà des seules retraites — voir docs/vieillesse-donnees.md.
// Une page simple (une courbe, quelques chiffres-clés, une carte) plutôt
// qu'une grille de cartes : ce dossier n'a qu'une seule mesure géographique
// chargée pour l'instant (l'APA à domicile).
type StatsVieillesse struct {
	CourbeSVG        template.HTML
	Debut, Fin       int
	ValeurDebut      float64
	ValeurFin        float64
	CasPensions      float64
	RegimesSpeciaux  float64
	CofogTotal       float64
	PartCofog        float64
	APABeneficiaires int
	APADepenses      float64
	CarteAPA         CarteTerritoire

	// Qui paie : le budget de l'État ne porte qu'une fraction du total COFOG
	// (docs/vieillesse-donnees.md § 3) — le reste vient de la Sécurité
	// sociale, jamais de l'État.
	TotalEtat float64
	PartEtat  float64
	EcartSecu float64

	// Pression démographique et âge de départ (docs/retraite-donnees.md § 2,
	// déjà chargés) — rappelés ici plutôt que rechargés.
	RatioAnnee              int
	RatioCotisantsRetraites float64
	AgeAnnee                int
	AgeEnsemble             float64
	AgeFemmes               float64
	AgeHommes               float64
}

func loadVieillesse(ctx context.Context, pool *pgxpool.Pool) (*StatsVieillesse, error) {
	st := &StatsVieillesse{}

	// Vieillesse + survivants (COFOG GF1002+GF1003), 1995-2024.
	rows, err := pool.Query(ctx, `
		SELECT mv.annee, sum(mv.valeur)/1e3 AS md_eur
		FROM core.macro_value mv JOIN ref.macro_serie rs ON rs.code = mv.serie_code
		WHERE rs.cofog IN ('GF1002','GF1003')
		GROUP BY mv.annee ORDER BY mv.annee`)
	if err != nil {
		return nil, err
	}
	var pts []PointAnnee
	for rows.Next() {
		var p PointAnnee
		if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
			rows.Close()
			return nil, err
		}
		pts = append(pts, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) > 0 {
		st.Debut, st.Fin = pts[0].Annee, pts[len(pts)-1].Annee
		st.ValeurDebut, st.ValeurFin = pts[0].Valeur, pts[len(pts)-1].Valeur
		st.CourbeSVG = courbe(pts, func(v float64) string { return Decimal(v, 1) + " Md€" })
	}

	if err := pool.QueryRow(ctx, `
		SELECT sum(credit_paiement) FILTER (WHERE mission_libelle = 'Pensions'),
		       sum(credit_paiement) FILTER (WHERE mission_libelle = 'Régimes sociaux et de retraite')
		FROM core.budget_programme WHERE exercice = 2025`).
		Scan(&st.CasPensions, &st.RegimesSpeciaux); err != nil {
		return nil, err
	}
	st.CasPensions /= 1e9
	st.RegimesSpeciaux /= 1e9

	if len(pts) > 0 {
		st.CofogTotal = pts[len(pts)-1].Valeur
	}
	var totalPublic float64
	if err := pool.QueryRow(ctx, `
		SELECT sum(mv.valeur)/1e3 FROM core.macro_value mv JOIN ref.macro_serie rs ON rs.code = mv.serie_code
		WHERE rs.cofog IN ('GF01','GF02','GF03','GF04','GF05','GF06','GF07','GF08','GF09','GF10')
		  AND mv.annee = $1`, st.Fin).Scan(&totalPublic); err != nil {
		return nil, err
	}
	if totalPublic > 0 {
		st.PartCofog = 100 * st.CofogTotal / totalPublic
	}
	st.TotalEtat = st.CasPensions + st.RegimesSpeciaux
	if st.CofogTotal > 0 {
		st.PartEtat = 100 * st.TotalEtat / st.CofogTotal
		st.EcartSecu = st.CofogTotal - st.TotalEtat
	}

	if err := pool.QueryRow(ctx, `
		SELECT annee, ratio_demographique FROM core.cotisants_retraites_ratio
		ORDER BY annee DESC LIMIT 1`).
		Scan(&st.RatioAnnee, &st.RatioCotisantsRetraites); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT annee, age_ensemble, age_femmes, age_hommes FROM core.age_depart_retraite
		ORDER BY annee DESC LIMIT 1`).
		Scan(&st.AgeAnnee, &st.AgeEnsemble, &st.AgeFemmes, &st.AgeHommes); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT sum(nb_beneficiaires), sum(depenses_total_eur)/1e9
		FROM core.apa_domicile WHERE annee = 2024`).
		Scan(&st.APABeneficiaires, &st.APADepenses); err != nil {
		return nil, err
	}

	// Carte : bénéficiaires de l'APA à domicile pour 100 000 habitants, par
	// département — même construction que la carte des généralistes
	// (cmd/build/territoires.go), CASE plutôt que lpad pour ne pas tronquer
	// les codes DOM à trois chiffres.
	carteRows, err := pool.Query(ctx, `
		SELECT m.dep, max(m.libelle_departement),
		       100000.0*max(m.nb_beneficiaires)/nullif(max(pop.p),0)
		FROM (SELECT *, CASE WHEN code_departement ~ '^[0-9]$'
		                 THEN '0'||code_departement ELSE code_departement END AS dep
		        FROM core.apa_domicile WHERE annee = 2024) m
		JOIN (SELECT code_departement AS dep, sum(value) p
		      FROM core.commune_indicator ci
		      JOIN ref.commune rc ON rc.code_insee=ci.commune_code AND rc.cog_millesime=ci.cog_millesime
		      WHERE ci.indicator_code='ofgl.population_totale' AND ci.period_year=2023
		      GROUP BY 1) pop ON pop.dep = m.dep
		GROUP BY m.dep`)
	if err != nil {
		return nil, err
	}
	var cases []CaseCarte
	for carteRows.Next() {
		var cc CaseCarte
		var v *float64
		if err := carteRows.Scan(&cc.Code, &cc.Nom, &v); err != nil {
			carteRows.Close()
			return nil, err
		}
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
		fin, err := jeuContours(ctx, pool, "DEPARTEMENT", tolPleine)
		if err != nil {
			return nil, err
		}
		format := func(v float64) string { return Decimal(v, 0) }
		st.CarteAPA = CarteTerritoire{
			Slug: "apa-domicile", Titre: "Bénéficiaires de l'APA à domicile pour 100 000 habitants",
			Question: "Où l'allocation personnalisée d'autonomie à domicile compte-t-elle le plus de bénéficiaires ?",
			Note: fmt.Sprintf("Compte une présence, pas un besoin couvert : un département dense en "+
				"bénéficiaires peut aussi être un département où la population est plus âgée en proportion — "+
				"cette carte ne rapporte pas à la population de 75 ans ou plus, faute de ce chiffre par "+
				"département dans ce même chargement. %d bénéficiaires, %s Md€ de dépenses couvertes "+
				"(22 %% des lignes sans donnée, exclues du calcul plutôt que comptées à zéro).",
				st.APABeneficiaires, Decimal(st.APADepenses, 2)),
			Source: "DREES, enquête Aide sociale, 2024 ; OFGL, population 2023",
			Page: PageCarte{
				Slug: "apa-domicile", Titre: "Bénéficiaires de l'APA à domicile pour 100 000 habitants",
				Question: "Où l'allocation personnalisée d'autonomie à domicile compte-t-elle le plus de bénéficiaires ?",
				Source:   "DREES, enquête Aide sociale, 2024 ; OFGL, population 2023",
				Section:  "Vieillesse", SectionURL: "vieillesse", SectionIndexURL: "vieillesse",
				Carte:      pleine(fin, cases, "bénéficiaires pour 100 000 hab.", format),
				Classement: classement(cases, fin.Noms, format),
			},
		}
	}

	return st, nil
}
