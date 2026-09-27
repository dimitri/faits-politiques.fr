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

// PMSI-MCO (activité hospitalière, champ Médecine-Chirurgie-Obstétrique),
// depuis le portail data-essentiel de l'ATIH — pas ScanSante, interactif et
// sans export stable. Voir docs/sante-donnees.md.
var SourcePMSIMCO = archive.Source{
	Slug: "atih-pmsi-mco", Label: "ATIH — PMSI-MCO, activité hospitalière (data-essentiel)",
	Publisher: "Agence technique de l'information sur l'hospitalisation (ATIH)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Open Database License (ODbL) 1.0", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : ATIH, data-essentiel.atih.sante.fr",
	Cadence:     "annuelle",
	Notes: "Trois jeux distincts (national par type d'hospitalisation, par établissement, par " +
		"patient), chacun avec sa propre grille de croisement — voir les commentaires des trois " +
		"tables pour ce qui se somme et ce qui ne se somme pas. ScanSante (scansante.fr), le " +
		"portail habituellement cité pour le PMSI, est une interface interactive sans export " +
		"stable trouvé ; data-essentiel.atih.sante.fr, moins connu, publie les mêmes familles de " +
		"chiffres en JSON/CSV sous ODbL.",
}

func exportJSONATIH(dataset string) string {
	return "https://data-essentiel.atih.sante.fr/api/explore/v2.1/catalog/datasets/" + dataset + "/exports/json"
}

