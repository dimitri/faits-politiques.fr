package budget

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le seul jeu URSSAF qui donne un montant réellement ENCAISSÉ plutôt qu'une
// assiette ou un allègement — voir le commentaire de
// db/migrations/0068_urssaf_encaissements.sql pour ses deux limites (trois
// millésimes, maille région).
var SourceURSSAFEncaissements = archive.Source{
	Slug:        "urssaf-encaissements-annuels",
	Label:       "URSSAF — encaissements annuels par région et catégorie",
	Publisher:   "Urssaf Caisse nationale",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Open Database License (ODbL) 1.0",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Urssaf Caisse nationale, données ouvertes (ODbL)",
	Cadence:     "annuelle",
	Notes: "ODbL : partage à l'identique, voir urssaf-exonerations. " +
		"Dormant depuis le 5 juillet 2023 : seuls 2020, 2021 et 2022 sont publiés. " +
		"Maille géographique : la région (par Urssaf régionale), pas le département.",
}

// categorieEntreprise : les deux catégories détaillées qui correspondent à des
// cotisations versées PAR une entreprise. Codées en dur plutôt que devinées
// par mot-clé : le libellé « secteur privé (hors GEN) » est celui que l'Urssaf
// publie au 14 septembre 2026, et une correspondance par sous-chaîne serait
// aussi fragile qu'une correspondance exacte face à une reformulation future —
// autant l'assumer explicitement ici.
var categorieEntreprise = map[string]bool{
	"Cotisations et contributions sur revenus d'activité du secteur privé (hors GEN)":        true,
	"Cotisations et contributions sur revenus d'activité des grandes entreprises nationales": true,
}

type ligneEncaissement struct {
	Annee              string   `json:"annee"`
	Organisme          string   `json:"organisme"`
	Region             string   `json:"region"`
	CodeRegion         string   `json:"code_region"`
	Categorie          string   `json:"categorie_encaissements"`
	CategorieDetaillee string   `json:"categorie_encaissements_detaillee"`
	Montant            *float64 `json:"montant_des_encaissements"`
}

func IngestURSSAFEncaissements(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceURSSAFEncaissements)
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

	f, err := arch.Fetch(ctx, srcID, runID,
		exportJSON("open.urssaf.fr", "encaissements-annuels-des-urssaf"), ".json")
	if err != nil {
		return fail(err)
	}
	var lignes []ligneEncaissement
	if err := lireJSON(f.Path, &lignes); err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("URSSAF encaissements : export vide"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.encaissement_urssaf`); err != nil {
		return fail(err)
	}

	var rows [][]any
	var sansMontant int
	for _, l := range lignes {
		annee, err := strconv.Atoi(l.Annee[:4])
		if err != nil {
			return fail(fmt.Errorf("URSSAF encaissements : année illisible %q", l.Annee))
		}
		if l.Montant == nil {
			sansMontant++
			continue
		}
		rows = append(rows, []any{
			annee, l.Organisme, l.Region, l.CodeRegion,
			l.Categorie, l.CategorieDetaillee, categorieEntreprise[l.CategorieDetaillee],
			*l.Montant, "SECTEUR_PRIVE_URSSAF", srcID, f.DocumentID,
		})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "encaissement_urssaf"},
		[]string{"annee", "organisme", "region", "code_region",
			"categorie", "categorie_detaillee", "categorie_entreprise",
			"montant_eur", "perimetre", "source_id", "document_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("URSSAF encaissements : %w", err))
	}

	var annees, min, max int
	var totalEntreprises, totalTout float64
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT annee), min(annee), max(annee),
		       coalesce(sum(montant_eur) FILTER (WHERE categorie_entreprise
		                 AND annee = (SELECT max(annee) FROM core.encaissement_urssaf)), 0),
		       coalesce(sum(montant_eur) FILTER (WHERE
		                 annee = (SELECT max(annee) FROM core.encaissement_urssaf)), 0)
		  FROM core.encaissement_urssaf`).Scan(&annees, &min, &max, &totalEntreprises, &totalTout); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes_chargees": n, "rejet_sans_montant": sansMontant, "millesimes": annees}, "")
	fmt.Printf("  URSSAF encaissements : %d lignes, %d à %d (entreprises %.1f Md€ sur %.1f Md€ en %d)\n",
		n, min, max, totalEntreprises/1e9, totalTout/1e9, max)
	return nil
}
