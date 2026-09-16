package macro

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

// « Les vieux c'est 50 % du budget de l'État » avait un point de départ
// discutable, mais l'héritage comme facteur d'accès à la richesse ne l'est
// pas : Insee mesure directement la part des ménages aisés qui ont hérité,
// et la compare à l'ensemble des ménages. Ce fichier charge cette mesure et,
// dans la même fiche, une comparaison directe patrimoine/niveau de vie
// (concentration, indice de Gini) qui manquait à docs/repartition-richesse-
// donnees.md. Voir la migration 0118.
var SourceHeritageConcentration = archive.Source{
	Slug: "insee-heritage-concentration", Label: "INSEE — héritage et concentration patrimoine/niveau de vie",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, \"France, portrait social\", édition 2025, Éclairage 3",
	Cadence:     "irrégulière (au gré des éclairages de l'édition annuelle)",
	Notes: "Enquête Histoire de vie et Patrimoine 2020-2021 (patrimoine, héritage) et enquête " +
		"Revenus fiscaux et sociaux 2021 (niveau de vie). Mesure la part des MÉNAGES ayant hérité, " +
		"pas la part de la RICHESSE qui provient de l'héritage — deux questions différentes.",
}

const urlHeritageConcentration = "https://www.insee.fr/fr/statistiques/fichier/8612590/FPORSOC25-E3.xlsx"

var categoriesMenage = []struct{ libelle, code string }{
	{"haut patrimoine et haut niveau de vie", "HAUT_PATRIMOINE_ET_NIVEAU_VIE"},
	{"haut patrimoine uniquement", "HAUT_PATRIMOINE_SEUL"},
	{"haut niveau de vie uniquement", "HAUT_NIVEAU_VIE_SEUL"},
	{"ensemble des ménages", "ENSEMBLE"},
}

func codeCategorieMenage(libelle string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(libelle))
	for _, c := range categoriesMenage {
		if l == c.libelle {
			return c.code, nil
		}
	}
	return "", fmt.Errorf("catégorie de ménage illisible : %q", libelle)
}

