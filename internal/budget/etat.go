package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// L'exécution budgétaire mensuelle de l'État.
//
// C'est la meilleure série d'exécution publiée — et la seule. Les jeux « PLF »
// de data.economie.gouv.fr ne forment pas une série : un jeu par millésime, des
// identifiants qui changent de forme chaque année (plf25-…, plf-2026-…), et
// plusieurs publiés à zéro ligne. Il n'existe AUCUNE série continue du budget
// voté de l'État en données ouvertes.
var SourceExecutionEtat = archive.Source{
	Slug:        "dgfip-situations-mensuelles",
	Label:       "DGFiP — situations mensuelles budgétaires de l'État",
	Publisher:   "Direction générale des finances publiques",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : ministère de l'Économie et des Finances, situations mensuelles budgétaires de l'État",
	Cadence:     "mensuelle",
	Notes: "Le jeu est PIVOTÉ : vingt-six lignes de postes et une colonne par date " +
		"d'arrêté. Il est déplié au chargement, et une colonne dont le nom ne se parse " +
		"pas en date fait ÉCHOUER le chargement — une colonne ignorée, c'est un mois " +
		"perdu sans trace. " +
		"Le titre annonce « les exercices 2013 à nos jours » ; le schéma publié ne porte " +
		"que les arrêtés depuis janvier 2024. L'écart est un fait de la source. " +
		"Montants CUMULÉS depuis le 1er janvier de l'exercice, en comptabilité " +
		"budgétaire : non comparables au déficit public au sens de Maastricht.",
}

// Les colonnes de date ont la forme JJ_MM_AAAA — « 31_01_2024 ». Le motif sert à
// les RECONNAÎTRE ; c'est time.Parse qui décide si la date existe, de sorte
// qu'un « 31_02_2024 » serait refusé plutôt que silencieusement accepté.
var reColonneDate = regexp.MustCompile(`^(\d{2})_(\d{2})_(\d{4})$`)

// Les cinq colonnes qui décrivent le poste, et non un arrêté.
var colonnesPoste = map[string]bool{
	"niveau_hierarchique":             true,
	"niveau_hierarchique_de_la_ligne": true,
	"categorie":                       true,
	"sous_categorie":                  true,
	"ligne_d_information":             true,
}

