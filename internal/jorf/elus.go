package jorf

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ElusVersion identifie la règle de reconnaissance, pas la donnée.
const ElusVersion = "thesaurus-elus-v1"

// NormalizeElus construit le thésaurus des élus et l'applique au corpus.
//
// Deux étapes, et la seconde est celle qui coûte :
//
//  1. le thésaurus : une ligne par personne ayant exercé un mandat NATIONAL,
//     avec sa requête de reconnaissance précalculée. La population est bornée
//     par une règle et non par une liste — ce sont les gens dont le Journal
//     officiel parle, et ils sont trois mille quatre cents, pas cinq cent mille ;
//  2. la reconnaissance : pour chaque nom, une recherche de PHRASE dans l'index
//     plein texte du corpus. C'est l'index qui travaille, pas un parcours.
func NormalizeElus(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM ref.elu_recherche`); err != nil {
		return err
	}

	// La population : quiconque a exercé un mandat national, à n'importe quelle
	// époque. Les 34 743 maires en fonction sont volontairement exclus — le
	// Journal officiel les nomme rarement, et les inclure multiplierait par dix
	// le coût de reconnaissance pour un gain proche de zéro.
	//
	// Un prénom ou un nom d'une seule lettre est écarté : « M. A. Dupont »
	// n'est pas reconnaissable, et l'admettre produirait des milliers de faux.
	res, err := tx.Exec(ctx, `
		INSERT INTO ref.elu_recherche (person_id, jeton, prenom, nom, requete, mandats)
		SELECT p.id,
		       'elu' || p.id,
		       p.given_name,
		       p.family_name,
		       phraseto_tsquery('fr', p.given_name || ' ' || p.family_name),
		       array_agg(DISTINCT m.mandate_type::text ORDER BY m.mandate_type::text)
		  FROM core.person p
		  JOIN core.mandate m ON m.person_id = p.id
		 WHERE m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN',
		                          'MINISTRE','PRESIDENT_REPUBLIQUE')
		   AND length(p.given_name) > 1 AND length(p.family_name) > 1
		   AND phraseto_tsquery('fr', p.given_name || ' ' || p.family_name) IS NOT NULL
		   AND numnode(phraseto_tsquery('fr', p.given_name || ' ' || p.family_name)) >= 2
		 GROUP BY p.id`)
	if err != nil {
		return fmt.Errorf("thésaurus : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("  thésaurus : %d élus reconnaissables\n", res.RowsAffected())

	// La reconnaissance se fait HORS transaction longue : elle dure, et la tenir
	// dans une transaction bloquerait le nettoyage des tables du corpus.
	if _, err := pool.Exec(ctx, `DELETE FROM jo.acte_elu`); err != nil {
		return err
	}

	// Une seule requête : pour chaque nom du thésaurus, les actes dont un bloc
	// contient la phrase. L'index GIN du corpus fait le travail.
	//
	// `homonymes` est compté sur la base entière, pas sur le thésaurus : ce qui
	// rend une citation ambiguë, c'est qu'une AUTRE personne porte le même nom,
	// qu'elle soit élue ou non.
	r2, err := pool.Exec(ctx, `
		INSERT INTO jo.acte_elu (texte_id, person_id, sections, homonymes, method_version)
		SELECT b.texte_id, e.person_id,
		       array_agg(DISTINCT b.section ORDER BY b.section),
		       (SELECT count(*) FROM core.person h
		         WHERE core.f_unaccent(lower(h.family_name)) = core.f_unaccent(lower(e.nom))
		           AND core.f_unaccent(lower(h.given_name))  = core.f_unaccent(lower(e.prenom)))::smallint,
		       $1
		  FROM ref.elu_recherche e
		  -- Sur la VUE MATÉRIALISÉE, où le vecteur est stocké — pas sur une
		  -- expression. Une recherche de phrase doit vérifier l'adjacence des
		  -- lexèmes sur chaque candidat, et un index fonctionnel l'oblige à
		  -- recalculer le vecteur : 1 807 ms par nom contre 30 ms, soit plus
		  -- d'une heure et demie pour les trois mille quatre cents noms du
		  -- thésaurus au lieu de six minutes.
		  JOIN jo.recherche_bloc b ON b.recherche @@ e.requete
		 GROUP BY b.texte_id, e.person_id, e.nom, e.prenom
		ON CONFLICT (texte_id, person_id) DO NOTHING`, ElusVersion)
	if err != nil {
		return fmt.Errorf("reconnaissance : %w", err)
	}

	if _, err := pool.Exec(ctx,
		`REFRESH MATERIALIZED VIEW jo.recherche_elu`); err != nil {
		return err
	}

	var actes, elus, ambigus int
	if err := pool.QueryRow(ctx, `
		SELECT count(DISTINCT texte_id), count(DISTINCT person_id),
		       count(*) FILTER (WHERE homonymes > 1)
		  FROM jo.acte_elu`).Scan(&actes, &elus, &ambigus); err != nil {
		return err
	}
	fmt.Printf("  reconnaissance : %d citations, %d actes, %d élus cités, %d citations ambiguës\n",
		r2.RowsAffected(), actes, elus, ambigus)
	return nil
}
