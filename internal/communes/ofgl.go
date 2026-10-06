package communes

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// L'OFGL (Observatoire des finances et de la gestion publique locales) est une
// structure partenariale entre l'État, les associations d'élus et la Banque
// postale, adossée au Comité des finances locales. Elle republie, sous forme
// exploitable, les comptes de gestion de toutes les collectivités : ce que
// chaque commune a dépensé, emprunté et perçu, année par année.
//
// Ce qu'elle apporte et qu'aucune autre source ne donne aussi proprement :
// les mêmes agrégats calculés de la même façon pour 35 000 communes, avec des
// ratios par habitant déjà faits — et surtout, les variables de strate
// (tranche de population, rural, montagne, touristique, QPV, tranche de
// revenu). Ces strates sont définies par l'Observatoire, indépendamment de
// notre question : c'est très préférable à des strates que nous aurions
// construites nous-mêmes.
var SourceOFGL = archive.Source{
	Slug: "ofgl-communes", Label: "OFGL — comptes des communes",
	Publisher:  "Observatoire des finances et de la gestion publique locales",
	Tier:       "PRIMARY_OFFICIAL",
	Licence:    "Licence Ouverte v2.0",
	ReuseClass: "OPEN",
	Attribution: "Source : OFGL (Observatoire des finances et de la gestion publique locales), " +
		"d'après les comptes de gestion de la DGFiP",
	Cadence: "annuelle",
	Notes: "Budget principal uniquement. Un montant communal ne mesure une politique " +
		"municipale que si la compétence n'a pas été transférée à l'intercommunalité : " +
		"croiser avec BANATIC avant toute comparaison.",
}

const (
	ofglPremierExercice = 2018
	ofglDernierExercice = 2025
	ofglDataset         = "ofgl-base-communes"
)

// Agrégat publié par l'OFGL -> code d'indicateur de ref.indicator. La valeur
// retenue est le montant PAR HABITANT, qui est ce que les libellés annoncent.
var ofglAgregats = map[string]string{
	"Encours de dette":                   "ofgl.dette_par_hab",
	"Dépenses d'investissement":          "ofgl.investissement_par_hab",
	"Dépenses de fonctionnement":         "ofgl.fonctionnement_par_hab",
	"Frais de personnel":                 "ofgl.masse_salariale_par_hab",
	"Epargne brute":                      "ofgl.epargne_brute_par_hab",
	"Dotation globale de fonctionnement": "ofgl.dgf_par_hab",
	"Recettes totales":                   "ofgl.recettes_totales_par_hab",
	"Impôts et taxes":                    "ofgl.impots_taxes_par_hab",
}

func ofglURL(exercice int) string {
	noms := make([]string, 0, len(ofglAgregats))
	for a := range ofglAgregats {
		noms = append(noms, `"`+a+`"`)
	}
	// ODSQL : les littéraux de chaîne se citent avec des guillemets doubles ;
	// l'apostrophe de « Dépenses d'investissement » casse la forme simple.
	where := fmt.Sprintf(`exer=date'%d' AND type_de_budget="Budget principal" AND agregat IN (%s)`,
		exercice, strings.Join(noms, ","))
	return "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/" + ofglDataset +
		"/exports/csv?delimiter=%3B&select=exer,com_code,agregat,euros_par_habitant,ptot&where=" +
		url.QueryEscape(where)
}