func IngestPMSIMCO(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePMSIMCO)
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	total := map[string]int64{}

	// MERGE plutôt que DELETE+COPY sur les trois tables : chacune exclusivement
	// possédée par ce connecteur, l'ancien DELETE payait le prix des triggers
	// RI pour l'intégralité de la table à chaque republication annuelle,
	// changement ou non.

	// --- national, par type d'hospitalisation
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("mco-chiffres-cles"), ".json")
		if err != nil {
			return fail(fmt.Errorf("mco-chiffres-cles : %w", err))
		}
		var lignes []struct {
			Annee       string  `json:"annee"`
			TypHospit   string  `json:"typ_hospit"`
			NbPat       int     `json:"nb_pat"`
			NbSej       int     `json:"nb_sej"`
			NbJours     int     `json:"nb_jours"`
			DureeMoySej float64 `json:"duree_moy_sej"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("mco-chiffres-cles : %w", err))
		}
		// Vérifié à l'écriture : 'Tous' doit être la somme exacte des deux
		// autres types, pour chaque année — sinon le jeu source a changé de
		// forme et le commentaire de la table ne serait plus vrai.
		parAnnee := map[string]map[string]int{}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("mco-chiffres-cles : année %q : %w", l.Annee, err))
			}
			if parAnnee[l.Annee] == nil {
				parAnnee[l.Annee] = map[string]int{}
			}
			parAnnee[l.Annee][l.TypHospit] = l.NbSej
			rows = append(rows, []any{annee, l.TypHospit, l.NbPat, l.NbSej, l.NbJours, l.DureeMoySej, srcID})
		}
		for annee, m := range parAnnee {
			tous, complet, ambu := m["Tous"], m["Hospitalisation complète"], m["Hospitalisation ambulatoire"]
			if tous == 0 || complet == 0 || ambu == 0 {
				return fail(fmt.Errorf("mco-chiffres-cles %s : un des trois types d'hospitalisation manque", annee))
			}
			if tous != complet+ambu {
				return fail(fmt.Errorf("mco-chiffres-cles %s : Tous (%d) ≠ complète (%d) + ambulatoire (%d)",
					annee, tous, complet, ambu))
			}
		}
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_pmsi_mco_national (
				annee smallint, typ_hospit text, nb_patients int, nb_sejours int,
				nb_jours int, duree_moy_sejour numeric, source_id bigint
			) ON COMMIT DROP`); err != nil {
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_pmsi_mco_national"},
			[]string{"annee", "typ_hospit", "nb_patients", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows)); err != nil {
			return fail(fmt.Errorf("pmsi_mco_national : %w", err))
		}
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.pmsi_mco_national AS tgt
			USING tmp_pmsi_mco_national AS src
			ON tgt.annee = src.annee AND tgt.typ_hospit = src.typ_hospit
			WHEN MATCHED AND (tgt.nb_patients, tgt.nb_sejours, tgt.nb_jours, tgt.duree_moy_sejour, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nb_patients, src.nb_sejours, src.nb_jours, src.duree_moy_sejour, src.source_id) THEN
			    UPDATE SET nb_patients = src.nb_patients, nb_sejours = src.nb_sejours,
			               nb_jours = src.nb_jours, duree_moy_sejour = src.duree_moy_sejour,
			               source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (annee, typ_hospit, nb_patients, nb_sejours, nb_jours, duree_moy_sejour, source_id)
			    VALUES (src.annee, src.typ_hospit, src.nb_patients, src.nb_sejours, src.nb_jours,
			            src.duree_moy_sejour, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return fail(fmt.Errorf("pmsi_mco_national : %w", err))
		}
		total["national"] = ct.RowsAffected()
	}

	// --- par établissement (région × catégorie)
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("mco-type-etab"), ".json")
		if err != nil {
			return fail(fmt.Errorf("mco-type-etab : %w", err))
		}
		var lignes []struct {
			Annee       string  `json:"annee"`
			Region      string  `json:"region"`
			CategEtab   string  `json:"categ_etab"`
			NbEtab      int     `json:"nb_etab"`
			NbSej       int     `json:"nb_sej"`
			NbJours     int     `json:"nb_jours"`
			DureeMoySej float64 `json:"duree_moy_sej"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("mco-type-etab : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			if l.Region == "Tous" || l.CategEtab == "Tous" {
				return fail(fmt.Errorf("mco-type-etab : ligne 'Tous' inattendue (%s/%s) — la table suppose une partition plate", l.Region, l.CategEtab))
			}
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("mco-type-etab : année %q : %w", l.Annee, err))
			}
			rows = append(rows, []any{annee, l.Region, l.CategEtab, l.NbEtab, l.NbSej, l.NbJours, l.DureeMoySej, srcID})
		}
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_pmsi_mco_par_etablissement (
				annee smallint, region text, categ_etab text, nb_etablissements int,
				nb_sejours int, nb_jours int, duree_moy_sejour numeric, source_id bigint
			) ON COMMIT DROP`); err != nil {
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_pmsi_mco_par_etablissement"},
			[]string{"annee", "region", "categ_etab", "nb_etablissements", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows)); err != nil {
			return fail(fmt.Errorf("pmsi_mco_par_etablissement : %w", err))
		}
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.pmsi_mco_par_etablissement AS tgt
			USING tmp_pmsi_mco_par_etablissement AS src
			ON tgt.annee = src.annee AND tgt.region = src.region AND tgt.categ_etab = src.categ_etab
			WHEN MATCHED AND (tgt.nb_etablissements, tgt.nb_sejours, tgt.nb_jours, tgt.duree_moy_sejour, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nb_etablissements, src.nb_sejours, src.nb_jours, src.duree_moy_sejour, src.source_id) THEN
			    UPDATE SET nb_etablissements = src.nb_etablissements, nb_sejours = src.nb_sejours,
			               nb_jours = src.nb_jours, duree_moy_sejour = src.duree_moy_sejour,
			               source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (annee, region, categ_etab, nb_etablissements, nb_sejours, nb_jours, duree_moy_sejour, source_id)
			    VALUES (src.annee, src.region, src.categ_etab, src.nb_etablissements, src.nb_sejours,
			            src.nb_jours, src.duree_moy_sejour, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return fail(fmt.Errorf("pmsi_mco_par_etablissement : %w", err))
		}
		total["etablissement"] = ct.RowsAffected()
	}

	// --- par patient (région × âge × sexe)
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("mco-age-patients"), ".json")
		if err != nil {
			return fail(fmt.Errorf("mco-age-patients : %w", err))
		}
		var lignes []struct {
			Annee       string  `json:"annee"`
			Region      string  `json:"region"`
			Age         string  `json:"age"`
			Sexe        string  `json:"sexe"`
			NbPat       int     `json:"nb_pat"`
			NbSej       int     `json:"nb_sej"`
			NbJours     int     `json:"nb_jours"`
			DureeMoySej float64 `json:"duree_moy_sej"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("mco-age-patients : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("mco-age-patients : année %q : %w", l.Annee, err))
			}
			rows = append(rows, []any{annee, l.Region, l.Age, l.Sexe, l.NbPat, l.NbSej, l.NbJours, l.DureeMoySej, srcID})
		}
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_pmsi_mco_par_patient (
				annee smallint, region text, age text, sexe text, nb_patients int,
				nb_sejours int, nb_jours int, duree_moy_sejour numeric, source_id bigint
			) ON COMMIT DROP`); err != nil {
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_pmsi_mco_par_patient"},
			[]string{"annee", "region", "age", "sexe", "nb_patients", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows)); err != nil {
			return fail(fmt.Errorf("pmsi_mco_par_patient : %w", err))
		}
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.pmsi_mco_par_patient AS tgt
			USING tmp_pmsi_mco_par_patient AS src
			ON tgt.annee = src.annee AND tgt.region = src.region AND tgt.age = src.age AND tgt.sexe = src.sexe
			WHEN MATCHED AND (tgt.nb_patients, tgt.nb_sejours, tgt.nb_jours, tgt.duree_moy_sejour, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nb_patients, src.nb_sejours, src.nb_jours, src.duree_moy_sejour, src.source_id) THEN
			    UPDATE SET nb_patients = src.nb_patients, nb_sejours = src.nb_sejours,
			               nb_jours = src.nb_jours, duree_moy_sejour = src.duree_moy_sejour,
			               source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (annee, region, age, sexe, nb_patients, nb_sejours, nb_jours, duree_moy_sejour, source_id)
			    VALUES (src.annee, src.region, src.age, src.sexe, src.nb_patients, src.nb_sejours,
			            src.nb_jours, src.duree_moy_sejour, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return fail(fmt.Errorf("pmsi_mco_par_patient : %w", err))
		}
		total["patient"] = ct.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total}, "")
	fmt.Printf("  PMSI-MCO (ATIH) : %d national, %d par établissement, %d par patient\n",
		total["national"], total["etablissement"], total["patient"])
	return nil
}

func lireJSONFichier(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
