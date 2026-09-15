// Package damir charge Open Damir (extraction ouverte du SNDS,
// remboursements de l'Assurance Maladie) — voir docs/sante-donnees.md § 4 et
// le commentaire de core.remboursement_national.
package damir

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "damir-v1"

// 36,6 millions de lignes pour le seul mois de janvier 2025 (~970 Mo
// compressés) : voir le commentaire de core.remboursement_national pour ce
// qui est conservé de cette masse, et pourquoi.
var SourceDamir = archive.Source{
	Slug: "open-damir", Label: "Open Damir — dépenses d'assurance maladie interrégimes (SNDS)",
	Publisher: "Caisse nationale de l'Assurance Maladie (CNAM)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : CNAM, Open Damir",
	Cadence:     "mensuelle",
	Notes: "Chaque fichier mensuel (~970 Mo compressés, 36,6 millions de lignes en janvier " +
		"2025) est agrégé en flux à l'ingestion — jamais chargé ligne à ligne. Agrégation par " +
		"mois de TRAITEMENT (FLX_ANN_MOI), pas de soins (SOI_ANN/SOI_MOI) : un fichier mensuel " +
		"contient des soins de centaines de mois différents (remboursements tardifs), qu'un " +
		"agrégat par mois de soins laisserait incomplet tant que les fichiers postérieurs ne " +
		"sont pas relus. Filtré sur PRS_REM_TYP=0, le filtre que le descriptif des variables " +
		"CNAM documente explicitement pour éviter de compter deux fois le dénombrement.",
}

const (
	pageURL    = "https://open-data-assurance-maladie.ameli.fr/depenses/download.php?Dir_Rep=Open_DAMIR&Annee=%d"
	fichierURL = "https://open-data-assurance-maladie.ameli.fr/depenses/download_file.php?token=%s&file=Open_DAMIR/A%d%02d.csv.gz"
)

var reToken = regexp.MustCompile(`token=([a-f0-9]+)`)

// Position des colonnes (0-indexées) dans le fichier plat, vérifiée sur un
// fichier réel plutôt que supposée depuis le seul descriptif CNAM.
const (
	colBenResReg = 3
	colPrsActNbr = 17
	colPrsRemBse = 21
	colPrsRemMnt = 22
	colPrsNat    = 39
	colPrsRemTyp = 42
)

// agregat accumule un (montant, actes) par (année, mois, région, nature de prestation).
type agregat struct {
	montant float64
	actes   int64
}

