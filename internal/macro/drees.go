package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La DREES publie le suivi mensuel des prestations de solidarité : combien de
// personnes dépendent du RSA, de l'AAH, de la prime d'activité ou de l'ASS, et
// dans quel département. C'est le pendant territorial et mensuel des comptes de
// la protection sociale, qui sont nationaux et annuels.
var SourceDreesSolidarityBenefits = archive.Source{
	Slug: "drees-prestations-solidarite", Label: "DREES — suivi mensuel des prestations de solidarité",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : DREES, suivi mensuel des prestations de solidarité",
	Cadence:     "mensuelle",
	Notes: "Trois échelles géographiques cohabitent dans le même fichier (France, " +
		"région, département) : sommer sans filtrer compterait chaque allocataire " +
		"trois fois. La colonne maturite distingue les valeurs définitives des " +
		"valeurs provisoires, que la DREES révise ensuite.",
}

// L'API Opendatasoft refuse offset + limit > 10 000 et renvoie un HTTP 400.
// Ce jeu en compte plus de 120 000 : la pagination échouerait à mi-parcours,
// silencieusement si l'on ne vérifiait pas le code de retour. L'export en un
// seul appel est le seul chemin correct.
const dreesBenefitsURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/donnees-mensuelles-sur-les-prestations-de-solidarite/exports/json"

type dreesBenefitRow struct {
	Month          string   `json:"mois"`
	Series         string   `json:"serie"`
	SeriesName     string   `json:"nom_serie"`
	RegionName     string   `json:"nom_region"`
	Region         string   `json:"region"`
	DepartmentName string   `json:"nom_departement"`
	Department     string   `json:"departement"`
	Value          *float64 `json:"valeur"`
	Maturity       string   `json:"maturite"`
	Unit           string   `json:"unite"`
	Comment        string   `json:"commentaire"`
}

func IngestSolidarityBenefits(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDreesSolidarityBenefits)
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

	f, err := arch.Fetch(ctx, srcID, runID, dreesBenefitsURL, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var records []dreesBenefitRow
	if err := json.Unmarshal(raw, &records); err != nil {
		return fail(fmt.Errorf("export DREES illisible : %w", err))
	}
	if len(records) == 0 {
		return fail(fmt.Errorf("export DREES vide"))
	}

	var rows [][]any
	seen := map[string]bool{}
	var noValue, noDate, duplicates int
	for _, rec := range records {
		// Une valeur absente n'est pas un zéro : la DREES ne publie pas toujours
		// le détail départemental d'une série. On l'écarte plutôt que de la
		// transformer en « aucun allocataire ».
		if rec.Value == nil {
			noValue++
			continue
		}
		// La source date au mois, pas au jour : « 2026-06 ». On ancre au
		// premier du mois plutôt que d'inventer une date de publication.
		month, err := time.Parse("2006-01", rec.Month)
		if err != nil {
			if month, err = time.Parse("2006-01-02", rec.Month); err != nil {
				noDate++
				continue
			}
		}
		// Le niveau se déduit des colonnes renseignées : la source ne le donne
		// pas explicitement, mais un département implique une région, et une
		// ligne sans l'un ni l'autre est nationale.
		// Piège : la DREES écrit « NA » — la chaîne, pas une valeur vide —
		// quand le découpage ne descend pas au département. Traiter « NA »
		// comme un code de département écrase toutes les régions sur une
		// seule clé : 6 604 lignes disparaissaient silencieusement avant
		// qu'on le voie. Une valeur manquante déguisée en valeur est le
		// piège le plus coûteux d'un fichier statistique.
		dept, region := absent(rec.Department), absent(rec.Region)
		level, code, name := "NATIONAL", "FR", "France"
		switch {
		case dept != "":
			level, code, name = "DEPARTEMENT", dept, rec.DepartmentName
		case region != "":
			level, code, name = "REGION", region, rec.RegionName
		}
		if name == "" {
			name = code
		}
		key := rec.Series + "|" + rec.Month + "|" + level + "|" + code
		if seen[key] {
			duplicates++
			continue
		}
		seen[key] = true
		rows = append(rows, []any{
			rec.Series, rec.SeriesName, month, level, code, name, *rec.Value,
			nilIfEmpty(rec.Unit), nilIfEmpty(rec.Maturity), nilIfEmpty(rec.Comment), srcID,
		})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Reconstruction : la DREES révise ses valeurs provisoires, donc compléter
	// laisserait cohabiter deux millésimes du même mois.
	if _, err := tx.Exec(ctx, `TRUNCATE core.prestation_solidarite`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "prestation_solidarite"},
		[]string{"serie", "nom_serie", "mois", "niveau", "code_geo", "nom_geo",
			"valeur", "unite", "maturite", "commentaire", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	// Convention de comptage, lue par cmd/verify : toute ligne reçue est soit
	// chargée, soit rejetée sous un motif nommé `rejet_*`. La somme doit
	// refermer. C'est ce contrôle qui aurait attrapé, dès le premier
	// chargement, les 6 604 lignes que le marqueur « NA » faisait disparaître :
	// elles n'étaient ni chargées ni rejetées, elles s'évaporaient.
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes_recues":     len(records),
		"lignes_chargees":   len(rows),
		"rejet_sans_valeur": noValue,
		"rejet_sans_date":   noDate,
		"rejet_doublon_cle": duplicates,
	}, "")
	fmt.Printf("  prestations de solidarité : %d lignes (%d sans valeur, %d sans date)\n",
		len(rows), noValue, noDate)
	return nil
}

// absent rend la chaîne vide pour tout ce que la source utilise comme marqueur
// de valeur manquante : le vide lui-même, et le « NA » littéral.
func absent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" || s == "na" {
		return ""
	}
	return s
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
