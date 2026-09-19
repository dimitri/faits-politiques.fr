package macro

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceEcartPrixDOM = archive.Source{
	Slug: "insee-ecsp-2022", Label: "Enquête de comparaison spatiale des prix (ECSP) 2022",
	Publisher: "Insee", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : Insee Première n° 1958, juillet 2023",
	Cadence:     "ponctuelle",
	Notes: "Enquête ponctuelle (1985, 1992, 2010, 2015, 2022), pas une série annuelle. " +
		"Champ hors fioul, gaz de ville, transports ferroviaires ; hors loyers pour Mayotte.",
}

// ecartPrixPoint : indice de Fisher (écart moyen général) et écart sur les
// seuls produits alimentaires — vérifié directement dans le fichier Excel
// joint à Insee Première n° 1958 (Figure 2 pour la série 2010-2015-2022,
// Tableau complémentaire 2 pour la ventilation alimentaire 2022). Les
// millésimes 1985 et 1992 existent dans la source (tableau complémentaire
// 1) mais avec une méthode non harmonisée avec 2010+ selon l'Insee
// elle-même — non repris ici pour ne pas comparer des séries que la
// source dit elle-même non comparables.
type ecartPrixPoint struct {
	territoire        string
	annee             int
	fisherGeneral     float64
	fisherAlimentaire *float64 // seulement pour 2022, seule année où ce dossier a vérifié la ventilation
}

func f64p(v float64) *float64 { return &v }

var ecartsPrixDOM = []ecartPrixPoint{
	{"Guadeloupe", 2010, 8.3, nil},
	{"Guadeloupe", 2015, 12.5, nil},
	{"Guadeloupe", 2022, 15.8, f64p(41.8)},
	{"Martinique", 2010, 9.7, nil},
	{"Martinique", 2015, 12.3, nil},
	{"Martinique", 2022, 13.8, f64p(40.2)},
	{"Guyane", 2010, 13.0, nil},
	{"Guyane", 2015, 11.6, nil},
	{"Guyane", 2022, 13.7, f64p(39.4)},
	{"La Réunion", 2010, 6.2, nil},
	{"La Réunion", 2015, 7.1, nil},
	{"La Réunion", 2022, 8.9, f64p(36.7)},
	// Mayotte 2010 : "nd" (non disponible) dans la source — pas de ligne ici,
	// pas un zéro qui laisserait croire à une absence d'écart.
	{"Mayotte (hors loyers)", 2015, 6.9, nil},
	{"Mayotte (hors loyers)", 2022, 10.3, f64p(30.2)},
}

// IngestEcartPrixDOM charge l'écart de prix entre les DOM et la France
// métropolitaine (Insee, ECSP 2022). Voir docs/outre-mer-donnees.md.
func IngestEcartPrixDOM(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEcartPrixDOM)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "ecart-prix-dom-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.ecart_prix_dom`); err != nil {
		return fail(err)
	}
	for _, p := range ecartsPrixDOM {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.ecart_prix_dom (territoire, annee, fisher_general_pct, fisher_alimentaire_pct, source_id)
			VALUES ($1,$2,$3,$4,$5)`, p.territoire, p.annee, p.fisherGeneral, p.fisherAlimentaire, srcID); err != nil {
			return fail(fmt.Errorf("%s %d : insertion : %w", p.territoire, p.annee, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(ecartsPrixDOM)}, "")
	fmt.Printf("  Écart de prix DOM/métropole (Insee ECSP) : %d lignes\n", len(ecartsPrixDOM))
	return nil
}
