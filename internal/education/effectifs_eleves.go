package education

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les effectifs d'élèves du premier degré, une donnée citée dans
// docs/education-donnees.md § 2 mais jamais chargée avant ce chargement —
// nécessaire pour confirmer si la légère baisse d'ETP enseignants suit la
// démographie scolaire ou s'en écarte.
var SourceEffectifsEleves = archive.Source{
	Slug: "depp-effectifs-eleves-premier-degre", Label: "Depp — effectifs d'élèves des écoles (premier degré)",
	Publisher: "Direction de l'évaluation, de la prospective et de la performance (Depp)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : ministère de l'Éducation nationale (Depp)",
	Cadence:     "annuelle (à la rentrée scolaire)",
	Notes: "Premier degré seulement (écoles maternelles et élémentaires) — ne couvre pas les " +
		"collèges et lycées. Agrégat national par secteur et rentrée scolaire, calculé " +
		"côté serveur (group_by) plutôt qu'en téléchargeant les 859 000 lignes par école, " +
		"puisque seul le total nourrit la question posée (docs/education-donnees.md § 2).",
}

const effectifsElevesURL = "https://data.education.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"fr-en-ecoles-effectifs-nb_classes/records?select=rentree_scolaire,secteur," +
	"sum(nombre_total_eleves)%20as%20total_eleves,count(*)%20as%20nb_ecoles" +
	"&group_by=rentree_scolaire,secteur&limit=100"

func IngestEffectifsEleves(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEffectifsEleves)
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

	f, err := arch.Fetch(ctx, srcID, runID, effectifsElevesURL, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var rep struct {
		TotalCount int `json:"total_count"`
		Results    []struct {
			RentreeScolaire string  `json:"rentree_scolaire"`
			Secteur         string  `json:"secteur"`
			TotalEleves     float64 `json:"total_eleves"`
			NbEcoles        int     `json:"nb_ecoles"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &rep); err != nil {
		return fail(err)
	}
	if len(rep.Results) == 0 {
		return fail(fmt.Errorf("effectifs élèves : réponse vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.education_effectif_eleves`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, r := range rep.Results {
		annee, err := strconv.Atoi(strings.SplitN(r.RentreeScolaire, "-", 2)[0])
		if err != nil {
			return fail(fmt.Errorf("rentrée scolaire %q : %w", r.RentreeScolaire, err))
		}
		rows = append(rows, []any{annee, r.Secteur, r.NbEcoles, int64(r.TotalEleves), srcID})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "education_effectif_eleves"},
		[]string{"annee", "secteur", "nombre_ecoles", "nombre_eleves", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("education_effectif_eleves : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  effectifs d'élèves, premier degré (Depp) : %d lignes\n", n)
	return nil
}
