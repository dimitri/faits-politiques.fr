package sante

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// « Comment les médecins sont rémunérés » commence ici : le secteur
// conventionnel (1, 2, 2 Optam, non conventionné) détermine si le tarif est
// fixé par l'Assurance Maladie ou libre. Voir docs/sante-donnees.md § 2.
var SourceSecteurConventionnel = archive.Source{
	Slug: "ameli-secteurs-conventionnels", Label: "Ameli — professionnels de santé libéraux par secteur conventionnel",
	Publisher: "Caisse nationale de l'Assurance Maladie (Cnam)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Cnam, data.ameli.fr",
	Cadence:     "annuelle",
	Notes: "Toutes les professions de santé libérales sont chargées, pas seulement les " +
		"médecins — même nomenclature de secteur pour toutes. Contient des lignes d'agrégat " +
		"(région « FRANCE », département « 999 — Tout département ») mêlées aux lignes de " +
		"détail : ne jamais sommer sans filtrer, voir le commentaire de la table.",
}

const secteurConventionnelURL = "https://data.ameli.fr/api/explore/v2.1/catalog/datasets/" +
	"demographie-secteurs-conventionnels/exports/json"

type ligneSecteur struct {
	Annee              string `json:"annee"`
	ProfessionSante    string `json:"profession_sante"`
	Region             string `json:"region"`
	LibelleRegion      string `json:"libelle_region"`
	Departement        string `json:"departement"`
	LibelleDepartement string `json:"libelle_departement"`
	SecteurCode        string `json:"secteur_conventionnel"`
	SecteurLibelle     string `json:"libelle_secteur_conventionnel"`
	Effectif           int    `json:"effectif"`
}

func IngestSecteurConventionnel(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSecteurConventionnel)
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

	f, err := arch.Fetch(ctx, srcID, runID, secteurConventionnelURL, ".json")
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var lignes []ligneSecteur
	if err := json.Unmarshal(b, &lignes); err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("secteurs conventionnels : export vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	var rows [][]any
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee)
		if err != nil {
			return fail(fmt.Errorf("année illisible %q", l.Annee))
		}
		rows = append(rows, []any{
			annee, l.ProfessionSante, l.Region, l.LibelleRegion,
			l.Departement, l.LibelleDepartement, l.SecteurCode, l.SecteurLibelle,
			l.Effectif, srcID,
		})
	}
	// MERGE plutôt que DELETE+COPY : table entière, ce connecteur en est
	// l'unique propriétaire ; l'ancien DELETE payait le prix des triggers RI
	// pour l'intégralité de la table à chaque republication annuelle,
	// changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_medecin_secteur_effectif (
			annee smallint, profession_sante text, code_region text, libelle_region text,
			code_departement text, libelle_departement text, secteur_code text, secteur_libelle text,
			effectif int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_medecin_secteur_effectif"},
		[]string{"annee", "profession_sante", "code_region", "libelle_region",
			"code_departement", "libelle_departement", "secteur_code", "secteur_libelle",
			"effectif", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("medecin_secteur_effectif : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.medecin_secteur_effectif AS tgt
		USING tmp_medecin_secteur_effectif AS src
		ON tgt.annee = src.annee AND tgt.profession_sante = src.profession_sante
		   AND tgt.code_region = src.code_region AND tgt.code_departement = src.code_departement
		   AND tgt.secteur_code = src.secteur_code
		WHEN MATCHED AND (tgt.libelle_region, tgt.libelle_departement, tgt.secteur_libelle,
		                   tgt.effectif, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.libelle_region, src.libelle_departement, src.secteur_libelle,
		                   src.effectif, src.source_id) THEN
		    UPDATE SET libelle_region = src.libelle_region, libelle_departement = src.libelle_departement,
		               secteur_libelle = src.secteur_libelle, effectif = src.effectif, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, profession_sante, code_region, libelle_region, code_departement,
		            libelle_departement, secteur_code, secteur_libelle, effectif, source_id)
		    VALUES (src.annee, src.profession_sante, src.code_region, src.libelle_region, src.code_departement,
		            src.libelle_departement, src.secteur_code, src.secteur_libelle, src.effectif, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("medecin_secteur_effectif : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	n := ct.RowsAffected()
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows), "touchees": n}, "")
	fmt.Printf("  secteurs conventionnels (Ameli) : %d lignes (%d touchées par la fusion)\n", len(rows), n)
	return nil
}
