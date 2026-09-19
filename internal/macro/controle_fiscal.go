package macro

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceControleFiscal = archive.Source{
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

type pointControleFiscal struct {
	annee                   int
	notifieM, encaisseM     float64
	notifieCalcule, notifie bool
}

var controleFiscalPoints = []pointControleFiscal{
	{annee: 2015, notifie: true, notifieM: 16121, encaisseM: 9590},
	{annee: 2016, notifie: true, notifieM: 15292, encaisseM: 8612},
	{annee: 2017, notifie: true, notifieM: 13981, encaisseM: 8077},
	{annee: 2018, notifie: true, notifieM: 12916, encaisseM: 7737},
	{annee: 2019, notifie: true, notifieM: 11450, encaisseM: 10973},
	{annee: 2020, notifie: true, notifieM: 8876, encaisseM: 7790},
	{annee: 2021, notifie: true, notifieM: 13284, encaisseM: 10651},
	// 2022 et 2023 : encaissé publié (Sénat), notifié non retrouvé dans une
	// source primaire — colonne laissée vide plutôt que devinée.
	{annee: 2022, encaisseM: 10600},
	{annee: 2023, encaisseM: 10600},
	// 2024 : encaissé publié (11,4 Md€) et écart notifié/encaissé publié
	// (5,2 Md€) dans le même rapport (l25-139-314) — notifié = somme des
	// deux, déduit et non cité tel quel par la source.
	{annee: 2024, notifie: true, notifieCalcule: true, notifieM: 16600, encaisseM: 11400},
}

// IngestControleFiscal charge les résultats du contrôle fiscal 2015-2024
// (notifié/encaissé, France entière). Voir docs/fraude-fiscale-donnees.md.
func IngestControleFiscal(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceControleFiscal)
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
	for _, p := range controleFiscalPoints {
		var notifie *float64
		if p.notifie {
			notifie = &p.notifieM
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.controle_fiscal_resultats
				(annee, montant_notifie_m, notifie_calcule, montant_encaisse_m, source_id)
			VALUES ($1,$2,$3,$4,$5)`,
			p.annee, notifie, p.notifieCalcule, p.encaisseM, srcID); err != nil {
			return fail(fmt.Errorf("%d : insertion : %w", p.annee, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees": len(controleFiscalPoints)}, "")
	fmt.Printf("  contrôle fiscal (résultats) : %d années\n", len(controleFiscalPoints))
	return nil
}
