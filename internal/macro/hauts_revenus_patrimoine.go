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
	"github.com/xuri/excelize/v2"
)

// Ce que le dernier décile de niveau de vie (core.filosofi_decile_national,
// D1 à D9 seulement) ne peut pas montrer : à l'intérieur du dixième le plus
// aisé, le 1 %, le 0,1 % et le 0,01 % ne vivent pas la même chose. Deux
// fiches Insee, une pour le revenu, une pour le patrimoine — deux notions
// distinctes, jamais additionnées ici. Voir docs/repartition-richesse-donnees.md
// et le commentaire de la migration 0117.
var SourceTopIncomesWealth = archive.Source{
	Slug: "insee-hauts-revenus-patrimoine", Label: "INSEE — hauts revenus et hauts patrimoines",
	Publisher: "INSEE (Insee-DGFiP-Cnaf-Cnav-CCMSA, Filosofi ; enquêtes Patrimoine)",
	Tier:      "PRIMARY_OFFICIAL",
	License:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, \"Les revenus et le patrimoine des ménages\", édition 2024",
	Cadence:     "annuelle (édition), millésimes des données irréguliers selon la fiche",
	Notes: "Fiche \"Très hauts revenus\" (seuils 2021, série de parts 2004-2021) et fiche " +
		"\"Les hauts patrimoines\" (2015 et 2021 seulement, patrimoine BRUT). Le revenu et le " +
		"patrimoine sont deux notions distinctes, chargées séparément, jamais additionnées.",
}

const (
	topIncomesURL = "https://www.insee.fr/fr/statistiques/fichier/7941389/RPM2024-F10.xlsx"
	topWealthURL  = "https://www.insee.fr/fr/statistiques/fichier/8272285/RPM2024-F28.xlsx"
)

// parseEuros : les montants sont écrits avec la virgule comme séparateur
// de milliers dans ce classeur (ex. "1,160,070"), jamais comme décimale —
// vérifié sur les totaux (chaque revenu avant redistribution est cohérent
// avec les niveaux de vie voisins une fois la virgule retirée).
func parseEuros(s string) (int, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	return strconv.Atoi(s)
}

// reThreshold : "(D5)", "(D9)", "(Q99)", "(Q99,9)", "(Q99,99)" -> D5, D9, Q99,
// Q99_9, Q99_99 (la virgule décimale du taux devient un underscore, seul
// séparateur admis par le CHECK de la migration 0117).
var reThreshold = regexp.MustCompile(`^\(([A-Z0-9,]+)\)$`)

func thresholdCode(raw string) (string, error) {
	m := reThreshold.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", fmt.Errorf("seuil illisible : %q", raw)
	}
	return strings.ReplaceAll(m[1], ",", "_"), nil
}

// groupCode : le libellé en toutes lettres d'une ligne du tableau
// complémentaire vers son code — comparaison sur des fragments stables
// (les espaces insécables et la ponctuation varient selon le rendu Excel).
func groupCode(label string) (string, error) {
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "90") && strings.Contains(l, "modestes"):
		return "90_MODESTES", nil
	case strings.Contains(l, "9") && strings.Contains(l, "suivants") && !strings.Contains(l, "0,9"):
		return "9_SUIVANTS", nil
	case strings.Contains(l, "0,9") && strings.Contains(l, "suivants"):
		return "0_9_SUIVANTS", nil
	case strings.Contains(l, "0,1") && strings.Contains(l, "plus aisés"):
		return "0_1_PLUS_AISES", nil
	case strings.Contains(l, "les 1") && strings.Contains(l, "plus aisés"):
		return "1_PLUS_AISES", nil
	}
	return "", fmt.Errorf("groupe illisible : %q", label)
}

