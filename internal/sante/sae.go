package sante

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SAE (Statistique annuelle des établissements de santé) : publiée par la
// Drees en une archive .7z par année, contenant une cinquantaine de
// bordereaux thématiques (SAS + CSV), pas un jeu tabulaire directement
// interrogeable — voir docs/sante-donnees.md § 1 pour ce que ça écarte
// (bordereaux non chargés). Deux bordereaux chargés ici : Q24 (personnel non
// médical par fonction) et URGENCES2 (passages aux urgences, colonne PASSU)
// — les deux dont les colonnes sont directement lisibles, contrairement à
// Q23 (codes PERSO non résolus) ou HPR (310 établissements seulement — trop
// étroit pour représenter la capacité nationale, voir la note).
//
// Dépendance externe assumée : ce connecteur invoque le binaire `7z` en sous-
// processus — le seul connecteur de ce dépôt à sortir du Go pur, parce que la
// bibliothèque standard ne lit pas le format 7-zip et qu'ajouter une
// dépendance Go pour un seul jeu de données semblait disproportionné. Échoue
// explicitement, message clair, si `7z` n'est pas sur le PATH.
var SourceSAE = archive.Source{
	Slug: "drees-sae-bases-statistiques", Label: "Drees — SAE, bases statistiques (Q24 personnel, URGENCES2 passages)",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees, SAE (Statistique annuelle des établissements de santé)",
	Cadence:     "annuelle",
	Notes: "L'archive complète (~50 bordereaux, une par année) couvre lits, activité, " +
		"équipements et personnel ; Q24 (personnel non médical par fonction) et URGENCES2 " +
		"(passages aux urgences) sont chargés ici, 2013-2024 — le format CSV combiné n'existe " +
		"pas avant 2013 dans cette archive (vérifié sur le catalogue des pièces jointes). " +
		"2020 n'a aucun bordereau Q24 (ni aucun Q2x) dans l'archive source : vérifié, pas " +
		"une erreur d'extraction — la collecte du personnel a été perturbée cette année-là ; " +
		"URGENCES2, lui, est présent pour 2020. Décimales à virgule dans certains bordereaux " +
		"de cette même archive (HPR notamment) mais à point dans Q24 — vérifié empiriquement, " +
		"pas une convention garantie d'un bordereau à l'autre.",
}

const (
	saeAnneeDebut = 2013
	saeAnneeFin   = 2024
)

func saeURL(annee int) string {
	return fmt.Sprintf("https://data.drees.solidarites-sante.gouv.fr/api/datasets/1.0/708_bases-statistiques-sae/"+
		"attachments/sae_%d_bases_statistiques_formats_sas_csv_7z/", annee)
}

