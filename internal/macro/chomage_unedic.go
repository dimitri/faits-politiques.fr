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
var SourceUnemploymentUnedic = archive.Source{
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
	reBracketSheet   = regexp.MustCompile(`(?i)tranche`)
	reSheetDate      = regexp.MustCompile(`au (\d{1,2})(?:er)? (\p{L}+) (\d{4})`)
	reClosedBracket  = regexp.MustCompile(`^(\d[\d ]*)\s*-\s*(\d[\d ]*)$`)
	reOpenBracketMax = regexp.MustCompile(`^(\d[\d ]*)\s*et plus$`)
	frenchMonths     = map[string]int{
		"janvier": 1, "février": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6,
		"juillet": 7, "août": 8, "septembre": 9, "octobre": 10, "novembre": 11, "décembre": 12,
	}
)

func IngestUnemploymentUnedic(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceUnemploymentUnedic)
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

	var allRows [][]any
	var quartersLoaded, rejectedSheets int
	for _, sheet := range x.sheetNames() {
		if !reBracketSheet.MatchString(sheet) || !strings.Contains(strings.ToLower(sheet), "montant") {
			continue
		}
		rows, err := loadUnedicSheet(x, sheet)
		if err != nil {
			rejectedSheets++
			continue
		}
		for _, r := range rows {
			allRows = append(allRows, append(r, srcID))
		}
		quartersLoaded++
	}
	if quartersLoaded == 0 {
		return fail(fmt.Errorf("aucune feuille de tranches reconnue sur %d feuilles", len(x.sheetNames())))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour tous les trimestres à chaque republication, changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_chomage_tranche_unedic (
			date_reference date, tranche_min int, tranche_max int, effectif int, pct numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_chomage_tranche_unedic"},
		[]string{"date_reference", "tranche_min", "tranche_max", "effectif", "pct", "source_id"},
		pgx.CopyFromRows(allRows)); err != nil {
		return fail(fmt.Errorf("core.chomage_tranche_unedic : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.chomage_tranche_unedic AS tgt
		USING tmp_chomage_tranche_unedic AS src
		ON tgt.date_reference = src.date_reference AND tgt.tranche_min = src.tranche_min
		WHEN MATCHED AND (tgt.tranche_max, tgt.effectif, tgt.pct, tgt.source_id)
		                  IS DISTINCT FROM (src.tranche_max, src.effectif, src.pct, src.source_id) THEN
		    UPDATE SET tranche_max = src.tranche_max, effectif = src.effectif,
		               pct = src.pct, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (date_reference, tranche_min, tranche_max, effectif, pct, source_id)
		    VALUES (src.date_reference, src.tranche_min, src.tranche_max, src.effectif, src.pct, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"trimestres_charges": quartersLoaded, "lignes_chargees": len(allRows),
			"rejet_feuille_non_reconnue": rejectedSheets, "touchees": touchees}, "")
	fmt.Printf("  répartition par tranche d'indemnisation : %d trimestres, %d lignes (Unédic), %d touchées par la fusion\n",
		quartersLoaded, len(allRows), touchees)
	return nil
}

// loadUnedicSheet traite une feuille "Tranches de montant(s)_<mois année>" :
// la date est relue dans le TITRE de la feuille (« au 30 juin 2025 »), plus
// fiable que le nom d'onglet (variantes de casse et d'apostrophe selon les
// trimestres). La colonne « Ensemble Assurance chômage » n'est pas à position
// fixe : son nombre de sous-allocations a changé (ADM ajouté en 2023), donc sa
// colonne. On la retrouve par en-tête plutôt que par lettre.
func loadUnedicSheet(x *xlsxFile, sheet string) ([][]any, error) {
	sheetRows, err := x.rows(sheet)
	if err != nil {
		return nil, err
	}
	if len(sheetRows) < 4 {
		return nil, fmt.Errorf("feuille %q trop courte", sheet)
	}

	// Ni le titre ni l'en-tête ne sont à un numéro de ligne fixe : certaines
	// feuilles portent une ou deux lignes vides au-dessus (mise en forme
	// héritée d'une édition à l'autre). On les retrouve par leur CONTENU.
	var date, headcountCol string
	var headerIdx = -1
	for i, l := range sheetRows {
		if date == "" {
			for _, v := range l {
				if m := reSheetDate.FindStringSubmatch(v); m != nil {
					day, _ := strconv.Atoi(m[1])
					monthNum, ok := frenchMonths[strings.ToLower(m[2])]
					if !ok {
						continue
					}
					year, _ := strconv.Atoi(m[3])
					date = fmt.Sprintf("%04d-%02d-%02d", year, monthNum, day)
					break
				}
			}
		}
		for col, v := range l {
			if v == "Ensemble Assurance chômage" {
				headcountCol, headerIdx = col, i
			}
		}
		if date != "" && headerIdx >= 0 {
			break
		}
	}
	if date == "" {
		return nil, fmt.Errorf("feuille %q : date introuvable", sheet)
	}
	if headcountCol == "" {
		return nil, fmt.Errorf("feuille %q : colonne « Ensemble Assurance chômage » introuvable", sheet)
	}
	pctCol := nextColumn(headcountCol)

	var rows [][]any
	var total float64
	for _, l := range sheetRows[headerIdx+1:] {
		label := l["B"]
		count, ok1 := numericValue(l, headcountCol)
		pct, ok2 := numericValue(l, pctCol)
		if !ok1 || !ok2 {
			continue
		}
		label = strings.ReplaceAll(label, " ", " ")
		if mm := reClosedBracket.FindStringSubmatch(label); mm != nil {
			min, _ := strconv.Atoi(strings.ReplaceAll(mm[1], " ", ""))
			max, _ := strconv.Atoi(strings.ReplaceAll(mm[2], " ", ""))
			rows = append(rows, []any{date, min, max, int64(count), pct * 100})
			total += pct
		} else if mm := reOpenBracketMax.FindStringSubmatch(label); mm != nil {
			min, _ := strconv.Atoi(strings.ReplaceAll(mm[1], " ", ""))
			rows = append(rows, []any{date, min, nil, int64(count), pct * 100})
			total += pct
		} else if label == "Total" {
			break
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("feuille %q : aucune tranche reconnue", sheet)
	}
	if total < 0.99 || total > 1.01 {
		return nil, fmt.Errorf("feuille %q : les tranches totalisent %.3f, pas 1", sheet, total)
	}
	return rows, nil
}

func nextColumn(col string) string {
	// Une seule lettre suffit ici : les feuilles Unédic ne dépassent pas Z.
	if len(col) != 1 {
		return col
	}
	return string(rune(col[0]) + 1)
}
