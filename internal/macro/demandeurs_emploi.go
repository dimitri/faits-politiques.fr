package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les demandeurs d'emploi INSCRITS à France Travail — la mesure la plus citée
// dans le débat public français, distincte du taux de chômage au sens du BIT
// (core.chomage_taux_trimestriel, une enquête, pas une inscription
// administrative). Voir docs/chomage-donnees.md.
var SourceDemandeursEmploi = archive.Source{
	Slug: "dares-defm-categorie", Label: "Dares — demandeurs d'emploi inscrits par catégorie",
	Publisher: "Direction de l'animation de la recherche, des études et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Dares, France Travail",
	Cadence:     "mensuelle",
	Notes: "Données CVS-CJO (corrigées des variations saisonnières et du nombre de jours " +
		"ouvrés) : à comparer d'un mois à l'autre, pas pour un cumul annuel exact. Champ " +
		"agrégé (sexe, âge, ancienneté, tranche d'heures = Total) : la ventilation fine " +
		"existe dans la source mais n'est pas chargée.",
}

// L'API Opendatasoft de la Dares refuse offset+limit > 10 000, comme celles
// de la DREES et de data.economie.gouv.fr déjà rencontrées dans ce projet :
// l'export en un seul appel est le seul chemin correct pour ~10 500 lignes.
const demandeursEmploiURL = "https://data.dares.travail-emploi.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"dares_defm_stock_france_cvs/exports/json?where=" +
	"sexe%3D%22Total%22%20and%20tranche_d_age%3D%22Total%22%20and%20" +
	"tranche_d_heures_travaillees%3D%22Total%22%20and%20anciennete%3D%22Total%22"

type ligneDEFM struct {
	Date      string   `json:"date"`
	Champ     string   `json:"champ"`
	Categorie string   `json:"categorie"`
	Nombre    *float64 `json:"nombre_de_demandeurs_d_emploi"`
}

var champDEFM = map[string]string{"France": "FRANCE", "France métropolitaine": "FRANCE_METRO"}

func IngestDemandeursEmploi(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDemandeursEmploi)
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

	f, err := arch.Fetch(ctx, srcID, runID, demandeursEmploiURL, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var lignes []ligneDEFM
	if err := json.Unmarshal(raw, &lignes); err != nil {
		return fail(fmt.Errorf("export illisible : %w", err))
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("export vide"))
	}

	var rows [][]any
	var rejetChamp, rejetValeur int
	for _, l := range lignes {
		champ, ok := champDEFM[l.Champ]
		if !ok {
			rejetChamp++
			continue
		}
		if l.Nombre == nil {
			rejetValeur++
			continue
		}
		dateMois := l.Date + "-01"
		rows = append(rows, []any{dateMois, champ, l.Categorie, *l.Nombre, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne reconnue sur %d", len(lignes)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité de la série mensuelle à chaque republication,
	// changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_demandeur_emploi_categorie (
			date_mois date, champ text, categorie text, effectif numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_demandeur_emploi_categorie"},
		[]string{"date_mois", "champ", "categorie", "effectif", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("demandeur_emploi_categorie : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.demandeur_emploi_categorie AS tgt
		USING tmp_demandeur_emploi_categorie AS src
		ON tgt.date_mois = src.date_mois AND tgt.champ = src.champ AND tgt.categorie = src.categorie
		WHEN MATCHED AND (tgt.effectif, tgt.source_id) IS DISTINCT FROM (src.effectif, src.source_id) THEN
		    UPDATE SET effectif = src.effectif, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (date_mois, champ, categorie, effectif, source_id)
		    VALUES (src.date_mois, src.champ, src.categorie, src.effectif, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": len(rows), "rejet_champ_inconnu": rejetChamp,
			"rejet_sans_valeur": rejetValeur, "touchees": n}, "")
	fmt.Printf("  demandeurs d'emploi inscrits par catégorie : %d lignes (%d touchées par la fusion)\n", len(rows), n)
	return nil
}
