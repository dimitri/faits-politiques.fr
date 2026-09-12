package main

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/media"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ingestMedia lit les pages Wikipédia de référence — une décision éditoriale,
// donc dans data/ — et récupère les portraits et logos librement réutilisables.
func ingestMedia(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive,
	dataDir, mediaDir string) error {

	var cibles []media.Cible

	cand, err := readCSV(filepath.Join(dataDir, "candidats.csv"))
	if err != nil {
		return err
	}
	for _, row := range cand {
		page := row["wikipedia_fr"]
		if page == "" {
			continue
		}
		// Un candidat est une personne, qu'il ait siégé ou non. La fiche est
		// créée si elle n'existe pas, à partir de la déclaration de candidature
		// — c'est la source, et elle est citée sur la page. Ces personnes ne
		// portent pas d'identifiant de l'Assemblée : la normalisation de l'AN,
		// qui n'efface que ce qu'elle possède, ne les touche pas.
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO core.person (slug, family_name, given_name)
			VALUES ($1,$2,$3)
			ON CONFLICT (slug) DO UPDATE SET family_name = EXCLUDED.family_name
			RETURNING id`, row["slug"], row["nom"], row["prenom"]).Scan(&id); err != nil {
			return err
		}
		cibles = append(cibles, media.Cible{PersonID: &id, Kind: "PORTRAIT",
			PageFR: page, Libelle: row["prenom"] + " " + row["nom"]})
	}

	orgs, err := readCSV(filepath.Join(dataDir, "organisations.csv"))
	if err != nil {
		return err
	}
	for _, row := range orgs {
		page, code := row["wikipedia_fr"], row["code_cnccfp"]
		if page == "" || code == "" {
			continue
		}
		var id int64
		if err := pool.QueryRow(ctx, `
			SELECT organization_id FROM core.organization_identifier
			WHERE scheme = 'CNCCFP' AND value = $1`, code).Scan(&id); err != nil {
			continue
		}
		cibles = append(cibles, media.Cible{OrganizationID: &id, Kind: "LOGO",
			PageFR: page, Libelle: row["libelle"]})
	}

	return media.Ingest(ctx, pool, arch, mediaDir, cibles)
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, nil
	}
	var out []map[string]string
	for _, rec := range recs[1:] {
		m := map[string]string{}
		for i, h := range recs[0] {
			if i < len(rec) {
				m[strings.TrimSpace(h)] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}
