package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le taux de remplacement : la part du revenu d'avant la retraite que la
// pension remplace — en quantiles, pas en moyenne, pour ne pas cacher la
// dispersion. Voir docs/retraite-donnees.md.
var SourceTauxRemplacement = archive.Source{
	Slug: "drees-taux-remplacement-retraite", Label: "Drees — taux de remplacement à la retraite",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees",
	Cadence:     "annuelle",
	Notes: "100 = pension égale au revenu d'avant la retraite. Plusieurs revenus de " +
		"référence coexistent (revenus personnels, niveau de vie, retraite / revenus du " +
		"travail) : ne jamais comparer deux taux calculés sur des références différentes.",
}

const tauxRemplacementURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"repartition-des-taux-de-remplacement-entre-les-revenus-juste-avant-et-juste-apres-la-retraite/exports/json"

type ligneTauxRemplacement struct {
	Annee       string   `json:"premiere_annee_pleine_de_retraite"`
	Caract      string   `json:"caracteristique"`
	Categorie   string   `json:"categorie"`
	Sexe        string   `json:"sexe"`
	Revenu      string   `json:"revenu_utilise_pour_le_calcul_du_taux_de_remplacement"`
	Q10         *float64 `json:"taux_de_remplacement_quantile_a_10"`
	Q25         *float64 `json:"taux_de_remplacement_quantile_a_25"`
	Q50         *float64 `json:"taux_de_remplacement_quantile_a_50"`
	Q75         *float64 `json:"taux_de_remplacement_quantile_a_75"`
	Q90         *float64 `json:"taux_de_remplacement_quantile_a_90"`
	PartSous100 *float64 `json:"part_de_la_categorie_ayant_un_taux_de_remplacement_inferieur_a_100_en"`
}

func IngestTauxRemplacement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTauxRemplacement)
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

	f, err := arch.Fetch(ctx, srcID, runID, tauxRemplacementURL, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var lignes []ligneTauxRemplacement
	if err := json.Unmarshal(raw, &lignes); err != nil {
		return fail(fmt.Errorf("export illisible : %w", err))
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("export vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.taux_remplacement_retraite`); err != nil {
		return fail(err)
	}
	var rows [][]any
	var rejets int
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee)
		if err != nil {
			rejets++
			continue
		}
		rows = append(rows, []any{annee, l.Caract, l.Categorie, l.Sexe, l.Revenu,
			l.Q10, l.Q25, l.Q50, l.Q75, l.Q90, l.PartSous100, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne reconnue"))
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "taux_remplacement_retraite"},
		[]string{"premiere_annee_retraite", "caracteristique", "categorie", "sexe", "revenu_reference",
			"taux_q10", "taux_q25", "taux_q50", "taux_q75", "taux_q90", "part_taux_inferieur_100", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("taux_remplacement_retraite : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": n, "rejet_annee_illisible": rejets}, "")
	fmt.Printf("  taux de remplacement à la retraite : %d lignes\n", n)
	return nil
}
