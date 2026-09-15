package sante

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La certification HAS des établissements de santé (6e cycle) : la seule
// mesure de qualité normalisée publiée en open data, par établissement.
// Trois fichiers CSV normalisés, tous petits (quelques centaines de Ko) —
// sans commune mesure avec la complexité de SAE ou de FINESS. Voir
// docs/sante-donnees.md § 4.
var SourceCertificationHAS = archive.Source{
	Slug: "has-certification-etablissements", Label: "HAS — certification des établissements de santé (6e cycle)",
	Publisher: "Haute Autorité de Santé", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : HAS, data.gouv.fr",
	Cadence:     "continue (au fil des visites de certification)",
	Notes: "Seul le 6e cycle (référentiel 2025-) est chargé, pas les cycles antérieurs " +
		"(publiés séparément, référentiel différent, non comparable terme à terme).",
}

const (
	hasDemarcheURL = "https://minio.data.has-sante.fr/etl-certification/data/prod/tables/demarche.csv"
	hasGeoURL      = "https://minio.data.has-sante.fr/etl-certification/data/prod/tables/etablissement_geo.csv"
	hasChapitreURL = "https://minio.data.has-sante.fr/etl-certification/data/prod/tables/resultat_chapitre.csv"
)

func IngestCertificationHAS(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCertificationHAS)
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

	fDemarche, err := arch.Fetch(ctx, srcID, runID, hasDemarcheURL, ".csv")
	if err != nil {
		return fail(fmt.Errorf("demarche : %w", err))
	}
	fGeo, err := arch.Fetch(ctx, srcID, runID, hasGeoURL, ".csv")
	if err != nil {
		return fail(fmt.Errorf("etablissement_geo : %w", err))
	}
	fChapitre, err := arch.Fetch(ctx, srcID, runID, hasChapitreURL, ".csv")
	if err != nil {
		return fail(fmt.Errorf("resultat_chapitre : %w", err))
	}

	geo, err := lireCSVMap(fGeo.Path)
	if err != nil {
		return fail(fmt.Errorf("etablissement_geo : %w", err))
	}
	// Un code_demarche apparaît une fois par établissement concerné par la
	// démarche ; le principal (Site_Principal=True) donne le FINESS de
	// référence quand plusieurs sites sont visités ensemble.
	finessParDemarche := map[string]struct{ finesset, finessej, rs string }{}
	for _, r := range geo {
		if _, deja := finessParDemarche[r["code_demarche"]]; deja && r["Site_Principal"] != "True" {
			continue
		}
		finessParDemarche[r["code_demarche"]] = struct{ finesset, finessej, rs string }{
			r["FINESS_EG"], r["FINESS_EJ"], r["RS_eg"],
		}
	}

	demarches, err := lireCSVMap(fDemarche.Path)
	if err != nil {
		return fail(fmt.Errorf("demarche : %w", err))
	}
	chapitres, err := lireCSVMap(fChapitre.Path)
	if err != nil {
		return fail(fmt.Errorf("resultat_chapitre : %w", err))
	}
	if len(demarches) == 0 {
		return fail(fmt.Errorf("HAS : aucune démarche lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.certification_has_chapitre`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.certification_has_demarche`); err != nil {
		return fail(err)
	}

	var rowsDemarche [][]any
	for _, d := range demarches {
		f := finessParDemarche[d["code_demarche"]]
		rowsDemarche = append(rowsDemarche, []any{
			d["code_demarche"], nilSiVide(f.finesset), nilSiVide(f.finessej), nilSiVide(f.rs),
			d["id_cycle"], d["id_version"],
			intOuNil(d["annee_visite"]), intOuNil(d["mois_visite"]),
			dateISOOuNil(d["date_de_decision"]),
			nilSiVide(d["Decision_de_la_CCES"]), srcID,
		})
	}
	n1, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "certification_has_demarche"},
		[]string{"code_demarche", "nofinesset", "nofinessej", "raison_sociale",
			"cycle", "version", "annee_visite", "mois_visite", "date_decision", "decision", "source_id"},
		pgx.CopyFromRows(rowsDemarche))
	if err != nil {
		return fail(fmt.Errorf("certification_has_demarche : %w", err))
	}

	var rowsChapitre [][]any
	for _, c := range chapitres {
		num, err := strconv.Atoi(c["id_chapitre"])
		if err != nil {
			continue
		}
		var score *float64
		if s := strings.ReplaceAll(strings.TrimSpace(c["moy_chapitre"]), ",", "."); s != "" {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				score = &v
			}
		}
		rowsChapitre = append(rowsChapitre, []any{c["code_demarche"], num, c["chapitre"], score, srcID})
	}
	n2, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "certification_has_chapitre"},
		[]string{"code_demarche", "chapitre_num", "chapitre_libelle", "score", "source_id"},
		pgx.CopyFromRows(rowsChapitre))
	if err != nil {
		return fail(fmt.Errorf("certification_has_chapitre : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"demarches": n1, "chapitres": n2}, "")
	fmt.Printf("  certification HAS : %d démarches, %d résultats de chapitre\n", n1, n2)
	return nil
}

// lireCSVMap lit un CSV avec en-tête en []map[string]string — le format le
// plus simple pour ces trois fichiers, tous petits (quelques centaines de Ko).
func lireCSVMap(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	entetes, err := r.Read()
	if err != nil {
		return nil, err
	}
	var out []map[string]string
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		m := make(map[string]string, len(entetes))
		for i, h := range entetes {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func dateISOOuNil(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func intOuNil(s string) *int {
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}
