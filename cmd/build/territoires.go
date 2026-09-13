package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les territoires : ce que le site sait de chaque département, en cartes.
//
// C'est aujourd'hui la donnée la mieux couverte du site — 34 869 communes sur
// 34 875 pour les finances, la totalité pour la délinquance — très loin devant
// la donnée politique. L'ordre des cartes suit cette couverture.
type CarteTerritoire struct {
	Slug, Titre, Question, Source, Note string
	Carte                               Carte
}

type StatsTerritoires struct {
	Cartes                 []CarteTerritoire
	Communes, Departements int
	Annee                  int
}

func loadTerritoires(ctx context.Context, pool *pgxpool.Pool) (*StatsTerritoires, error) {
	contours, _, vb, err := contoursDept(ctx, pool, 0.006)
	if err != nil {
		return nil, err
	}
	st := &StatsTerritoires{Departements: len(contours), Annee: 2023}
	_ = pool.QueryRow(ctx, `SELECT count(DISTINCT commune_code) FROM core.commune_indicator`).
		Scan(&st.Communes)

	eur := func(v float64) string { return Nombre(int(v+0.5)) + " €" }
	pour1000 := func(v float64) string { return fmt.Sprintf("%.1f ‰", v) }
	pct := func(v float64) string { return fmt.Sprintf("%.0f %%", v) }

	// Indicateurs financiers : moyenne pondérée par la population, ce qui
	// revient à reconstituer le total puis à le diviser par les habitants.
	fin := func(code string) ([]CaseCarte, error) {
		rows, err := pool.Query(ctx, `
			SELECT c.code_departement, max(c.nom_clair),
			       sum(d.value*p.value)/nullif(sum(p.value),0)
			FROM core.commune_indicator d
			JOIN core.commune_indicator p ON p.commune_code=d.commune_code
			 AND p.period_year=d.period_year AND p.indicator_code='ofgl.population_totale'
			JOIN ref.commune c ON c.code_insee=d.commune_code AND c.cog_millesime=d.cog_millesime
			WHERE d.indicator_code=$1 AND d.period_year=2023
			GROUP BY 1`, code)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []CaseCarte
		for rows.Next() {
			var cc CaseCarte
			var v *float64
			if err := rows.Scan(&cc.Code, &cc.Nom, &v); err != nil {
				return nil, err
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Valeur = *v
			}
			out = append(out, cc)
		}
		return out, rows.Err()
	}

	for _, d := range []struct{ slug, code, titre, question, note string }{
		{"dette", "ofgl.dette_par_hab", "Dette des communes par habitant",
			"Combien chaque commune doit-elle, rapporté à ses habitants ?",
			"C'est la dette des communes seules : ni celle du département, ni celle de la région, ni celle de l'État."},
		{"investissement", "ofgl.investissement_par_hab", "Investissement communal par habitant",
			"Combien les communes dépensent-elles en équipement ?",
			"L'investissement d'une année n'est pas un rythme : une seule opération lourde suffit à faire bouger un petit territoire."},
		{"epargne", "ofgl.epargne_brute_par_hab", "Épargne brute par habitant",
			"Ce qui reste du fonctionnement pour investir et rembourser.",
			"Une épargne négative signale un budget de fonctionnement déficitaire."},
		{"masse-salariale", "ofgl.masse_salariale_par_hab", "Masse salariale communale par habitant",
			"Ce que coûte le personnel communal, par habitant.",
			"Elle dépend d'abord de ce que la commune gère en propre plutôt que par une intercommunalité : deux territoires ne sont pas comparables sans savoir qui fait quoi."},
	} {
		cases, err := fin(d.code)
		if err != nil {
			return nil, err
		}
		st.Cartes = append(st.Cartes, CarteTerritoire{
			Slug: d.slug, Titre: d.titre, Question: d.question, Note: d.note,
			Source: "OFGL / DGCL, exercice 2023",
			Carte:  choroplethe(contours, vb, cases, "€ par habitant", eur),
		})
	}

	// Densité associative : un fait sur la vie locale, sans jugement possible.
	rows, err := pool.Query(ctx, `
		SELECT c.code_departement, max(c.nom_clair),
		       1000.0*count(a.rna_id)/nullif(sum(DISTINCT 0)+max(pop.p),0)
		FROM ref.commune c
		JOIN (SELECT code_departement AS dep, sum(value) p
		      FROM core.commune_indicator ci
		      JOIN ref.commune rc ON rc.code_insee=ci.commune_code AND rc.cog_millesime=ci.cog_millesime
		      WHERE ci.indicator_code='ofgl.population_totale' AND ci.period_year=2023
		      GROUP BY 1) pop ON pop.dep=c.code_departement
		LEFT JOIN core.association a ON a.commune_code=c.code_insee
		GROUP BY c.code_departement`)
	if err == nil {
		var cases []CaseCarte
		for rows.Next() {
			var cc CaseCarte
			var v *float64
			if err := rows.Scan(&cc.Code, &cc.Nom, &v); err != nil {
				break
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Valeur = *v
			}
			cases = append(cases, cc)
		}
		rows.Close()
		st.Cartes = append(st.Cartes, CarteTerritoire{
			Slug: "associations", Titre: "Associations pour 1 000 habitants",
			Question: "Où la vie associative déclarée est-elle la plus dense ?",
			Note:     "Le répertoire recense les associations déclarées depuis 1901 et n'enregistre pas toujours les dissolutions : le compte penche vers le haut, surtout dans les départements anciens.",
			Source:   "RNA — répertoire national des associations",
			Carte:    choroplethe(contours, vb, cases, "pour 1 000 habitants", pour1000),
		})
	}

	// Part des sièges municipaux dont la nuance nomme un parti. C'est la carte
	// qui dit pourquoi une « carte des partis » n'existe pas.
	prows, err := pool.Query(ctx, `
		WITH s AS (
		  SELECT c.code_departement dep, max(c.nom_clair) nom, ml.nuance_code nc, sum(ml.sieges_cm) sg
		  FROM core.municipal_list ml
		  JOIN ref.commune c ON c.code_insee=ml.commune_code AND c.cog_millesime=ml.cog_millesime
		  WHERE ml.scrutin_annee=2026 AND ml.sieges_cm>0
		    AND ml.nuance_code IS NOT NULL AND ml.nuance_code<>''
		  GROUP BY 1,3)
		SELECT dep, max(nom), 100.0*coalesce(sum(sg) FILTER (WHERE nc IN
		  ('LLR','LRN','LSOC','LFI','LCOM','LVEC','LUDR','LUXD','LEXD','LECO')),0)/sum(sg)
		FROM s GROUP BY 1`)
	if err == nil {
		var cases []CaseCarte
		for prows.Next() {
			var cc CaseCarte
			var v *float64
			if err := prows.Scan(&cc.Code, &cc.Nom, &v); err != nil {
				break
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Valeur = *v
			}
			cases = append(cases, cc)
		}
		prows.Close()
		st.Cartes = append(st.Cartes, CarteTerritoire{
			Slug: "part-partisane", Titre: "Sièges municipaux dont la nuance nomme un parti",
			Question: "Les élections municipales sont-elles des élections de partis ?",
			Note:     "Non, très majoritairement : 82,7 % des sièges nuancés portent une nuance « divers », que le ministère de l'Intérieur refuse d'attribuer à un parti. Treize départements sont à zéro.",
			Source:   "Ministère de l'Intérieur, municipales 2026",
			Carte:    choroplethe(contours, vb, cases, "part des sièges", pct),
		})
	}
	return st, nil
}
