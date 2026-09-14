package macro

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La composition du revenu des ménages, par grand type — de quoi simuler un
// socle universel sur des catégories réelles plutôt que sur un forfait par
// personne. Voir docs/revenu-universel-microsimulation.md.
var SourceMenagesDREES = archive.Source{
	Slug: "drees-composition-revenu-menages", Label: "DREES — composition du revenu des ménages",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee-DGFiP-Cnaf-Cnav-CCMSA, enquête Revenus fiscaux et sociaux, calculs Drees",
	Cadence:     "annuelle",
	Notes: "Champ : France métropolitaine, ménages vivant dans un logement ordinaire, " +
		"dont le revenu déclaré est positif ou nul et dont la personne de référence " +
		"n'est pas étudiante — un univers plus étroit que le recensement.",
}

const drees4aXLSXURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"4230_indicateurs-de-pauvrete-avant-et-apres-redistribution-de-niveau-de-vie-et-d/attachments/" +
	"02_la_composition_du_revenu_des_menages_pauvres_ou_modestes_vf_xlsx"

// Les deux feuilles utilisées, tableau 4a (montants par ménage) et 4b (montants
// par UC), portent les mêmes dix types de ménage dans le même ordre, mais pas
// dans les mêmes colonnes : 4a porte ses libellés en colonne B et ses valeurs
// de E à O, 4b ses libellés en colonne A et ses valeurs de D à N. Repéré à la
// main sur le classeur du 14 septembre 2026 (voir le commentaire de
// typeMenageCol4a pour ce que change une colonne déplacée).
var typeMenageCol4a = map[string]string{
	"personne_seule": "E", "monoparentale_1_enfant": "F", "monoparentale_2p_enfants": "G",
	"couple_sans_enfant": "H", "couple_1_enfant": "I", "couple_2_enfants": "J",
	"couple_3_enfants": "K", "couple_4p_enfants": "L",
	"complexe_sans_enfant": "M", "complexe_avec_enfants": "N", "ensemble": "O",
}

var typeMenageCol4b = map[string]string{
	"personne_seule": "D", "monoparentale_1_enfant": "E", "monoparentale_2p_enfants": "F",
	"couple_sans_enfant": "G", "couple_1_enfant": "H", "couple_2_enfants": "I",
	"couple_3_enfants": "J", "couple_4p_enfants": "K",
	"complexe_sans_enfant": "L", "complexe_avec_enfants": "M", "ensemble": "N",
}

func IngestMenagesDREES(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMenagesDREES)
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

	f, err := arch.Fetch(ctx, srcID, runID, drees4aXLSXURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	l4a, err := x.rows("Tableau 4a")
	if err != nil {
		return fail(err)
	}
	l4b, err := x.rows("Tableau 4b")
	if err != nil {
		return fail(err)
	}

	revenuInitial, err := ligneParLibelle(l4a, "B", "Revenu initial")
	if err != nil {
		return fail(fmt.Errorf("tableau 4a : %w", err))
	}
	prestNonContrib, err := ligneParLibelle(l4a, "B", "Prestations sociales non contributives")
	if err != nil {
		return fail(fmt.Errorf("tableau 4a : %w", err))
	}
	impotsDirects, err := ligneParLibelle(l4a, "B", "Impôts directs")
	if err != nil {
		return fail(fmt.Errorf("tableau 4a : %w", err))
	}
	revenuDisponible, err := ligneParLibelle(l4a, "B", "Revenu disponible")
	if err != nil {
		return fail(fmt.Errorf("tableau 4a : %w", err))
	}
	revenuInitialUC, err := ligneParLibelle(l4b, "A", "Revenu initial")
	if err != nil {
		return fail(fmt.Errorf("tableau 4b : %w", err))
	}

	const annee = 2023
	var rows [][]any
	for typ, col4a := range typeMenageCol4a {
		col4b := typeMenageCol4b[typ]
		ri, ok1 := valeurNumerique(revenuInitial, col4a)
		riUC, ok2 := valeurNumerique(revenuInitialUC, col4b)
		pnc, ok3 := valeurNumerique(prestNonContrib, col4a)
		imp, ok4 := valeurNumerique(impotsDirects, col4a)
		rd, ok5 := valeurNumerique(revenuDisponible, col4a)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || riUC == 0 {
			return fail(fmt.Errorf("type %s incomplet (4a:%s 4b:%s)", typ, col4a, col4b))
		}
		uc := ri / riUC
		rows = append(rows, []any{typ, annee, uc, ri, pnc, imp, rd, srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.menage_type_drees WHERE annee = $1`, annee); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "menage_type_drees"},
		[]string{"type_menage", "annee", "uc_empirique", "revenu_initial_menage",
			"prestations_non_contrib_menage", "impots_directs_menage", "revenu_disponible_menage", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"types_charges": len(rows)}, "")
	fmt.Printf("  composition du revenu des ménages : %d types (Drees, ERFS %d)\n", len(rows), annee)
	return nil
}

// ligneParLibelle trouve, dans une feuille où labelCol porte le libellé de
// chaque ligne, celle dont le libellé COMMENCE par le préfixe donné — les
// classeurs Drees suffixent leurs libellés d'un exposant de note ("1", "5")
// que la correspondance exacte casserait.
func ligneParLibelle(lignes []map[string]string, labelCol, prefixe string) (map[string]string, error) {
	for _, l := range lignes {
		if v, ok := l[labelCol]; ok && len(v) >= len(prefixe) && v[:len(prefixe)] == prefixe {
			return l, nil
		}
	}
	return nil, fmt.Errorf("ligne %q introuvable en colonne %s", prefixe, labelCol)
}

func valeurNumerique(ligne map[string]string, col string) (float64, bool) {
	v, ok := ligne[col]
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
