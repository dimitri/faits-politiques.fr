package sante

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PMSI-SMR (Soins médicaux et de réadaptation) et PMSI-HAD (Hospitalisation
// à domicile), les deux champs restés non explorés lors du chargement du
// MCO — voir docs/sante-donnees.md § 1.5 et le commentaire de la migration
// 0108. Même portail, mêmes principes (data-essentiel.atih.sante.fr,
// ODbL), mais aucun schéma partagé avec MCO : chaque jeu garde ses colonnes
// réelles, jusqu'aux valeurs NULL que la source elle-même publie (HP n'a
// pas de notion de séjour en SMR).
var SourcePMSISMRHAD = archive.Source{
	Slug: "atih-pmsi-smr-had", Label: "ATIH — PMSI-SMR et PMSI-HAD (data-essentiel)",
	Publisher: "Agence technique de l'information sur l'hospitalisation (ATIH)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Open Database License (ODbL) 1.0", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : ATIH, data-essentiel.atih.sante.fr",
	Cadence:     "annuelle",
	Notes: "Six jeux (trois par champ). SMR distingue HC/HP (hospitalisation complète/partielle) " +
		"et publie une durée de PRISE EN CHARGE distincte de la durée de séjour ; HP n'a pas de " +
		"séjour au sens statistique (nb_sejours et duree_moy_sejour valent NULL dans la source " +
		"elle-même). HAD n'a aucune sous-catégorie d'hospitalisation. Aucun des deux ne publie de " +
		"ligne 'Tous' par âge/sexe dans son jeu par patient, à la différence du MCO.",
}

func IngestPMSISMRHAD(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePMSISMRHAD)
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

	// --- SMR régional (chiffres clés), annee × région × HC/HP/Tous
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_smr_regional`); err != nil {
		return fail(err)
	}
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("smr-chiffres-cles"), ".json")
		if err != nil {
			return fail(fmt.Errorf("smr-chiffres-cles : %w", err))
		}
		var lignes []struct {
			Annee         string   `json:"annee"`
			Region        string   `json:"region"`
			TypeHosp      string   `json:"type_hosp"`
			NbEtab        int      `json:"nb_etab"`
			NbPat         int      `json:"nb_pat"`
			NbSej         *int     `json:"nb_sej"`
			NbJours       int      `json:"nb_jours"`
			DureeMoySej   *float64 `json:"duree_moy_sej"`
			DureeMoyPec   float64  `json:"duree_moy_pec"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("smr-chiffres-cles : %w", err))
		}
		// Vérifié à l'écriture, pour la même raison que MCO : nb_jours doit
		// s'additionner exactement HC+HP=Tous, pour chaque région. nb_patients
		// n'est PAS vérifié de la même façon : un patient peut cumuler HC et
		// HP dans l'année, donc Tous < HC+HP est attendu, pas une anomalie.
		// Clé (année, région) — une région peut apparaître pour plusieurs
		// années dans cet export ; l'oublier ferait comparer le nb_jours
		// 'Tous' d'une année aux lignes HC/HP d'une autre.
		type cleAnneeRegion struct {
			annee  int
			region string
		}
		parAnneeRegion := map[cleAnneeRegion]map[string]int{}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("smr-chiffres-cles : année %q : %w", l.Annee, err))
			}
			c := cleAnneeRegion{annee, l.Region}
			if parAnneeRegion[c] == nil {
				parAnneeRegion[c] = map[string]int{}
			}
			parAnneeRegion[c][l.TypeHosp] = l.NbJours
			rows = append(rows, []any{annee, l.Region, l.TypeHosp, l.NbEtab, l.NbPat, l.NbSej, l.NbJours, l.DureeMoySej, l.DureeMoyPec, srcID})
		}
		for c, m := range parAnneeRegion {
			_, aHC := m["HC"]
			_, aHP := m["HP"]
			if !aHC && !aHP {
				// Certaines années/régions ne publient que la ligne 'Tous',
				// sans détail HC/HP (vérifié : 2021/Normandie, entre autres) —
				// une lacune de la source à cette maille, pas une anomalie de
				// lecture. Rien à vérifier sans les deux termes.
				continue
			}
			tous, hc, hp := m["Tous"], m["HC"], m["HP"]
			if tous != hc+hp {
				return fail(fmt.Errorf("smr-chiffres-cles %d/%s : nb_jours Tous (%d) ≠ HC (%d) + HP (%d)", c.annee, c.region, tous, hc, hp))
			}
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_smr_regional"},
			[]string{"annee", "region", "type_hosp", "nb_etablissements", "nb_patients", "nb_sejours", "nb_jours", "duree_moy_sejour", "duree_moy_prise_charge", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_smr_regional : %w", err))
		}
		total["smr_regional"] = n
	}

	// --- SMR par établissement (région × catégorie)
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_smr_par_etablissement`); err != nil {
		return fail(err)
	}
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("smr-etab"), ".json")
		if err != nil {
			return fail(fmt.Errorf("smr-etab : %w", err))
		}
		var lignes []struct {
			Annee     string `json:"annee"`
			Region    string `json:"region"`
			CategEtab string `json:"categ_etab"`
			NbEtab    int    `json:"nb_etab"`
			NbJours   int    `json:"nb_jours"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("smr-etab : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("smr-etab : année %q : %w", l.Annee, err))
			}
			rows = append(rows, []any{annee, l.Region, l.CategEtab, l.NbEtab, l.NbJours, srcID})
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_smr_par_etablissement"},
			[]string{"annee", "region", "categ_etab", "nb_etablissements", "nb_jours", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_smr_par_etablissement : %w", err))
		}
		total["smr_etablissement"] = n
	}

	// --- SMR par patient (âge × sexe × HC/HP/Tous, sans région)
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_smr_par_patient`); err != nil {
		return fail(err)
	}
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("smr-age-patients"), ".json")
		if err != nil {
			return fail(fmt.Errorf("smr-age-patients : %w", err))
		}
		// nb_sej est publié comme une CHAÎNE dans ce jeu (contrairement à
		// smr-chiffres-cles où c'est un entier) — vérifié sur l'export complet
		// avant d'écrire ce décodage, pas supposé.
		var lignes []struct {
			Annee       string   `json:"annee"`
			Age         string   `json:"age"`
			Sexe        string   `json:"sexe"`
			TypeHosp    string   `json:"type_hosp"`
			NbPat       int      `json:"nb_pat"`
			NbSej       string   `json:"nb_sej"`
			NbJours     int      `json:"nb_jours"`
			DureeMoySej *float64 `json:"duree_moy_sej"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("smr-age-patients : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("smr-age-patients : année %q : %w", l.Annee, err))
			}
			// "NA" est la marque de valeur manquante de ce jeu pour nb_sej
			// (vérifié sur l'export complet) — comme pour type_hosp='HP' dans
			// smr-chiffres-cles, l'absence de séjour n'est pas une erreur de
			// lecture.
			var nbSej *int
			if l.NbSej != "" && l.NbSej != "NA" {
				v, err := strconv.Atoi(l.NbSej)
				if err != nil {
					return fail(fmt.Errorf("smr-age-patients : nb_sej %q illisible : %w", l.NbSej, err))
				}
				nbSej = &v
			}
			rows = append(rows, []any{annee, l.Age, l.Sexe, l.TypeHosp, l.NbPat, nbSej, l.NbJours, l.DureeMoySej, srcID})
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_smr_par_patient"},
			[]string{"annee", "age", "sexe", "type_hosp", "nb_patients", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_smr_par_patient : %w", err))
		}
		total["smr_patient"] = n
	}

	// --- HAD régional (chiffres clés), annee × région, sans type d'hospit.
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_had_regional`); err != nil {
		return fail(err)
	}
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("had-chiffres-cles"), ".json")
		if err != nil {
			return fail(fmt.Errorf("had-chiffres-cles : %w", err))
		}
		var lignes []struct {
			Annee            string  `json:"annee"`
			Region           string  `json:"region"`
			NbPat            int     `json:"nb_pat"`
			NbSej            int     `json:"nb_sej"`
			NbJours          int     `json:"nb_jours"`
			DureeMoyenneSej  float64 `json:"duree_moyenne_sej"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("had-chiffres-cles : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("had-chiffres-cles : année %q : %w", l.Annee, err))
			}
			rows = append(rows, []any{annee, l.Region, l.NbPat, l.NbSej, l.NbJours, l.DureeMoyenneSej, srcID})
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_had_regional"},
			[]string{"annee", "region", "nb_patients", "nb_sejours", "nb_jours", "duree_moyenne_sejour", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_had_regional : %w", err))
		}
		total["had_regional"] = n
	}

	// --- HAD par établissement (région × catégorie)
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_had_par_etablissement`); err != nil {
		return fail(err)
	}
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("had-type-etab"), ".json")
		if err != nil {
			return fail(fmt.Errorf("had-type-etab : %w", err))
		}
		var lignes []struct {
			Annee     string  `json:"annee"`
			Region    string  `json:"region"`
			CategEtab string  `json:"categ_etab"`
			NbEtab    int     `json:"nb_etab"`
			NbSej     int     `json:"nb_sej"`
			NbJours   int     `json:"nb_jours"`
			DMS       float64 `json:"dms"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("had-type-etab : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("had-type-etab : année %q : %w", l.Annee, err))
			}
			rows = append(rows, []any{annee, l.Region, l.CategEtab, l.NbEtab, l.NbSej, l.NbJours, l.DMS, srcID})
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_had_par_etablissement"},
			[]string{"annee", "region", "categ_etab", "nb_etablissements", "nb_sejours", "nb_jours", "duree_moy_sejour", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_had_par_etablissement : %w", err))
		}
		total["had_etablissement"] = n
	}

	// --- HAD par patient (âge × sexe, sans région ni durée)
	if _, err := tx.Exec(ctx, `DELETE FROM core.pmsi_had_par_patient`); err != nil {
		return fail(err)
	}
	{
		f, err := arch.Fetch(ctx, srcID, runID, exportJSONATIH("had-age-patients"), ".json")
		if err != nil {
			return fail(fmt.Errorf("had-age-patients : %w", err))
		}
		// sexe est un entier (1/2) dans ce jeu, sexe_label son libellé — le
		// code numérique n'est pas repris, seul le libellé est stable dans le
		// temps.
		var lignes []struct {
			Annee     string `json:"annee"`
			Age       string `json:"age"`
			SexeLabel string `json:"sexe_label"`
			NbPat     int    `json:"nb_pat"`
			NbSej     int    `json:"nb_sej"`
			NbJours   int    `json:"nb_jours"`
		}
		if err := lireJSONFichier(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("had-age-patients : %w", err))
		}
		var rows [][]any
		for _, l := range lignes {
			annee, err := strconv.Atoi(l.Annee)
			if err != nil {
				return fail(fmt.Errorf("had-age-patients : année %q : %w", l.Annee, err))
			}
			rows = append(rows, []any{annee, l.Age, l.SexeLabel, l.NbPat, l.NbSej, l.NbJours, srcID})
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "pmsi_had_par_patient"},
			[]string{"annee", "age", "sexe_label", "nb_patients", "nb_sejours", "nb_jours", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("pmsi_had_par_patient : %w", err))
		}
		total["had_patient"] = n
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total}, "")
	fmt.Printf("  PMSI-SMR/HAD (ATIH) : %d+%d+%d SMR, %d+%d+%d HAD\n",
		total["smr_regional"], total["smr_etablissement"], total["smr_patient"],
		total["had_regional"], total["had_etablissement"], total["had_patient"])
	return nil
}
