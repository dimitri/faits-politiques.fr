package jorf

import (
	"context"
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MembreVersion identifie la RÈGLE de lecture des décrets, pas la donnée. Elle
// change quand la façon d'extraire change, de sorte qu'on puisse dire de quelle
// version d'analyse vient une ligne.
const MembreVersion = "composition-v1"

// NormalizeMembres relit les décrets de composition déjà scellés et en tire les
// membres du Gouvernement.
//
// Rien à télécharger : les actes sont dans core.acte_jo. Cette séparation est
// voulue — corriger la lecture d'une phrase ne doit pas obliger à retraverser un
// gigaoctet d'archive, et c'est le même choix que pour les exposés des motifs.
func NormalizeMembres(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT id, coalesce(date_texte, date_publi), contenu
		  FROM core.acte_jo
		 WHERE contenu IS NOT NULL
		   AND (titre_complet ~* 'composition du gouvernement|nomination du premier ministre'
		     OR titre ~* 'composition du gouvernement|nomination du premier ministre')
		 ORDER BY coalesce(date_texte, date_publi), id`)
	if err != nil {
		return err
	}
	type decret struct {
		id      string
		date    any
		contenu string
	}
	var decrets []decret
	for rows.Next() {
		var d decret
		if err := rows.Scan(&d.id, &d.date, &d.contenu); err != nil {
			rows.Close()
			return err
		}
		decrets = append(decrets, d)
	}
	rows.Close()
	if len(decrets) == 0 {
		return fmt.Errorf("aucun décret de composition en base : lancer -only=jorf-gouvernement d'abord")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Portée bornée à ce que ce connecteur produit.
	if _, err := tx.Exec(ctx, `DELETE FROM core.gouvernement_membre`); err != nil {
		return err
	}

	var nMembres, sansMembre int
	for _, d := range decrets {
		membres := LireComposition(d.contenu)
		if len(membres) == 0 {
			// Un décret dont on ne tire personne est un signal : soit le texte
			// est une simple formule de publication, soit la lecture a échoué.
			// Il est compté, pas passé sous silence.
			sansMembre++
			continue
		}
		for _, m := range membres {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.gouvernement_membre
				  (acte_id, date_effet, rang, sens, fonction, civilite, prenom, nom,
				   rattachement, portefeuille, method_version)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,''),nullif($10,''),$11)
				ON CONFLICT (acte_id, sens, fonction, nom, prenom, portefeuille) DO NOTHING`,
				d.id, d.date, m.Rang, m.Sens, m.Fonction, m.Civilite, m.Prenom, m.Nom,
				m.Rattachement, m.Portefeuille, MembreVersion); err != nil {
				return fmt.Errorf("%s, %s %s : %w", d.id, m.Prenom, m.Nom, err)
			}
			nMembres++
		}
	}

	// Le rapprochement avec les personnes déjà connues.
	//
	// Il se fait sur le nom ET le prénom, sans accents ni casse, et la particule
	// est retirée du patronyme : le Journal officiel écrit « de MONTCHALIN »,
	// notre base « Montchalin ». Le résultat n'est JAMAIS confirmé — un nom ne
	// prouve rien, et le décret ne porte aucune date de naissance pour trancher.
	// Un seul porteur donne CANDIDAT, plusieurs donnent AMBIGU.
	if _, err := tx.Exec(ctx, `
		WITH cible AS (
		  SELECT g.id,
		         core.f_unaccent(lower(regexp_replace(g.nom,
		           '^(d''|de |du |des |le |la |van |von )+', '', 'i'))) AS nom,
		         core.f_unaccent(lower(g.prenom)) AS prenom
		    FROM core.gouvernement_membre g
		), appariement AS (
		  SELECT c.id,
		         count(*) AS n,
		         min(p.id) AS person_id
		    FROM cible c
		    JOIN core.person p
		      ON core.f_unaccent(lower(p.family_name)) = c.nom
		     AND core.f_unaccent(lower(p.given_name))  = c.prenom
		   GROUP BY c.id
		)
		UPDATE core.gouvernement_membre g
		   SET person_id = CASE WHEN a.n = 1 THEN a.person_id END,
		       statut    = CASE WHEN a.n = 1 THEN 'CANDIDAT' ELSE 'AMBIGU' END,
		       homonymes = a.n
		  FROM appariement a
		 WHERE g.id = a.id`); err != nil {
		return fmt.Errorf("rapprochement : %w", err)
	}

	var confirmes, candidats, ambigus, absents int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE statut = 'CONFIRME'),
		       count(*) FILTER (WHERE statut = 'CANDIDAT'),
		       count(*) FILTER (WHERE statut = 'AMBIGU'),
		       count(*) FILTER (WHERE statut = 'ABSENT')
		  FROM core.gouvernement_membre`).Scan(&confirmes, &candidats, &ambigus, &absents); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("composition: %s read, %s of members (%d without an extracted member)",
		logs.Plural(len(decrets), "decree"), logs.Plural(nMembres, "mention"), sansMembre))
	logs.Notice(fmt.Sprintf("matching: %d candidate, %d ambiguous, %d not in the database",
		candidats, ambigus, absents))
	return nil
}
