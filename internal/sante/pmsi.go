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

	// --- national, par type d'hospitalisation
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_mco_national`); err != nil {
		return fail(err)
	}
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
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_mco_national"},
			[]string{"annee", "typ_hospit", "nb_patients", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_mco_national : %w", err))
		}
		total["national"] = n
	}

	// --- par établissement (région × catégorie)
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_mco_par_etablissement`); err != nil {
		return fail(err)
	}
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
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_mco_par_etablissement"},
			[]string{"annee", "region", "categ_etab", "nb_etablissements", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_mco_par_etablissement : %w", err))
		}
		total["etablissement"] = n
	}

	// --- par patient (région × âge × sexe)
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_mco_par_patient`); err != nil {
		return fail(err)
	}
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
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_mco_par_patient"},
			[]string{"annee", "region", "age", "sexe", "nb_patients", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_mco_par_patient : %w", err))
		}
		total["patient"] = n
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
