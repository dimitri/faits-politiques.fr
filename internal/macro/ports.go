package macro

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceTraficPortuaire = archive.Source{
	Slug: "sdes-trafic-portuaire", Label: "Trafic maritime de marchandises par port français",
	Publisher: "SDES (ministère de la Transition écologique)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte 2.0", ReuseClass: "OPEN",
	Attribution: "Source : SDES, data.statistiques.developpement-durable.gouv.fr",
	Cadence:     "annuelle",
}

const urlTraficPortuaire = "https://data.statistiques.developpement-durable.gouv.fr/dido/api/v1/datafiles/89c4e831-27ae-4fe7-8ff7-d6c82fa4a841/csv"

func aInt(s string) any {
	if s == "" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return n
}

// IngestTraficPortuaire charge le trafic des ports français, par port,
// année et sens de circulation (SDES). Voir docs/ports-donnees.md.
func IngestTraficPortuaire(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTraficPortuaire)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "trafic-portuaire-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlTraficPortuaire, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.Comma = ';'
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête illisible : %w", err))
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, must := range []string{"LOCODE_PORT", "PORT", "FACADE", "REGION", "MOUV", "ANNEE",
		"TONNAGE_TOT", "VRACS_LIQUIDES", "VRACS_SOLIDES", "CONT_TOT", "EVP_TOT", "RORO_TOT"} {
		if _, ok := col[must]; !ok {
			return fail(fmt.Errorf("colonne %q absente", must))
		}
	}

	var rows [][]any
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("ligne illisible : %w", err))
		}
		annee, err := strconv.Atoi(rec[col["ANNEE"]])
		if err != nil {
			return fail(fmt.Errorf("année illisible : %q", rec[col["ANNEE"]]))
		}
		mouv := rec[col["MOUV"]]
		if mouv != "Entree" && mouv != "Sortie" {
			return fail(fmt.Errorf("mouvement inconnu : %q", mouv))
		}
		rows = append(rows, []any{
			rec[col["LOCODE_PORT"]], rec[col["PORT"]],
			nullifEmpty(rec[col["FACADE"]]), nullifEmpty(rec[col["REGION"]]),
			mouv, annee,
			aInt(rec[col["TONNAGE_TOT"]]), aInt(rec[col["VRACS_LIQUIDES"]]), aInt(rec[col["VRACS_SOLIDES"]]),
			aInt(rec[col["CONT_TOT"]]), aInt(rec[col["EVP_TOT"]]), aInt(rec[col["RORO_TOT"]]),
			srcID,
		})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.trafic_portuaire`); err != nil {
		return fail(err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.trafic_portuaire
				(locode, port, facade, region, mouvement, annee,
				 tonnage_tot, vracs_liquides, vracs_solides, cont_tot, evp_tot, roro_tot, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, row...); err != nil {
			return fail(fmt.Errorf("insertion %v : %w", row[:2], err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows)}, "")
	fmt.Printf("  Trafic portuaire français : %d lignes\n", len(rows))
	return nil
}

func nullifEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
