package budget

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le budget de l'État par mission/programme/action — LOLF. Un seul jeu de
// données générique, pas un connecteur par mission : les budgets détaillés
// de la Police nationale et de la Défense (docs/securite-police-donnees.md,
// docs/defense-donnees.md) en sont deux LECTURES, pas deux CHARGEMENTS.
var SourcePLFDestination = archive.Source{
	Slug: "plf-depenses-selon-destination", Label: "PLF — dépenses de l'État par mission, programme et action",
	Publisher: "Direction du budget (ministère de l'Économie et des Finances)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Direction du budget, data.economie.gouv.fr",
	Cadence:     "annuelle (dépôt du PLF, généralement en octobre)",
	Notes: "Deux millésimes chargés (2024, 2025), sous deux schémas de champs différents d'une " +
		"édition à l'autre — voir normaliserLigne. Ce sont des montants VOTÉS au PROJET de loi de " +
		"finances (PLF), pas la loi de finances initiale telle qu'adoptée ni l'exécution réelle : " +
		"même piège voté/exécuté que docs/budget-donnees.md § 2, le champ `loi` le porte pour ne " +
		"jamais le perdre en aval. Couvre TOUTES les missions du budget général, pas seulement " +
		"Défense et Sécurités : Éducation nationale (« Enseignement scolaire ») et Pouvoirs publics " +
		"y sont aussi, pour des chantiers ultérieurs.",
}

type anneePLF struct {
	Annee  int
	URL    string
	Schema string
}

var anneesPLF = []anneePLF{
	{2024, exportJSON("data.economie.gouv.fr", "plf-2024-depenses-2024-selon-nomenclatures-destination-et-nature"), "2024"},
	{2025, exportJSON("data.economie.gouv.fr", "plf25-depenses-2025-selon-destination"), "2025"},
}

type ligneBudgetProgramme struct {
	Exercice                          int
	Loi, TypeBudget                   string
	Ministere                         *string
	MissionCode, MissionLibelle       string
	ProgrammeCode, ProgrammeLibelle   string
	ActionCode, ActionLibelle         *string
	SousActionCode, SousActionLibelle *string
	Categorie                         *int
	Titre                             int
	AE, CP                            *float64
}

