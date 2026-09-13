package senat

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Fusionner réunit les sénateurs et les personnes déjà connues d'une autre
// source.
//
// Le répertoire du Sénat ne publie aucun identifiant partagé : son matricule
// n'existe ni à l'Assemblée, ni au RNE, ni à la HATVP. Le connecteur créait
// donc une personne par matricule, et 982 d'entre elles doublonnaient un élu
// déjà en base — Patrick Abate y figurait deux fois, une fois comme sénateur
// avec 1 693 votes, une fois comme conseiller municipal avec son mandat local.
// Deux fiches pour un homme, et aucune des deux complète.
//
// Faute d'identifiant, le rapprochement se fait sur le NOM ET LA DATE DE
// NAISSANCE EXACTE, et seulement quand l'appariement est unique des deux côtés.
// C'est un rapprochement par nom, que ce projet évite partout ailleurs ; il est
// admis ici parce que la population est fermée (1 948 sénateurs depuis 1958),
// parce que la date de naissance au jour près écarte les homonymes, et parce
// que l'ambiguïté est mesurée plutôt que supposée : elle vaut zéro, et la
// fusion s'arrête si elle cesse de valoir zéro.
func Fusionner(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Les candidats : un sénateur (porteur d'un matricule) et une personne qui
	// n'en porte pas, de même nom et même date de naissance.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE fusion ON COMMIT DROP AS
		WITH paires AS (
		  SELECT s.id AS sen_id, o.id AS canon_id
		    FROM core.person s
		    JOIN core.person o
		      ON core.f_unaccent(lower(s.family_name)) = core.f_unaccent(lower(o.family_name))
		     AND core.f_unaccent(lower(s.given_name))  = core.f_unaccent(lower(o.given_name))
		     AND s.birth_date = o.birth_date
		     AND s.id <> o.id
		   WHERE s.birth_date IS NOT NULL
		     AND EXISTS (SELECT 1 FROM core.person_identifier i
		                  WHERE i.person_id = s.id AND i.scheme = 'SENAT_MATRICULE')
		     AND NOT EXISTS (SELECT 1 FROM core.person_identifier i
		                      WHERE i.person_id = o.id AND i.scheme = 'SENAT_MATRICULE')
		)
		SELECT sen_id, canon_id FROM paires p
		 WHERE (SELECT count(*) FROM paires q WHERE q.sen_id   = p.sen_id)   = 1
		   AND (SELECT count(*) FROM paires q WHERE q.canon_id = p.canon_id) = 1`); err != nil {
		return fmt.Errorf("appariement : %w", err)
	}

	var aFusionner, ambigus int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM fusion`).Scan(&aFusionner); err != nil {
		return err
	}
	// Les appariements écartés parce que multiples : ils sont comptés et
	// signalés, jamais devinés.
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM core.person s
		  JOIN core.person o
		    ON core.f_unaccent(lower(s.family_name)) = core.f_unaccent(lower(o.family_name))
		   AND core.f_unaccent(lower(s.given_name))  = core.f_unaccent(lower(o.given_name))
		   AND s.birth_date = o.birth_date AND s.id <> o.id
		 WHERE s.birth_date IS NOT NULL
		   AND EXISTS (SELECT 1 FROM core.person_identifier i
		                WHERE i.person_id = s.id AND i.scheme = 'SENAT_MATRICULE')
		   AND NOT EXISTS (SELECT 1 FROM core.person_identifier i
		                    WHERE i.person_id = o.id AND i.scheme = 'SENAT_MATRICULE')
		   AND NOT EXISTS (SELECT 1 FROM fusion f WHERE f.sen_id = s.id)`).Scan(&ambigus); err != nil {
		return err
	}

	if aFusionner == 0 {
		fmt.Printf("  fusion sénateurs : rien à fusionner (%d appariements ambigus écartés)\n", ambigus)
		return tx.Commit(ctx)
	}

	// La fiche de référence hérite de ce que le Sénat sait et qu'elle ignore.
	if _, err := tx.Exec(ctx, `
		UPDATE core.person c
		   SET death_date = coalesce(c.death_date, s.death_date),
		       profession = coalesce(c.profession, s.profession)
		  FROM fusion f JOIN core.person s ON s.id = f.sen_id
		 WHERE c.id = f.canon_id`); err != nil {
		return fmt.Errorf("report des attributs : %w", err)
	}

	// Les collisions, avant de déplacer quoi que ce soit.
	//
	// Un mandat ou une affiliation présents des deux côtés violeraient la
	// contrainte d'exclusion temporelle. La règle est de garder la période la
	// PLUS LARGE : le dump Dosleg couvre le mandat entier, le RNE n'en connaît
	// que la portion courante. Trois mandats sénatoriaux sont dans ce cas.
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.mandate m
		 USING fusion f, core.mandate c
		 WHERE m.person_id = f.sen_id AND c.person_id = f.canon_id
		   AND c.mandate_type = m.mandate_type AND c.validity && m.validity
		   AND NOT (m.validity @> c.validity AND m.validity <> c.validity)`); err != nil {
		return fmt.Errorf("mandats en double (côté sénat) : %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.mandate c
		 USING fusion f, core.mandate m
		 WHERE c.person_id = f.canon_id AND m.person_id = f.sen_id
		   AND c.mandate_type = m.mandate_type AND c.validity && m.validity
		   AND m.validity @> c.validity AND m.validity <> c.validity`); err != nil {
		return fmt.Errorf("mandats en double (côté canonique) : %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.affiliation a
		 USING fusion f, core.affiliation c
		 WHERE a.person_id = f.sen_id AND c.person_id = f.canon_id
		   AND a.organization_kind = 'PARLIAMENTARY_GROUP'
		   AND c.organization_kind = 'PARLIAMENTARY_GROUP'
		   AND c.validity && a.validity`); err != nil {
		return fmt.Errorf("affiliations chevauchantes : %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.ballot b USING fusion f, core.ballot c
		 WHERE b.person_id = f.sen_id AND c.person_id = f.canon_id
		   AND c.scrutin_id = b.scrutin_id`); err != nil {
		return fmt.Errorf("votes en double : %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.ballot_group_exception e USING fusion f, core.ballot_group_exception c
		 WHERE e.person_id = f.sen_id AND c.person_id = f.canon_id
		   AND c.scrutin_id = e.scrutin_id AND c.organization_id = e.organization_id`); err != nil {
		return fmt.Errorf("exceptions de groupe en double : %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.media m USING fusion f, core.media c
		 WHERE m.person_id = f.sen_id AND c.person_id = f.canon_id AND c.kind = m.kind`); err != nil {
		return fmt.Errorf("médias en double : %w", err)
	}

	// Le déplacement lui-même. Toute table qui référence core.person y figure :
	// une table oubliée serait soit effacée en cascade, soit orpheline, et dans
	// les deux cas silencieusement. La liste est confrontée au catalogue plus
	// bas, ce qui fait échouer la fusion si le schéma gagne une table.
	tables := []string{
		"core.acte_jo_mention", "core.affiliation", "core.amendement_author",
		"core.ballot", "core.ballot_group_exception", "core.compte_campagne",
		"core.declaration", "core.deport", "core.dossier_author",
		"core.intervention", "core.mandate", "core.media",
		"core.person_identifier", "core.texte_author",
	}
	var manquantes []string
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(t), '{}')
		  FROM (SELECT DISTINCT c.conrelid::regclass::text AS t
		          FROM pg_constraint c
		         WHERE c.contype = 'f' AND c.confrelid = 'core.person'::regclass) s
		 WHERE t <> ALL($1::text[])`, tables).Scan(&manquantes); err != nil {
		return fmt.Errorf("inventaire des références : %w", err)
	}
	// core.gouvernement référence la personne par une colonne nommée autrement.
	for _, t := range manquantes {
		if t == "core.gouvernement" {
			continue
		}
		return fmt.Errorf("table référençant core.person non traitée par la fusion : %s", t)
	}

	for _, t := range tables {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s x SET person_id = f.canon_id FROM fusion f WHERE x.person_id = f.sen_id`, t)); err != nil {
			return fmt.Errorf("déplacement %s : %w", t, err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.gouvernement g SET premier_ministre_person_id = f.canon_id
		  FROM fusion f WHERE g.premier_ministre_person_id = f.sen_id`); err != nil {
		return fmt.Errorf("déplacement core.gouvernement : %w", err)
	}

	res, err := tx.Exec(ctx, `DELETE FROM core.person p USING fusion f WHERE p.id = f.sen_id`)
	if err != nil {
		return fmt.Errorf("suppression des doublons : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("  fusion sénateurs : %d fiches réunies, %d supprimées, %d appariements ambigus écartés\n",
		aFusionner, res.RowsAffected(), ambigus)
	return nil
}
