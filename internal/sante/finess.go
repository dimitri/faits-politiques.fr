// Package sante charge les données ouvertes du système de santé français.
// FINESS est la clé pivot (docs/sante-donnees.md) : chargé en premier, sans
// aucune donnée d'activité ni de finances, pour que les sources secondaires
// (SAE, PMSI, SNDS/Damir, RPPS, HAS) s'y joignent plus tard sans dupliquer
// le référentiel des établissements.
package sante

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "sante-v1"

// Le jeu de données FINESS lui-même a été signalé comme remplacé par de
// nouveaux flux ANS (juillet 2026) au moment de l'écriture, mais son export
// CSV continue d'être republié régulièrement (12 mai 2026 au moment du
// chargement) — utilisé ici, avec cette réserve documentée plutôt que
// silencieuse. Voir docs/sante-donnees.md § 1.
var SourceFiness = archive.Source{
	Slug: "finess-etablissements", Label: "FINESS — référentiel des établissements sanitaires et sociaux",
	Publisher: "Agence du numérique en santé (ANS), via data.gouv.fr",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : ANS, FINESS (data.gouv.fr)",
	Cadence:     "annoncée mensuelle ; en pratique irrégulière",
	Notes: "Le jeu data.gouv.fr historique (etalab_cs1100502) est signalé « remplacé » par de " +
		"nouveaux flux quotidiens de l'ANS depuis le 20 juillet 2026, mais reste publié et à jour " +
		"au moment du chargement (édition du 12 mai 2026) — utilisé faute d'avoir confirmé et " +
		"documenté l'URL stable du nouveau flux ANS. Fichier plat sans en-tête, une ligne de " +
		"commentaire, puis des lignes préfixées « structureet », séparateur point-virgule, ordre " +
		"des champs fixé par la spécification etalab_cs1100502 (32 positions).",
}

const finessURL = "https://static.data.gouv.fr/resources/finess-extraction-du-fichier-des-etablissements/" +
	"20260512-091308/etalab-cs1100502-stock-20260512-0339.csv"

// Ordre des champs de la ligne « structureet », fixé par la spécification
// etalab_cs1100502 — voir le PDF de description du jeu de données. Un
// changement de format en amont romprait ce mappage positionnel ; c'est le
// risque assumé d'un fichier plat sans en-tête.
const (
	fNofinesset  = 1
	fNofinessej  = 2
	fRs          = 3
	fRsLongue    = 4
	fCommune     = 12
	fDept        = 13
	fLibDept     = 14
	fLigneAch    = 15
	fCategetab   = 18
	fLibCateg    = 19
	fCategagr    = 20
	fLibCategagr = 21
	fSiret       = 22
	fCodeMft     = 24
	fLibMft      = 25
	fCodeSph     = 26
	fLibSph      = 27
	fDateOuv     = 28
	fDateMaj     = 30
	nbChamps     = 32
)

