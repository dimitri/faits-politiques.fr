package macro

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les neuf déciles nationaux du niveau de vie — la distribution la plus fine
// que l'open data permette de charger sans microdonnées individuelles
// protégées (Ines, ERFS confidentielle : accès CASD uniquement, hors de
// portée d'un connecteur). Voir docs/revenu-universel-microsimulation.md § 6.
var SourceFilosofiDeciles = archive.Source{
	Slug: "insee-filosofi-cc", Label: "Insee — Filosofi, revenus et pauvreté, tous niveaux géographiques",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, Filosofi (Fichier localisé social et fiscal)",
	Cadence:     "annuelle",
	Notes: "Le fichier couvre commune, EPCI, département, région et France ; seule la " +
		"ligne GEO_OBJECT=FRANCE est retenue ici. Refonte méthodologique 2023 " +
		"(Filosofi 2) : non comparable terme à terme aux éditions 2012-2021.",
}

const filosofiCCZipURL = "https://www.insee.fr/fr/statistiques/fichier/8984752/FILOSOFI_CC_csv.zip"

func IngestFilosofiDeciles(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceFilosofiDeciles)
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

	f, err := arch.Fetch(ctx, srcID, runID, filosofiCCZipURL, ".zip")
	if err != nil {
		return fail(err)
	}
	recs, err := lireCSVDansZip(f.Path, "DS_FILOSOFI_CC_2023_data.csv", ';')
	if err != nil {
		return fail(err)
	}

	// Un décile par mesure FILOSOFI_MEASURE (D1_SL...D9_SL), pour la seule
	// ligne GEO_OBJECT=FRANCE. La médiane (D5) n'existe pas sous ce code : la
	// source la publie sous MED_SL, une mesure à part.
	const annee = 2023
	var rows [][]any
	trouve := map[int]bool{}
	for _, r := range recs {
		if r["GEO_OBJECT"] != "FRANCE" || r["TIME_PERIOD"] != strconv.Itoa(annee) {
			continue
		}
		mesure := r["FILOSOFI_MEASURE"]
		var decile int
		switch mesure {
		case "D1_SL", "D2_SL", "D3_SL", "D4_SL", "D6_SL", "D7_SL", "D8_SL", "D9_SL":
			decile = int(mesure[1] - '0')
		case "MED_SL":
			decile = 5
		default:
			continue
		}
		v, err := strconv.ParseFloat(r["OBS_VALUE"], 64)
		if err != nil {
			continue
		}
		// La source publie un montant ANNUEL ; le reste de la base (seuil de
		// pauvreté, composition du revenu des ménages) raisonne en euros
		// MENSUELS. Convertir ici évite qu'une requête future les mélange.
		rows = append(rows, []any{annee, decile, v / 12, srcID})
		trouve[decile] = true
	}
	if len(trouve) != 9 {
		return fail(fmt.Errorf("9 déciles attendus pour %d, %d trouvés", annee, len(trouve)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire
	// de la table, mais ne recharge qu'un seul millésime à la fois — une vue
	// scopée sur cette année, comme internal/communes/ofgl.go, évite qu'un
	// MERGE non scopé n'efface les millésimes précédents restés en base.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE TEMP TABLE tmp_filosofi_decile_national (
			annee smallint, decile smallint, niveau_vie_mensuel numeric, source_id bigint
		) ON COMMIT DROP;
		CREATE OR REPLACE TEMPORARY VIEW filosofi_decile_national_scope AS
		  SELECT * FROM core.filosofi_decile_national WHERE annee = %d
		  WITH LOCAL CHECK OPTION`, annee)); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_filosofi_decile_national"},
		[]string{"annee", "decile", "niveau_vie_mensuel", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO filosofi_decile_national_scope AS tgt
		USING tmp_filosofi_decile_national AS src
		ON tgt.annee = src.annee AND tgt.decile = src.decile
		WHEN MATCHED AND (tgt.niveau_vie_mensuel, tgt.source_id) IS DISTINCT FROM (src.niveau_vie_mensuel, src.source_id) THEN
		    UPDATE SET niveau_vie_mensuel = src.niveau_vie_mensuel, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, decile, niveau_vie_mensuel, source_id)
		    VALUES (src.annee, src.decile, src.niveau_vie_mensuel, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"deciles_charges": len(rows), "touchees": n}, "")
	fmt.Printf("  Filosofi, déciles nationaux du niveau de vie : 9 points (%d), %d touchés par la fusion\n", annee, n)
	return nil
}

// lireCSVDansZip lit un fichier précis à l'intérieur d'une archive zip — le
// classeur Filosofi en contient deux (données et métadonnées) et seul le
// premier nous intéresse.
func lireCSVDansZip(zipPath, nomFichier string, sep rune) ([]map[string]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	rc, err := zr.Open(nomFichier)
	if err != nil {
		return nil, fmt.Errorf("%s absent de %s : %w", nomFichier, zipPath, err)
	}
	defer rc.Close()

	r := csv.NewReader(rc)
	r.Comma = sep
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : fichier vide", nomFichier)
	}
	head := recs[0]
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = rec[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}
