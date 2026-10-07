package macro

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceTaxAudit = archive.Source{
	Slug: "senat-controle-fiscal-resultats", Label: "Résultats du contrôle fiscal, France entière",
	Publisher: "Sénat, commission des finances", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : Sénat, rapports n° r22-072 (2022-2023), l24-034-215-1 (2024), l25-139-314 (2025)",
	Cadence:     "annuelle",
	Notes: "Aucun dataset structuré (CSV/XLSX) n'a été trouvé pour cet indicateur : ni data.gouv.fr " +
		"(le jeu « Tableaux Statistiques de la DGFiP » ne couvre que les bases et taux d'imposition, " +
		"pas l'activité de contrôle) ni economie.gouv.fr (pare-feu bloquant l'accès direct) ne le " +
		"publient en table. Les montants notifiés (droits et pénalités mis en recouvrement, avant " +
		"recours) et encaissés (effectivement recouvrés) sont vérifiés un par un dans trois rapports " +
		"du Sénat, recoupés entre eux sur les années communes (2019, 2020, 2021 identiques dans les " +
		"trois). Le notifié 2022 et 2023 n'a été retrouvé dans aucune des trois sources primaires " +
		"consultées ; le notifié 2024 est déduit de l'écart notifié/encaissé que le rapport 2025 " +
		"publie explicitement (11,4 + 5,2 Md€), pas cité tel quel.",
}

type taxAuditPoint struct {
	year                       int
	assessedM, collectedM      float64
	assessedComputed, assessed bool
}

var taxAuditPoints = []taxAuditPoint{
	{year: 2015, assessed: true, assessedM: 16121, collectedM: 9590},
	{year: 2016, assessed: true, assessedM: 15292, collectedM: 8612},
	{year: 2017, assessed: true, assessedM: 13981, collectedM: 8077},
	{year: 2018, assessed: true, assessedM: 12916, collectedM: 7737},
	{year: 2019, assessed: true, assessedM: 11450, collectedM: 10973},
	{year: 2020, assessed: true, assessedM: 8876, collectedM: 7790},
	{year: 2021, assessed: true, assessedM: 13284, collectedM: 10651},
	// 2022 et 2023 : encaissé publié (Sénat), notifié non retrouvé dans une
	// source primaire — colonne laissée vide plutôt que devinée.
	{year: 2022, collectedM: 10600},
	{year: 2023, collectedM: 10600},
	// 2024 : encaissé publié (11,4 Md€) et écart notifié/encaissé publié
	// (5,2 Md€) dans le même rapport (l25-139-314) — notifié = somme des
	// deux, déduit et non cité tel quel par la source.
	{year: 2024, assessed: true, assessedComputed: true, assessedM: 16600, collectedM: 11400},
}

// IngestTaxAudit charge les résultats du contrôle fiscal 2015-2024
// (notifié/encaissé, France entière). Voir docs/fraude-fiscale-donnees.md.
func IngestTaxAudit(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTaxAudit)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "controle-fiscal-v1")
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
	if _, err := tx.Exec(ctx, `DELETE FROM core.controle_fiscal_resultats`); err != nil {
		return fail(err)
	}
	for _, p := range taxAuditPoints {
		var assessed *float64
		if p.assessed {
			assessed = &p.assessedM
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.controle_fiscal_resultats
				(annee, montant_notifie_m, notifie_calcule, montant_encaisse_m, source_id)
			VALUES ($1,$2,$3,$4,$5)`,
			p.year, assessed, p.assessedComputed, p.collectedM, srcID); err != nil {
			return fail(fmt.Errorf("%d : insertion : %w", p.year, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees": len(taxAuditPoints)}, "")
	fmt.Printf("  contrôle fiscal (résultats) : %d années\n", len(taxAuditPoints))
	return nil
}
