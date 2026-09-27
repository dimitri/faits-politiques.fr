package communes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La nuance politique n'est publiée nulle part ailleurs (D-022) : ni le RNE ni
// aucun fichier d'élus ne la porte. Elle n'existe que dans ces fichiers de
// résultats, attachée à une LISTE candidate.
var SourceMunicipales = archive.Source{
	Slug: "municipales-2026", Label: "Élections municipales 2026 — résultats",
	Publisher: "Ministère de l'Intérieur", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : ministère de l'Intérieur, résultats des élections municipales des 15 et 22 mars 2026",
	Cadence:     "par scrutin",
	Notes: "La nuance qualifie une liste, pas une personne, et n'est attribuée " +
		"qu'au-dessus d'un seuil de population : 9,4 % des communes seulement. " +
		"Les fichiers 2020 sont publiés sans licence explicite et ne sont pas ingérés.",
}

const (
	MunicipalesAnnee     = 2026
	CirculaireMillesime  = 2026
	municipalesT1URL     = "https://static.data.gouv.fr/resources/elections-municipales-2026-resultats-du-premier-tour/20260320-164339/municipales-2026-resultats-communes-2026-03-20.csv"
	municipalesT2URL     = "https://static.data.gouv.fr/resources/elections-municipales-2026-resultats-du-scond-tour/20260323-180124/municipales-2026-resultats-communes-2026-03-23-16h14.csv"
	maxListesParCommune  = 13
	CouleurMethodVersion = "couleur-v1-sieges-cm"
)

func IngestMunicipales(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMunicipales)
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

	tours := map[int]string{1: municipalesT1URL, 2: municipalesT2URL}
	type ligne struct {
		tour                     int
		commune, nuance, libelle string
		panneau                  int
		nom, prenom              string
		voix, cm, cc             *int
	}
	var lignes []ligne
	inconnues := map[string]bool{}

	for tour, url := range tours {
		f, err := arch.Fetch(ctx, srcID, runID, url, ".csv")
		if err != nil {
			return fail(err)
		}
		recs, err := lireCSV(f.Path, ';')
		if err != nil {
			return fail(err)
		}
		for _, r := range recs {
			commune := r["Code commune"]
			if commune == "" {
				continue
			}
			for i := 1; i <= maxListesParCommune; i++ {
				suf := " " + strconv.Itoa(i)
				lib := r["Libellé abrégé de liste"+suf]
				if lib == "" {
					lib = r["Libellé de liste"+suf]
				}
				if lib == "" {
					continue
				}
				pan, err := strconv.Atoi(r["Numéro de panneau"+suf])
				if err != nil {
					// Sans numéro de panneau, la ligne n'a pas de clé stable :
					// on prend le rang du bloc, qui est l'ordre du fichier.
					pan = i
				}
				lignes = append(lignes, ligne{
					tour: tour, commune: commune, panneau: pan,
					nuance:  r["Nuance liste"+suf],
					libelle: lib,
					nom:     r["Nom candidat"+suf], prenom: r["Prénom candidat"+suf],
					voix: entier(r["Voix"+suf]),
					cm:   entier(r["Sièges au CM"+suf]),
					cc:   entier(r["Sièges au CC"+suf]),
				})
			}
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Les communes absentes du COG ne sont pas insérées de force : une ligne de
	// résultat dont la commune n'existe pas dans le référentiel est un signal,
	// pas un détail à faire passer.
	rows, err := tx.Query(ctx,
		`SELECT code_insee FROM ref.commune WHERE cog_millesime = $1`, COGMillesime)
	if err != nil {
		return fail(err)
	}
	connues := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return fail(err)
		}
		connues[c] = true
	}
	rows.Close()

	var copyRows [][]any
	for _, l := range lignes {
		if !connues[l.commune] {
			inconnues[l.commune] = true
			continue
		}
		var nuance, mil any
		if l.nuance != "" {
			nuance, mil = l.nuance, CirculaireMillesime
		}
		copyRows = append(copyRows, []any{
			MunicipalesAnnee, l.tour, l.commune, COGMillesime, l.panneau,
			nuance, mil, l.libelle, nul(l.nom), nul(l.prenom),
			nulInt(l.voix), nulInt(l.cm), nulInt(l.cc), srcID,
		})
	}

	// MERGE plutôt que DELETE+COPY, scopé au scrutin par une vue temporaire :
	// core.municipal_list est partagée avec municipales2020.go (autre année),
	// et l'ancien DELETE payait le prix des triggers RI pour l'intégralité
	// des listes 2026 à chaque republication, changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_municipal_list (
			scrutin_annee int, tour smallint, commune_code text, cog_millesime int, panneau int,
			nuance_code text, circulaire_millesime int, libelle text, nom_candidat text,
			prenom_candidat text, voix int, sieges_cm int, sieges_cc int, source_id bigint
		) ON COMMIT DROP;
		CREATE OR REPLACE TEMPORARY VIEW municipal_list_2026 AS
		  SELECT * FROM core.municipal_list WHERE scrutin_annee = 2026
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	cols := []string{"scrutin_annee", "tour", "commune_code", "cog_millesime", "panneau",
		"nuance_code", "circulaire_millesime", "libelle", "nom_candidat", "prenom_candidat",
		"voix", "sieges_cm", "sieges_cc", "source_id"}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_municipal_list"}, cols,
		pgx.CopyFromRows(copyRows)); err != nil {
		return fail(fmt.Errorf("copie des listes : %w", err))
	}
	var n int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.municipal_list", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO municipal_list_2026 AS tgt
			USING tmp_municipal_list AS src
			ON tgt.scrutin_annee = src.scrutin_annee AND tgt.tour = src.tour
			   AND tgt.commune_code = src.commune_code AND tgt.panneau = src.panneau
			WHEN MATCHED AND (tgt.cog_millesime, tgt.nuance_code, tgt.circulaire_millesime,
			                   tgt.libelle, tgt.nom_candidat, tgt.prenom_candidat,
			                   tgt.voix, tgt.sieges_cm, tgt.sieges_cc, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.cog_millesime, src.nuance_code, src.circulaire_millesime,
			                   src.libelle, src.nom_candidat, src.prenom_candidat,
			                   src.voix, src.sieges_cm, src.sieges_cc, src.source_id) THEN
			    UPDATE SET cog_millesime = src.cog_millesime, nuance_code = src.nuance_code,
			               circulaire_millesime = src.circulaire_millesime, libelle = src.libelle,
			               nom_candidat = src.nom_candidat, prenom_candidat = src.prenom_candidat,
			               voix = src.voix, sieges_cm = src.sieges_cm, sieges_cc = src.sieges_cc,
			               source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (scrutin_annee, tour, commune_code, cog_millesime, panneau, nuance_code,
			            circulaire_millesime, libelle, nom_candidat, prenom_candidat,
			            voix, sieges_cm, sieges_cc, source_id)
			    VALUES (src.scrutin_annee, src.tour, src.commune_code, src.cog_millesime, src.panneau,
			            src.nuance_code, src.circulaire_millesime, src.libelle, src.nom_candidat,
			            src.prenom_candidat, src.voix, src.sieges_cm, src.sieges_cc, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		n = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion des listes : %w", err))
	}

	// Sans statistiques fraîches, le planificateur travaille à l'aveugle sur
	// une table qui vient d'être modifiée en masse.
	if _, err := tx.Exec(ctx, `ANALYZE core.municipal_list`); err != nil {
		return fail(err)
	}

	if err := couleurs(ctx, tx, MunicipalesAnnee); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"listes": len(copyRows), "listes_touchees": n, "communes_hors_cog": len(inconnues)}, "")
	fmt.Printf("  municipales %d : %d listes (%d touchées par la fusion)\n", MunicipalesAnnee, len(copyRows), n)
	if len(inconnues) > 0 {
		fmt.Printf("  %d communes des résultats absentes du COG %d (ignorées)\n",
			len(inconnues), COGMillesime)
	}
	return nil
}

