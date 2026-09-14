package macro

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La distribution des allocataires de l'Assurance chômage par tranche de
// montant d'indemnisation — comme core.pension_tranche_eir, pour mesurer sur
// une vraie distribution le biais d'une reprise fiscale non linéaire calculée
// sur une moyenne. Voir docs/revenu-universel-microsimulation.md.
var SourceChomageUnedic = archive.Source{
	Slug: "unedic-tranches-indemnisation", Label: "Unédic — répartition des allocataires par tranche d'indemnisation",
	Publisher: "Unédic / France Travail", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Unédic, Fichier national des allocataires (FNA)",
	Cadence:     "trimestrielle",
	Notes: "Allocataires de la solidarité-État (ASS, ATS, AER) exclus : leur montant " +
		"dépend des ressources, pas d'un salaire de référence, la source ne les " +
		"inclut pas. Colonne « Ensemble Assurance chômage » (ARE, AREF, CSP, et " +
		"ADM à partir de la publication qui l'introduit).",
}

const unedicXLSXURL = "https://www.data.gouv.fr/api/1/datasets/r/2aec0e50-1ec7-4809-b472-05d893a0c0f5"

var (
	reFeuilleTranches = regexp.MustCompile(`(?i)tranche`)
	reDateFeuille     = regexp.MustCompile(`au (\d{1,2})(?:er)? (\p{L}+) (\d{4})`)
	reTrancheFermee   = regexp.MustCompile(`^(\d[\d ]*)\s*-\s*(\d[\d ]*)$`)
	reTrancheOuverteM = regexp.MustCompile(`^(\d[\d ]*)\s*et plus$`)
	moisFR            = map[string]int{
		"janvier": 1, "février": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6,
		"juillet": 7, "août": 8, "septembre": 9, "octobre": 10, "novembre": 11, "décembre": 12,
	}
)

func IngestChomageUnedic(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceChomageUnedic)
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

	f, err := arch.Fetch(ctx, srcID, runID, unedicXLSXURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.chomage_tranche_unedic`); err != nil {
		return fail(err)
	}

	var trimestresCharges, lignesTotal, rejetFeuille int
	for _, feuille := range x.sheetNames() {
		if !reFeuilleTranches.MatchString(feuille) || !strings.Contains(strings.ToLower(feuille), "montant") {
			continue
		}
		n, err := chargerFeuilleUnedic(ctx, tx, x, feuille, srcID)
		if err != nil {
			rejetFeuille++
			continue
		}
		trimestresCharges++
		lignesTotal += n
	}
	if trimestresCharges == 0 {
		return fail(fmt.Errorf("aucune feuille de tranches reconnue sur %d feuilles", len(x.sheetNames())))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"trimestres_charges": trimestresCharges, "lignes_chargees": lignesTotal,
			"rejet_feuille_non_reconnue": rejetFeuille}, "")
	fmt.Printf("  répartition par tranche d'indemnisation : %d trimestres, %d lignes (Unédic)\n",
		trimestresCharges, lignesTotal)
	return nil
}

// chargerFeuilleUnedic traite une feuille "Tranches de montant(s)_<mois année>" :
// la date est relue dans le TITRE de la feuille (« au 30 juin 2025 »), plus
// fiable que le nom d'onglet (variantes de casse et d'apostrophe selon les
// trimestres). La colonne « Ensemble Assurance chômage » n'est pas à position
// fixe : son nombre de sous-allocations a changé (ADM ajouté en 2023), donc sa
// colonne. On la retrouve par en-tête plutôt que par lettre.
func chargerFeuilleUnedic(ctx context.Context, tx pgx.Tx, x *xlsxFile, feuille string, srcID int64) (int, error) {
	lignes, err := x.rows(feuille)
	if err != nil {
		return 0, err
	}
	if len(lignes) < 4 {
		return 0, fmt.Errorf("feuille %q trop courte", feuille)
	}

	// Ni le titre ni l'en-tête ne sont à un numéro de ligne fixe : certaines
	// feuilles portent une ou deux lignes vides au-dessus (mise en forme
	// héritée d'une édition à l'autre). On les retrouve par leur CONTENU.
	var date, colEffectif string
	var headerIdx = -1
	for i, l := range lignes {
		if date == "" {
			for _, v := range l {
				if m := reDateFeuille.FindStringSubmatch(v); m != nil {
					jour, _ := strconv.Atoi(m[1])
					moisNum, ok := moisFR[strings.ToLower(m[2])]
					if !ok {
						continue
					}
					annee, _ := strconv.Atoi(m[3])
					date = fmt.Sprintf("%04d-%02d-%02d", annee, moisNum, jour)
					break
				}
			}
		}
		for col, v := range l {
			if v == "Ensemble Assurance chômage" {
				colEffectif, headerIdx = col, i
			}
		}
		if date != "" && headerIdx >= 0 {
			break
		}
	}
	if date == "" {
		return 0, fmt.Errorf("feuille %q : date introuvable", feuille)
	}
	if colEffectif == "" {
		return 0, fmt.Errorf("feuille %q : colonne « Ensemble Assurance chômage » introuvable", feuille)
	}
	colPct := colonneSuivante(colEffectif)

	var rows [][]any
	var total float64
	for _, l := range lignes[headerIdx+1:] {
		lib := l["B"]
		eff, ok1 := valeurNumerique(l, colEffectif)
		pct, ok2 := valeurNumerique(l, colPct)
		if !ok1 || !ok2 {
			continue
		}
		lib = strings.ReplaceAll(lib, " ", " ")
		if mm := reTrancheFermee.FindStringSubmatch(lib); mm != nil {
			min, _ := strconv.Atoi(strings.ReplaceAll(mm[1], " ", ""))
			max, _ := strconv.Atoi(strings.ReplaceAll(mm[2], " ", ""))
			rows = append(rows, []any{date, min, max, int64(eff), pct * 100, srcID})
			total += pct
		} else if mm := reTrancheOuverteM.FindStringSubmatch(lib); mm != nil {
			min, _ := strconv.Atoi(strings.ReplaceAll(mm[1], " ", ""))
			rows = append(rows, []any{date, min, nil, int64(eff), pct * 100, srcID})
			total += pct
		} else if lib == "Total" {
			break
		}
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("feuille %q : aucune tranche reconnue", feuille)
	}
	if total < 0.99 || total > 1.01 {
		return 0, fmt.Errorf("feuille %q : les tranches totalisent %.3f, pas 1", feuille, total)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "chomage_tranche_unedic"},
		[]string{"date_reference", "tranche_min", "tranche_max", "effectif", "pct", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return 0, fmt.Errorf("feuille %q : %w", feuille, err)
	}
	return len(rows), nil
}

func colonneSuivante(col string) string {
	// Une seule lettre suffit ici : les feuilles Unédic ne dépassent pas Z.
	if len(col) != 1 {
		return col
	}
	return string(rune(col[0]) + 1)
}
