package international

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// L'espérance de vie, seule mesure de santé comparée chargée pour l'instant
// (docs/international-donnees.md § 7 — le terrain français lui-même est déjà
// couvert par le chantier santé, docs/sante-donnees.md). Écrite dans la même
// table que le PIB et l'épargne nette ajustée (core.indicateur_mondial) :
// le schéma porte déjà un source_id par ligne, ce jeu OCDE n'a donc pas
// besoin de sa propre table pour cohabiter avec la Banque mondiale.
var SourceSanteOCDE = archive.Source{
	Slug: "ocde-esperance-vie", Label: "OCDE — espérance de vie à la naissance",
	Publisher: "OCDE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : OCDE, Statistiques de la santé",
	Cadence:     "annuelle",
	Notes: "Neuf des dix pays de comparaison de ce dossier (core.indicateur_mondial) : l'Arabie " +
		"saoudite n'est pas couverte par cette série OCDE (ni membre ni partenaire clé pour cet " +
		"indicateur précis) — absence documentée, jamais complétée par une estimation.",
}

// Sous-ensemble de paysComparaisonMondiale (pib_epargne_nette.go) : l'Arabie
// saoudite (SA) est absente de cette série OCDE, vérifié avant d'écrire ce
// connecteur plutôt que découvert par une ligne manquante silencieuse.
var paysOCDESante = map[string]string{
	"FRA": "FR", "USA": "US", "JPN": "JP", "DEU": "DE", "GBR": "GB",
	"ITA": "IT", "CAN": "CA", "RUS": "RU", "CHN": "CN",
}

const espéranceVieURL = "https://sdmx.oecd.org/public/rest/data/OECD.ELS.HD,DSD_HEALTH_STAT@DF_LE,1.1/" +
	"FRA+USA+JPN+DEU+GBR+ITA+CAN+RUS+CHN.A.LFEXP.Y.Y0._T._Z._Z._Z._Z._Z._Z._Z?format=csv&startPeriod=2015"

func IngestSanteOCDE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSanteOCDE)
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

	f, err := arch.Fetch(ctx, srcID, runID, espéranceVieURL, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("espérance de vie : en-tête illisible : %w", err))
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	for _, col := range []string{"REF_AREA", "TIME_PERIOD", "OBS_VALUE"} {
		if _, ok := idx[col]; !ok {
			return fail(fmt.Errorf("espérance de vie : colonne %q absente de l'export SDMX — format changé", col))
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.indicateur_mondial WHERE indicateur = 'OCDE_ESPERANCE_VIE_NAISSANCE'`); err != nil {
		return fail(err)
	}

	var rows [][]any
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		pays3 := rec[idx["REF_AREA"]]
		pays2, ok := paysOCDESante[pays3]
		if !ok {
			return fail(fmt.Errorf("espérance de vie : pays inattendu %q — la clé de requête n'a pourtant demandé que les neuf pays de comparaison", pays3))
		}
		if rec[idx["OBS_VALUE"]] == "" {
			continue
		}
		annee, err := strconv.Atoi(rec[idx["TIME_PERIOD"]])
		if err != nil {
			return fail(fmt.Errorf("espérance de vie : année %q : %w", rec[idx["TIME_PERIOD"]], err))
		}
		valeur, err := strconv.ParseFloat(rec[idx["OBS_VALUE"]], 64)
		if err != nil {
			return fail(fmt.Errorf("espérance de vie : valeur %q : %w", rec[idx["OBS_VALUE"]], err))
		}
		rows = append(rows, []any{pays2, libellesPays[pays2], "OCDE_ESPERANCE_VIE_NAISSANCE", annee, valeur, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("espérance de vie : aucune valeur"))
	}

	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "indicateur_mondial"},
		[]string{"pays_code", "pays_label", "indicateur", "annee", "valeur", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("indicateur_mondial (espérance de vie) : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  Espérance de vie à la naissance (OCDE) : %d lignes, %d pays\n", n, len(paysOCDESante))
	return nil
}

var libellesPays = map[string]string{
	"FR": "France", "US": "United States", "JP": "Japan", "DE": "Germany", "GB": "United Kingdom",
	"IT": "Italy", "CA": "Canada", "RU": "Russian Federation", "CN": "China",
}
