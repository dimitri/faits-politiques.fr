package senat

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les mandats de sénateurs, que le dump Dosleg porte depuis 1936 et que nous
// n'exploitions pas.
//
// C'était le seul niveau sans historique : ODSEN_GENERAL ne publie les membres
// qu'au présent, et le RNE ne couvre que la mandature en cours. senat_raw.auteur
// porte datdeb et datfin par MATRICULE — appariement par identifiant, donc
// aucun risque d'homonymie.
//
// 2 496 lignes sur 5 873 portent une date de début. Les autres n'en reçoivent
// aucune : un mandat sans date n'est pas un mandat.
func NormalizeMandats(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Portée bornée à ce que ce connecteur produit : les mandats sénatoriaux
	// rattachés à une personne portant un matricule du Sénat.
	if _, err := tx.Exec(ctx, `
		DELETE FROM core.mandate m
		 WHERE m.mandate_type = 'SENATEUR' AND m.institution = 'SENAT'
		   AND EXISTS (SELECT 1 FROM core.person_identifier i
		               WHERE i.person_id = m.person_id AND i.scheme = 'SENAT_MATRICULE')`); err != nil {
		return err
	}

	// La table des auteurs répète un sénateur UNE FOIS PAR QUALITÉ exercée —
	// membre d'une commission, rapporteur, président de groupe — avec des
	// périodes qui se chevauchent largement. Les insérer telles quelles faisait
	// rejeter la quasi-totalité par la contrainte d'exclusion : 123 mandats
	// retenus sur 2 496 lignes, sans que rien ne le signale.
	//
	// Les périodes sont donc FUSIONNÉES par sénateur : un mandat est l'union
	// des périodes continues, et une interruption réelle en ouvre un nouveau.
	// C'est la lecture juste — un sénateur réélu sans discontinuer a un mandat
	// continu, pas douze mandats superposés.
	res, err := tx.Exec(ctx, `
		WITH periodes AS (
		  SELECT i.person_id,
		         a.datdeb::date AS debut,
		         coalesce(a.datfin::date, CURRENT_DATE) AS fin
		    FROM senat_raw.auteur a
		    JOIN core.person_identifier i
		      ON i.scheme = 'SENAT_MATRICULE' AND i.value = trim(a.autmat)
		   WHERE a.datdeb IS NOT NULL
		     AND (a.datfin IS NULL OR a.datfin >= a.datdeb)
		), bornes AS (
		  SELECT person_id, debut, fin,
		         -- Une nouvelle période commence quand elle débute après la fin
		         -- maximale de tout ce qui précède, à plus d'un jour d'écart.
		         CASE WHEN debut > max(fin) OVER (
		                PARTITION BY person_id ORDER BY debut
		                ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING) + 1
		              THEN 1 ELSE 0 END AS rupture
		    FROM periodes
		), groupes AS (
		  SELECT person_id, debut, fin,
		         sum(rupture) OVER (PARTITION BY person_id ORDER BY debut) AS bloc
		    FROM bornes
		)
		INSERT INTO core.mandate (person_id, mandate_type, institution, validity)
		SELECT person_id, 'SENATEUR', 'SENAT',
		       daterange(min(debut), max(fin), '[]')
		  FROM groupes GROUP BY person_id, bloc
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return fmt.Errorf("mandats sénatoriaux : %w", err)
	}

	var depuis, jusqua string
	_ = tx.QueryRow(ctx, `
		SELECT coalesce(min(lower(validity))::text,''), coalesce(max(coalesce(upper(validity),CURRENT_DATE))::text,'')
		  FROM core.mandate WHERE mandate_type='SENATEUR' AND institution='SENAT'`).Scan(&depuis, &jusqua)

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("  mandats de sénateurs %d, de %s à %s\n", res.RowsAffected(), depuis, jusqua)
	return nil
}
