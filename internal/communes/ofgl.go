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
	ofglFirstFiscalYear = 2018
	ofglLastFiscalYear  = 2025
	ofglDataset         = "ofgl-base-communes"
)

// Agrégat publié par l'OFGL -> code d'indicateur de ref.indicator. La valeur
// retenue est le montant PAR HABITANT, qui est ce que les libellés annoncent.
var ofglAggregates = map[string]string{
	"Encours de dette":                   "ofgl.dette_par_hab",
	"Dépenses d'investissement":          "ofgl.investissement_par_hab",
	"Dépenses de fonctionnement":         "ofgl.fonctionnement_par_hab",
	"Frais de personnel":                 "ofgl.masse_salariale_par_hab",
	"Epargne brute":                      "ofgl.epargne_brute_par_hab",
	"Dotation globale de fonctionnement": "ofgl.dgf_par_hab",
	"Recettes totales":                   "ofgl.recettes_totales_par_hab",
	"Impôts et taxes":                    "ofgl.impots_taxes_par_hab",
}

func ofglURL(fiscalYear int) string {
	names := make([]string, 0, len(ofglAggregates))
	for a := range ofglAggregates {
		names = append(names, `"`+a+`"`)
	}
	// ODSQL : les littéraux de chaîne se citent avec des guillemets doubles ;
	// l'apostrophe de « Dépenses d'investissement » casse la forme simple.
	where := fmt.Sprintf(`exer=date'%d' AND type_de_budget="Budget principal" AND agregat IN (%s)`,
		fiscalYear, strings.Join(names, ","))
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

	connues, err := knownCommunes(ctx, pool)
	if err != nil {
		return fail(err)
	}

	type key struct {
		commune, indicator string
		year               int
	}
	values := map[key]float64{}
	outsideCOG := map[string]bool{}

	for year := ofglFirstFiscalYear; year <= ofglLastFiscalYear; year++ {
		f, err := arch.Fetch(ctx, srcID, runID, ofglURL(year), ".csv")
		if err != nil {
			return fail(fmt.Errorf("exercice %d : %w", year, err))
		}
		recs, err := readCSV(f.Path, ';')
		if err != nil {
			return fail(err)
		}
		for _, r := range recs {
			com := r["com_code"]
			if !connues[com] {
				if com != "" {
					outsideCOG[com] = true
				}
				continue
			}
			code, ok := ofglAggregates[r["agregat"]]
			if !ok {
				continue
			}
			if v, err := strconv.ParseFloat(r["euros_par_habitant"], 64); err == nil {
				values[key{com, code, year}] = v
			}
			// La population est publiée sur chaque ligne ; une seule suffit.
			if p, err := strconv.ParseFloat(r["ptot"], 64); err == nil {
				values[key{com, "ofgl.population_totale", year}] = p
			}
		}
		fmt.Printf("    exercice %d : %d lignes\n", year, len(recs))
	}

	rows := make([][]any, 0, len(values))
	for k, v := range values {
		rows = append(rows, []any{k.commune, COGVintage, k.indicator, k.year, v, srcID, "COMMUNE"})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (indicator_code LIKE
	// 'ofgl.%', 2,5 millions de lignes sur les 35 000 communes) payait le
	// prix des triggers RI (FK vers ref.commune) pour l'intégralité de son
	// périmètre à chaque republication annuelle, changement ou non.
	// commune_indicator_ofgl, une vue scopée plutôt que core.commune_
	// indicator directement : la table est partagée avec internal/eau/
	// budget_annexe.go et internal/communes/fiscalite_locale.go, chacun sur
	// son propre préfixe d'indicator_code.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_commune_indicator_ofgl (
			commune_code text, cog_millesime int, indicator_code text,
			period_year int, value numeric, source_id bigint, budget_scope core.budget_scope
		) ON COMMIT DROP;
		CREATE OR REPLACE TEMPORARY VIEW commune_indicator_ofgl AS
		  SELECT * FROM core.commune_indicator WHERE indicator_code LIKE 'ofgl.%'
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_commune_indicator_ofgl"},
		[]string{"commune_code", "cog_millesime", "indicator_code", "period_year",
			"value", "source_id", "budget_scope"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("copie des indicateurs : %w", err))
	}
	var n int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.commune_indicator", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO commune_indicator_ofgl AS tgt
			USING tmp_commune_indicator_ofgl AS src
			ON tgt.commune_code = src.commune_code AND tgt.budget_scope = src.budget_scope
			   AND tgt.indicator_code = src.indicator_code AND tgt.period_year = src.period_year
			WHEN MATCHED AND (tgt.cog_millesime, tgt.value, tgt.source_id)
			                  IS DISTINCT FROM (src.cog_millesime, src.value, src.source_id) THEN
			    UPDATE SET cog_millesime = src.cog_millesime, value = src.value, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (commune_code, cog_millesime, indicator_code, period_year, value, source_id, budget_scope)
			    VALUES (src.commune_code, src.cog_millesime, src.indicator_code, src.period_year,
			            src.value, src.source_id, src.budget_scope)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
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
		map[string]any{"indicateurs": n, "communes_hors_cog": len(outsideCOG)}, "")
	fmt.Printf("  OFGL : %d valeurs touchées sur %d-%d\n", n, ofglFirstFiscalYear, ofglLastFiscalYear)
	if len(outsideCOG) > 0 {
		fmt.Printf("  %d communes de l'OFGL absentes du COG %d (communes disparues : ignorées)\n",
			len(outsideCOG), COGVintage)
	}
	return nil
}

func knownCommunes(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx,
		`SELECT code_insee FROM ref.commune WHERE cog_millesime = $1`, COGVintage)
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