func IngestFiness(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceFiness)
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

	f, err := arch.Fetch(ctx, srcID, runID, finessURL, ".csv")
	if err != nil {
		return fail(err)
	}

	fichier, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer fichier.Close()

	scanner := bufio.NewScanner(fichier)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ligneNum := 0
	var rows [][]any
	var ignorees int
	for scanner.Scan() {
		ligneNum++
		if ligneNum == 1 {
			continue // ligne de commentaire ("finess;etalab;111;date")
		}
		champs := strings.Split(scanner.Text(), ";")
		if len(champs) < nbChamps || champs[0] != "structureet" {
			ignorees++
			continue
		}
		codeDept := champs[fDept]
		codeCommune := champs[fCommune]
		var codeInsee *string
		if codeDept != "" && codeCommune != "" {
			s := codeDept + codeCommune
			codeInsee = &s
		}
		rows = append(rows, []any{
			champs[fNofinesset], champs[fNofinessej], champs[fRs], nilSiVide(champs[fRsLongue]),
			nilSiVide(codeCommune), codeInsee, nilSiVide(codeDept), nilSiVide(champs[fLibDept]),
			nilSiVide(champs[fLigneAch]),
			champs[fCategetab], nilSiVide(champs[fLibCateg]),
			champs[fCategagr], nilSiVide(champs[fLibCategagr]),
			nilSiVide(champs[fSiret]), nilSiVide(champs[fCodeMft]), nilSiVide(champs[fLibMft]),
			nilSiVide(champs[fCodeSph]), nilSiVide(champs[fLibSph]),
			dateOuNil(champs[fDateOuv]), dateOuNil(champs[fDateMaj]),
			srcID,
		})
	}
	if err := scanner.Err(); err != nil {
		return fail(err)
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("FINESS : aucune ligne « structureet » lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_finess_etablissement (
			nofinesset text, nofinessej text, raison_sociale text, raison_sociale_longue text,
			code_commune text, code_insee text, code_departement text, libelle_departement text,
			ligne_acheminement text, categorie_code text, categorie_libelle text, categorie_agregat_code text,
			categorie_agregat_libelle text, siret text, code_mft text, libelle_mft text, code_sph text,
			libelle_sph text, date_ouverture date, date_maj date, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_finess_etablissement"},
		[]string{"nofinesset", "nofinessej", "raison_sociale", "raison_sociale_longue",
			"code_commune", "code_insee", "code_departement", "libelle_departement", "ligne_acheminement",
			"categorie_code", "categorie_libelle", "categorie_agregat_code", "categorie_agregat_libelle",
			"siret", "code_mft", "libelle_mft", "code_sph", "libelle_sph",
			"date_ouverture", "date_maj", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("finess_etablissement : %w", err))
	}
	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité des 100 000+ établissements à chaque republication,
	// changement ou non.
	var n int64
	err = bulkload.SansContraintesFK(ctx, tx, "ref.finess_etablissement", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO ref.finess_etablissement AS tgt
			USING tmp_finess_etablissement AS src
			ON tgt.nofinesset = src.nofinesset
			WHEN MATCHED AND (tgt.nofinessej, tgt.raison_sociale, tgt.raison_sociale_longue, tgt.code_commune,
			                   tgt.code_insee, tgt.code_departement, tgt.libelle_departement, tgt.ligne_acheminement,
			                   tgt.categorie_code, tgt.categorie_libelle, tgt.categorie_agregat_code,
			                   tgt.categorie_agregat_libelle, tgt.siret, tgt.code_mft, tgt.libelle_mft,
			                   tgt.code_sph, tgt.libelle_sph, tgt.date_ouverture, tgt.date_maj, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nofinessej, src.raison_sociale, src.raison_sociale_longue, src.code_commune,
			                   src.code_insee, src.code_departement, src.libelle_departement, src.ligne_acheminement,
			                   src.categorie_code, src.categorie_libelle, src.categorie_agregat_code,
			                   src.categorie_agregat_libelle, src.siret, src.code_mft, src.libelle_mft,
			                   src.code_sph, src.libelle_sph, src.date_ouverture, src.date_maj, src.source_id) THEN
			    UPDATE SET nofinessej = src.nofinessej, raison_sociale = src.raison_sociale,
			               raison_sociale_longue = src.raison_sociale_longue, code_commune = src.code_commune,
			               code_insee = src.code_insee, code_departement = src.code_departement,
			               libelle_departement = src.libelle_departement, ligne_acheminement = src.ligne_acheminement,
			               categorie_code = src.categorie_code, categorie_libelle = src.categorie_libelle,
			               categorie_agregat_code = src.categorie_agregat_code,
			               categorie_agregat_libelle = src.categorie_agregat_libelle, siret = src.siret,
			               code_mft = src.code_mft, libelle_mft = src.libelle_mft, code_sph = src.code_sph,
			               libelle_sph = src.libelle_sph, date_ouverture = src.date_ouverture,
			               date_maj = src.date_maj, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (nofinesset, nofinessej, raison_sociale, raison_sociale_longue, code_commune, code_insee,
			            code_departement, libelle_departement, ligne_acheminement, categorie_code,
			            categorie_libelle, categorie_agregat_code, categorie_agregat_libelle, siret, code_mft,
			            libelle_mft, code_sph, libelle_sph, date_ouverture, date_maj, source_id)
			    VALUES (src.nofinesset, src.nofinessej, src.raison_sociale, src.raison_sociale_longue,
			            src.code_commune, src.code_insee, src.code_departement, src.libelle_departement,
			            src.ligne_acheminement, src.categorie_code, src.categorie_libelle,
			            src.categorie_agregat_code, src.categorie_agregat_libelle, src.siret, src.code_mft,
			            src.libelle_mft, src.code_sph, src.libelle_sph, src.date_ouverture, src.date_maj,
			            src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		n = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion finess_etablissement : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n, "ignorees": ignorees}, "")
	fmt.Printf("  FINESS établissements : %d lignes (%d lignes non « structureet » ignorées)\n", n, ignorees)
	return nil
}

func nilSiVide(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// dateOuNil : le fichier publie 1900-01-01 par défaut pour une date absente
// (la spécification le dit explicitement) — traité comme une vraie absence,
// pas comme une date réelle.
func dateOuNil(s string) *time.Time {
	if s == "" || s == "1900-01-01" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestFiness(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestSecteurConventionnel(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestCertificationHAS(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestPMSIMCO(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestPMSISMRHAD(ctx, pool, arch); err != nil {
		return err
	}
	return IngestHonoraires(ctx, pool, arch)
}