// couleurs applique la règle de déduction, et marque explicitement les cas où
// elle ne tranche pas plutôt que de les laisser vides.
//
//	« la couleur d'une commune est la nuance de la liste ayant obtenu le plus
//	  de sièges au conseil municipal, au tour où le conseil a été pourvu »
func couleurs(ctx context.Context, tx pgx.Tx, annee int) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM derived.commune_couleur WHERE scrutin_annee = $1`, annee); err != nil {
		return err
	}
	// Le tour retenu est le dernier où des sièges ont été attribués : un conseil
	// pourvu au premier tour n'a pas de second tour, et quand il y en a un,
	// c'est lui qui fait le conseil.
	//
	// Tout se joue en fonctions de fenêtre plutôt qu'en auto-jointures. La
	// première version comparait la table à elle-même pour détecter les ex
	// aequo ; sur une table fraîchement remplie par COPY, sans statistiques, le
	// planificateur choisissait une boucle imbriquée et la requête tournait
	// toujours au bout de trois minutes. Une passe de fenêtrage ne peut pas
	// dégénérer ainsi.
	_, err := tx.Exec(ctx, `
		WITH pourvu AS (
		  SELECT commune_code, max(tour) AS tour
		    FROM core.municipal_list
		   WHERE scrutin_annee = $1 AND coalesce(sieges_cm, 0) > 0
		   GROUP BY commune_code
		), listes AS (
		  SELECT m.id, m.commune_code, m.tour, m.nuance_code, m.circulaire_millesime,
		         m.panneau, coalesce(m.sieges_cm, 0) AS sieges, coalesce(m.voix, 0) AS voix
		    FROM core.municipal_list m
		    JOIN pourvu p ON p.commune_code = m.commune_code AND p.tour = m.tour
		   WHERE m.scrutin_annee = $1
		), classe AS (
		  SELECT l.*,
		         row_number() OVER (PARTITION BY commune_code
		                            ORDER BY sieges DESC, voix DESC, panneau) AS rang,
		         -- Combien de listes de cette commune ont exactement ce nombre
		         -- de sièges : lu sur la ligne de tête, c'est le test d'ex aequo.
		         count(*) OVER (PARTITION BY commune_code, sieges) AS memes_sieges
		    FROM listes l
		)
		INSERT INTO derived.commune_couleur
		  (commune_code, scrutin_annee, nuance_code, circulaire_millesime,
		   tour, municipal_list_id, sieges_cm, statut, method_version)
		SELECT c.commune_code, $1,
		       CASE WHEN c.memes_sieges = 1 THEN c.nuance_code END,
		       CASE WHEN c.memes_sieges = 1 THEN c.circulaire_millesime END,
		       c.tour, c.id, c.sieges,
		       CASE WHEN c.memes_sieges > 1     THEN 'EX_AEQUO'
		            WHEN c.nuance_code IS NULL  THEN 'SANS_NUANCE'
		            ELSE 'NUANCEE' END,
		       $2
		  FROM classe c WHERE c.rang = 1`,
		annee, CouleurMethodVersion)
	return err
}

func entier(s string) *int {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

func nul(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nulInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
