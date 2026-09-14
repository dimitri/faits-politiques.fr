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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SAE (Statistique annuelle des établissements de santé) : publiée par la
// Drees en une archive .7z contenant une cinquantaine de bordereaux
// thématiques (SAS + CSV), pas un jeu tabulaire directement interrogeable —
// voir docs/sante-donnees.md § 1 pour ce que ça écarte (bordereaux non
// chargés). Seul bordereau Q24 (personnel non médical par fonction) est
// chargé ici : ses colonnes sont directement lisibles, contrairement à Q23
// (codes PERSO non résolus) ou HPR (310 établissements seulement — trop
// étroit pour représenter la capacité nationale, voir la note).
//
// Dépendance externe assumée : ce connecteur invoque le binaire `7z` en sous-
// processus — le seul connecteur de ce dépôt à sortir du Go pur, parce que la
// bibliothèque standard ne lit pas le format 7-zip et qu'ajouter une
// dépendance Go pour un seul jeu de données semblait disproportionné. Échoue
// explicitement, message clair, si `7z` n'est pas sur le PATH.
var SourceSAE = archive.Source{
	Slug: "drees-sae-bases-statistiques", Label: "Drees — SAE, bases statistiques (bordereau Q24, personnel)",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees, SAE (Statistique annuelle des établissements de santé)",
	Cadence:     "annuelle",
	Notes: "L'archive complète (~50 bordereaux) couvre lits, activité, équipements et personnel ; " +
		"seul le bordereau Q24 (personnel non médical par fonction) est chargé ici. Décimales à " +
		"virgule dans certains bordereaux de cette même archive (HPR notamment) mais à point dans " +
		"Q24 — vérifié empiriquement, pas une convention garantie d'un bordereau à l'autre.",
}

const saeURL = "https://data.drees.solidarites-sante.gouv.fr/api/datasets/1.0/708_bases-statistiques-sae/" +
	"attachments/sae_2024_bases_statistiques_formats_sas_csv_7z/"

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

	f, err := arch.Fetch(ctx, srcID, runID, saeURL, ".7z")
	if err != nil {
		return fail(err)
	}

	tmp, err := os.MkdirTemp("", "sae-q24-*")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(tmp)

	cmd := exec.CommandContext(ctx, "7z", "e", f.Path, "-o"+tmp, "-y", "-r", "Q24_2024r.csv")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fail(fmt.Errorf("extraction 7z : %w : %s", err, out))
	}
	csvPath := filepath.Join(tmp, "Q24_2024r.csv")
	if _, err := os.Stat(csvPath); err != nil {
		return fail(fmt.Errorf("Q24_2024r.csv absent de l'archive après extraction : %w", err))
	}

	lignes, err := lireCSVQ24(csvPath)
	if err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("SAE Q24 : aucune ligne lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.sae_personnel_fonction`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, l := range lignes {
		rows = append(rows, []any{
			l.Annee, l.FI, l.FIEJ,
			l.Direc, l.Dirsoin, l.Admin, l.Admto, l.Cadre, l.Infns, l.Infsp,
			l.Aides, l.Ashau, l.Psych, l.Sagfe, l.Reedu, l.Soito, l.Educs,
			l.Assis, l.Eduto, l.Phlab, l.Techn, l.Etppnm,
			srcID,
		})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "sae_personnel_fonction"},
		[]string{"annee", "nofinesset", "nofinessej",
			"etp_direction", "etp_direction_soins", "etp_administratif", "etp_admin_technique_ouvrier",
			"etp_cadre", "etp_infirmier", "etp_infirmier_specialise", "etp_aide_soignant",
			"etp_agent_service_hospitalier", "etp_psychologue", "etp_sage_femme", "etp_reeducation",
			"etp_social_educatif", "etp_educateur_specialise", "etp_assistant_service_social",
			"etp_autre_educatif", "etp_pharmacie_labo", "etp_technique", "etp_total_pnm",
			"source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("sae_personnel_fonction : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  SAE Q24 (personnel par fonction) : %d lignes\n", n)
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