func IngestOFGL(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceOFGL)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// tmp_ofgl_brut : tout ce que lireCSV a renvoyé, sans filtre ni dédoublonnage
	// — l'appartenance au COG et le dédoublonnage (une commune peut apparaître
	// deux fois pour le même indicateur/exercice selon la source) se font
	// maintenant en SQL, une seule fois, dans le MERGE plus bas, plutôt que
	// dans une carte Go reconstruite en mémoire à chaque run. ParseFloat reste
	// en Go : un échec de conversion est une anomalie de ligne, pas un filtre
	// contre une référence — différent par nature de l'appartenance au COG.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_ofgl_brut (
			commune_code text, indicator_code text, period_year int, value numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	for ex := ofglPremierExercice; ex <= ofglDernierExercice; ex++ {
		f, err := arch.Fetch(ctx, srcID, runID, ofglURL(ex), ".csv")
		if err != nil {
			return fail(fmt.Errorf("exercice %d : %w", ex, err))
		}
		recs, err := lireCSV(f.Path, ';')
		if err != nil {
			return fail(err)
		}
		rows := make([][]any, 0, len(recs)*2)
		for _, r := range recs {
			com := r["com_code"]
			code, ok := ofglAgregats[r["agregat"]]
			if !ok {
				continue
			}
			if v, err := strconv.ParseFloat(r["euros_par_habitant"], 64); err == nil {
				rows = append(rows, []any{com, code, ex, v, srcID})
			}
			// La population est publiée sur chaque ligne ; une seule suffit.
			if p, err := strconv.ParseFloat(r["ptot"], 64); err == nil {
				rows = append(rows, []any{com, "ofgl.population_totale", ex, p, srcID})
			}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_ofgl_brut"},
			[]string{"commune_code", "indicator_code", "period_year", "value", "source_id"},
			pgx.CopyFromRows(rows)); err != nil {
			return fail(fmt.Errorf("exercice %d : copie : %w", ex, err))
		}
		fmt.Printf("    exercice %d : %d lignes\n", ex, len(recs))
	}

	// Index support à la fois le DISTINCT ON du MERGE ci-dessous (même triplet
	// que la clé de dédoublonnage) et le NOT EXISTS du comptage hors-COG.
	if _, err := tx.Exec(ctx, `
		CREATE INDEX ON tmp_ofgl_brut (commune_code, indicator_code, period_year)`); err != nil {
		return fail(err)
	}

	var horsCOG int64
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT t.commune_code) FROM tmp_ofgl_brut t
		 WHERE NOT EXISTS (
		   SELECT 1 FROM ref.commune c
		    WHERE c.code_insee = t.commune_code AND c.cog_millesime = $1)`,
		COGMillesime).Scan(&horsCOG); err != nil {
		return fail(err)
	}

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (indicator_code LIKE
	// 'ofgl.%', 2,5 millions de lignes sur les 35 000 communes) payait le
	// prix des triggers RI (FK vers ref.commune) pour l'intégralité de son
	// périmètre à chaque republication annuelle, changement ou non.
	// commune_indicator_ofgl, une vue scopée plutôt que core.commune_
	// indicator directement : la table est partagée avec internal/eau/
	// budget_annexe.go et internal/communes/fiscalite_locale.go, chacun sur
	// son propre préfixe d'indicator_code.
	//
	// USING fait en un seul SELECT ce qui demandait avant une carte Go
	// (connues[com], valeurs[cle{...}]) : DISTINCT ON dédoublonne sur la même
	// clé que l'ancienne carte (dernière valeur rencontrée — l'ordre n'a
	// jamais été significatif, en Go pas plus qu'ici), et le JOIN ref.commune
	// ne retient que les communes du millésime courant.
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW commune_indicator_ofgl AS
		  SELECT * FROM core.commune_indicator WHERE indicator_code LIKE 'ofgl.%'
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	var n int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.commune_indicator", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO commune_indicator_ofgl AS tgt
			USING (
			  SELECT DISTINCT ON (t.commune_code, t.indicator_code, t.period_year)
			         t.commune_code, $1::int AS cog_millesime, t.indicator_code, t.period_year,
			         t.value, t.source_id, 'COMMUNE'::core.budget_scope AS budget_scope
			    FROM tmp_ofgl_brut t
			    JOIN ref.commune c ON c.code_insee = t.commune_code AND c.cog_millesime = $1
			   ORDER BY t.commune_code, t.indicator_code, t.period_year
			) AS src
			ON tgt.commune_code = src.commune_code AND tgt.budget_scope = src.budget_scope
			   AND tgt.indicator_code = src.indicator_code AND tgt.period_year = src.period_year
			WHEN MATCHED AND (tgt.cog_millesime, tgt.value, tgt.source_id)
			                  IS DISTINCT FROM (src.cog_millesime, src.value, src.source_id) THEN
			    UPDATE SET cog_millesime = src.cog_millesime, value = src.value, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (commune_code, cog_millesime, indicator_code, period_year, value, source_id, budget_scope)
			    VALUES (src.commune_code, src.cog_millesime, src.indicator_code, src.period_year,
			            src.value, src.source_id, src.budget_scope)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`,
			COGMillesime)
		if err != nil {
			return err
		}
		n = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion des indicateurs : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"indicateurs": n, "communes_hors_cog": horsCOG}, "")
	fmt.Printf("  OFGL : %d valeurs touchées sur %d-%d\n", n, ofglPremierExercice, ofglDernierExercice)
	if horsCOG > 0 {
		fmt.Printf("  %d communes de l'OFGL absentes du COG %d (communes disparues : ignorées)\n",
			horsCOG, COGMillesime)
	}
	return nil
}

func communesConnues(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx,
		`SELECT code_insee FROM ref.commune WHERE cog_millesime = $1`, COGMillesime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out[c] = true
	}
	return out, rows.Err()
}
