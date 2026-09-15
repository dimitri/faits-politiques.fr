// Package vieillesse charge les données propres au dossier vieillesse
// (docs/vieillesse-donnees.md) qui ne relèvent d'aucun autre domaine déjà
// couvert par ce dépôt — la branche autonomie en premier lieu, un angle
// mort jusqu'ici du dossier santé comme du dossier retraite.
package vieillesse

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// L'APA à domicile, seule mesure directe de la dépendance dans ce dépôt —
// voir le commentaire de la migration 0112 pour les trois réserves réelles
// (rupture de périmètre 2017, données déclaratives parfois manquantes,
// double compte évité sur la Corse).
var SourceAPA = archive.Source{
	Slug: "drees-apa-domicile", Label: "DREES — APA à domicile, bénéficiaires et dépenses",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques (DREES)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : DREES, enquête Aide sociale",
	Cadence:     "annuelle",
	Notes: "France métropolitaine et DROM, hors Mayotte. Rupture de périmètre en 2017 (avant : " +
		"seuls les intervenants à domicile ; à partir de 2017 : l'ensemble des dépenses couvertes " +
		"par l'APA à domicile). Données déclaratives des conseils départementaux, parfois " +
		"manquantes ('ND' dans la source, chargé en NULL). Ne couvre pas l'APA en établissement.",
}

const apaURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"apa-et-pch-montants-verses/attachments/apa_a_domicile_depenses_couvertes_2010_2024_xlsx"

// Le classeur mélange trois séparateurs de milliers selon la cellule :
// virgule, espace normal et espace insécable (U+00A0) — vérifié sur
// l'export complet, pas supposé après le premier échec de lecture.
func nettoyerNombre(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{",", " ", " ", " "} {
		s = strings.ReplaceAll(s, sep, "")
	}
	return s
}

func IngestAPA(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAPA)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "vieillesse-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, apaURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur DREES illisible : %w", err))
	}
	defer wb.Close()

	// nb_beneficiaires : une seule feuille, une colonne par année.
	beneficiaires, err := lireBeneficiaires(wb)
	if err != nil {
		return fail(err)
	}

	type cleDep struct {
		annee int
		dep   string
	}
	depenses := map[cleDep]int64{}
	perimetres := map[int]string{}
	for annee := 2010; annee <= 2024; annee++ {
		perim := "ENSEMBLE_APA_DOMICILE"
		if annee < 2017 {
			perim = "INTERVENANTS_DOMICILE"
		}
		perimetres[annee] = perim
		m, err := lireDepenses(wb, annee)
		if err != nil {
			return fail(fmt.Errorf("dépenses %d : %w", annee, err))
		}
		for dep, v := range m {
			depenses[cleDep{annee, dep}] = v
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE core.apa_domicile`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for _, b := range beneficiaires {
		var depensesEur *int64
		if v, ok := depenses[cleDep{b.annee, b.dep}]; ok {
			depensesEur = &v
		}
		var nbBenef *int32
		if b.valeur != nil {
			v := int32(*b.valeur)
			nbBenef = &v
		}
		rows = append(rows, []any{b.annee, b.dep, b.libelle, nbBenef, depensesEur, perimetres[b.annee], srcID})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "apa_domicile"},
		[]string{"annee", "code_departement", "libelle_departement", "nb_beneficiaires", "depenses_total_eur", "perimetre_depenses", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("apa_domicile : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  APA à domicile (DREES) : %d lignes, 2010-2024\n", n)
	return nil
}

type ligneBenef struct {
	annee   int
	dep     string
	libelle string
	valeur  *int64
}

// La feuille "APA_dom" recense un même code Corse à deux endroits — "20"
// (Collectivité de Corse, série complète) et "2A"/"2B" (série partielle,
// qui se recoupe avec "20" sur les années où les deux existent). Seul "20"
// est retenu, voir le commentaire de la migration 0112.
func lireBeneficiaires(wb *excelize.File) ([]ligneBenef, error) {
	rows, err := wb.GetRows("APA_dom")
	if err != nil {
		return nil, fmt.Errorf("feuille 'APA_dom' : %w", err)
	}
	if len(rows) < 7 {
		return nil, fmt.Errorf("feuille 'APA_dom' trop courte (%d lignes) — format changé", len(rows))
	}
	entete := rows[5]
	var annees []int
	for _, h := range entete[3:] {
		a, err := strconv.Atoi(strings.TrimSpace(h))
		if err != nil {
			return nil, fmt.Errorf("en-tête 'APA_dom' %q illisible — format changé", h)
		}
		annees = append(annees, a)
	}
	var out []ligneBenef
	for _, l := range rows[6:] {
		if len(l) < 3 {
			continue
		}
		dep := strings.TrimSpace(l[1])
		if dep == "" || dep == "2A" || dep == "2B" || strings.HasPrefix(strings.TrimSpace(l[0]), "TOTAL") {
			continue
		}
		libelle := strings.TrimSpace(l[2])
		for i, annee := range annees {
			col := 3 + i
			if col >= len(l) {
				continue
			}
			brut := nettoyerNombre(l[col])
			if brut == "" || brut == "ND" {
				out = append(out, ligneBenef{annee, dep, libelle, nil})
				continue
			}
			v, err := strconv.ParseInt(brut, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%s %d : bénéficiaires %q illisible : %w", libelle, annee, l[col], err)
			}
			out = append(out, ligneBenef{annee, dep, libelle, &v})
		}
	}
	return out, nil
}

// Une feuille par année ("2010".."2024"), en-tête sur la ligne 9 (index 8),
// le dernier libellé de colonne variant selon le millésime ("Total des
// dépenses d'intervenants à domicile" avant 2017, "TOTAL" ensuite) — la
// dernière colonne non vide de chaque ligne est toujours ce total, quel
// que soit son libellé exact.
func lireDepenses(wb *excelize.File, annee int) (map[string]int64, error) {
	nom := strconv.Itoa(annee)
	rows, err := wb.GetRows(nom)
	if err != nil {
		return nil, fmt.Errorf("feuille %q : %w", nom, err)
	}
	if len(rows) < 10 {
		return nil, fmt.Errorf("feuille %q trop courte (%d lignes) — format changé", nom, len(rows))
	}
	out := map[string]int64{}
	for _, l := range rows[9:] {
		if len(l) < 3 {
			continue
		}
		dep := strings.TrimSpace(l[1])
		if dep == "" || dep == "2A" || dep == "2B" || strings.HasPrefix(strings.TrimSpace(l[0]), "TOTAL") {
			continue
		}
		dernier := strings.TrimSpace(l[len(l)-1])
		brut := nettoyerNombre(dernier)
		if brut == "" || brut == "ND" {
			continue
		}
		v, err := strconv.ParseInt(brut, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s : total %q illisible : %w", dep, dernier, err)
		}
		out[dep] = v
	}
	return out, nil
}
