package macro

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceFrancophonie = archive.Source{
	Slug: "odsef-francoscope", Label: "ODSEF/OIF — Francoscope, population et francophones par entité",
	Publisher: "Observatoire démographique et statistique de l'espace francophone (ODSEF, Université Laval) " +
		"et Observatoire de la langue française de l'OIF",
	Tier: "PRIMARY_OFFICIAL", Licence: "Réutilisation libre avec attribution (ODSEF)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : ODSEF (Université Laval) / Observatoire de la langue française de l'OIF, Francoscope",
	Cadence:     "ponctuelle",
	Notes: "Le fichier mélange, dans une même liste à plat, des pays souverains, des territoires " +
		"(collectivités d'outre-mer notamment), des entités infranationales (provinces canadiennes, " +
		"Louisiane, la Sarre, la Fédération Wallonie-Bruxelles) et deux sous-totaux (« France Outre-mer », " +
		"« France (ensemble) ») — classés ici par type_entite pour qu'aucune carte ni aucun total ne les " +
		"confonde. Les colonnes « Région », « Sous-région », « Statut OIF », « Langue officielle » et " +
		"« Planète » du fichier source sont des codes numériques internes à l'outil Francoscope, sans " +
		"légende publiée dans le fichier lui-même : non chargées, faute de pouvoir vérifier leur sens plutôt " +
		"que de le deviner.",
}

const urlFrancoscope = "https://outils-odsef-fss.ulaval.ca/francoscope/tab/ODSEF_Francoscope_20250320.ods"

// typeEntiteFrancophonie classe chaque ligne du fichier — la seule partie de
// ce connecteur qui demande un jugement plutôt qu'une lecture directe,
// documentée ligne par ligne pour rester vérifiable.
func typeEntiteFrancophonie(entite string) string {
	switch entite {
	case "France Outre-mer", "France (ensemble)":
		return "agregat"
	case "Alberta", "Colombie-Britannique", "Île-du-Prince-Édouard", "Manitoba", "Nouveau-Brunswick",
		"Nouvelle-Écosse", "Nunavut", "Ontario", "Québec", "Saskatchewan", "Terre-Neuve-et-Labrador",
		"Territoires du Nord-Ouest", "Yukon", "Louisiane", "Sarre (Land allemand)", "Fédération Wallonie-Bruxelles":
		return "sous-national"
	case "Mayotte", "Réunion", "Guyane française", "Guadeloupe", "Martinique", "Saint-Barthélemy",
		"Saint-Martin (FR)", "Saint-Pierre-et-Miquelon", "Nouvelle-Calédonie", "Îles Wallis-et-Futuna",
		"Polynésie française", "Aruba", "îles Vierges Américaines", "Porto Rico":
		return "territoire"
	default:
		return "pays"
	}
}

// ligneOds : une ligne de la feuille Francoscope, colonnes 0 à 9 (No,
// Région, Sous-région, Entité, Statut OIF, Langue officielle, Planète,
// Population 2025, Francophone %, Francophone n) — seules les colonnes 3,
// 7, 8, 9 sont retenues (voir SourceFrancophonie.Notes).
type odsCell struct {
	ValueType string   `xml:"value-type,attr"`
	Value     string   `xml:"value,attr"`
	P         []string `xml:"p"`
}

type odsRow struct {
	Cells []odsCell `xml:"table-cell"`
}

type odsTable struct {
	Name string   `xml:"name,attr"`
	Rows []odsRow `xml:"table-row"`
}

type odsBody struct {
	Tables []odsTable `xml:"body>spreadsheet>table"`
}

