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
var SourceHautsRevenusPatrimoine = archive.Source{
	Slug: "insee-hauts-revenus-patrimoine", Label: "INSEE — hauts revenus et hauts patrimoines",
	Publisher: "INSEE (Insee-DGFiP-Cnaf-Cnav-CCMSA, Filosofi ; enquêtes Patrimoine)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, \"Les revenus et le patrimoine des ménages\", édition 2024",
	Cadence:     "annuelle (édition), millésimes des données irréguliers selon la fiche",
	Notes: "Fiche \"Très hauts revenus\" (seuils 2021, série de parts 2004-2021) et fiche " +
		"\"Les hauts patrimoines\" (2015 et 2021 seulement, patrimoine BRUT). Le revenu et le " +
		"patrimoine sont deux notions distinctes, chargées séparément, jamais additionnées.",
}

const (
	urlHautsRevenus  = "https://www.insee.fr/fr/statistiques/fichier/7941389/RPM2024-F10.xlsx"
	urlHautsPatrimoines = "https://www.insee.fr/fr/statistiques/fichier/8272285/RPM2024-F28.xlsx"
)

// nettoyerEuros : les montants sont écrits avec la virgule comme séparateur
// de milliers dans ce classeur (ex. "1,160,070"), jamais comme décimale —
// vérifié sur les totaux (chaque revenu avant redistribution est cohérent
// avec les niveaux de vie voisins une fois la virgule retirée).
func nettoyerEuros(s string) (int, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	return strconv.Atoi(s)
}

// reSeuil : "(D5)", "(D9)", "(Q99)", "(Q99,9)", "(Q99,99)" -> D5, D9, Q99,
// Q99_9, Q99_99 (la virgule décimale du taux devient un underscore, seul
// séparateur admis par le CHECK de la migration 0117).
var reSeuil = regexp.MustCompile(`^\(([A-Z0-9,]+)\)$`)

func codeSeuil(brut string) (string, error) {
	m := reSeuil.FindStringSubmatch(strings.TrimSpace(brut))
	if m == nil {
		return "", fmt.Errorf("seuil illisible : %q", brut)
	}
	return strings.ReplaceAll(m[1], ",", "_"), nil
}

// codeGroupe : le libellé en toutes lettres d'une ligne du tableau
// complémentaire vers son code — comparaison sur des fragments stables
// (les espaces insécables et la ponctuation varient selon le rendu Excel).
func codeGroupe(libelle string) (string, error) {
	l := strings.ToLower(libelle)
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
	return "", fmt.Errorf("groupe illisible : %q", libelle)
}

