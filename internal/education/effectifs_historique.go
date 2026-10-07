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

var SourceRERSEffectifsHistorique = archive.Source{
	Slug: "depp-rers-effectifs-historique", Label: "Depp — RERS, effectifs d'élèves 1er et 2nd degrés (tableaux 3.01, 4.01)",
	Publisher:   "Direction de l'évaluation, de la prospective et de la performance (Depp)",
	Tier:        "PRIMARY_OFFICIAL",
	License:     "Licence Ouverte v2.0",
	ReuseClass:  "ATTRIBUTION", // voir depense_eleve.go : licence présumée, non confirmée sur le site
	Attribution: "Source : ministère de l'Éducation nationale (Depp), RERS Interactif",
	Cadence:     "annuelle (à la rentrée scolaire)",
	Notes: "Effectifs en milliers, tels que publiés. Le premier degré (tableau 3.01) remonte à 1960 " +
		"; le second degré (tableau 4.01, collèges et lycées) ne remonte qu'à 1994, la Depp ne " +
		"publiant pas cette série plus loin dans RERS Interactif. Deux années (1999 et 2011) " +
		"apparaissent en double dans le premier degré, avec deux valeurs différentes — une rupture " +
		"de série dans la source elle-même (changement de périmètre ou de méthode, non expliqué " +
		"dans le texte du tableau), pas une erreur de ce chargement : seule la valeur la plus " +
		"récente (celle qui prolonge la série vers l'année suivante) est retenue, l'autre écartée.",
}

type effectifColonne struct {
	degre, niveau string
}

// lireEvolutionEffectifs télécharge et lit un seul tableau RERS (3.01 ou
// 4.01, figure 1) — pas d'accès base ici, seulement le fichier : les deux
// appels (premier et second degré) sont combinés dans UNE seule transaction
// par l'appelant, pour une fusion (MERGE) unique plutôt que deux.
//
// diviseur ramène à la même unité (milliers d'élèves) deux tableaux RERS
// qui ne publient pas dans la même échelle : 3.01 (premier degré) est déjà
// en milliers ("3937,2"), 4.01 (second degré) en effectifs bruts
// ("3386832") — vérifié contre le texte du tableau 4.01 lui-même ("3 360 000
// élèves étudient au collège" pour une valeur brute de 3 359 928). Sans
// cette conversion, toute comparaison entre degrés serait fausse d'un
// facteur 1000.
func lireEvolutionEffectifs(ctx context.Context, arch *archive.Archive, srcID, runID int64,
	theme, sousTheme string, diviseur float64, colonnes []effectifColonne) ([][]any, error) {

	f, err := arch.Fetch(ctx, srcID, runID, urlRERS(theme, sousTheme, "01"), ".csv")
	if err != nil {
		return nil, err
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.Comma = ';'
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s/%s : en-tête illisible : %w", theme, sousTheme, err)
	}
	if len(header) != len(colonnes)+1 {
		return nil, fmt.Errorf("%s/%s : %d colonnes attendues, %d trouvées (%q)",
			theme, sousTheme, len(colonnes)+1, len(header), header)
	}

	nombre := func(s string) (float64, error) {
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, ",", ".")
		return strconv.ParseFloat(s, 64)
	}

	vu := map[int]bool{}
	var rows [][]any
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		annee, err := strconv.Atoi(rec[0])
		if err != nil {
			return nil, fmt.Errorf("%s/%s : année %q illisible : %w", theme, sousTheme, rec[0], err)
		}
		if vu[annee] {
			continue // rupture de série (voir Notes) : seule la première occurrence est gardée
		}
		vu[annee] = true
		for i, c := range colonnes {
			v, err := nombre(rec[i+1])
			if err != nil {
				return nil, fmt.Errorf("%s/%s %d %s : valeur %q illisible : %w",
					theme, sousTheme, annee, c.niveau, rec[i+1], err)
			}
			rows = append(rows, []any{annee, c.degre, c.niveau, v / diviseur, editionRERS, srcID})
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s/%s : aucune ligne lue", theme, sousTheme)
	}
	return rows, nil
}

// IngestEffectifsHistorique charge les tableaux RERS 3.01 (premier degré,
// 1960-2025) et 4.01 (second degré, 1994-2025), figure 1 de chacun — la
// série annuelle par niveau, pas les figures suivantes du même tableau
// (répartitions public/privé ou par génération de naissance, hors du
// périmètre d'une série longue par niveau).
func IngestEffectifsHistorique(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceRERSEffectifsHistorique)
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

	rows1, err := lireEvolutionEffectifs(ctx, arch, srcID, runID, "03_EL1D", "01_EVO", 1,
		[]effectifColonne{{"premier", "preelementaire"}, {"premier", "elementaire"}})
	if err != nil {
		return fail(err)
	}
	rows2, err := lireEvolutionEffectifs(ctx, arch, srcID, runID, "04_EL2D", "01_EVO", 1000,
		[]effectifColonne{
			{"second", "college"},
			{"second", "lycee_general_technologique"},
			{"second", "lycee_professionnel"},
		})
	if err != nil {
		return fail(err)
	}
	rows := append(rows1, rows2...)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_education_effectif_eleves_historique (
			annee integer, degre text, niveau text, effectif numeric, edition_rers integer, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_education_effectif_eleves_historique"},
		[]string{"annee", "degre", "niveau", "effectif", "edition_rers", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("education_effectif_eleves_historique : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.education_effectif_eleves_historique AS tgt
		USING tmp_education_effectif_eleves_historique AS src
		ON tgt.annee = src.annee AND tgt.niveau = src.niveau
		WHEN MATCHED AND (tgt.degre, tgt.effectif, tgt.edition_rers, tgt.source_id)
		                  IS DISTINCT FROM (src.degre, src.effectif, src.edition_rers, src.source_id) THEN
		    UPDATE SET degre = src.degre, effectif = src.effectif,
		               edition_rers = src.edition_rers, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, degre, niveau, effectif, edition_rers, source_id)
		    VALUES (src.annee, src.degre, src.niveau, src.effectif, src.edition_rers, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion education_effectif_eleves_historique : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows), "touchees": ct.RowsAffected()}, "")
	fmt.Printf("  effectifs d'élèves 1960-2025 (RERS %d, 1er+2nd degrés) : %d lignes\n", editionRERS, len(rows))
	return nil
}