func IngestPLFDestination(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePLFDestination)
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

	var toutes []ligneBudgetProgramme
	for _, an := range anneesPLF {
		f, err := arch.Fetch(ctx, srcID, runID, an.URL, ".json")
		if err != nil {
			return fail(fmt.Errorf("PLF %d : %w", an.Annee, err))
		}
		var brut []map[string]any
		if err := lireJSON(f.Path, &brut); err != nil {
			return fail(fmt.Errorf("PLF %d : %w", an.Annee, err))
		}
		if len(brut) == 0 {
			return fail(fmt.Errorf("PLF %d : export vide", an.Annee))
		}
		for _, r := range brut {
			ln, err := normaliserLigne(an.Annee, an.Schema, r)
			if err != nil {
				return fail(fmt.Errorf("PLF %d : %w", an.Annee, err))
			}
			toutes = append(toutes, ln)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	var rows [][]any
	for _, l := range toutes {
		rows = append(rows, []any{
			l.Exercice, l.Loi, l.TypeBudget, l.Ministere,
			l.MissionCode, l.MissionLibelle, l.ProgrammeCode, l.ProgrammeLibelle,
			l.ActionCode, l.ActionLibelle, l.SousActionCode, l.SousActionLibelle,
			l.Categorie, l.Titre, l.AE, l.CP, srcID,
		})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_budget_programme (
			exercice smallint NOT NULL,
			loi text NOT NULL,
			type_budget text NOT NULL,
			ministere text,
			mission_code text NOT NULL,
			mission_libelle text NOT NULL,
			programme_code text NOT NULL,
			programme_libelle text NOT NULL,
			action_code text,
			action_libelle text,
			sous_action_code text,
			sous_action_libelle text,
			categorie smallint,
			titre smallint NOT NULL,
			autorisation_engagement numeric,
			credit_paiement numeric,
			source_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_budget_programme"},
		[]string{"exercice", "loi", "type_budget", "ministere",
			"mission_code", "mission_libelle", "programme_code", "programme_libelle",
			"action_code", "action_libelle", "sous_action_code", "sous_action_libelle",
			"categorie", "titre", "autorisation_engagement", "credit_paiement", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("budget_programme : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.budget_programme AS tgt
		USING tmp_budget_programme AS src
		ON tgt.exercice = src.exercice AND tgt.loi = src.loi AND tgt.type_budget = src.type_budget
			AND tgt.mission_code = src.mission_code AND tgt.programme_code = src.programme_code
			AND tgt.action_code IS NOT DISTINCT FROM src.action_code
			AND tgt.sous_action_code IS NOT DISTINCT FROM src.sous_action_code
			AND tgt.categorie IS NOT DISTINCT FROM src.categorie
			AND tgt.titre = src.titre
		WHEN MATCHED AND (tgt.ministere, tgt.mission_libelle, tgt.programme_libelle,
				tgt.action_libelle, tgt.sous_action_libelle,
				tgt.autorisation_engagement, tgt.credit_paiement, tgt.source_id)
			IS DISTINCT FROM (src.ministere, src.mission_libelle, src.programme_libelle,
				src.action_libelle, src.sous_action_libelle,
				src.autorisation_engagement, src.credit_paiement, src.source_id) THEN
			UPDATE SET ministere = src.ministere, mission_libelle = src.mission_libelle,
				programme_libelle = src.programme_libelle, action_libelle = src.action_libelle,
				sous_action_libelle = src.sous_action_libelle,
				autorisation_engagement = src.autorisation_engagement,
				credit_paiement = src.credit_paiement, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (exercice, loi, type_budget, ministere, mission_code, mission_libelle,
				programme_code, programme_libelle, action_code, action_libelle,
				sous_action_code, sous_action_libelle, categorie, titre,
				autorisation_engagement, credit_paiement, source_id)
			VALUES (src.exercice, src.loi, src.type_budget, src.ministere, src.mission_code,
				src.mission_libelle, src.programme_code, src.programme_libelle, src.action_code,
				src.action_libelle, src.sous_action_code, src.sous_action_libelle, src.categorie,
				src.titre, src.autorisation_engagement, src.credit_paiement, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("budget_programme, fusion : %w", err))
	}
	n := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n, "exercices": len(anneesPLF)}, "")
	fmt.Printf("  budget par mission/programme (PLF) : %d lignes touchées par la fusion, %d exercices\n", n, len(anneesPLF))
	return nil
}

// normaliserLigne absorbe le changement de schéma d'une édition du PLF à
// l'autre : le champ « mission » porte le LIBELLÉ en 2024 et le CODE en
// 2025 (l'inverse pour « code_mission »/« libelle_mission ») — une inversion
// vérifiée sur les jeux réels, pas supposée.
func normaliserLigne(annee int, schema string, r map[string]any) (ligneBudgetProgramme, error) {
	var l ligneBudgetProgramme
	l.Exercice = annee
	switch schema {
	case "2024":
		l.Loi = "PLF"
		l.TypeBudget = asStr(r["type_mission"])
		l.Ministere = asStrPtr(r["ministere"])
		l.MissionCode = asStr(r["code_mission"])
		l.MissionLibelle = asStr(r["mission"])
		l.ProgrammeCode = asStr(r["programme"])
		l.ProgrammeLibelle = asStr(r["libelle_programme"])
		l.ActionCode = asStrPtr(r["action"])
		l.ActionLibelle = asStrPtr(r["libelle_action"])
		l.SousActionCode = asStrPtr(r["sous_action"])
		l.SousActionLibelle = asStrPtr(r["libelle_sousaction"])
		var err error
		if l.Categorie, err = asIntPtr(r["categorie"]); err != nil {
			return l, fmt.Errorf("categorie : %w", err)
		}
		titre, err := asInt(r["code_titre"])
		if err != nil {
			return l, fmt.Errorf("code_titre : %w", err)
		}
		l.Titre = titre
		l.AE = asFloatPtr(r["ae_plf"])
		l.CP = asFloatPtr(r["cp_plf"])
	case "2025":
		l.Loi = asStr(r["loi"])
		l.TypeBudget = asStr(r["typebudget"])
		l.Ministere = asStrPtr(r["libelle_ministere"])
		l.MissionCode = asStr(r["mission"])
		l.MissionLibelle = asStr(r["libelle_mission"])
		l.ProgrammeCode = asStr(r["programme"])
		l.ProgrammeLibelle = asStr(r["libelle_programme"])
		l.ActionCode = asStrPtr(r["action"])
		l.ActionLibelle = asStrPtr(r["libelle_action"])
		l.SousActionCode = asStrPtr(r["sous_action"])
		l.SousActionLibelle = asStrPtr(r["libelle_sous_action"])
		var err error
		if l.Categorie, err = asIntPtr(r["categorie"]); err != nil {
			return l, fmt.Errorf("categorie : %w", err)
		}
		titre, err := asInt(r["titre"])
		if err != nil {
			return l, fmt.Errorf("titre : %w", err)
		}
		l.Titre = titre
		l.AE = asFloatPtr(r["autorisation_engagement"])
		l.CP = asFloatPtr(r["credit_de_paiement"])
	default:
		return l, fmt.Errorf("schéma inconnu %q", schema)
	}
	if l.MissionLibelle == "" || l.ProgrammeLibelle == "" {
		return l, fmt.Errorf("ligne sans mission ou programme")
	}
	return l, nil
}

// asStr convertit une valeur JSON hétérogène (chaîne, nombre, absente) en
// chaîne canonique — un entier publié en flottant (176.0) devient "176", pas
// "176.0", pour que la même valeur s'écrive pareil quel que soit le schéma.
func asStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

func asStrPtr(v any) *string {
	s := asStr(v)
	if s == "" {
		return nil
	}
	return &s
}

func asIntPtr(v any) (*int, error) {
	s := asStr(v)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func asInt(v any) (int, error) {
	s := asStr(v)
	if s == "" {
		return 0, fmt.Errorf("valeur entière manquante")
	}
	return strconv.Atoi(s)
}

func asFloatPtr(v any) *float64 {
	switch x := v.(type) {
	case float64:
		return &x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}