func IngestTopIncomesWealth(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTopIncomesWealth)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "hauts-revenus-patrimoine-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	fIncomes, err := arch.Fetch(ctx, srcID, runID, topIncomesURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wbIncomes, err := excelize.OpenFile(fIncomes.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur hauts revenus illisible : %w", err))
	}
	defer wbIncomes.Close()

	fWealth, err := arch.Fetch(ctx, srcID, runID, topWealthURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wbWealth, err := excelize.OpenFile(fWealth.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur hauts patrimoines illisible : %w", err))
	}
	defer wbWealth.Close()

	thresholds, err := readTopIncomeThresholds(wbIncomes)
	if err != nil {
		return fail(err)
	}
	shares, err := readIncomeShareByGroup(wbIncomes)
	if err != nil {
		return fail(err)
	}
	wealth, err := readTopWealth(wbWealth)
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `TRUNCATE core.filosofi_haut_revenu, core.revenu_part_groupe, core.patrimoine_haut`); err != nil {
		return fail(err)
	}

	var thresholdRows [][]any
	for _, t := range thresholds {
		thresholdRows = append(thresholdRows, []any{t.year, t.threshold, t.incomeBefore, t.livingStandard, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "filosofi_haut_revenu"},
		[]string{"annee", "seuil", "revenu_avant_redistribution_eur", "niveau_de_vie_eur", "source_id"},
		pgx.CopyFromRows(thresholdRows)); err != nil {
		return fail(fmt.Errorf("filosofi_haut_revenu : %w", err))
	}

	var shareRows [][]any
	for _, sh := range shares {
		shareRows = append(shareRows, []any{sh.year, sh.group, sh.sharePct, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "revenu_part_groupe"},
		[]string{"annee", "groupe", "part_pct", "source_id"},
		pgx.CopyFromRows(shareRows)); err != nil {
		return fail(fmt.Errorf("revenu_part_groupe : %w", err))
	}

	var wealthRows [][]any
	for _, w := range wealth {
		var massShare *float64
		if w.massShareOK {
			massShare = &w.massShare
		}
		wealthRows = append(wealthRows, []any{w.year, w.bracket, w.lowerThreshold, w.average, massShare, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "patrimoine_haut"},
		[]string{"annee", "tranche", "seuil_bas_eur", "patrimoine_moyen_eur", "part_masse_pct", "source_id"},
		pgx.CopyFromRows(wealthRows)); err != nil {
		return fail(fmt.Errorf("patrimoine_haut : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"seuils": len(thresholds), "parts": len(shares), "patrimoines": len(wealth)}, "")
	fmt.Printf("  hauts revenus et patrimoines (Insee) : %d seuils, %d parts, %d tranches de patrimoine\n",
		len(thresholds), len(shares), len(wealth))
	return nil
}

type topIncomeThresholdRow struct {
	year                         int
	threshold                    string
	incomeBefore, livingStandard int
}

// readTopIncomeThresholds : feuille "Figure 1", un seul millésime (2021, dans
// le titre de la feuille — pas une colonne, à extraire du titre).
func readTopIncomeThresholds(wb *excelize.File) ([]topIncomeThresholdRow, error) {
	rows, err := wb.GetRows("Figure 1")
	if err != nil {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts revenus) : %w", err)
	}
	if len(rows) < 9 {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts revenus) trop courte (%d lignes) — format changé", len(rows))
	}
	year, err := titleYear(rows[0][0])
	if err != nil {
		return nil, err
	}
	var out []topIncomeThresholdRow
	for _, row := range rows[4:9] {
		if len(row) < 4 {
			return nil, fmt.Errorf("ligne de seuil trop courte : %q", row)
		}
		code, err := thresholdCode(row[1])
		if err != nil {
			return nil, err
		}
		before, err := parseEuros(row[2])
		if err != nil {
			return nil, fmt.Errorf("%s : revenu avant redistribution %q illisible : %w", code, row[2], err)
		}
		livingStandard, err := parseEuros(row[3])
		if err != nil {
			return nil, fmt.Errorf("%s : niveau de vie %q illisible : %w", code, row[3], err)
		}
		out = append(out, topIncomeThresholdRow{year, code, before, livingStandard})
	}
	return out, nil
}

type incomeShareRow struct {
	year     int
	group    string
	sharePct float64
}

// readIncomeShareByGroup : feuille "Tableau complémentaire", une ligne par
// groupe, une colonne par année (2004 à 2021). Les en-têtes 2012 et 2013
// portent un renvoi de note ("20121", "20132") : seuls les quatre premiers
// caractères sont l'année.
func readIncomeShareByGroup(wb *excelize.File) ([]incomeShareRow, error) {
	rows, err := wb.GetRows("Tableau complémentaire")
	if err != nil {
		return nil, fmt.Errorf("feuille 'Tableau complémentaire' : %w", err)
	}
	if len(rows) < 8 {
		return nil, fmt.Errorf("feuille 'Tableau complémentaire' trop courte (%d lignes) — format changé", len(rows))
	}
	header := rows[2]
	if len(header) < 2 {
		return nil, fmt.Errorf("en-tête 'Tableau complémentaire' illisible")
	}
	var years []int
	for _, h := range header[1:] {
		h = strings.TrimSpace(h)
		if len(h) < 4 {
			return nil, fmt.Errorf("en-tête année illisible : %q", h)
		}
		y, err := strconv.Atoi(h[:4])
		if err != nil {
			return nil, fmt.Errorf("en-tête année illisible : %q : %w", h, err)
		}
		years = append(years, y)
	}
	var out []incomeShareRow
	for _, row := range rows[3:8] {
		if len(row) < len(years)+1 {
			return nil, fmt.Errorf("ligne de groupe trop courte : %q", row)
		}
		group, err := groupCode(row[0])
		if err != nil {
			return nil, err
		}
		for i, year := range years {
			v, err := strconv.ParseFloat(strings.TrimSpace(row[i+1]), 64)
			if err != nil {
				return nil, fmt.Errorf("%s %d : part %q illisible : %w", group, year, row[i+1], err)
			}
			out = append(out, incomeShareRow{year, group, v})
		}
	}
	return out, nil
}

type topWealthRow struct {
	year                    int
	bracket                 string
	lowerThreshold, average int
	massShare               float64
	massShareOK             bool
}

var wealthBrackets = []struct{ label, code string }{
	{"90e au 95e", "P90_P95"},
	{"95e au 99e", "P95_P99"},
	{"supérieur au 99e", "SUP_P99"},
}

// readTopWealth : feuille "Figure 1" du classeur hauts patrimoines,
// deux millésimes CÔTE À CÔTE (2015 et 2021), une seule fois la part de
// masse (2021 seulement — la source ne la publie pas pour 2015).
func readTopWealth(wb *excelize.File) ([]topWealthRow, error) {
	rows, err := wb.GetRows("Figure 1")
	if err != nil {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts patrimoines) : %w", err)
	}
	if len(rows) < 7 {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts patrimoines) trop courte (%d lignes) — format changé", len(rows))
	}
	var out []topWealthRow
	for _, row := range rows[4:7] {
		if len(row) < 8 {
			return nil, fmt.Errorf("ligne de patrimoine trop courte : %q", row)
		}
		var code string
		for _, b := range wealthBrackets {
			if strings.Contains(strings.ToLower(row[0]), b.label) {
				code = b.code
				break
			}
		}
		if code == "" {
			return nil, fmt.Errorf("tranche de patrimoine illisible : %q", row[0])
		}
		threshold2015, err := parseEuros(row[1])
		if err != nil {
			return nil, fmt.Errorf("%s 2015 : seuil %q illisible : %w", code, row[1], err)
		}
		threshold2021, err := parseEuros(row[2])
		if err != nil {
			return nil, fmt.Errorf("%s 2021 : seuil %q illisible : %w", code, row[2], err)
		}
		average2015, err := parseEuros(row[4])
		if err != nil {
			return nil, fmt.Errorf("%s 2015 : moyenne %q illisible : %w", code, row[4], err)
		}
		average2021, err := parseEuros(row[5])
		if err != nil {
			return nil, fmt.Errorf("%s 2021 : moyenne %q illisible : %w", code, row[5], err)
		}
		massShare2021, err := strconv.ParseFloat(strings.TrimSpace(row[7]), 64)
		if err != nil {
			return nil, fmt.Errorf("%s 2021 : part de masse %q illisible : %w", code, row[7], err)
		}
		out = append(out,
			topWealthRow{2015, code, threshold2015, average2015, 0, false},
			topWealthRow{2021, code, threshold2021, average2021, massShare2021, true})
	}
	return out, nil
}

var reTitleYear = regexp.MustCompile(`\b(20\d\d)\b`)

func titleYear(title string) (int, error) {
	m := reTitleYear.FindStringSubmatch(title)
	if m == nil {
		return 0, fmt.Errorf("aucun millésime trouvé dans le titre %q", title)
	}
	return strconv.Atoi(m[1])
}
