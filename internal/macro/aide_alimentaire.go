package macro

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le dispositif de suivi Insee-Drees de l'aide alimentaire, six réseaux
// nationaux. Voir docs/pauvrete-donnees.md, où l'aide alimentaire éclaire une
// pauvreté qui échappe au seul seuil monétaire (core.pauvrete_seuil_annuel) :
// un foyer au-dessus du seuil peut recourir à l'aide alimentaire, et
// inversement.
var SourceFoodAid = archive.Source{
	Slug: "drees-aide-alimentaire", Label: "Drees — dispositif de suivi de l'aide alimentaire en France",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	License:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee-Drees, dispositif de suivi de l'aide alimentaire en France",
	Cadence:     "trimestrielle (au moment de la collecte)",
	Notes: "Dernière édition publiée par la Drees : juillet 2021, données arrêtées en 2021 " +
		"(2020 pour la Fédération française des banques alimentaires, 2020-2021 seulement pour " +
		"le Secours populaire) — la série ne semble pas avoir été reconduite depuis, signalé " +
		"explicitement plutôt que présenté comme actuel. Six réseaux aux indicateurs " +
		"hétérogènes (colis, repas, tonnes ou euros selon le réseau) : voir le commentaire de " +
		"core.aide_alimentaire pour le format long qui les rassemble sans les confondre.",
}

const foodAidURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"laide-alimentaire-en-france-depuis-2019/attachments/l_aide_alimentaire_en_france_depuis_2019_xlsx"

// Un intitulé de colonne peut porter un retour à la ligne dans le classeur
// source ("Volumes \n(en tonnes)") : normalisé avant correspondance.
var indicatorLabel = map[string]string{
	"Volumes (en tonnes)":                "volume_tonnes",
	"Foyers inscrits":                    "foyers_inscrits",
	"Personnes inscrites":                "personnes_inscrites",
	"Hommes":                             "hommes",
	"Femmes":                             "femmes",
	"Personnes entre 0 et 3 ans":         "age_0_3",
	"Personnes entre 4 et 14 ans":        "age_4_14",
	"Personnes entre 15 et 25 ans":       "age_15_25",
	"Personnes entre 26 et 59 ans":       "age_26_59",
	"Personnes de 60 ans ou plus":        "age_60_plus",
	"Colis (en nombre)":                  "colis_nombre",
	"Centres actifs":                     "centres_actifs",
	"Personnes entre 26 et 49 ans":       "age_26_49",
	"Personnes entre 50 et 64 ans":       "age_50_64",
	"Personnes de 65 ans ou plus":        "age_65_plus",
	"Personnes entre 26 et 64 ans":       "age_26_64",
	"Repas (en nombre)":                  "repas_nombre",
	"Dépenses d'aide directe (en euros)": "depenses_aide_directe_eur",
}

var rePeriod = regexp.MustCompile(`^(\d{4}) - (Total année|(\d)(?:er|ère|ème|e) trimestre)$`)

// Les Restos du Cœur (Tableau 4) ne suivent pas le calendrier civil des cinq
// autres réseaux : leurs deux campagnes de distribution (hiver et été)
// s'intitulent « Décembre 2020 à Février 2021 », « Juin 2020 à Août 2020»,
// etc. — un format de période à part, jamais un trimestre ou une année civile.
var reCampaign = regexp.MustCompile(`^\p{L}+ (\d{4}) à \p{L}+ \d{4}$`)

type associationSheet struct{ Sheet, Name string }

var foodAidSheets = []associationSheet{
	{"Tableau 1", "ANDES"},
	{"Tableau 2", "Croix-Rouge française"},
	{"Tableau 3", "Fédération française des banques alimentaires"},
	{"Tableau 4", "Restos du Cœur"},
	{"Tableau 5", "Secours catholique"},
	{"Tableau 6", "Secours populaire français"},
}

