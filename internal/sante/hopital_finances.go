package sante

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// SourceHopitalFinances : la Drees publie, dans le fichier Excel qui
// accompagne chaque édition de son Panorama « Les établissements de santé »,
// une fiche dédiée à la situation économique et financière des hôpitaux
// publics — vérifiée directement (fichier téléchargé, feuilles inspectées
// cellule par cellule), pas devinée depuis le seul PDF. Le site de la Drees
// rejette les requêtes sans en-tête User-Agent/Referer d'apparence
// classique (pare-feu applicatif) — d'où FetchEntetes plutôt que Fetch.
var SourceHopitalFinances = archive.Source{
	Slug: "drees-hopital-public-finances", Label: "Drees — situation économique et financière des hôpitaux publics",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees, Panorama « Les établissements de santé »",
	Cadence:     "annuelle",
	Notes: "Deux des douze feuilles du fichier sont chargées : le compte de résultat " +
		"(Graphique 1, France entière) et le déficit en % des recettes par catégorie " +
		"d'établissement (Tableau 1). Les six autres feuilles porteuses de séries " +
		"(effort d'investissement, capacité d'autofinancement, dotation aux " +
		"amortissements, surendettement, marge brute, produits/charges du budget " +
		"principal 2019-2024) empilent plusieurs sous-tableaux par feuille, sans repère " +
		"structurel exploitable simplement — non chargées, à traiter séparément.",
}

const urlHopitalFinances = "https://drees.solidarites-sante.gouv.fr/sites/default/files/2026-07/" +
	"ES%202026%20-%20Fiche%2025%20-%20La%20situation%20%C3%A9conomique%20et%20financi%C3%A8re%20des%20h%C3%B4pitaux%20publics.xlsx"

