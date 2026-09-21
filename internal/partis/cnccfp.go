// Package partis ingère les référentiels sur les organisations politiques :
// comptes publics déposés à la CNCCFP, et classifications tierces.
package partis

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "partis/1"

// La CNCCFP est le registre de fait des partis politiques français : elle leur
// attribue un identifiant et publie leurs comptes d'ensemble. Il n'existe
// aucun registre des ADHÉRENTS — ce serait d'ailleurs illégal, l'appartenance
// partisane étant une opinion politique au sens de l'article 9 du RGPD.
var SourceCNCCFP = archive.Source{
	Slug: "cnccfp-comptes", Label: "CNCCFP — Comptes des partis et groupements politiques",
	Publisher: "Commission nationale des comptes de campagne et des financements politiques",
	Tier:      "PRIMARY_OFFICIAL", Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : CNCCFP, comptes des partis et groupements politiques",
	Cadence:     "annuelle",
	Notes: "CSV à séparateur point-virgule, BOM UTF-8, 166 colonnes. " +
		"Un parti absent d'un exercice n'a pas cessé d'exister : il peut ne pas avoir déposé.",
}

// Exercices disponibles au format CSV sur data.gouv.fr. Les exercices
// antérieurs ne sont publiés qu'en tableur ou en PDF et ne sont pas ingérés.
var ComptesURLs = map[int]string{
	2024: "https://static.data.gouv.fr/resources/comptes-des-partis-et-groupements-politiques/20260210-110641/comptes-partis-exercice-2024.csv",
	2023: "https://static.data.gouv.fr/resources/comptes-des-partis-et-groupements-politiques/20260210-120352/comptes-partis-exercice-2023.csv",
	2022: "https://static.data.gouv.fr/resources/comptes-des-partis-et-groupements-politiques/20260210-121141/comptes-partis-exercice-2022.csv",
	2021: "https://static.data.gouv.fr/resources/comptes-des-partis-et-groupements-politiques/20260210-151846/comptes-partis-exercice-2021.csv",
}

// IngestComptes télécharge, scelle et charge les comptes de chaque exercice.
func IngestComptes(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCNCCFP)
	if err != nil {
		return err
	}

	// Le catalogue dit quelle colonne CNCCFP alimente quel poste : la
	// sélection de 15 postes parmi 166 est un choix, il est donc explicite.
	postes := map[string]string{} // colonne source -> code de poste
	rows, err := pool.Query(ctx, `SELECT code, colonne_source FROM ref.party_account_poste`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var code, col string
		if err := rows.Scan(&code, &col); err != nil {
			return err
		}
		postes[col] = code
	}
	rows.Close()

	for exercice, url := range ComptesURLs {
		runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
		if err != nil {
			return err
		}
		f, err := arch.Fetch(ctx, srcID, runID, url, ".csv")
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return fmt.Errorf("comptes %d : %w", exercice, err)
		}
		n, err := loadComptes(ctx, pool, f.Path, exercice, srcID, postes)
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return fmt.Errorf("comptes %d : %w", exercice, err)
		}
		arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"partis": n}, "")
		logs.Notice(fmt.Sprintf("party accounts %d: %s", exercice, logs.Plural(n, "party")))
	}
	return nil
}

func loadComptes(ctx context.Context, pool *pgxpool.Pool, path string, exercice int,
	srcID int64, postes map[string]string) (int, error) {

	fh, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer fh.Close()

	r := csv.NewReader(fh)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return 0, err
	}
	head[0] = strings.TrimPrefix(head[0], "\ufeff") // BOM
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.TrimSpace(h)] = i
	}
	iCode, iNom := idx["Code_CNCCFP"], idx["Nom_du_parti"]
	if iCode == 0 && head[0] != "Code_CNCCFP" {
		return 0, fmt.Errorf("colonne Code_CNCCFP absente")
	}

	n := 0
	var lignes [][]any
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) <= iNom {
			continue
		}
		code := strings.TrimSpace(rec[iCode])
		nom := strings.TrimSpace(rec[iNom])
		if code == "" || nom == "" {
			continue
		}

		orgID, err := upsertParti(ctx, pool, code, nom)
		if err != nil {
			return 0, err
		}
		for col, poste := range postes {
			i, ok := idx[col]
			if !ok || i >= len(rec) {
				continue
			}
			v := strings.TrimSpace(strings.ReplaceAll(rec[i], ",", "."))
			if v == "" {
				continue // absence de valeur : jamais convertie en zéro
			}
			montant, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			lignes = append(lignes, []any{orgID, exercice, poste, montant})
		}
		n++
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_account_line (
			organization_id bigint, exercice int, poste text, montant numeric
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_account_line"},
		[]string{"organization_id", "exercice", "poste", "montant"},
		pgx.CopyFromRows(lignes)); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.party_account_line (organization_id, exercice, poste, montant, source_id)
		SELECT organization_id, exercice, poste, montant, $1 FROM tmp_account_line
		ON CONFLICT (organization_id, exercice, poste) DO UPDATE SET montant = EXCLUDED.montant`,
		srcID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

// upsertParti retrouve l'organisation par son identifiant CNCCFP, qui est
// stable, plutôt que par son nom, qui change.
func upsertParti(ctx context.Context, pool *pgxpool.Pool, code, nom string) (int64, error) {
	var orgID int64
	err := pool.QueryRow(ctx, `
		SELECT organization_id FROM core.organization_identifier
		WHERE scheme = 'CNCCFP' AND value = $1`, code).Scan(&orgID)
	if err == nil {
		return orgID, nil
	}

	slug := "parti-" + Slugify(nom)
	for i := 0; ; i++ {
		s := slug
		if i > 0 {
			s = fmt.Sprintf("%s-%s", slug, code)
		}
		err = pool.QueryRow(ctx, `
			INSERT INTO core.organization (slug, kind, name)
			VALUES ($1,'PARTY',$2)
			ON CONFLICT (slug) DO NOTHING
			RETURNING id`, s, nom).Scan(&orgID)
		if err == nil {
			break
		}
		if i > 0 {
			return 0, fmt.Errorf("parti %s (%s) : %w", nom, code, err)
		}
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO core.organization_identifier (organization_id, scheme, value)
		VALUES ($1,'CNCCFP',$2) ON CONFLICT (scheme, value) DO NOTHING`, orgID, code)
	return orgID, err
}
