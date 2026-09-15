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
var SourceAideAlimentaire = archive.Source{
	Slug: "drees-aide-alimentaire", Label: "Drees — dispositif de suivi de l'aide alimentaire en France",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee-Drees, dispositif de suivi de l'aide alimentaire en France",
	Cadence:     "trimestrielle (au moment de la collecte)",
	Notes: "Dernière édition publiée par la Drees : juillet 2021, données arrêtées en 2021 " +
		"(2020 pour la Fédération française des banques alimentaires, 2020-2021 seulement pour " +
		"le Secours populaire) — la série ne semble pas avoir été reconduite depuis, signalé " +
		"explicitement plutôt que présenté comme actuel. Six réseaux aux indicateurs " +
		"hétérogènes (colis, repas, tonnes ou euros selon le réseau) : voir le commentaire de " +
		"core.aide_alimentaire pour le format long qui les rassemble sans les confondre.",
}

const aideAlimentaireURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
	"laide-alimentaire-en-france-depuis-2019/attachments/l_aide_alimentaire_en_france_depuis_2019_xlsx"

// Un intitulé de colonne peut porter un retour à la ligne dans le classeur
// source ("Volumes \n(en tonnes)") : normalisé avant correspondance.
var libelleIndicateur = map[string]string{
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

var reperiode = regexp.MustCompile(`^(\d{4}) - (Total année|(\d)(?:er|ère|ème|e) trimestre)$`)

// Les Restos du Cœur (Tableau 4) ne suivent pas le calendrier civil des cinq
// autres réseaux : leurs deux campagnes de distribution (hiver et été)
// s'intitulent « Décembre 2020 à Février 2021 », « Juin 2020 à Août 2020»,
// etc. — un format de période à part, jamais un trimestre ou une année civile.
var recampagne = regexp.MustCompile(`^\p{L}+ (\d{4}) à \p{L}+ \d{4}$`)

type feuilleAssociation struct{ Feuille, Nom string }

var feuillesAideAlimentaire = []feuilleAssociation{
	{"Tableau 1", "ANDES"},
	{"Tableau 2", "Croix-Rouge française"},
	{"Tableau 3", "Fédération française des banques alimentaires"},
	{"Tableau 4", "Restos du Cœur"},
	{"Tableau 5", "Secours catholique"},
	{"Tableau 6", "Secours populaire français"},
}

func IngestAideAlimentaire(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAideAlimentaire)
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

	f, err := arch.Fetch(ctx, srcID, runID, aideAlimentaireURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.aide_alimentaire`); err != nil {
		return fail(err)
	}

	var total int64
	for _, fa := range feuillesAideAlimentaire {
		lignes, err := x.rows(fa.Feuille)
		if err != nil {
			return fail(fmt.Errorf("%s (%s) : %w", fa.Feuille, fa.Nom, err))
		}
		rows, err := ligneAssociation(lignes, fa.Nom, srcID)
		if err != nil {
			return fail(fmt.Errorf("%s (%s) : %w", fa.Feuille, fa.Nom, err))
		}
		if len(rows) == 0 {
			return fail(fmt.Errorf("%s (%s) : aucune donnée reconnue", fa.Feuille, fa.Nom))
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "aide_alimentaire"},
			[]string{"association", "periode_type", "annee", "trimestre", "periode_libelle", "indicateur", "valeur", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("%s (%s) : %w", fa.Feuille, fa.Nom, err))
		}
		total += n
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total}, "")
	fmt.Printf("  aide alimentaire (Insee-Drees) : %d lignes, %d réseaux\n", total, len(feuillesAideAlimentaire))
	return nil
}

// ligneAssociation repère la ligne d'en-tête (colonne A = "Période") puis lit
// les lignes suivantes tant que la colonne A ressemble à une période connue —
// même principe que colonneAnnees pour minima_sociaux.go, adapté à un
// en-tête en ligne plutôt qu'en colonne.
func ligneAssociation(lignes []map[string]string, association string, srcID int64) ([][]any, error) {
	var colonnes map[string]string // lettre de colonne -> indicateur
	var rows [][]any
	for _, l := range lignes {
		if l["A"] == "Période" {
			colonnes = map[string]string{}
			for col, libelle := range l {
				if col == "A" {
					continue
				}
				norm := strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(libelle)), " ")
				if ind, ok := libelleIndicateur[norm]; ok {
					colonnes[col] = ind
				}
			}
			continue
		}
		if colonnes == nil {
			continue // avant l'en-tête : titre, source, champ
		}

		var periodeType string
		var annee int
		var trimestre *int
		var periodeLibelle *string

		if m := reperiode.FindStringSubmatch(l["A"]); m != nil {
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			annee = a
			if m[2] == "Total année" {
				periodeType = "ANNEE"
			} else {
				t, err := strconv.Atoi(m[3])
				if err != nil {
					return nil, err
				}
				periodeType = "TRIMESTRE"
				trimestre = &t
			}
		} else if m := recampagne.FindStringSubmatch(l["A"]); m != nil {
			a, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			periodeType = "CAMPAGNE"
			annee = a
			libelle := l["A"]
			periodeLibelle = &libelle
		} else {
			continue // fin du tableau (ligne vide ou note de bas de page)
		}

		for col, ind := range colonnes {
			v, ok := l[col]
			if !ok || v == "" {
				continue
			}
			val, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil {
				continue
			}
			rows = append(rows, []any{association, periodeType, annee, trimestre, periodeLibelle, ind, val, srcID})
		}
	}
	return rows, nil
}