func IngestHautsRevenusPatrimoine(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceHautsRevenusPatrimoine)
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

	fRevenus, err := arch.Fetch(ctx, srcID, runID, urlHautsRevenus, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wbRevenus, err := excelize.OpenFile(fRevenus.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur hauts revenus illisible : %w", err))
	}
	defer wbRevenus.Close()

	fPatrimoines, err := arch.Fetch(ctx, srcID, runID, urlHautsPatrimoines, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wbPatrimoines, err := excelize.OpenFile(fPatrimoines.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur hauts patrimoines illisible : %w", err))
	}
	defer wbPatrimoines.Close()

	seuils, err := lireSeuilsHautRevenu(wbRevenus)
	if err != nil {
		return fail(err)
	}
	parts, err := lireRevenuPartGroupe(wbRevenus)
	if err != nil {
		return fail(err)
	}
	patrimoines, err := lirePatrimoineHaut(wbPatrimoines)
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

	var rowsSeuils [][]any
	for _, s := range seuils {
		rowsSeuils = append(rowsSeuils, []any{s.annee, s.seuil, s.revenuAvant, s.niveauDeVie, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "filosofi_haut_revenu"},
		[]string{"annee", "seuil", "revenu_avant_redistribution_eur", "niveau_de_vie_eur", "source_id"},
		pgx.CopyFromRows(rowsSeuils)); err != nil {
		return fail(fmt.Errorf("filosofi_haut_revenu : %w", err))
	}

	var rowsParts [][]any
	for _, p := range parts {
		rowsParts = append(rowsParts, []any{p.annee, p.groupe, p.partPct, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "revenu_part_groupe"},
		[]string{"annee", "groupe", "part_pct", "source_id"},
		pgx.CopyFromRows(rowsParts)); err != nil {
		return fail(fmt.Errorf("revenu_part_groupe : %w", err))
	}

	var rowsPatrimoine [][]any
	for _, p := range patrimoines {
		var part *float64
		if p.partMasseOK {
			part = &p.partMasse
		}
		rowsPatrimoine = append(rowsPatrimoine, []any{p.annee, p.tranche, p.seuilBas, p.moyen, part, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "patrimoine_haut"},
		[]string{"annee", "tranche", "seuil_bas_eur", "patrimoine_moyen_eur", "part_masse_pct", "source_id"},
		pgx.CopyFromRows(rowsPatrimoine)); err != nil {
		return fail(fmt.Errorf("patrimoine_haut : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"seuils": len(seuils), "parts": len(parts), "patrimoines": len(patrimoines)}, "")
	fmt.Printf("  hauts revenus et patrimoines (Insee) : %d seuils, %d parts, %d tranches de patrimoine\n",
		len(seuils), len(parts), len(patrimoines))
	return nil
}

type ligneSeuilRevenu struct {
	annee                  int
	seuil                  string
	revenuAvant, niveauDeVie int
}

// lireSeuilsHautRevenu : feuille "Figure 1", un seul millésime (2021, dans
// le titre de la feuille — pas une colonne, à extraire du titre).
func lireSeuilsHautRevenu(wb *excelize.File) ([]ligneSeuilRevenu, error) {
	rows, err := wb.GetRows("Figure 1")
	if err != nil {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts revenus) : %w", err)
	}
	if len(rows) < 9 {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts revenus) trop courte (%d lignes) — format changé", len(rows))
	}
	annee, err := anneeDuTitre(rows[0][0])
	if err != nil {
		return nil, err
	}
	var out []ligneSeuilRevenu
	for _, l := range rows[4:9] {
		if len(l) < 4 {
			return nil, fmt.Errorf("ligne de seuil trop courte : %q", l)
		}
		code, err := codeSeuil(l[1])
		if err != nil {
			return nil, err
		}
		avant, err := nettoyerEuros(l[2])
		if err != nil {
			return nil, fmt.Errorf("%s : revenu avant redistribution %q illisible : %w", code, l[2], err)
		}
		niveau, err := nettoyerEuros(l[3])
		if err != nil {
			return nil, fmt.Errorf("%s : niveau de vie %q illisible : %w", code, l[3], err)
		}
		out = append(out, ligneSeuilRevenu{annee, code, avant, niveau})
	}
	return out, nil
}

type lignePartGroupe struct {
	annee   int
	groupe  string
	partPct float64
}

// lireRevenuPartGroupe : feuille "Tableau complémentaire", une ligne par
// groupe, une colonne par année (2004 à 2021). Les en-têtes 2012 et 2013
// portent un renvoi de note ("20121", "20132") : seuls les quatre premiers
// caractères sont l'année.
func lireRevenuPartGroupe(wb *excelize.File) ([]lignePartGroupe, error) {
	rows, err := wb.GetRows("Tableau complémentaire")
	if err != nil {
		return nil, fmt.Errorf("feuille 'Tableau complémentaire' : %w", err)
	}
	if len(rows) < 8 {
		return nil, fmt.Errorf("feuille 'Tableau complémentaire' trop courte (%d lignes) — format changé", len(rows))
	}
	entete := rows[2]
	if len(entete) < 2 {
		return nil, fmt.Errorf("en-tête 'Tableau complémentaire' illisible")
	}
	var annees []int
	for _, h := range entete[1:] {
		h = strings.TrimSpace(h)
		if len(h) < 4 {
			return nil, fmt.Errorf("en-tête année illisible : %q", h)
		}
		a, err := strconv.Atoi(h[:4])
		if err != nil {
			return nil, fmt.Errorf("en-tête année illisible : %q : %w", h, err)
		}
		annees = append(annees, a)
	}
	var out []lignePartGroupe
	for _, l := range rows[3:8] {
		if len(l) < len(annees)+1 {
			return nil, fmt.Errorf("ligne de groupe trop courte : %q", l)
		}
		groupe, err := codeGroupe(l[0])
		if err != nil {
			return nil, err
		}
		for i, annee := range annees {
			v, err := strconv.ParseFloat(strings.TrimSpace(l[i+1]), 64)
			if err != nil {
				return nil, fmt.Errorf("%s %d : part %q illisible : %w", groupe, annee, l[i+1], err)
			}
			out = append(out, lignePartGroupe{annee, groupe, v})
		}
	}
	return out, nil
}

type lignePatrimoineHaut struct {
	annee               int
	tranche             string
	seuilBas, moyen     int
	partMasse           float64
	partMasseOK         bool
}

var tranchesPatrimoine = []struct{ libelle, code string }{
	{"90e au 95e", "P90_P95"},
	{"95e au 99e", "P95_P99"},
	{"supérieur au 99e", "SUP_P99"},
}

// lirePatrimoineHaut : feuille "Figure 1" du classeur hauts patrimoines,
// deux millésimes CÔTE À CÔTE (2015 et 2021), une seule fois la part de
// masse (2021 seulement — la source ne la publie pas pour 2015).
func lirePatrimoineHaut(wb *excelize.File) ([]lignePatrimoineHaut, error) {
	rows, err := wb.GetRows("Figure 1")
	if err != nil {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts patrimoines) : %w", err)
	}
	if len(rows) < 7 {
		return nil, fmt.Errorf("feuille 'Figure 1' (hauts patrimoines) trop courte (%d lignes) — format changé", len(rows))
	}
	var out []lignePatrimoineHaut
	for _, l := range rows[4:7] {
		if len(l) < 8 {
			return nil, fmt.Errorf("ligne de patrimoine trop courte : %q", l)
		}
		var code string
		for _, t := range tranchesPatrimoine {
			if strings.Contains(strings.ToLower(l[0]), t.libelle) {
				code = t.code
				break
			}
		}
		if code == "" {
			return nil, fmt.Errorf("tranche de patrimoine illisible : %q", l[0])
		}
		seuil2015, err := nettoyerEuros(l[1])
		if err != nil {
			return nil, fmt.Errorf("%s 2015 : seuil %q illisible : %w", code, l[1], err)
		}
		seuil2021, err := nettoyerEuros(l[2])
		if err != nil {
			return nil, fmt.Errorf("%s 2021 : seuil %q illisible : %w", code, l[2], err)
		}
		moyen2015, err := nettoyerEuros(l[4])
		if err != nil {
			return nil, fmt.Errorf("%s 2015 : moyenne %q illisible : %w", code, l[4], err)
		}
		moyen2021, err := nettoyerEuros(l[5])
		if err != nil {
			return nil, fmt.Errorf("%s 2021 : moyenne %q illisible : %w", code, l[5], err)
		}
		part2021, err := strconv.ParseFloat(strings.TrimSpace(l[7]), 64)
		if err != nil {
			return nil, fmt.Errorf("%s 2021 : part de masse %q illisible : %w", code, l[7], err)
		}
		out = append(out,
			lignePatrimoineHaut{2015, code, seuil2015, moyen2015, 0, false},
			lignePatrimoineHaut{2021, code, seuil2021, moyen2021, part2021, true})
	}
	return out, nil
}

var reAnneeTitre = regexp.MustCompile(`\b(20\d\d)\b`)

func anneeDuTitre(titre string) (int, error) {
	m := reAnneeTitre.FindStringSubmatch(titre)
	if m == nil {
		return 0, fmt.Errorf("aucun millésime trouvé dans le titre %q", titre)
	}
	return strconv.Atoi(m[1])
}
