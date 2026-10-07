package macro

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le seuil de pauvreté, chargé pour lui-même — pas seulement pour la
// simulation de docs/revenu-universel-microsimulation.md qui s'en sert comme
// unité de socle. C'est l'Insee Première le plus lu de l'année, et sa série
// longue (1996-2023) ne demandait qu'à être chargée une fois disponible.
var SourcePovertyINSEE = archive.Source{
	Slug: "insee-pauvrete-niveau-de-vie", Label: "Insee — Niveau de vie et pauvreté",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	License: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, enquêtes Revenus fiscaux et sociaux",
	Cadence:     "annuelle",
	Notes: "Refonte de l'enquête ERFS en 2021 : les niveaux publiés depuis ne sont " +
		"pas directement comparables à ceux d'avant. La valeur 2020 est publiée mais " +
		"signalée fragile par l'Insee (difficultés de collecte pendant le confinement).",
}

const povertyXLSXURL = "https://www.insee.fr/fr/statistiques/fichier/8600989/ip2063.xlsx"

func IngestPoverty(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePovertyINSEE)
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

	f, err := arch.Fetch(ctx, srcID, runID, povertyXLSXURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	// « Tableau complémentaire 3 » : une ligne par indicateur, une colonne par
	// année de 1996 à 2023, pour les deux seuils (60 % et 50 % de la médiane).
	// La feuille ne porte pas d'en-tête de colonne exploitable (les années sont
	// en ligne 3) : on repère chaque ligne par le début de son libellé en
	// colonne A, méthode déjà utilisée par internal/presidentielle pour les
	// mêmes classeurs Insee.
	sheetRows, err := x.rows("Tableau complémentaire 3")
	if err != nil {
		return fail(err)
	}
	yearByCol, err := headerYears(sheetRows)
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_pauvrete_seuil_annuel (
			annee int, seuil_relatif numeric, seuil_euros numeric, nb_pauvres_milliers int,
			taux_pauvrete_pct numeric, intensite_pauvrete_pct numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var rows [][]any
	var currentRelative float64
	byThreshold := map[float64]map[int]map[string]float64{0.6: {}, 0.5: {}}
	for _, row := range sheetRows {
		label := row["A"]
		switch label {
		case "Seuil à 60 % de la médiane":
			currentRelative = 0.6
			continue
		case "Seuil à 50 % de la médiane":
			currentRelative = 0.5
			continue
		}
		if currentRelative == 0 || label == "" {
			continue
		}
		var field string
		switch {
		case label == "Nombre de personnes pauvres (en milliers)":
			field = "nb"
		case label == "Taux de pauvreté (en %)":
			field = "taux"
		case label == "Seuil de pauvreté (en euros constants de 2023 par mois)":
			field = "seuil"
		case label == "Intensité de la pauvreté (en %)":
			field = "intensite"
		default:
			continue
		}
		for col, year := range yearByCol {
			v, ok := row[col]
			if !ok {
				continue
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			if byThreshold[currentRelative][year] == nil {
				byThreshold[currentRelative][year] = map[string]float64{}
			}
			byThreshold[currentRelative][year][field] = f
		}
	}

	var missing int
	for threshold, byYear := range byThreshold {
		for year, fields := range byYear {
			thresholdEUR, okS := fields["seuil"]
			count, okN := fields["nb"]
			rate, okT := fields["taux"]
			intensity, okI := fields["intensite"]
			if !okS || !okN || !okT || !okI {
				missing++
				continue
			}
			// La source publie ce nombre en milliers (ex. 9792 = 9 792 000
			// personnes) : la colonne porte l'unité dans son nom, la valeur
			// n'est pas reconvertie.
			rows = append(rows, []any{year, threshold, thresholdEUR, int64(count), rate, intensity, srcID})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne extraite du tableau complémentaire 3"))
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_pauvrete_seuil_annuel"},
		[]string{"annee", "seuil_relatif", "seuil_euros", "nb_pauvres_milliers", "taux_pauvrete_pct",
			"intensite_pauvrete_pct", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("insertion pauvrete_seuil_annuel : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la série 1996-2023 à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.pauvrete_seuil_annuel AS tgt
		USING tmp_pauvrete_seuil_annuel AS src
		ON tgt.annee = src.annee AND tgt.seuil_relatif = src.seuil_relatif
		WHEN MATCHED AND (tgt.seuil_euros, tgt.nb_pauvres_milliers, tgt.taux_pauvrete_pct,
		                   tgt.intensite_pauvrete_pct, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.seuil_euros, src.nb_pauvres_milliers, src.taux_pauvrete_pct,
		                   src.intensite_pauvrete_pct, src.source_id) THEN
		    UPDATE SET seuil_euros = src.seuil_euros, nb_pauvres_milliers = src.nb_pauvres_milliers,
		               taux_pauvrete_pct = src.taux_pauvrete_pct,
		               intensite_pauvrete_pct = src.intensite_pauvrete_pct, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, seuil_relatif, seuil_euros, nb_pauvres_milliers, taux_pauvrete_pct,
		            intensite_pauvrete_pct, source_id)
		    VALUES (src.annee, src.seuil_relatif, src.seuil_euros, src.nb_pauvres_milliers,
		            src.taux_pauvrete_pct, src.intensite_pauvrete_pct, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion pauvrete_seuil_annuel : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": len(rows), "rejet_annee_incomplete": missing, "touchees": touchees}, "")
	fmt.Printf("  seuil de pauvreté : %d lignes (60%% et 50%% de la médiane, 1996-2023), %d touchées par la fusion\n",
		len(rows), touchees)
	return nil
}

// headerYears repère, dans une feuille où les années forment une ligne
// (pas la première : les classeurs Insee font précéder les données d'un titre
// et d'une sous-légende), la correspondance colonne -> année à quatre chiffres.
func headerYears(rows []map[string]string) (map[string]int, error) {
	for _, row := range rows {
		out := map[string]int{}
		for col, v := range row {
			if col == "A" {
				continue
			}
			if n, err := strconv.Atoi(v); err == nil && n >= 1990 && n <= 2100 {
				out[col] = n
			}
		}
		if len(out) >= 10 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("ligne d'en-tête des années introuvable")
}
