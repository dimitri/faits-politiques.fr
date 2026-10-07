// Package jeunesse charge les données propres au dossier jeunesse
// (docs/jeunesse-donnees.md) : l'accompagnement de la jeunesse en miroir
// du dossier vieillesse — études supérieures, apprentissage, premiers
// emplois.
package jeunesse

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InserJeunes : le devenir des apprentis, la seule mesure de ce dépôt qui
// répond à « l'apprentissage mène-t-il à un emploi » plutôt que « combien
// d'apprentis ». Voir le commentaire de la migration 0113 pour la limite
// réelle (aucun effectif par CFA, donc aucune moyenne nationale pondérée
// possible depuis ce seul jeu).
var SourceInserJeunes = archive.Source{
	Slug: "depp-inserjeunes-apprentissage", Label: "DEPP — InserJeunes, insertion des apprentis par CFA",
	Publisher: "Direction de l'évaluation, de la prospective et de la performance (DEPP)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : DEPP, enquête InserJeunes",
	Cadence:     "annuelle",
	Notes: "Six promotions cumulées (2018-2019 à 2023-2024), par CFA et niveau de formation. Aucun " +
		"effectif publié par CFA : ne permet pas de calculer un taux national pondéré, seulement des " +
		"statistiques descriptives sur les taux eux-mêmes.",
}

const inserJeunesURL = "https://data.education.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"fr-en-inserjeunes-cfa-niveau_formation/exports/json?limit=-1"

func IngestInserJeunes(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceInserJeunes)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "vieillesse-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, inserJeunesURL, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var records []struct {
		Year                     string   `json:"annee"`
		UAI                      string   `json:"uai"`
		Label                    string   `json:"libelle"`
		Region                   string   `json:"region"`
		TrainingLevel            string   `json:"niveau_formation"`
		ContinuingStudiesRate    *float64 `json:"taux_poursuite_etudes"`
		EmploymentRate6Months    *float64 `json:"taux_emploi_6_mois"`
		TrainingInterruptionRate *float64 `json:"taux_interruption_formation"`
		InterruptedContractsRate *float64 `json:"taux_contrats_interrompus"`
		EmploymentRate12Months   *float64 `json:"taux_emploi_12_mois"`
		EmploymentRate18Months   *float64 `json:"taux_emploi_18_mois"`
		EmploymentRate24Months   *float64 `json:"taux_emploi_24_mois"`
	}
	if err := json.Unmarshal(b, &records); err != nil {
		return fail(fmt.Errorf("inserjeunes : %w", err))
	}
	if len(records) == 0 {
		return fail(fmt.Errorf("inserjeunes : aucune ligne"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE core.insertion_apprentissage`); err != nil {
		return fail(err)
	}

	// Une ligne peut apparaître plusieurs fois pour le même (année, UAI,
	// niveau) dans l'export brut (regroupements de spécialités distincts
	// remontés sous le même niveau) — la première rencontrée est conservée,
	// les suivantes ignorées plutôt que de faire échouer tout le chargement
	// sur une contrainte d'unicité.
	seen := map[[3]string]bool{}
	var rows [][]any
	for _, l := range records {
		if l.UAI == "" || l.TrainingLevel == "" {
			continue
		}
		key := [3]string{l.Year, l.UAI, l.TrainingLevel}
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, []any{l.Year, l.UAI, l.Label, l.Region, l.TrainingLevel,
			l.ContinuingStudiesRate, l.EmploymentRate6Months, l.TrainingInterruptionRate,
			l.InterruptedContractsRate, l.EmploymentRate12Months, l.EmploymentRate18Months, l.EmploymentRate24Months, srcID})
	}

	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "insertion_apprentissage"},
		[]string{"annee_cumul", "uai", "libelle_etablissement", "region", "niveau_formation",
			"taux_poursuite_etudes", "taux_emploi_6_mois", "taux_interruption_formation",
			"taux_contrats_interrompus", "taux_emploi_12_mois", "taux_emploi_18_mois", "taux_emploi_24_mois", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("insertion_apprentissage : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  InserJeunes apprentissage (DEPP) : %d lignes\n", n)
	return nil
}
