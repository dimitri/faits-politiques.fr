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
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_taux_remplacement_retraite (
			premiere_annee_retraite int, caracteristique text, categorie text, sexe text, revenu_reference text,
			taux_q10 numeric, taux_q25 numeric, taux_q50 numeric, taux_q75 numeric, taux_q90 numeric,
			part_taux_inferieur_100 numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
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
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_taux_remplacement_retraite"},
		[]string{"premiere_annee_retraite", "caracteristique", "categorie", "sexe", "revenu_reference",
			"taux_q10", "taux_q25", "taux_q50", "taux_q75", "taux_q90", "part_taux_inferieur_100", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("taux_remplacement_retraite : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité des catégories et millésimes à chaque republication.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.taux_remplacement_retraite AS tgt
		USING tmp_taux_remplacement_retraite AS src
		ON tgt.premiere_annee_retraite = src.premiere_annee_retraite AND tgt.caracteristique = src.caracteristique
		   AND tgt.categorie = src.categorie AND tgt.sexe = src.sexe AND tgt.revenu_reference = src.revenu_reference
		WHEN MATCHED AND (tgt.taux_q10, tgt.taux_q25, tgt.taux_q50, tgt.taux_q75, tgt.taux_q90,
		                   tgt.part_taux_inferieur_100, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.taux_q10, src.taux_q25, src.taux_q50, src.taux_q75, src.taux_q90,
		                   src.part_taux_inferieur_100, src.source_id) THEN
		    UPDATE SET taux_q10 = src.taux_q10, taux_q25 = src.taux_q25, taux_q50 = src.taux_q50,
		               taux_q75 = src.taux_q75, taux_q90 = src.taux_q90,
		               part_taux_inferieur_100 = src.part_taux_inferieur_100, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (premiere_annee_retraite, caracteristique, categorie, sexe, revenu_reference,
		            taux_q10, taux_q25, taux_q50, taux_q75, taux_q90, part_taux_inferieur_100, source_id)
		    VALUES (src.premiere_annee_retraite, src.caracteristique, src.categorie, src.sexe, src.revenu_reference,
		            src.taux_q10, src.taux_q25, src.taux_q50, src.taux_q75, src.taux_q90,
		            src.part_taux_inferieur_100, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion taux_remplacement_retraite : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_touchees": n, "rejet_annee_illisible": rejets}, "")
	fmt.Printf("  taux de remplacement à la retraite : %d lignes touchées par la fusion\n", n)
	return nil
}
