package sante

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Combien un médecin gagne, pas seulement dans quel secteur il exerce — voir
// docs/sante-donnees.md § 2 (répartition par secteur, déjà chargée) et le
// commentaire de la migration 0109 pour les deux pièges de ce jeu (NS
// laundered en 0 par le champ *_integer de la source ; code_departement
// '999' à deux niveaux d'agrégat selon la région).
var SourceHonoraires = archive.Source{
	Slug: "ameli-honoraires", Label: "Ameli — montants des honoraires des professionnels de santé libéraux",
	Publisher: "Caisse nationale de l'Assurance Maladie (Cnam)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : Cnam, data.ameli.fr",
	Cadence:     "annuelle",
	Notes: "38 professions, 2010-2024. Les quatre champs de montant (totaux et moyens) portent la " +
		"valeur littérale 'NS' (non significatif, secret statistique sur petit effectif) plutôt " +
		"qu'un nombre — 15 % des lignes pour les totaux, vérifié sur l'export complet. La source " +
		"publie aussi un champ compagnon *_integer qui remplace 'NS' par 0, jamais utilisé ici : " +
		"un montant non significatif n'est pas un montant nul.",
}

const honorairesURL = "https://data.ameli.fr/api/explore/v2.1/catalog/datasets/honoraires/exports/json?limit=-1"