func IngestHeritageConcentration(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceHeritageConcentration)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "heritage-concentration-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlHeritageConcentration, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur héritage/concentration illisible : %w", err))
	}
	defer wb.Close()

	heritages, err := lireHeritage(wb)
	if err != nil {
		return fail(err)
	}
	concentration, ginis, err := lireConcentration(wb)
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `TRUNCATE core.menage_heritage, core.concentration_patrimoine_niveau_vie, core.gini_patrimoine_niveau_vie`); err != nil {
		return fail(err)
	}

	var rowsHeritage [][]any
	for _, h := range heritages {
		rowsHeritage = append(rowsHeritage, []any{h.categorie, h.trancheAge, h.partHerite, h.partDonation, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "menage_heritage"},
		[]string{"categorie", "tranche_age", "part_herite_pct", "part_donation_pct", "source_id"},
		pgx.CopyFromRows(rowsHeritage)); err != nil {
		return fail(fmt.Errorf("menage_heritage : %w", err))
	}

	var rowsConcentration [][]any
	for _, c := range concentration {
		rowsConcentration = append(rowsConcentration, []any{c.position, c.massePatrimoine, c.masseNiveauVie, srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "concentration_patrimoine_niveau_vie"},
		[]string{"position_distribution", "masse_patrimoine_pct", "masse_niveau_vie_pct", "source_id"},
		pgx.CopyFromRows(rowsConcentration)); err != nil {
		return fail(fmt.Errorf("concentration_patrimoine_niveau_vie : %w", err))
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO core.gini_patrimoine_niveau_vie (annee, indice_patrimoine, indice_niveau_vie, source_id)
		VALUES ($1, $2, $3, $4)`, 2021, ginis.patrimoine, ginis.niveauDeVie, srcID); err != nil {
		return fail(fmt.Errorf("gini_patrimoine_niveau_vie : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"heritages": len(heritages), "concentration": len(concentration)}, "")
	fmt.Printf("  héritage et concentration (Insee) : %d lignes d'héritage, %d lignes de concentration, indices de Gini %.3f / %.3f\n",
		len(heritages), len(concentration), ginis.patrimoine, ginis.niveauDeVie)
	return nil
}

type ligneHeritage struct {
	categorie, trancheAge          string
	partHerite, partDonation       float64
}

// lireHeritage : Figures 7a (hérité) et 7b (donation) ont la même mise en
// page (catégorie en ligne, 40-59 / 60 ou plus / tous âges en colonne) —
// lues ensemble puis assemblées par (catégorie, tranche d'âge).
func lireHeritage(wb *excelize.File) ([]ligneHeritage, error) {
	herite, err := lireTableauAge(wb, "Figure 7a")
	if err != nil {
		return nil, fmt.Errorf("Figure 7a (héritage) : %w", err)
	}
	donation, err := lireTableauAge(wb, "Figure 7b")
	if err != nil {
		return nil, fmt.Errorf("Figure 7b (donation) : %w", err)
	}
	out := make([]ligneHeritage, 0, len(herite))
	for k, v := range herite {
		d, ok := donation[k]
		if !ok {
			return nil, fmt.Errorf("%v : aucune donnée de donation correspondante", k)
		}
		out = append(out, ligneHeritage{k.categorie, k.trancheAge, v, d})
	}
	return out, nil
}

type cleAgeCategorie struct{ categorie, trancheAge string }

// lireTableauAge : une feuille "Figure 7a"/"Figure 7b", catégories en ligne
// (4-7), colonnes 40-59 ans / 60 ans ou plus / tous âges (index 1, 2, 3).
func lireTableauAge(wb *excelize.File, feuille string) (map[cleAgeCategorie]float64, error) {
	rows, err := wb.GetRows(feuille)
	if err != nil {
		return nil, err
	}
	if len(rows) < 8 {
		return nil, fmt.Errorf("feuille trop courte (%d lignes) — format changé", len(rows))
	}
	out := map[cleAgeCategorie]float64{}
	tranches := []string{"40_59", "60_PLUS", "TOUS_AGES"}
	for _, l := range rows[4:8] {
		if len(l) < 4 {
			return nil, fmt.Errorf("ligne trop courte : %q", l)
		}
		code, err := codeCategorieMenage(l[0])
		if err != nil {
			return nil, err
		}
		for i, tranche := range tranches {
			v, err := strconv.ParseFloat(strings.TrimSpace(l[i+1]), 64)
			if err != nil {
				return nil, fmt.Errorf("%s %s : %q illisible : %w", code, tranche, l[i+1], err)
			}
			out[cleAgeCategorie{code, tranche}] = v
		}
	}
	return out, nil
}

type ligneConcentration struct {
	position                       string
	massePatrimoine, masseNiveauVie float64
}
type ginisMenage struct{ patrimoine, niveauDeVie float64 }

// lireConcentration : feuille "Encadré – Figure", six lignes de position
// dans la distribution puis une ligne "Indice de Gini" à part.
func lireConcentration(wb *excelize.File) ([]ligneConcentration, ginisMenage, error) {
	rows, err := wb.GetRows("Encadré – Figure")
	if err != nil {
		return nil, ginisMenage{}, fmt.Errorf("feuille 'Encadré – Figure' : %w", err)
	}
	if len(rows) < 10 {
		return nil, ginisMenage{}, fmt.Errorf("feuille 'Encadré – Figure' trop courte (%d lignes) — format changé", len(rows))
	}
	var out []ligneConcentration
	var g ginisMenage
	for _, l := range rows[3:10] {
		if len(l) < 3 {
			continue
		}
		label := strings.TrimSpace(l[0])
		// "Inférieure au 2e décile1" : le "1" est un renvoi de note collé au
		// mot, pas une partie du libellé — jamais présent ailleurs dans cette
		// colonne (vérifié sur les six lignes de la feuille).
		if strings.HasSuffix(label, "décile1") {
			label = strings.TrimSuffix(label, "1")
		}
		patrimoine, err1 := strconv.ParseFloat(strings.TrimSpace(l[1]), 64)
		niveau, err2 := strconv.ParseFloat(strings.TrimSpace(l[2]), 64)
		if err1 != nil || err2 != nil {
			return nil, ginisMenage{}, fmt.Errorf("ligne %q illisible", l)
		}
		if strings.EqualFold(label, "Indice de Gini") {
			g = ginisMenage{patrimoine, niveau}
			continue
		}
		out = append(out, ligneConcentration{label, patrimoine, niveau})
	}
	if g == (ginisMenage{}) {
		return nil, ginisMenage{}, fmt.Errorf("ligne 'Indice de Gini' introuvable — format changé")
	}
	return out, g, nil
}