func IngestDamir(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, annees []int) error {
	srcID, err := arch.EnsureSource(ctx, SourceDamir)
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

	type ligneNationale struct {
		annee, mois   int
		montant, base float64
		actes         int64
	}
	var national []ligneNationale
	// region_prestation : (annee, mois, region, prsNat) -> agrégat. Purgée
	// et réécrite en entier à chaque exécution (mêmes années rechargées).
	regionPrestation := map[[4]any]*agregat{}

	client := &http.Client{Timeout: 60 * time.Second}

	for _, annee := range annees {
		page, err := client.Get(fmt.Sprintf(pageURL, annee))
		if err != nil {
			return fail(fmt.Errorf("%d : page de téléchargement : %w", annee, err))
		}
		corps, err := io.ReadAll(page.Body)
		page.Body.Close()
		if err != nil {
			return fail(fmt.Errorf("%d : lecture de la page : %w", annee, err))
		}
		m := reToken.FindStringSubmatch(string(corps))
		if m == nil {
			return fail(fmt.Errorf("%d : jeton de téléchargement introuvable — la page a peut-être changé", annee))
		}
		token := m[1]

		for mois := 1; mois <= 12; mois++ {
			url := fmt.Sprintf(fichierURL, token, annee, mois)
			f, err := arch.Fetch(ctx, srcID, runID, url, ".csv.gz")
			if err != nil {
				return fail(fmt.Errorf("%04d-%02d : %w", annee, mois, err))
			}
			montant, base, actes, err := agregerFichier(f.Path, regionPrestation, annee, mois)
			if err != nil {
				return fail(fmt.Errorf("%04d-%02d : %w", annee, mois, err))
			}
			national = append(national, ligneNationale{annee, mois, montant, base, actes})
			fmt.Printf("  Open Damir %04d-%02d : %.1f M€ remboursés, %d actes\n", annee, mois, montant/1e6, actes)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// --- national : petit volume, TRUNCATE+COPY simple, la contrainte de
	// clé (déjà en place depuis la migration) ne coûte rien à cette échelle.
	if _, err := tx.Exec(ctx, `TRUNCATE core.remboursement_national`); err != nil {
		return fail(err)
	}
	var lotNat [][]any
	for _, l := range national {
		lotNat = append(lotNat, []any{l.annee, l.mois, l.montant, l.base, l.actes, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "remboursement_national"},
		[]string{"annee", "mois", "montant_rembourse", "base_remboursement", "nb_actes", "source_id"},
		pgx.CopyFromRows(lotNat)); err != nil {
		return fail(fmt.Errorf("remboursement_national : %w", err))
	}

	// --- région × prestation : volume réel (dizaines de milliers de lignes
	// par année). TRUNCATE dans la même transaction que le COPY (plutôt que
	// DELETE) pour que Postgres traite la table comme neuve dans cette
	// transaction — sans quoi DELETE laisserait des lignes mortes que
	// seul un VACUUM ultérieur récupérerait. L'index unique est retiré
	// avant le COPY et reconstruit d'un coup après : un COPY massif dans
	// une table indexée maintient l'index ligne à ligne, ce qui coûte bien
	// plus cher qu'un seul CREATE INDEX final sur les données déjà en place.
	if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS core.remboursement_region_prestation_uniq`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `TRUNCATE core.remboursement_region_prestation`); err != nil {
		return fail(err)
	}
	var lotReg [][]any
	for cle, ag := range regionPrestation {
		lotReg = append(lotReg, []any{cle[0], cle[1], cle[2], cle[3], ag.montant, ag.actes, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "remboursement_region_prestation"},
		[]string{"annee", "mois", "region_code", "prs_nat_code", "montant_rembourse", "nb_actes", "source_id"},
		pgx.CopyFromRows(lotReg)); err != nil {
		return fail(fmt.Errorf("remboursement_region_prestation : %w", err))
	}
	if _, err := tx.Exec(ctx, `
		CREATE UNIQUE INDEX remboursement_region_prestation_uniq
		  ON core.remboursement_region_prestation (annee, mois, region_code, prs_nat_code)`); err != nil {
		return fail(fmt.Errorf("index remboursement_region_prestation : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"mois_charges": len(national), "lignes_region_prestation": len(lotReg)}, "")
	fmt.Printf("  Open Damir : %d mois chargés, %d lignes région×prestation\n", len(national), len(lotReg))
	return nil
}

// agregerFichier lit un fichier .csv.gz en flux (jamais chargé entier en
// mémoire) et accumule les montants/actes filtrés sur PRS_REM_TYP=0 —
// national (retourné) et région×prestation (accumulé dans regionPrestation,
// partagé entre tous les mois chargés).
func agregerFichier(path string, regionPrestation map[[4]any]*agregat, annee, mois int) (montantTotal, baseTotal float64, actesTotal int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, 0, 0, err
	}
	defer gz.Close()

	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() {
		return 0, 0, 0, fmt.Errorf("fichier vide")
	}
	nColonnes := len(strings.Split(sc.Text(), ";"))

	for sc.Scan() {
		l := strings.Split(sc.Text(), ";")
		if len(l) < nColonnes {
			continue // ligne tronquée en fin de fichier
		}
		if l[colPrsRemTyp] != "0" {
			continue // voir le commentaire de core.remboursement_national : filtre CNAM obligatoire
		}
		montant, e1 := strconv.ParseFloat(l[colPrsRemMnt], 64)
		base, e2 := strconv.ParseFloat(l[colPrsRemBse], 64)
		actes, e3 := strconv.ParseFloat(l[colPrsActNbr], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		montantTotal += montant
		baseTotal += base
		actesTotal += int64(actes)

		cle := [4]any{annee, mois, l[colBenResReg], l[colPrsNat]}
		a := regionPrestation[cle]
		if a == nil {
			a = &agregat{}
			regionPrestation[cle] = a
		}
		a.montant += montant
		a.actes += int64(actes)
	}
	if err := sc.Err(); err != nil {
		return 0, 0, 0, fmt.Errorf("lecture : %w", err)
	}
	return montantTotal, baseTotal, actesTotal, nil
}

// Ingest charge l'année la plus récente complète au moment de l'écriture.
// Chaque année supplémentaire ajoute ~11 Go de téléchargement (970 Mo × 12) :
// étendre la liste plutôt que changer la valeur par défaut sans le
// documenter, si un chargement plus profond est décidé.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return IngestDamir(ctx, pool, arch, []int{2025})
}