func IngestExecutionEtat(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceExecutionEtat)
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

	f, err := arch.Fetch(ctx, srcID, runID,
		exportJSON("data.economie.gouv.fr", "situations-mensuelles-budgetaires-series-longues"), ".json")
	if err != nil {
		return fail(err)
	}
	// Décodé en map : le schéma gagne une colonne chaque mois, une structure Go
	// serait périmée à la publication suivante.
	var lignes []map[string]json.RawMessage
	if err := lireJSON(f.Path, &lignes); err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("situations mensuelles : export vide"))
	}

	// Les dates d'arrêté sont établies UNE FOIS, sur la première ligne, puis
	// exigées identiques partout : une ligne à qui il manquerait une colonne
	// passerait autrement inaperçue.
	dates, err := colonnesDate(lignes[0])
	if err != nil {
		return fail(err)
	}
	if len(dates) == 0 {
		return fail(fmt.Errorf("situations mensuelles : aucune colonne de date — le format a changé"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	var rows [][]any
	var renseignes int
	for i, l := range lignes {
		// Toute colonne inconnue est une anomalie : soit une nouvelle dimension
		// que le chargement ignorerait, soit une date mal formée.
		for nom := range l {
			if colonnesPoste[nom] {
				continue
			}
			if !reColonneDate.MatchString(nom) {
				return fail(fmt.Errorf(
					"situations mensuelles, ligne %d : colonne %q ni poste connu ni date JJ_MM_AAAA", i+1, nom))
			}
		}
		niveau, err := entier(l["niveau_hierarchique"])
		if err != nil {
			return fail(fmt.Errorf("situations mensuelles, ligne %d : niveau illisible : %w", i+1, err))
		}
		categorie := texte(l["categorie"])
		sousCat := texte(l["sous_categorie"])
		ligne := texte(l["ligne_d_information"])
		if ligne == "" {
			return fail(fmt.Errorf("situations mensuelles, ligne %d : intitulé vide", i+1))
		}
		for _, d := range dates {
			brut, present := l[d.colonne]
			if !present {
				return fail(fmt.Errorf(
					"situations mensuelles, ligne %d (%s) : colonne %q absente alors qu'elle existe ailleurs",
					i+1, ligne, d.colonne))
			}
			v, err := reel(brut)
			if err != nil {
				return fail(fmt.Errorf("situations mensuelles, ligne %d (%s), %s : %w",
					i+1, ligne, d.colonne, err))
			}
			if v != nil {
				renseignes++
			}
			rows = append(rows, []any{
				d.date, int16(d.date.Year()), niveau, categorie, sousCat, ligne, nulF(v),
				"ETAT_BUDGET_GENERAL", "BUDGETAIRE", "EXECUTION", srcID, f.DocumentID,
			})
		}
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_execution_etat (
			date_arrete date NOT NULL,
			exercice smallint NOT NULL,
			niveau smallint NOT NULL,
			categorie text NOT NULL,
			sous_categorie text NOT NULL,
			ligne text NOT NULL,
			montant_eur double precision,
			perimetre text NOT NULL,
			comptabilite text NOT NULL,
			stade text NOT NULL,
			source_id bigint NOT NULL,
			document_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_execution_etat"},
		[]string{"date_arrete", "exercice", "niveau", "categorie", "sous_categorie",
			"ligne", "montant_eur", "perimetre", "comptabilite", "stade",
			"source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("situations mensuelles : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.execution_etat AS tgt
		USING tmp_execution_etat AS src
		ON tgt.date_arrete = src.date_arrete AND tgt.categorie = src.categorie
			AND tgt.sous_categorie = src.sous_categorie AND tgt.ligne = src.ligne
		WHEN MATCHED AND (tgt.exercice, tgt.niveau, tgt.montant_eur, tgt.perimetre,
				tgt.comptabilite, tgt.stade, tgt.source_id, tgt.document_id)
			IS DISTINCT FROM (src.exercice, src.niveau, src.montant_eur, src.perimetre,
				src.comptabilite, src.stade, src.source_id, src.document_id) THEN
			UPDATE SET exercice = src.exercice, niveau = src.niveau, montant_eur = src.montant_eur,
				perimetre = src.perimetre, comptabilite = src.comptabilite, stade = src.stade,
				source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (date_arrete, exercice, niveau, categorie, sous_categorie, ligne,
				montant_eur, perimetre, comptabilite, stade, source_id, document_id)
			VALUES (src.date_arrete, src.exercice, src.niveau, src.categorie, src.sous_categorie,
				src.ligne, src.montant_eur, src.perimetre, src.comptabilite, src.stade,
				src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("situations mensuelles, fusion : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"postes": len(lignes), "arretes": len(dates), "lignes": n, "renseignes": renseignes}, "")
	fmt.Printf("  État : %d postes × %d arrêtés = %d lignes touchées par la fusion (%d renseignées), du %s au %s\n",
		len(lignes), len(dates), n, renseignes,
		dates[0].date.Format("2006-01-02"), dates[len(dates)-1].date.Format("2006-01-02"))
	return nil
}

type arrete struct {
	colonne string
	date    time.Time
}

// colonnesDate reconnaît les colonnes d'arrêté et REFUSE tout ce qui n'est ni
// une colonne de poste connue ni une date valide. C'est le contrôle qui empêche
// qu'un mois disparaisse en silence parce que la source a changé de convention.
func colonnesDate(l map[string]json.RawMessage) ([]arrete, error) {
	var out []arrete
	for nom := range l {
		if colonnesPoste[nom] {
			continue
		}
		m := reColonneDate.FindStringSubmatch(nom)
		if m == nil {
			return nil, fmt.Errorf("colonne %q : ni poste connu ni date JJ_MM_AAAA", nom)
		}
		// time.Parse en mode strict : « 31_02_2024 » ressort comme le 2 mars, on
		// le rejette en comparant la date reconstruite au texte d'origine.
		d, err := time.Parse("02_01_2006", nom)
		if err != nil {
			return nil, fmt.Errorf("colonne %q : %w", nom, err)
		}
		if d.Format("02_01_2006") != nom {
			return nil, fmt.Errorf("colonne %q : date inexistante au calendrier", nom)
		}
		out = append(out, arrete{colonne: nom, date: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].date.Before(out[j].date) })
	return out, nil
}

func texte(r json.RawMessage) string {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		return s
	}
	return ""
}

func entier(r json.RawMessage) (int, error) {
	if len(r) == 0 {
		return 0, fmt.Errorf("valeur absente")
	}
	var n int
	if err := json.Unmarshal(r, &n); err != nil {
		return 0, err
	}
	return n, nil
}

// reel accepte un nombre, un nombre écrit comme une chaîne, et l'absence de
// valeur. Il refuse tout le reste : un texte inattendu dans une colonne de
// montant est une anomalie de source, pas un zéro.
func reel(r json.RawMessage) (*float64, error) {
	if len(r) == 0 || string(r) == "null" || string(r) == `""` {
		return nil, nil
	}
	var f float64
	if err := json.Unmarshal(r, &f); err == nil {
		return &f, nil
	}
	var s string
	if err := json.Unmarshal(r, &s); err != nil {
		return nil, fmt.Errorf("montant illisible %s", r)
	}
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("montant illisible %q", s)
	}
	return &f, nil
}