func IngestFoodAid(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceFoodAid)
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

	f, err := arch.Fetch(ctx, srcID, runID, foodAidURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	var allRows [][]any
	for _, fa := range foodAidSheets {
		sheetRows, err := x.rows(fa.Sheet)
		if err != nil {
			return fail(fmt.Errorf("%s (%s) : %w", fa.Sheet, fa.Name, err))
		}
		rows, err := associationRows(sheetRows, fa.Name, srcID)
		if err != nil {
			return fail(fmt.Errorf("%s (%s) : %w", fa.Sheet, fa.Name, err))
		}
		if len(rows) == 0 {
			return fail(fmt.Errorf("%s (%s) : aucune donnée reconnue", fa.Sheet, fa.Name))
		}
		allRows = append(allRows, rows...)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité des six réseaux à chaque republication, changement
	// ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_aide_alimentaire (
			association text, periode_type text, annee smallint, trimestre smallint,
			periode_libelle text, indicateur text, valeur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_aide_alimentaire"},
		[]string{"association", "periode_type", "annee", "trimestre", "periode_libelle", "indicateur", "valeur", "source_id"},
		pgx.CopyFromRows(allRows)); err != nil {
		return fail(fmt.Errorf("core.aide_alimentaire : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.aide_alimentaire AS tgt
		USING tmp_aide_alimentaire AS src
		ON tgt.association = src.association AND tgt.periode_type = src.periode_type
		   AND tgt.annee = src.annee
		   AND COALESCE(tgt.trimestre, 0) = COALESCE(src.trimestre, 0)
		   AND COALESCE(tgt.periode_libelle, '') = COALESCE(src.periode_libelle, '')
		   AND tgt.indicateur = src.indicateur
		WHEN MATCHED AND (tgt.valeur, tgt.source_id) IS DISTINCT FROM (src.valeur, src.source_id) THEN
		    UPDATE SET valeur = src.valeur, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (association, periode_type, annee, trimestre, periode_libelle, indicateur, valeur, source_id)
		    VALUES (src.association, src.periode_type, src.annee, src.trimestre, src.periode_libelle,
		            src.indicateur, src.valeur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(allRows), "touchees": touchees}, "")
	fmt.Printf("  aide alimentaire (Insee-Drees) : %d lignes, %d réseaux (%d touchées par la fusion)\n",
		len(allRows), len(foodAidSheets), touchees)
	return nil
}

// associationRows repère la ligne d'en-tête (colonne A = "Période") puis lit
// les lignes suivantes tant que la colonne A ressemble à une période connue —
// même principe que yearColumns pour minima_sociaux.go, adapté à un
// en-tête en ligne plutôt qu'en colonne.
func associationRows(sheetRows []map[string]string, association string, srcID int64) ([][]any, error) {
	var columns map[string]string // lettre de colonne -> indicateur
	var rows [][]any
	for _, l := range sheetRows {
		if l["A"] == "Période" {
			columns = map[string]string{}
			for col, label := range l {
				if col == "A" {
					continue
				}
				normalized := strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(label)), " ")
				if ind, ok := indicatorLabel[normalized]; ok {
					columns[col] = ind
				}
			}
			continue
		}
		if columns == nil {
			continue // avant l'en-tête : titre, source, champ
		}

		var periodType string
		var year int
		var quarter *int
		var periodLabel *string

		if m := rePeriod.FindStringSubmatch(l["A"]); m != nil {
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			year = a
			if m[2] == "Total année" {
				periodType = "ANNEE"
			} else {
				t, err := strconv.Atoi(m[3])
				if err != nil {
					return nil, err
				}
				periodType = "TRIMESTRE"
				quarter = &t
			}
		} else if m := reCampaign.FindStringSubmatch(l["A"]); m != nil {
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			periodType = "CAMPAGNE"
			year = a
			label := l["A"]
			periodLabel = &label
		} else {
			continue // fin du tableau (ligne vide ou note de bas de page)
		}

		for col, ind := range columns {
			v, ok := l[col]
			if !ok || v == "" {
				continue
			}
			val, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil {
				continue
			}
			rows = append(rows, []any{association, periodType, year, quarter, periodLabel, ind, val, srcID})
		}
	}
	return rows, nil
}
