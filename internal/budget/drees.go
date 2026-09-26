package budget

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les comptes de la protection sociale sont la seule série longue du champ
// social en données ouvertes — 1959 → 2024, soixante-six millésimes. Le budget
// le plus lourd des deux est le moins documenté : c'est un fait sur l'état de
// l'open data français, pas une lacune de recherche.
var SourceDREES = archive.Source{
	Slug:        "drees-comptes-protection-sociale",
	Label:       "DREES — Les comptes de la protection sociale",
	Publisher:   "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : DREES, comptes de la protection sociale",
	Cadence:     "annuelle",
	Notes: "Périmètre PROTECTION SOCIALE, plus large que la loi de financement de la " +
		"sécurité sociale : l'assurance chômage et les retraites complémentaires y sont " +
		"comptées. Rapprocher ces montants d'un solde de LFSS serait une faute. " +
		"Le jeu est HIÉRARCHIQUE sur deux axes (ps_niveau 0-4, si_niveau 0-2) : sommer " +
		"toutes les lignes compte chaque euro plusieurs fois. " +
		"L'API Opendatasoft refuse offset + limit > 10 000 ; le chargement passe par " +
		"/exports/json, qui rend les 15 654 lignes en un appel.",
}

type ligneDREES struct {
	Annee     string   `json:"annee"`
	PsNiveau  string   `json:"ps_niveau"`
	PsCode    string   `json:"ps_code"`
	PsLib     string   `json:"ps_lib"`
	Risque    string   `json:"risque"`
	NomRegime string   `json:"nom_regime"`
	SiNiveau  string   `json:"si_niveau"`
	SiCode    string   `json:"si_code"`
	SiNom     string   `json:"si_nom"`
	Val       *float64 `json:"val"`
}

func IngestProtectionSociale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDREES)
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

	f, err := arch.Fetch(ctx, srcID, runID,
		exportJSON("data.drees.solidarites-sante.gouv.fr", "305_les-comptes-de-la-protection-sociale"), ".json")
	if err != nil {
		return fail(err)
	}
	var lignes []ligneDREES
	if err := lireJSON(f.Path, &lignes); err != nil {
		return fail(err)
	}
	// Une source qui rend zéro ligne sans erreur HTTP est le cas le plus
	// dangereux : le chargement « réussit » et vide la table. Il échoue ici.
	if len(lignes) == 0 {
		return fail(fmt.Errorf("DREES : export vide — le jeu a changé d'identifiant ou l'API a répondu à côté"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	rows := make([][]any, 0, len(lignes))
	var ignorees int
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee)
		if err != nil {
			return fail(fmt.Errorf("DREES : année illisible %q", l.Annee))
		}
		psn, err := strconv.Atoi(l.PsNiveau)
		if err != nil {
			return fail(fmt.Errorf("DREES : ps_niveau illisible %q", l.PsNiveau))
		}
		sin, err := strconv.Atoi(l.SiNiveau)
		if err != nil {
			return fail(fmt.Errorf("DREES : si_niveau illisible %q", l.SiNiveau))
		}
		if l.Val == nil {
			// Rien n'est inventé : une valeur absente ne devient pas zéro, la
			// ligne n'est simplement pas chargée, et le compte est affiché.
			ignorees++
			continue
		}
		rows = append(rows, []any{
			annee, psn, l.PsCode, l.PsLib, l.Risque,
			sin, l.SiCode, l.SiNom, l.NomRegime, *l.Val,
			"PROTECTION_SOCIALE", "NATIONALE", "EXECUTION", srcID, f.DocumentID,
		})
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_protection_sociale (
			annee smallint NOT NULL,
			ps_niveau smallint NOT NULL,
			ps_code text NOT NULL,
			ps_libelle text NOT NULL,
			risque text NOT NULL,
			si_niveau smallint NOT NULL,
			si_code text NOT NULL,
			si_nom text NOT NULL,
			regime text NOT NULL,
			valeur_meur double precision NOT NULL,
			perimetre text NOT NULL,
			comptabilite text NOT NULL,
			stade text NOT NULL,
			source_id bigint NOT NULL,
			document_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_protection_sociale"},
		[]string{"annee", "ps_niveau", "ps_code", "ps_libelle", "risque",
			"si_niveau", "si_code", "si_nom", "regime", "valeur_meur",
			"perimetre", "comptabilite", "stade", "source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("DREES : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.protection_sociale AS tgt
		USING tmp_protection_sociale AS src
		ON tgt.annee = src.annee AND tgt.ps_code = src.ps_code
			AND tgt.si_code = src.si_code AND tgt.regime = src.regime
		WHEN MATCHED AND (tgt.ps_niveau, tgt.ps_libelle, tgt.risque, tgt.si_niveau, tgt.si_nom,
				tgt.valeur_meur, tgt.perimetre, tgt.comptabilite, tgt.stade,
				tgt.source_id, tgt.document_id)
			IS DISTINCT FROM (src.ps_niveau, src.ps_libelle, src.risque, src.si_niveau, src.si_nom,
				src.valeur_meur, src.perimetre, src.comptabilite, src.stade,
				src.source_id, src.document_id) THEN
			UPDATE SET ps_niveau = src.ps_niveau, ps_libelle = src.ps_libelle, risque = src.risque,
				si_niveau = src.si_niveau, si_nom = src.si_nom, valeur_meur = src.valeur_meur,
				perimetre = src.perimetre, comptabilite = src.comptabilite, stade = src.stade,
				source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (annee, ps_niveau, ps_code, ps_libelle, risque, si_niveau, si_code, si_nom,
				regime, valeur_meur, perimetre, comptabilite, stade, source_id, document_id)
			VALUES (src.annee, src.ps_niveau, src.ps_code, src.ps_libelle, src.risque,
				src.si_niveau, src.si_code, src.si_nom, src.regime, src.valeur_meur,
				src.perimetre, src.comptabilite, src.stade, src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("DREES, fusion : %w", err))
	}
	n := ct.RowsAffected()

	var annees int
	var min, max int
	if err := tx.QueryRow(ctx,
		`SELECT count(DISTINCT annee), min(annee), max(annee) FROM core.protection_sociale`).
		Scan(&annees, &min, &max); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes": n, "annees": annees, "sans_valeur": ignorees}, "")
	fmt.Printf("  DREES : %d lignes touchées par la fusion, %d millésimes de %d à %d (%d sans valeur, non chargées)\n",
		n, annees, min, max, ignorees)
	return nil
}