func IngestHonoraires(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceHonoraires)
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

	f, err := arch.Fetch(ctx, srcID, runID, honorairesURL, ".json")
	if err != nil {
		return fail(err)
	}
	var lignes []struct {
		Annee                     string `json:"annee"`
		ProfessionSante           string `json:"profession_sante"`
		Region                    string `json:"region"`
		LibelleRegion             string `json:"libelle_region"`
		Departement               string `json:"departement"`
		LibelleDepartement        string `json:"libelle_departement"`
		HonoSansDepassementTotaux string `json:"hono_sans_depassement_totaux"`
		DepassementsTotaux        string `json:"depassements_totaux"`
		HonoSansDepassementMoyens string `json:"hono_sans_depassement_moyens"`
		DepassementsMoyens        string `json:"depassements_moyens"`
		TauxDepassementS2         string `json:"taux_depassement_s2"`
		TauxDepassementS2Optam    string `json:"taux_depassement_s2_optam"`
		TauxDepassementS2NonOptam string `json:"taux_depassement_s2_non_optam"`
	}
	if err := lireJSONFichier(f.Path, &lignes); err != nil {
		return fail(fmt.Errorf("honoraires : %w", err))
	}

	// "NS" (non significatif) devient NULL, jamais 0 — voir le commentaire de
	// la migration 0109. Les autres champs numériques sont censés toujours
	// être présents dans ce jeu (vérifié sur l'export complet avant
	// d'écrire ce connecteur) : une valeur absente ou illisible y échoue
	// plutôt que d'être devinée.
	versNullableInt := func(s string) (*int64, error) {
		if s == "NS" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	// Les trois taux de dépassement portent "NC" (non concerné — la
	// profession n'a pas d'effectif en secteur 2, la question ne se pose
	// pas), un sentinel distinct de "NS" mais qui devient NULL de la même
	// façon : ni l'un ni l'autre n'est un taux de zéro.
	versNullableFloat := func(s string) (*float64, error) {
		if s == "NC" {
			return nil, nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}
		return &v, nil
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
			return fail(fmt.Errorf("honoraires : année %q : %w", l.Annee, err))
		}
		honoTotal, err := versNullableInt(l.HonoSansDepassementTotaux)
		if err != nil {
			return fail(fmt.Errorf("honoraires : hono_sans_depassement_totaux %q : %w", l.HonoSansDepassementTotaux, err))
		}
		depTotal, err := versNullableInt(l.DepassementsTotaux)
		if err != nil {
			return fail(fmt.Errorf("honoraires : depassements_totaux %q : %w", l.DepassementsTotaux, err))
		}
		honoMoyen, err := versNullableInt(l.HonoSansDepassementMoyens)
		if err != nil {
			return fail(fmt.Errorf("honoraires : hono_sans_depassement_moyens %q : %w", l.HonoSansDepassementMoyens, err))
		}
		depMoyen, err := versNullableInt(l.DepassementsMoyens)
		if err != nil {
			return fail(fmt.Errorf("honoraires : depassements_moyens %q : %w", l.DepassementsMoyens, err))
		}
		tauxS2, err := versNullableFloat(l.TauxDepassementS2)
		if err != nil {
			return fail(fmt.Errorf("honoraires : taux_depassement_s2 %q : %w", l.TauxDepassementS2, err))
		}
		tauxOptam, err := versNullableFloat(l.TauxDepassementS2Optam)
		if err != nil {
			return fail(fmt.Errorf("honoraires : taux_depassement_s2_optam %q : %w", l.TauxDepassementS2Optam, err))
		}
		tauxNonOptam, err := versNullableFloat(l.TauxDepassementS2NonOptam)
		if err != nil {
			return fail(fmt.Errorf("honoraires : taux_depassement_s2_non_optam %q : %w", l.TauxDepassementS2NonOptam, err))
		}
		rows = append(rows, []any{
			annee, l.ProfessionSante, l.Region, l.LibelleRegion, l.Departement, l.LibelleDepartement,
			honoTotal, depTotal, honoMoyen, depMoyen, tauxS2, tauxOptam, tauxNonOptam, srcID,
		})
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_medecin_honoraires (
			annee smallint, profession_sante text, code_region text, libelle_region text,
			code_departement text, libelle_departement text, hono_sans_depassement_total bigint,
			depassements_total bigint, hono_sans_depassement_moyen bigint, depassements_moyen bigint,
			taux_depassement_s2 numeric, taux_depassement_s2_optam numeric,
			taux_depassement_s2_non_optam numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_medecin_honoraires"},
		[]string{"annee", "profession_sante", "code_region", "libelle_region", "code_departement", "libelle_departement",
			"hono_sans_depassement_total", "depassements_total", "hono_sans_depassement_moyen", "depassements_moyen",
			"taux_depassement_s2", "taux_depassement_s2_optam", "taux_depassement_s2_non_optam", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("medecin_honoraires : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité de 2010-2024 à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.medecin_honoraires AS tgt
		USING tmp_medecin_honoraires AS src
		ON tgt.annee = src.annee AND tgt.profession_sante = src.profession_sante
		   AND tgt.code_region = src.code_region AND tgt.code_departement = src.code_departement
		WHEN MATCHED AND (tgt.libelle_region, tgt.libelle_departement, tgt.hono_sans_depassement_total,
		                   tgt.depassements_total, tgt.hono_sans_depassement_moyen, tgt.depassements_moyen,
		                   tgt.taux_depassement_s2, tgt.taux_depassement_s2_optam,
		                   tgt.taux_depassement_s2_non_optam, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.libelle_region, src.libelle_departement, src.hono_sans_depassement_total,
		                   src.depassements_total, src.hono_sans_depassement_moyen, src.depassements_moyen,
		                   src.taux_depassement_s2, src.taux_depassement_s2_optam,
		                   src.taux_depassement_s2_non_optam, src.source_id) THEN
		    UPDATE SET libelle_region = src.libelle_region, libelle_departement = src.libelle_departement,
		               hono_sans_depassement_total = src.hono_sans_depassement_total,
		               depassements_total = src.depassements_total,
		               hono_sans_depassement_moyen = src.hono_sans_depassement_moyen,
		               depassements_moyen = src.depassements_moyen, taux_depassement_s2 = src.taux_depassement_s2,
		               taux_depassement_s2_optam = src.taux_depassement_s2_optam,
		               taux_depassement_s2_non_optam = src.taux_depassement_s2_non_optam, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, profession_sante, code_region, libelle_region, code_departement, libelle_departement,
		            hono_sans_depassement_total, depassements_total, hono_sans_depassement_moyen,
		            depassements_moyen, taux_depassement_s2, taux_depassement_s2_optam,
		            taux_depassement_s2_non_optam, source_id)
		    VALUES (src.annee, src.profession_sante, src.code_region, src.libelle_region, src.code_departement,
		            src.libelle_departement, src.hono_sans_depassement_total, src.depassements_total,
		            src.hono_sans_depassement_moyen, src.depassements_moyen, src.taux_depassement_s2,
		            src.taux_depassement_s2_optam, src.taux_depassement_s2_non_optam, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion medecin_honoraires : %w", err))
	}
	n := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  Honoraires des médecins (Ameli) : %d lignes\n", n)
	return nil
}
