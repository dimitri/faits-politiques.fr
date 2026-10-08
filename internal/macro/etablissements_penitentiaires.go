package macro

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

var SourcePrisonFacilities = archive.Source{
	Slug: "justice-etablissements-penitentiaires", Label: "Répartition des personnes détenues par établissement",
	Publisher: "Ministère de la Justice (GENESIS/DGAP)", Tier: "PRIMARY_OFFICIAL",
	License: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : ministère de la Justice, statistique mensuelle des établissements pénitentiaires",
	Cadence:     "mensuelle",
	Notes: "Ni identifiant officiel ni coordonnées géographiques dans ce fichier — un géocodage par nom et " +
		"commune serait nécessaire pour une carte, non fait ici. La Direction Interrégionale de Rennes est " +
		"publiée hors QLCO (quartiers de longue captivité outre-mer), une réserve de la source elle-même.",
}

// Août 2026 au moment de l'écriture ; l'URL suit un format prévisible
// (année-mois/..._01MMAAAA.xlsx), vérifié directement — pas de découverte
// automatique du dernier mois publié pour cette version.
const prisonFacilitiesURL = "https://www.justice.gouv.fr/sites/default/files/2026-08/statistique_etablissements_personnes_ecrouees_01082026.xlsx"

// tenRegionalDirections : les feuilles Tab14 à Tab23 du classeur
// mensuel, une par direction interrégionale — vérifié à l'exécution
// (chaque feuille commence par « Tableau NN : ... Direction Interrégionale
// de X »), pas une liste supposée depuis la documentation.
var tenRegionalDirections = []string{
	"Tab14", "Tab15", "Tab16", "Tab17", "Tab18",
	"Tab19", "Tab20", "Tab21", "Tab22", "Tab23",
}

var reReferenceDate = regexp.MustCompile(`(\d{1,2})(?:er)? (\S+) (\d{4})`)
var reRegionalDirection = regexp.MustCompile(`Direction (Interrégionale de .+|d'Outre-Mer)$`)

var monthNumber = map[string]int{
	"janvier": 1, "février": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6,
	"juillet": 7, "août": 8, "septembre": 9, "octobre": 10, "novembre": 11, "décembre": 12,
}

// parseReferenceDate lit « Effectifs actualisés au 1er août 2026 ».
func parseReferenceDate(s string) (time.Time, error) {
	m := reReferenceDate.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("date de référence illisible : %q", s)
	}
	day, err := strconv.Atoi(m[1])
	if err != nil {
		return time.Time{}, err
	}
	month, ok := monthNumber[strings.ToLower(m[2])]
	if !ok {
		return time.Time{}, fmt.Errorf("mois inconnu : %q", m[2])
	}
	year, err := strconv.Atoi(m[3])
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// parseFrenchInt lit un nombre français, espaces (dont insécables) tolérées :
// "2 455" -> 2455.
func parseFrenchInt(s string) (int, error) {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return 0, fmt.Errorf("vide")
	}
	return strconv.Atoi(s)
}

// parseDensity lit "98,5 %" -> 98.5.
func parseDensity(s string) (float64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, " ", "")
	return strconv.ParseFloat(s, 64)
}

// IngestPrisonFacilities charge, pour chacune des dix
// directions interrégionales, la population détenue par établissement et
// par quartier. Voir docs/justice-donnees.md.
func IngestPrisonFacilities(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePrisonFacilities)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "etablissements-penitentiaires-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, prisonFacilitiesURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur illisible : %w", err))
	}
	defer wb.Close()

	type row struct {
		facility, wing, region   string
		normCapacity, opCapacity *int
		inmates                  int
		density                  *float64
		refDate                  time.Time
	}
	var rows []row

	for _, sheet := range tenRegionalDirections {
		sheetRows, err := wb.GetRows(sheet)
		if err != nil {
			return fail(fmt.Errorf("%s : %w", sheet, err))
		}
		if len(sheetRows) < 8 {
			return fail(fmt.Errorf("%s : feuille trop courte (%d lignes)", sheet, len(sheetRows)))
		}
		title := strings.Join(sheetRows[0], " ")
		mDir := reRegionalDirection.FindStringSubmatch(title)
		if mDir == nil {
			return fail(fmt.Errorf("%s : direction interrégionale introuvable dans le titre %q", sheet, title))
		}
		region := strings.TrimSpace(strings.Split(mDir[1], "(")[0])
		region = strings.TrimPrefix(region, "Interrégionale de ")
		region = strings.TrimPrefix(region, "d'")
		region = strings.TrimSpace(region)
		refDate, err := parseReferenceDate(strings.Join(sheetRows[1], " "))
		if err != nil {
			return fail(fmt.Errorf("%s : %w", sheet, err))
		}
		header := sheetRows[6]
		if len(header) < 6 || header[0] != "Etablissement" {
			return fail(fmt.Errorf("%s : en-tête inattendu %v", sheet, header))
		}
		for i := 7; i < len(sheetRows); i++ {
			r := sheetRows[i]
			if len(r) < 6 {
				continue // notes de bas de page, hors tableau
			}
			facility := strings.TrimSpace(r[0])
			if facility == "" || strings.HasPrefix(facility, "Total") || strings.HasPrefix(facility, "Direction Interrégionale") {
				continue
			}
			inmates, err := parseFrenchInt(r[4])
			if err != nil {
				continue // ligne non numérique (résidu de mise en forme), pas une erreur bloquante
			}
			row := row{facility: facility, wing: strings.TrimSpace(r[1]), region: region,
				inmates: inmates, refDate: refDate}
			if n, err := parseFrenchInt(r[2]); err == nil {
				row.normCapacity = &n
			}
			if n, err := parseFrenchInt(r[3]); err == nil {
				row.opCapacity = &n
			}
			if d, err := parseDensity(r[5]); err == nil {
				row.density = &d
			}
			rows = append(rows, row)
		}
	}
	if len(rows) < 150 {
		return fail(fmt.Errorf("seulement %d lignes lues, attendu au moins 150 (dix directions interrégionales)", len(rows)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.etablissement_penitentiaire`); err != nil {
		return fail(err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.etablissement_penitentiaire
				(etablissement, quartier, direction_interregionale, capacite_norme,
				 capacite_operationnelle, ecroues_detenus, densite_pct, date_reference, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (etablissement, quartier, direction_interregionale, date_reference) DO NOTHING`,
			row.facility, row.wing, row.region, row.normCapacity, row.opCapacity,
			row.inmates, row.density, row.refDate, srcID); err != nil {
			return fail(fmt.Errorf("%s / %s : insertion : %w", row.facility, row.wing, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows)}, "")
	fmt.Printf("  Établissements pénitentiaires : %d lignes (établissement × quartier)\n", len(rows))
	return nil
}
