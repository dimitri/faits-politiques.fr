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

var SourceEtablissementsPenitentiaires = archive.Source{
	Slug: "justice-etablissements-penitentiaires", Label: "Répartition des personnes détenues par établissement",
	Publisher: "Ministère de la Justice (GENESIS/DGAP)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : ministère de la Justice, statistique mensuelle des établissements pénitentiaires",
	Cadence:     "mensuelle",
	Notes: "Ni identifiant officiel ni coordonnées géographiques dans ce fichier — un géocodage par nom et " +
		"commune serait nécessaire pour une carte, non fait ici. La Direction Interrégionale de Rennes est " +
		"publiée hors QLCO (quartiers de longue captivité outre-mer), une réserve de la source elle-même.",
}

// Août 2026 au moment de l'écriture ; l'URL suit un format prévisible
// (année-mois/..._01MMAAAA.xlsx), vérifié directement — pas de découverte
// automatique du dernier mois publié pour cette version.
const urlEtablissementsPenitentiaires = "https://www.justice.gouv.fr/sites/default/files/2026-08/statistique_etablissements_personnes_ecrouees_01082026.xlsx"

// dixDirectionsInterregionales : les feuilles Tab14 à Tab23 du classeur
// mensuel, une par direction interrégionale — vérifié à l'exécution
// (chaque feuille commence par « Tableau NN : ... Direction Interrégionale
// de X »), pas une liste supposée depuis la documentation.
var dixDirectionsInterregionales = []string{
	"Tab14", "Tab15", "Tab16", "Tab17", "Tab18",
	"Tab19", "Tab20", "Tab21", "Tab22", "Tab23",
}

var reDateReference = regexp.MustCompile(`(\d{1,2})(?:er)? (\S+) (\d{4})`)
var reDirectionInterregionale = regexp.MustCompile(`Direction (Interrégionale de .+|d'Outre-Mer)$`)

var moisNumero = map[string]int{
	"janvier": 1, "février": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6,
	"juillet": 7, "août": 8, "septembre": 9, "octobre": 10, "novembre": 11, "décembre": 12,
}

// parserDateReference lit « Effectifs actualisés au 1er août 2026 ».
func parserDateReference(s string) (time.Time, error) {
	m := reDateReference.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("date de référence illisible : %q", s)
	}
	jour, err := strconv.Atoi(m[1])
	if err != nil {
		return time.Time{}, err
	}
	mois, ok := moisNumero[strings.ToLower(m[2])]
	if !ok {
		return time.Time{}, fmt.Errorf("mois inconnu : %q", m[2])
	}
	annee, err := strconv.Atoi(m[3])
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(annee, time.Month(mois), jour, 0, 0, 0, 0, time.UTC), nil
}

// parserEntier lit un nombre français, espaces (dont insécables) tolérées :
// "2 455" -> 2455.
func parserEntier(s string) (int, error) {
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

// parserDensite lit "98,5 %" -> 98.5.
func parserDensite(s string) (float64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, " ", "")
	return strconv.ParseFloat(s, 64)
}

// IngestEtablissementsPenitentiaires charge, pour chacune des dix
// directions interrégionales, la population détenue par établissement et
// par quartier. Voir docs/justice-donnees.md.
func IngestEtablissementsPenitentiaires(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEtablissementsPenitentiaires)
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

	f, err := arch.Fetch(ctx, srcID, runID, urlEtablissementsPenitentiaires, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur illisible : %w", err))
	}
	defer wb.Close()

	type ligne struct {
		etablissement, quartier, direction string
		capaciteNorme, capaciteOp          *int
		ecroues                            int
		densite                            *float64
		dateRef                            time.Time
	}
	var lignes []ligne

	for _, sheet := range dixDirectionsInterregionales {
		rows, err := wb.GetRows(sheet)
		if err != nil {
			return fail(fmt.Errorf("%s : %w", sheet, err))
		}
		if len(rows) < 8 {
			return fail(fmt.Errorf("%s : feuille trop courte (%d lignes)", sheet, len(rows)))
		}
		titre := strings.Join(rows[0], " ")
		mDir := reDirectionInterregionale.FindStringSubmatch(titre)
		if mDir == nil {
			return fail(fmt.Errorf("%s : direction interrégionale introuvable dans le titre %q", sheet, titre))
		}
		direction := strings.TrimSpace(strings.Split(mDir[1], "(")[0])
		direction = strings.TrimPrefix(direction, "Interrégionale de ")
		direction = strings.TrimPrefix(direction, "d'")
		direction = strings.TrimSpace(direction)
		dateRef, err := parserDateReference(strings.Join(rows[1], " "))
		if err != nil {
			return fail(fmt.Errorf("%s : %w", sheet, err))
		}
		header := rows[6]
		if len(header) < 6 || header[0] != "Etablissement" {
			return fail(fmt.Errorf("%s : en-tête inattendu %v", sheet, header))
		}
		for i := 7; i < len(rows); i++ {
			r := rows[i]
			if len(r) < 6 {
				continue // notes de bas de page, hors tableau
			}
			etab := strings.TrimSpace(r[0])
			if etab == "" || strings.HasPrefix(etab, "Total") || strings.HasPrefix(etab, "Direction Interrégionale") {
				continue
			}
			ecroues, err := parserEntier(r[4])
			if err != nil {
				continue // ligne non numérique (résidu de mise en forme), pas une erreur bloquante
			}
			l := ligne{etablissement: etab, quartier: strings.TrimSpace(r[1]), direction: direction,
				ecroues: ecroues, dateRef: dateRef}
			if n, err := parserEntier(r[2]); err == nil {
				l.capaciteNorme = &n
			}
			if n, err := parserEntier(r[3]); err == nil {
				l.capaciteOp = &n
			}
			if d, err := parserDensite(r[5]); err == nil {
				l.densite = &d
			}
			lignes = append(lignes, l)
		}
	}
	if len(lignes) < 150 {
		return fail(fmt.Errorf("seulement %d lignes lues, attendu au moins 150 (dix directions interrégionales)", len(lignes)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.etablissement_penitentiaire`); err != nil {
		return fail(err)
	}
	for _, l := range lignes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.etablissement_penitentiaire
				(etablissement, quartier, direction_interregionale, capacite_norme,
				 capacite_operationnelle, ecroues_detenus, densite_pct, date_reference, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (etablissement, quartier, direction_interregionale, date_reference) DO NOTHING`,
			l.etablissement, l.quartier, l.direction, l.capaciteNorme, l.capaciteOp,
			l.ecroues, l.densite, l.dateRef, srcID); err != nil {
			return fail(fmt.Errorf("%s / %s : insertion : %w", l.etablissement, l.quartier, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(lignes)}, "")
	fmt.Printf("  Établissements pénitentiaires : %d lignes (établissement × quartier)\n", len(lignes))
	return nil
}
