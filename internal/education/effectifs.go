// Package education charge les données ouvertes de l'Éducation nationale
// (Depp, data.education.gouv.fr). Voir docs/education-donnees.md.
package education

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "education-v1"

func exportJSON(dataset string) string {
	return "https://data.education.gouv.fr/api/explore/v2.1/catalog/datasets/" + dataset + "/exports/json"
}

func lireJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

var SourceEffectifsPersonnel = archive.Source{
	Slug: "depp-personnels-etablissements", Label: "Depp — personnels des établissements du premier et second degré",
	Publisher: "Direction de l'évaluation, de la prospective et de la performance (Depp)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : ministère de l'Éducation nationale (Depp)",
	Cadence:     "annuelle (à la rentrée scolaire)",
	Notes: "Deux jeux distincts, asymétriques : le premier degré publie 2024 ET 2025, le second " +
		"degré seulement 2024 au moment du chargement. Le premier degré ne publie pas la même " +
		"décomposition par statut (titulaire/agrégé/certifié) que le second degré, qui la publie " +
		"par établissement. Rien sur les AESH en tant que catégorie séparée : ils sont mêlés, si " +
		"présents, aux « personnels de vie scolaire » du second degré, sans identification propre.",
}

// Les deux jeux (premier et second degré) ne nomment pas leurs champs à
// l'identique — le second degré écrit « etp_enseignants_hommes_et_femmes »,
// le premier « etp_D_enseignants_hommes_et_femmes » (un « d_ » en plus) — et,
// vérifié sur les exports réels, le MÊME champ numérique peut apparaître tantôt
// comme nombre JSON (5.2) tantôt comme chaîne ("4.2") d'une ligne à l'autre du
// même fichier. D'où un décodage en map[string]any plutôt qu'une struct typée,
// et des conversions tolérantes (asStr/asFloatPtr) plutôt qu'un json.Unmarshal
// qui échouerait sur la première incohérence de type.
type degreJeu struct {
	Degre           string
	Dataset         string
	ChampEnseignant string
}

var jeuxDegre = []degreJeu{
	{"PREMIER", "fr-en-indicateurs_personnels_etablissements1d", "etp_d_enseignants_hommes_et_femmes"},
	{"SECOND", "fr-en-indicateurs_personnels_etablissements2d", "etp_enseignants_hommes_et_femmes"},
}

