package communes

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
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
	"Encours de dette":           "ofgl.dette_par_hab",
	"Dépenses d'investissement":  "ofgl.investissement_par_hab",
	"Dépenses de fonctionnement": "ofgl.fonctionnement_par_hab",
	"Frais de personnel":         "ofgl.masse_salariale_par_hab",
	"Epargne brute":              "ofgl.epargne_brute_par_hab",
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

	connues, err := communesConnues(ctx, pool)
	if err != nil {
		return fail(err)
	}

	type cle struct {
		commune, indicateur string
		annee               int
	}
	valeurs := map[cle]float64{}
	horsCOG := map[string]bool{}

	for ex := ofglPremierExercice; ex <= ofglDernierExercice; ex++ {
		f, err := arch.Fetch(ctx, srcID, runID, ofglURL(ex), ".csv")
		if err != nil {
			return fail(fmt.Errorf("exercice %d : %w", ex, err))
		}
		recs, err := lireCSV(f.Path, ';')
		if err != nil {
			return fail(err)
		}
		for _, r := range recs {
			com := r["com_code"]
			if !connues[com] {
				if com != "" {
					horsCOG[com] = true
				}
				continue
			}
			code, ok := ofglAgregats[r["agregat"]]
			if !ok {
				continue
			}
			if v, err := strconv.ParseFloat(r["euros_par_habitant"], 64); err == nil {
				valeurs[cle{com, code, ex}] = v
			}
			// La population est publiée sur chaque ligne ; une seule suffit.
			if p, err := strconv.ParseFloat(r["ptot"], 64); err == nil {
				valeurs[cle{com, "ofgl.population_totale", ex}] = p
			}
		}
		fmt.Printf("    exercice %d : %d lignes\n", ex, len(recs))
	}

	rows := make([][]any, 0, len(valeurs))
	for k, v := range valeurs {
		rows = append(rows, []any{k.commune, COGMillesime, k.indicateur, k.annee, v, srcID, "COMMUNE"})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM core.commune_indicator WHERE indicator_code LIKE 'ofgl.%'`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "commune_indicator"},
		[]string{"commune_code", "cog_millesime", "indicator_code", "period_year",
			"value", "source_id", "budget_scope"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("copie des indicateurs : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"indicateurs": n, "communes_hors_cog": len(horsCOG)}, "")
	fmt.Printf("  OFGL : %d valeurs sur %d-%d\n", n, ofglPremierExercice, ofglDernierExercice)
	if len(horsCOG) > 0 {
		fmt.Printf("  %d communes de l'OFGL absentes du COG %d (communes disparues : ignorées)\n",
			len(horsCOG), COGMillesime)
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