func IngestSAE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSAE)
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

	if _, err := exec.LookPath("7z"); err != nil {
		return fail(fmt.Errorf("binaire 7z introuvable sur le PATH : %w", err))
	}

	var toutesQ24 []ligneQ24
	var toutesUrgences []ligneUrgences
	for annee := saeAnneeDebut; annee <= saeAnneeFin; annee++ {
		f, err := arch.Fetch(ctx, srcID, runID, saeURL(annee), ".7z")
		if err != nil {
			return fail(fmt.Errorf("exercice %d : %w", annee, err))
		}

		tmp, err := os.MkdirTemp("", fmt.Sprintf("sae-%d-*", annee))
		if err != nil {
			return fail(err)
		}

		q24Fichier := fmt.Sprintf("Q24_%dr.csv", annee)
		urgFichier := fmt.Sprintf("URGENCES2_%dr.csv", annee)
		cmd := exec.CommandContext(ctx, "7z", "e", f.Path, "-o"+tmp, "-y", "-r", q24Fichier, urgFichier)
		out, err := cmd.CombinedOutput()
		if err != nil {
			os.RemoveAll(tmp)
			return fail(fmt.Errorf("exercice %d, extraction 7z : %w : %s", annee, err, out))
		}

		// 2020 est le seul exercice de la période sans bordereau Q24 dans
		// l'archive (vérifié : aucun fichier Q2x, année de perturbation de
		// la collecte SAE liée au covid) — une absence réelle de la source,
		// pas une erreur d'extraction : sautée pour cette seule année,
		// jamais silencieusement pour une autre.
		nQ24Annee := 0
		q24Path := filepath.Join(tmp, q24Fichier)
		if _, err := os.Stat(q24Path); err != nil {
			if annee != 2020 {
				os.RemoveAll(tmp)
				return fail(fmt.Errorf("exercice %d : %s absent de l'archive après extraction : %w", annee, q24Fichier, err))
			}
		} else {
			lignesQ24, err := lireCSVQ24(q24Path)
			if err != nil {
				os.RemoveAll(tmp)
				return fail(fmt.Errorf("exercice %d, Q24 : %w", annee, err))
			}
			toutesQ24 = append(toutesQ24, lignesQ24...)
			nQ24Annee = len(lignesQ24)
		}

		urgPath := filepath.Join(tmp, urgFichier)
		if _, err := os.Stat(urgPath); err != nil {
			os.RemoveAll(tmp)
			return fail(fmt.Errorf("exercice %d : %s absent de l'archive après extraction : %w", annee, urgFichier, err))
		}
		lignesUrg, err := lireCSVUrgences(urgPath)
		if err != nil {
			os.RemoveAll(tmp)
			return fail(fmt.Errorf("exercice %d, URGENCES2 : %w", annee, err))
		}
		toutesUrgences = append(toutesUrgences, lignesUrg...)

		os.RemoveAll(tmp)
		if nQ24Annee == 0 {
			fmt.Printf("    SAE %d : bordereau Q24 absent de la source (connu, année covid), %d lignes URGENCES2\n",
				annee, len(lignesUrg))
		} else {
			fmt.Printf("    SAE %d : %d lignes Q24, %d lignes URGENCES2\n", annee, nQ24Annee, len(lignesUrg))
		}
	}
	if len(toutesQ24) == 0 {
		return fail(fmt.Errorf("SAE Q24 : aucune ligne lue"))
	}
	if len(toutesUrgences) == 0 {
		return fail(fmt.Errorf("SAE URGENCES2 : aucune ligne lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY sur les deux bordereaux : chacun
	// exclusivement possédé par ce connecteur, l'ancien DELETE payait le
	// prix des triggers RI pour l'intégralité de la table à chaque
	// republication annuelle, changement ou non.
	var rowsQ24 [][]any
	for _, l := range toutesQ24 {
		rowsQ24 = append(rowsQ24, []any{
			l.Annee, l.FI, l.FIEJ,
			l.Direc, l.Dirsoin, l.Admin, l.Admto, l.Cadre, l.Infns, l.Infsp,
			l.Aides, l.Ashau, l.Psych, l.Sagfe, l.Reedu, l.Soito, l.Educs,
			l.Assis, l.Eduto, l.Phlab, l.Techn, l.Etppnm,
			srcID,
		})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_sae_personnel_fonction (
			annee int, nofinesset text, nofinessej text,
			etp_direction numeric, etp_direction_soins numeric, etp_administratif numeric,
			etp_admin_technique_ouvrier numeric, etp_cadre numeric, etp_infirmier numeric,
			etp_infirmier_specialise numeric, etp_aide_soignant numeric, etp_agent_service_hospitalier numeric,
			etp_psychologue numeric, etp_sage_femme numeric, etp_reeducation numeric,
			etp_social_educatif numeric, etp_educateur_specialise numeric, etp_assistant_service_social numeric,
			etp_autre_educatif numeric, etp_pharmacie_labo numeric, etp_technique numeric, etp_total_pnm numeric,
			source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_sae_personnel_fonction"},
		[]string{"annee", "nofinesset", "nofinessej",
			"etp_direction", "etp_direction_soins", "etp_administratif", "etp_admin_technique_ouvrier",
			"etp_cadre", "etp_infirmier", "etp_infirmier_specialise", "etp_aide_soignant",
			"etp_agent_service_hospitalier", "etp_psychologue", "etp_sage_femme", "etp_reeducation",
			"etp_social_educatif", "etp_educateur_specialise", "etp_assistant_service_social",
			"etp_autre_educatif", "etp_pharmacie_labo", "etp_technique", "etp_total_pnm",
			"source_id"},
		pgx.CopyFromRows(rowsQ24)); err != nil {
		return fail(fmt.Errorf("sae_personnel_fonction : %w", err))
	}
	var nQ24 int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.sae_personnel_fonction", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.sae_personnel_fonction AS tgt
			USING tmp_sae_personnel_fonction AS src
			ON tgt.nofinesset = src.nofinesset AND tgt.annee = src.annee
			WHEN MATCHED AND (tgt.nofinessej, tgt.etp_direction, tgt.etp_direction_soins, tgt.etp_administratif,
			                   tgt.etp_admin_technique_ouvrier, tgt.etp_cadre, tgt.etp_infirmier,
			                   tgt.etp_infirmier_specialise, tgt.etp_aide_soignant, tgt.etp_agent_service_hospitalier,
			                   tgt.etp_psychologue, tgt.etp_sage_femme, tgt.etp_reeducation, tgt.etp_social_educatif,
			                   tgt.etp_educateur_specialise, tgt.etp_assistant_service_social, tgt.etp_autre_educatif,
			                   tgt.etp_pharmacie_labo, tgt.etp_technique, tgt.etp_total_pnm, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nofinessej, src.etp_direction, src.etp_direction_soins, src.etp_administratif,
			                   src.etp_admin_technique_ouvrier, src.etp_cadre, src.etp_infirmier,
			                   src.etp_infirmier_specialise, src.etp_aide_soignant, src.etp_agent_service_hospitalier,
			                   src.etp_psychologue, src.etp_sage_femme, src.etp_reeducation, src.etp_social_educatif,
			                   src.etp_educateur_specialise, src.etp_assistant_service_social, src.etp_autre_educatif,
			                   src.etp_pharmacie_labo, src.etp_technique, src.etp_total_pnm, src.source_id) THEN
			    UPDATE SET nofinessej = src.nofinessej, etp_direction = src.etp_direction,
			               etp_direction_soins = src.etp_direction_soins, etp_administratif = src.etp_administratif,
			               etp_admin_technique_ouvrier = src.etp_admin_technique_ouvrier, etp_cadre = src.etp_cadre,
			               etp_infirmier = src.etp_infirmier, etp_infirmier_specialise = src.etp_infirmier_specialise,
			               etp_aide_soignant = src.etp_aide_soignant,
			               etp_agent_service_hospitalier = src.etp_agent_service_hospitalier,
			               etp_psychologue = src.etp_psychologue, etp_sage_femme = src.etp_sage_femme,
			               etp_reeducation = src.etp_reeducation, etp_social_educatif = src.etp_social_educatif,
			               etp_educateur_specialise = src.etp_educateur_specialise,
			               etp_assistant_service_social = src.etp_assistant_service_social,
			               etp_autre_educatif = src.etp_autre_educatif, etp_pharmacie_labo = src.etp_pharmacie_labo,
			               etp_technique = src.etp_technique, etp_total_pnm = src.etp_total_pnm,
			               source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (annee, nofinesset, nofinessej, etp_direction, etp_direction_soins, etp_administratif,
			            etp_admin_technique_ouvrier, etp_cadre, etp_infirmier, etp_infirmier_specialise,
			            etp_aide_soignant, etp_agent_service_hospitalier, etp_psychologue, etp_sage_femme,
			            etp_reeducation, etp_social_educatif, etp_educateur_specialise,
			            etp_assistant_service_social, etp_autre_educatif, etp_pharmacie_labo, etp_technique,
			            etp_total_pnm, source_id)
			    VALUES (src.annee, src.nofinesset, src.nofinessej, src.etp_direction, src.etp_direction_soins,
			            src.etp_administratif, src.etp_admin_technique_ouvrier, src.etp_cadre, src.etp_infirmier,
			            src.etp_infirmier_specialise, src.etp_aide_soignant, src.etp_agent_service_hospitalier,
			            src.etp_psychologue, src.etp_sage_femme, src.etp_reeducation, src.etp_social_educatif,
			            src.etp_educateur_specialise, src.etp_assistant_service_social, src.etp_autre_educatif,
			            src.etp_pharmacie_labo, src.etp_technique, src.etp_total_pnm, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		nQ24 = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("sae_personnel_fonction : %w", err))
	}

	var rowsUrg [][]any
	for _, l := range toutesUrgences {
		rowsUrg = append(rowsUrg, []any{l.Annee, l.FI, l.FIEJ, l.TypeUrgence, l.Passages, srcID})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_sae_urgences_passages (
			annee int, nofinesset text, nofinessej text, type_urgence text, passages int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_sae_urgences_passages"},
		[]string{"annee", "nofinesset", "nofinessej", "type_urgence", "passages", "source_id"},
		pgx.CopyFromRows(rowsUrg)); err != nil {
		return fail(fmt.Errorf("sae_urgences_passages : %w", err))
	}
	var nUrg int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.sae_urgences_passages", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.sae_urgences_passages AS tgt
			USING tmp_sae_urgences_passages AS src
			ON tgt.nofinesset = src.nofinesset AND tgt.annee = src.annee AND tgt.type_urgence = src.type_urgence
			WHEN MATCHED AND (tgt.nofinessej, tgt.passages, tgt.source_id)
			                  IS DISTINCT FROM (src.nofinessej, src.passages, src.source_id) THEN
			    UPDATE SET nofinessej = src.nofinessej, passages = src.passages, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (annee, nofinesset, nofinessej, type_urgence, passages, source_id)
			    VALUES (src.annee, src.nofinesset, src.nofinessej, src.type_urgence, src.passages, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		nUrg = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("sae_urgences_passages : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes_q24": len(rowsQ24), "q24_touchees": nQ24,
		"lignes_urgences": len(rowsUrg), "urgences_touchees": nUrg}, "")
	fmt.Printf("  SAE : %d lignes Q24 (%d touchées), %d lignes URGENCES2 (%d touchées), %d-%d\n",
		len(rowsQ24), nQ24, len(rowsUrg), nUrg, saeAnneeDebut, saeAnneeFin)
	return nil
}

type ligneQ24 struct {
	Annee                                                                                       int
	FI, FIEJ                                                                                    string
	Direc, Dirsoin, Admin, Admto, Cadre, Infns, Infsp, Aides, Ashau, Psych, Sagfe, Reedu, Soito *float64
	Educs, Assis, Eduto, Phlab, Techn, Etppnm                                                   *float64
}

func lireCSVQ24(path string) ([]ligneQ24, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	entetes, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range entetes {
		idx[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	col := func(rec []string, nom string) string {
		i, ok := idx[nom]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	numCol := func(rec []string, nom string) *float64 {
		s := col(rec, nom)
		if s == "" {
			return nil
		}
		s = strings.ReplaceAll(s, ",", ".")
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &v
	}

	var out []ligneQ24
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if len(rec) == 0 {
			continue
		}
		// DISCI est un code HIÉRARCHIQUE, pas une partition plate : un
		// établissement porte une ligne par discipline (1000, 2100, 2300...)
		// PLUS une ligne 9999 qui est déjà leur somme. Vérifié empiriquement
		// (010000024 : 1000 + 2000 = 9999 à l'euro d'ETP près) — additionner
		// toutes les lignes doublerait ou triplerait chaque effectif. Seule
		// la ligne 9999 (le total déjà calculé par la Drees) est retenue.
		if col(rec, "DISCI") != "9999" {
			continue
		}
		annee, err := strconv.Atoi(col(rec, "AN"))
		if err != nil {
			return nil, fmt.Errorf("SAE Q24 : année illisible sur la ligne FI=%s", col(rec, "FI"))
		}
		out = append(out, ligneQ24{
			Annee: annee, FI: col(rec, "FI"), FIEJ: col(rec, "FI_EJ"),
			Direc: numCol(rec, "DIREC"), Dirsoin: numCol(rec, "DIRSOIN"),
			Admin: numCol(rec, "ADMIN"), Admto: numCol(rec, "ADMTO"), Cadre: numCol(rec, "CADRE"),
			Infns: numCol(rec, "INFNS"), Infsp: numCol(rec, "INFSP"), Aides: numCol(rec, "AIDES"),
			Ashau: numCol(rec, "ASHAU"), Psych: numCol(rec, "PSYCH"), Sagfe: numCol(rec, "SAGFE"),
			Reedu: numCol(rec, "REEDU"), Soito: numCol(rec, "SOITO"), Educs: numCol(rec, "EDUCS"),
			Assis: numCol(rec, "ASSIS"), Eduto: numCol(rec, "EDUTO"), Phlab: numCol(rec, "PHLAB"),
			Techn: numCol(rec, "TECHN"), Etppnm: numCol(rec, "ETPPNM"),
		})
	}
	return out, nil
}

type ligneUrgences struct {
	Annee       int
	FI, FIEJ    string
	TypeUrgence string
	Passages    *int
}

// lireCSVUrgences lit le bordereau URGENCES2 — une ligne par établissement
// ET par type d'accueil (GEN/PED/AMU, colonne URG), pas une ligne par
// établissement : un CHU avec un accueil général et un accueil pédiatrique
// distinct porte deux lignes, jamais un doublon (vérifié : FI identique,
// URG différent). PASSU est le nombre de passages, la colonne qui répond à
// « combien de patients aux urgences », pas un temps d'attente.
func lireCSVUrgences(path string) ([]ligneUrgences, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	entetes, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range entetes {
		idx[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	col := func(rec []string, nom string) string {
		i, ok := idx[nom]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	intCol := func(rec []string, nom string) *int {
		s := col(rec, nom)
		if s == "" {
			return nil
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		return &v
	}

	var out []ligneUrgences
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if len(rec) == 0 {
			continue
		}
		annee, err := strconv.Atoi(col(rec, "AN"))
		if err != nil {
			return nil, fmt.Errorf("SAE URGENCES2 : année illisible sur la ligne FI=%s", col(rec, "FI"))
		}
		typeUrg := col(rec, "URG")
		if typeUrg == "" {
			continue // ligne sans type d'accueil renseigné, pas un accueil réel
		}
		out = append(out, ligneUrgences{
			Annee: annee, FI: col(rec, "FI"), FIEJ: col(rec, "FI_EJ"),
			TypeUrgence: typeUrg, Passages: intCol(rec, "PASSU"),
		})
	}
	return out, nil
}
