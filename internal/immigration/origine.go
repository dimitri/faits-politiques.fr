package immigration

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const melodiOrigineURL = "https://api.insee.fr/melodi/data/DS_RP_TD_IMMI_AGESEX_PAYSNAISS_R_PRINC" +
	"?GEO=FRANCE-FM&maxResult=10000"

// paysLib : le regroupement Insee des pays de naissance — les pays qui pèsent
// individuellement (Algérie, Maroc, Tunisie, Italie, Espagne, Portugal,
// Turquie) et cinq agrégats pour le reste. Ce n'est pas la liste des 195 pays
// du monde : c'est celle que l'Insee choisit de publier à ce niveau de détail.
var paysLib = map[string]string{
	"12": "Algérie", "504": "Maroc", "788": "Tunisie", "380": "Italie",
	"620": "Portugal", "724": "Espagne", "792": "Turquie",
	"EUR_OTH": "Autres pays d'Europe", "UE27_OTH": "Autres pays de l'Union européenne",
	"AFR_OTH": "Autres pays d'Afrique", "ROW": "Reste du monde", "_T": "Total",
}

func IngestOrigine(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMelodiImmigration)
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

	obs, err := melodiLire(ctx, arch, srcID, runID, melodiOrigineURL)
	if err != nil {
		return fail(err)
	}

	var rows [][]any
	var rejets int
	for _, o := range obs {
		annee, err := strconv.Atoi(o.Dimensions["TIME_PERIOD"])
		if err != nil {
			rejets++
			continue
		}
		sexe, okS := sexeLib[o.Dimensions["SEX"]]
		age, okA := ageLib[o.Dimensions["AGE"]]
		pays := o.Dimensions["AREA_COUNTRY"]
		lib, okP := paysLib[pays]
		if !okS || !okA || !okP {
			rejets++
			continue
		}
		rows = append(rows, []any{annee, sexe, age, pays, lib, o.Measures.OBSVALUENIVEAU.Value, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne reconnue sur %d observations", len(obs)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.population_immigree_origine`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "population_immigree_origine"},
		[]string{"annee", "sexe", "age_tranche", "pays_code", "pays_libelle", "population", "source_id"},
		pgx.CopyFromRows(dedupe(rows)))
	if err != nil {
		return fail(fmt.Errorf("population_immigree_origine : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": n, "rejet_dimension_inconnue": rejets}, "")
	fmt.Printf("  population immigrée par pays de naissance : %d lignes (%d rejetées)\n", n, rejets)
	return nil
}