func IngestEffectifsPersonnel(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEffectifsPersonnel)
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_education_personnel_etablissement (
			annee smallint, degre text, identifiant_etablissement text, nom_etablissement text,
			nature_etablissement text, code_departement text, code_academie text, secteur text,
			etp_total numeric, etp_enseignants numeric, etp_vie_scolaire numeric,
			proportion_non_titulaires numeric, proportion_agreges numeric, proportion_certifies numeric,
			source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var total int64
	for _, jeu := range jeuxDegre {
		f, err := arch.Fetch(ctx, srcID, runID, exportJSON(jeu.Dataset), ".json")
		if err != nil {
			return fail(fmt.Errorf("%s : %w", jeu.Degre, err))
		}
		var lignes []map[string]any
		if err := lireJSON(f.Path, &lignes); err != nil {
			return fail(fmt.Errorf("%s : %w", jeu.Degre, err))
		}
		if len(lignes) == 0 {
			return fail(fmt.Errorf("%s : export vide", jeu.Degre))
		}

		var rows [][]any
		for _, l := range lignes {
			identifiant := asStr(l["identifiant_de_l_etablissement"])
			secteur, err := normaliserSecteur(asStr(l["secteur"]))
			if err != nil {
				return fail(fmt.Errorf("%s, établissement %s : %w", jeu.Degre, identifiant, err))
			}
			anneeStr := asStr(l["annee_de_la_rentree_scolaire"])
			annee, err := strconv.Atoi(anneeStr)
			if err != nil {
				return fail(fmt.Errorf("%s : année illisible %q", jeu.Degre, anneeStr))
			}
			rows = append(rows, []any{
				annee, jeu.Degre, identifiant, asStr(l["nom_de_l_etablissement"]),
				asStrPtr(l["nature_de_l_etablissement"]),
				asStrPtr(l["code_departement"]), asStrPtr(l["code_academie"]), secteur,
				asFloatPtr(l["etp_total"]), asFloatPtr(l[jeu.ChampEnseignant]),
				asFloatPtr(l["etp_de_personnels_de_vie_scolaire"]),
				asFloatPtr(l["proportion_non_titulaires"]), asFloatPtr(l["proportion_agreges"]),
				asFloatPtr(l["proportion_certifies"]),
				srcID,
			})
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_education_personnel_etablissement"},
			[]string{"annee", "degre", "identifiant_etablissement", "nom_etablissement", "nature_etablissement",
				"code_departement", "code_academie", "secteur",
				"etp_total", "etp_enseignants", "etp_vie_scolaire",
				"proportion_non_titulaires", "proportion_agreges", "proportion_certifies",
				"source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("%s : %w", jeu.Degre, err))
		}
		total += n
	}

	ct, err := tx.Exec(ctx, `
		MERGE INTO core.education_personnel_etablissement AS tgt
		USING tmp_education_personnel_etablissement AS src
		     ON tgt.annee = src.annee AND tgt.degre = src.degre
		    AND tgt.identifiant_etablissement = src.identifiant_etablissement
		WHEN MATCHED AND (tgt.nom_etablissement, tgt.nature_etablissement, tgt.code_departement,
		                   tgt.code_academie, tgt.secteur, tgt.etp_total, tgt.etp_enseignants,
		                   tgt.etp_vie_scolaire, tgt.proportion_non_titulaires, tgt.proportion_agreges,
		                   tgt.proportion_certifies, tgt.source_id)
		     IS DISTINCT FROM (src.nom_etablissement, src.nature_etablissement, src.code_departement,
		                        src.code_academie, src.secteur, src.etp_total, src.etp_enseignants,
		                        src.etp_vie_scolaire, src.proportion_non_titulaires, src.proportion_agreges,
		                        src.proportion_certifies, src.source_id)
		THEN UPDATE SET nom_etablissement = src.nom_etablissement, nature_etablissement = src.nature_etablissement,
		     code_departement = src.code_departement, code_academie = src.code_academie, secteur = src.secteur,
		     etp_total = src.etp_total, etp_enseignants = src.etp_enseignants, etp_vie_scolaire = src.etp_vie_scolaire,
		     proportion_non_titulaires = src.proportion_non_titulaires, proportion_agreges = src.proportion_agreges,
		     proportion_certifies = src.proportion_certifies, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		     INSERT (annee, degre, identifiant_etablissement, nom_etablissement, nature_etablissement,
		             code_departement, code_academie, secteur, etp_total, etp_enseignants, etp_vie_scolaire,
		             proportion_non_titulaires, proportion_agreges, proportion_certifies, source_id)
		     VALUES (src.annee, src.degre, src.identifiant_etablissement, src.nom_etablissement,
		             src.nature_etablissement, src.code_departement, src.code_academie, src.secteur,
		             src.etp_total, src.etp_enseignants, src.etp_vie_scolaire, src.proportion_non_titulaires,
		             src.proportion_agreges, src.proportion_certifies, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion education_personnel_etablissement : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	touchees := ct.RowsAffected()
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total}, "")
	fmt.Printf("  personnels des établissements (Depp) : %d lignes reçues, %d touchées par la fusion\n", total, touchees)
	return nil
}

// asStr/asStrPtr/asFloatPtr : conversions tolérantes à l'hétérogénéité de
// type observée dans les exports Opendatasoft de l'Éducation nationale (même
// champ, tantôt nombre, tantôt chaîne — voir le commentaire sur jeuxDegre).
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

func normaliserSecteur(s string) (string, error) {
	switch s {
	case "Public":
		return "PUBLIC", nil
	case "Privé", "Privé sous contrat", "Privé hors contrat":
		return "PRIVE", nil
	default:
		return "", fmt.Errorf("secteur inconnu %q", s)
	}
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestEffectifsPersonnel(ctx, pool, arch); err != nil {
		return err
	}
	return IngestEffectifsEleves(ctx, pool, arch)
}