var enteteNavigateur = http.Header{
	"User-Agent": {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"},
	"Referer":    {"https://drees.solidarites-sante.gouv.fr/"},
}

func IngestHopitalFinances(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceHopitalFinances)
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

	f, err := arch.FetchEntetes(ctx, srcID, runID, urlHopitalFinances, ".xlsx", enteteNavigateur)
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur illisible : %w", err))
	}
	defer wb.Close()

	resultats, err := chargerCompteResultat(wb)
	if err != nil {
		return fail(fmt.Errorf("compte de résultat : %w", err))
	}
	deficits, err := chargerDeficitCategorie(wb)
	if err != nil {
		return fail(fmt.Errorf("déficit par catégorie : %w", err))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.hopital_public_resultat`); err != nil {
		return fail(err)
	}
	var rowsR [][]any
	for _, l := range resultats {
		rowsR = append(rowsR, []any{l.Annee, l.Indicateur, l.MontantMEUR, srcID})
	}
	nR, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "hopital_public_resultat"},
		[]string{"annee", "indicateur", "montant_meur", "source_id"}, pgx.CopyFromRows(rowsR))
	if err != nil {
		return fail(fmt.Errorf("hopital_public_resultat : %w", err))
	}

	if _, err := tx.Exec(ctx, `DELETE FROM core.hopital_public_deficit_categorie`); err != nil {
		return fail(err)
	}
	var rowsD [][]any
	for _, l := range deficits {
		rowsD = append(rowsD, []any{l.Annee, l.Categorie, l.DeficitPct, srcID})
	}
	nD, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "hopital_public_deficit_categorie"},
		[]string{"annee", "categorie", "deficit_pct_recettes", "source_id"}, pgx.CopyFromRows(rowsD))
	if err != nil {
		return fail(fmt.Errorf("hopital_public_deficit_categorie : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"resultat": nR, "deficit_categorie": nD}, "")
	fmt.Printf("  Hôpitaux publics, finances (Drees) : %d lignes compte de résultat, %d lignes déficit par catégorie\n", nR, nD)
	return nil
}

type ligneResultat struct {
	Annee       int
	Indicateur  string
	MontantMEUR float64
}

var indicateursResultat = map[string]string{
	"Résultat d'exploitation": "RESULTAT_EXPLOITATION",
	"Résultat financier":      "RESULTAT_FINANCIER",
	"Résultat exceptionnel":   "RESULTAT_EXCEPTIONNEL",
	"Résultat net":            "RESULTAT_NET",
}

// parseMontantMEUR : les valeurs de ce classeur utilisent la virgule comme
// séparateur de milliers (« 1,880 » = 1880, jamais 1,88) — vérifié sur la
// cohérence de grandeur entre valeurs voisines d'une même série, pas deviné
// depuis la seule apparence du séparateur.
func parseMontantMEUR(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, fmt.Errorf("valeur vide")
	}
	return strconv.ParseFloat(s, 64)
}

// chargerCompteResultat lit la feuille « Graphique 1 » : ligne 3 (indice 3)
// porte les années à partir de la colonne 2, chaque ligne d'indicateur suit
// le même calage de colonnes. Seuls les quatre indicateurs de haut niveau
// sont retenus (pas « dont compte 7722 », un sous-détail du résultat
// d'exploitation, ni les lignes de note).
func chargerCompteResultat(wb *excelize.File) ([]ligneResultat, error) {
	sheet, err := trouverFeuille(wb, "Graphique 1")
	if err != nil {
		return nil, err
	}
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	if len(rows) < 5 {
		return nil, fmt.Errorf("feuille %q : moins de lignes qu'attendu", sheet)
	}
	entete := rows[3]
	var out []ligneResultat
	vus := map[string]bool{}
	for _, r := range rows[4:] {
		if len(r) < 2 {
			continue
		}
		code, ok := indicateursResultat[strings.TrimSpace(r[1])]
		if !ok {
			continue
		}
		if vus[code] {
			return nil, fmt.Errorf("indicateur %q en double dans la feuille — le format a peut-être changé", r[1])
		}
		vus[code] = true
		for col := 2; col < len(entete) && col < len(r); col++ {
			annee, err := strconv.Atoi(strings.TrimSpace(entete[col]))
			if err != nil {
				continue
			}
			v, err := parseMontantMEUR(r[col])
			if err != nil {
				return nil, fmt.Errorf("%s, %d : %w", code, annee, err)
			}
			out = append(out, ligneResultat{Annee: annee, Indicateur: code, MontantMEUR: v})
		}
	}
	requis := []string{"RESULTAT_EXPLOITATION", "RESULTAT_FINANCIER", "RESULTAT_EXCEPTIONNEL", "RESULTAT_NET"}
	for _, code := range requis {
		if !vus[code] {
			return nil, fmt.Errorf("indicateur %q absent — le format a peut-être changé", code)
		}
	}
	return out, nil
}

type ligneDeficit struct {
	Annee      int
	Categorie  string
	DeficitPct float64
}

// chargerDeficitCategorie lit la feuille « Tableau 1 » : ligne 3 porte les
// années à partir de la colonne 4 (colonnes 2 et 3 sont l'effectif 2024 et
// le poids dans les recettes, hors périmètre de cette table).
func chargerDeficitCategorie(wb *excelize.File) ([]ligneDeficit, error) {
	sheet, err := trouverFeuille(wb, "Tableau 1")
	if err != nil {
		return nil, err
	}
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	if len(rows) < 5 {
		return nil, fmt.Errorf("feuille %q : moins de lignes qu'attendu", sheet)
	}
	entete := rows[3]
	var out []ligneDeficit
	for _, r := range rows[4:] {
		if len(r) < 5 {
			continue
		}
		categorie := strings.TrimSpace(r[1])
		if categorie == "" {
			continue
		}
		for col := 4; col < len(entete) && col < len(r); col++ {
			annee, err := strconv.Atoi(strings.TrimSpace(entete[col]))
			if err != nil {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(r[col]), 64)
			if err != nil {
				return nil, fmt.Errorf("%s, %d : valeur illisible %q : %w", categorie, annee, r[col], err)
			}
			out = append(out, ligneDeficit{Annee: annee, Categorie: categorie, DeficitPct: v})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("aucune ligne lue")
	}
	return out, nil
}

// trouverFeuille cherche une feuille par sous-chaîne plutôt que par égalité
// stricte : le nom exact varie d'une édition à l'autre (espaces doublés,
// « ES_2026_F25 » vs « ES2026_F25 »), vérifié sur cette édition précise.
func trouverFeuille(wb *excelize.File, sousChaine string) (string, error) {
	for _, s := range wb.GetSheetList() {
		if strings.Contains(s, sousChaine) {
			return s, nil
		}
	}
	return "", fmt.Errorf("aucune feuille ne contient %q — le format a peut-être changé", sousChaine)
}