// IngestFrancophonie charge la population, le pourcentage et le nombre de
// francophones par entité (Francoscope, ODSEF/OIF).
func IngestFrancophonie(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceFrancophonie)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "francophonie-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlFrancoscope, ".ods")
	if err != nil {
		return fail(err)
	}
	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur ODS illisible : %w", err))
	}
	defer zr.Close()
	var contentXML []byte
	for _, zf := range zr.File {
		if zf.Name == "content.xml" {
			rc, err := zf.Open()
			if err != nil {
				return fail(err)
			}
			contentXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fail(err)
			}
			break
		}
	}
	if contentXML == nil {
		return fail(fmt.Errorf("content.xml absent du classeur"))
	}
	var doc odsBody
	if err := xml.Unmarshal(contentXML, &doc); err != nil {
		return fail(fmt.Errorf("content.xml illisible : %w", err))
	}

	var feuille *odsTable
	for i, t := range doc.Tables {
		if strings.HasPrefix(t.Name, "FRANCOSCOPE") {
			feuille = &doc.Tables[i]
			break
		}
	}
	if feuille == nil {
		return fail(fmt.Errorf("aucune feuille FRANCOSCOPE-* trouvée"))
	}
	if len(feuille.Rows) < 2 {
		return fail(fmt.Errorf("feuille %s : moins de 2 lignes", feuille.Name))
	}
	entete := feuille.Rows[0].Cells
	attendu := []string{"Entité", "Population 2025", "Francophone (%)", "Francophone (n)"}
	idx := map[string]int{}
	for i, c := range entete {
		if len(c.P) > 0 {
			idx[strings.Join(c.P, "")] = i
		}
	}
	for _, col := range attendu {
		if _, ok := idx[col]; !ok {
			return fail(fmt.Errorf("colonne %q absente — le format a peut-être changé", col))
		}
	}

	nombre := func(c odsCell) (*float64, error) {
		if c.ValueType != "float" {
			return nil, nil
		}
		v, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("valeur numérique illisible %q : %w", c.Value, err)
		}
		return &v, nil
	}

	var rows [][]any
	for i, r := range feuille.Rows[1:] {
		if len(r.Cells) <= idx["Entité"] {
			continue
		}
		entite := strings.Join(r.Cells[idx["Entité"]].P, "")
		if entite == "" {
			continue // lignes vides en fin de feuille
		}
		pop, err := nombre(r.Cells[idx["Population 2025"]])
		if err != nil {
			return fail(fmt.Errorf("%s (ligne %d) : population : %w", entite, i+2, err))
		}
		pct, err := nombre(r.Cells[idx["Francophone (%)"]])
		if err != nil {
			return fail(fmt.Errorf("%s (ligne %d) : pourcentage : %w", entite, i+2, err))
		}
		n, err := nombre(r.Cells[idx["Francophone (n)"]])
		if err != nil {
			return fail(fmt.Errorf("%s (ligne %d) : nombre de francophones : %w", entite, i+2, err))
		}
		rows = append(rows, []any{entite, typeEntiteFrancophonie(entite), pop, pct, n, srcID})
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_francophonie_entite (
			entite text, type_entite text, population_2025_milliers numeric,
			francophone_pct numeric, francophone_milliers numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_francophonie_entite"},
		[]string{"entite", "type_entite", "population_2025_milliers", "francophone_pct", "francophone_milliers", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("core.francophonie_entite : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire
	// de la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité des entités à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.francophonie_entite AS tgt
		USING tmp_francophonie_entite AS src
		ON tgt.entite = src.entite
		WHEN MATCHED AND (tgt.type_entite, tgt.population_2025_milliers, tgt.francophone_pct,
		                   tgt.francophone_milliers, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.type_entite, src.population_2025_milliers, src.francophone_pct,
		                   src.francophone_milliers, src.source_id) THEN
		    UPDATE SET type_entite = src.type_entite, population_2025_milliers = src.population_2025_milliers,
		               francophone_pct = src.francophone_pct, francophone_milliers = src.francophone_milliers,
		               source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (entite, type_entite, population_2025_milliers, francophone_pct, francophone_milliers, source_id)
		    VALUES (src.entite, src.type_entite, src.population_2025_milliers, src.francophone_pct,
		            src.francophone_milliers, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"entites": len(rows), "touchees": touchees}, "")
	fmt.Printf("  Francophonie (ODSEF/OIF) : %d entités (%d touchées par la fusion)\n", len(rows), touchees)
	return nil
}
