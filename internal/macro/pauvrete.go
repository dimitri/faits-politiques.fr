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
var SourcePauvreteINSEE = archive.Source{
	Slug: "insee-pauvrete-niveau-de-vie", Label: "Insee — Niveau de vie et pauvreté",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, enquêtes Revenus fiscaux et sociaux",
	Cadence:     "annuelle",
	Notes: "Refonte de l'enquête ERFS en 2021 : les niveaux publiés depuis ne sont " +
		"pas directement comparables à ceux d'avant. La valeur 2020 est publiée mais " +
		"signalée fragile par l'Insee (difficultés de collecte pendant le confinement).",
}

const pauvreteXLSXURL = "https://www.insee.fr/fr/statistiques/fichier/8600989/ip2063.xlsx"

func IngestPauvrete(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePauvreteINSEE)
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

	f, err := arch.Fetch(ctx, srcID, runID, pauvreteXLSXURL, ".xlsx")
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
	lignes, err := x.rows("Tableau complémentaire 3")
	if err != nil {
		return fail(err)
	}
	annees, err := anneesEnTete(lignes)
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.pauvrete_seuil_annuel`); err != nil {
		return fail(err)
	}

	var rows [][]any
	var relatifCourant float64
	acc := map[float64]map[int]map[string]float64{0.6: {}, 0.5: {}}
	for _, l := range lignes {
		lib := l["A"]
		switch lib {
		case "Seuil à 60 % de la médiane":
			relatifCourant = 0.6
			continue
		case "Seuil à 50 % de la médiane":
			relatifCourant = 0.5
			continue
		}
		if relatifCourant == 0 || lib == "" {
			continue
		}
		var champ string
		switch {
		case lib == "Nombre de personnes pauvres (en milliers)":
			champ = "nb"
		case lib == "Taux de pauvreté (en %)":
			champ = "taux"
		case lib == "Seuil de pauvreté (en euros constants de 2023 par mois)":
			champ = "seuil"
		case lib == "Intensité de la pauvreté (en %)":
			champ = "intensite"
		default:
			continue
		}
		for col, an := range annees {
			v, ok := l[col]
			if !ok {
				continue
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			if acc[relatifCourant][an] == nil {
				acc[relatifCourant][an] = map[string]float64{}
			}
			acc[relatifCourant][an][champ] = f
		}
	}

	var manquants int
	for relatif, parAnnee := range acc {
		for an, champs := range parAnnee {
			seuil, okS := champs["seuil"]
			nb, okN := champs["nb"]
			taux, okT := champs["taux"]
			intensite, okI := champs["intensite"]
			if !okS || !okN || !okT || !okI {
				manquants++
				continue
			}
			// La source publie ce nombre en milliers (ex. 9792 = 9 792 000
			// personnes) : la colonne porte l'unité dans son nom, la valeur
			// n'est pas reconvertie.
			rows = append(rows, []any{an, relatif, seuil, int64(nb), taux, intensite, srcID})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne extraite du tableau complémentaire 3"))
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pauvrete_seuil_annuel"},
		[]string{"annee", "seuil_relatif", "seuil_euros", "nb_pauvres_milliers", "taux_pauvrete_pct",
			"intensite_pauvrete_pct", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("insertion pauvrete_seuil_annuel : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": len(rows), "rejet_annee_incomplete": manquants}, "")
	fmt.Printf("  seuil de pauvreté : %d lignes (60%% et 50%% de la médiane, 1996-2023)\n", len(rows))
	return nil
}

// anneesEnTete repère, dans une feuille où les années forment une ligne
// (pas la première : les classeurs Insee font précéder les données d'un titre
// et d'une sous-légende), la correspondance colonne -> année à quatre chiffres.
func anneesEnTete(lignes []map[string]string) (map[string]int, error) {
	for _, l := range lignes {
		out := map[string]int{}
		for col, v := range l {
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
