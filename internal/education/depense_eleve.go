package education

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// editionRERS : le millésime de RERS Interactif interrogé par ce connecteur
// et par effectifs_historique.go — toujours le plus récent disponible,
// jamais figé à l'édition imprimée citée la première fois qu'une série a
// été repérée (les éditions se révisent : le total 2023 "ensemble" valait
// 10 470 € dans l'édition 2025, 10 730 € dans l'édition 2026, même déflateur
// PIB recalculé sur une base plus récente).
const editionRERS = 2026

// urlRERS construit l'URL d'un tableau RERS Interactif — un CSV servi à une
// adresse prévisible mais non documentée ni liée depuis les pages HTML du
// site (/data/<édition>/<thème>/<sous-thème>/<figure>/data.csv), trouvée en
// rejouant le trafic réseau de l'application JavaScript (chromium headless,
// --log-net-log) plutôt qu'en devinant un schéma d'URL.
func urlRERS(theme, sousTheme, figure string) string {
	return fmt.Sprintf("https://rers.depp.education.fr/data/%d/%s/%s/%s/data.csv",
		editionRERS, theme, sousTheme, figure)
}

var SourceRERSDepenseEleve = archive.Source{
	Slug: "depp-rers-depense-eleve", Label: "Depp — RERS, la dépense par élève et par étudiant (tableau 10.05)",
	Publisher:  "Direction de l'évaluation, de la prospective et de la performance (Depp)",
	Tier:       "PRIMARY_OFFICIAL",
	License:    "Licence Ouverte v2.0",
	ReuseClass: "ATTRIBUTION", // aucune mention de licence trouvée sur rers.depp.education.fr
	// (page /conditions et /mentions-legales vérifiées, aucun texte de licence) —
	// présumée Licence Ouverte par défaut (publication Depp, service public),
	// à confirmer avant tout usage commercial du jeu rechargé ici.
	Attribution: "Source : ministère de l'Éducation nationale (Depp), RERS Interactif",
	Cadence:     "annuelle",
	Notes: "Euros constants, base = dernière année de la série à chaque édition (prix 2024 pour " +
		"l'édition 2026 — confirmé par la publication RERS 2026, pas par le CSV lui-même qui ne " +
		"porte pas cette mention). Dernière année (2024) marquée provisoire (\"2024p\") dans la " +
		"source : chargée telle quelle, avec provisoire=true.",
}

// IngestDepenseEleve charge RERS 10.05 figure 1 (série annuelle 1980-2024,
// par degré) — pas la figure 2 du même tableau (mêmes données, seulement
// neuf années repères 1980/1990/2000/2005/2010/2015/2020/2022/2023,
// redondante avec la série annuelle ici chargée en entier).
func IngestDepenseEleve(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceRERSDepenseEleve)
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

	f, err := arch.Fetch(ctx, srcID, runID, urlRERS("10_BUD", "05_DEPEL", "01"), ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.Comma = ';'
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête illisible : %w", err))
	}
	// header: "";"Premier degré";"Second degré";"Supérieur";"Ensemble"
	niveaux := map[int]string{1: "premier_degre", 2: "second_degre", 3: "superieur", 4: "ensemble"}
	if len(header) != 5 {
		return fail(fmt.Errorf("colonnes inattendues : %q", header))
	}

	nombre := func(s string) (float64, error) {
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, " ", "") // espace insécable, séparateur de milliers
		s = strings.ReplaceAll(s, ",", ".")
		return strconv.ParseFloat(s, 64)
	}

	var rows [][]any
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		anneeStr := strings.TrimSuffix(rec[0], "p")
		provisoire := strings.HasSuffix(rec[0], "p")
		annee, err := strconv.Atoi(anneeStr)
		if err != nil {
			return fail(fmt.Errorf("année %q illisible : %w", rec[0], err))
		}
		for col, niveau := range niveaux {
			v, err := nombre(rec[col])
			if err != nil {
				return fail(fmt.Errorf("%d %s : valeur %q illisible : %w", annee, niveau, rec[col], err))
			}
			rows = append(rows, []any{annee, niveau, provisoire, v, 2024, editionRERS, srcID})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("dépense par élève : aucune ligne lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_education_depense_eleve (
			annee integer, niveau text, provisoire boolean, depense_euros_constants numeric,
			prix_annee integer, edition_rers integer, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_education_depense_eleve"},
		[]string{"annee", "niveau", "provisoire", "depense_euros_constants", "prix_annee", "edition_rers", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("education_depense_eleve : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.education_depense_eleve AS tgt
		USING tmp_education_depense_eleve AS src
		ON tgt.annee = src.annee AND tgt.niveau = src.niveau
		WHEN MATCHED AND (tgt.provisoire, tgt.depense_euros_constants, tgt.prix_annee, tgt.edition_rers, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.provisoire, src.depense_euros_constants, src.prix_annee, src.edition_rers, src.source_id) THEN
		    UPDATE SET provisoire = src.provisoire, depense_euros_constants = src.depense_euros_constants,
		               prix_annee = src.prix_annee, edition_rers = src.edition_rers, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, niveau, provisoire, depense_euros_constants, prix_annee, edition_rers, source_id)
		    VALUES (src.annee, src.niveau, src.provisoire, src.depense_euros_constants, src.prix_annee,
		            src.edition_rers, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion education_depense_eleve : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n, "touchees": ct.RowsAffected()}, "")
	fmt.Printf("  dépense par élève (RERS %d) : %d lignes\n", editionRERS, n)
	return nil
}
