// Package bulkload porte une seule technique : retirer les contraintes de
// clé étrangère d'une table le temps d'un gros chargement, pour les
// réinstaller ensuite — voir SansContraintesFK.
package bulkload

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SansContraintesFK exécute fn après avoir retiré, puis réinstallé, toutes
// les contraintes de clé étrangère de table — DANS tx, la même transaction
// que fn utilise pour son chargement.
//
// La raison n'est pas un index manquant (voir internal/europe,
// chargerVotesNominatifs : EXPLAIN ANALYZE y a mesuré ~120s de trigger RI,
// sur trois FK dont les tables parentes sont déjà correctement indexées,
// pour charger 1,97M lignes dans core.ballot) — c'est le déclenchement du
// trigger lui-même, une fois PAR LIGNE insérée. Retirer la contrainte
// pendant le chargement puis la réinstaller la revalide en UN scan
// ensembliste, la même donnée vérifiée une fois plutôt que N.
//
// Le DROP CONSTRAINT prend un verrou ACCESS EXCLUSIVE sur table, tenu
// jusqu'au COMMIT de tx : un AUTRE connecteur qui écrirait sur la même
// table pendant ce temps (core.ballot : internal/an, internal/senat,
// internal/europe écrivent tous les trois dedans, et senat/europe tournent
// dans la même vague de fpctl ingest — voir Source.Dependances dans
// internal/ingest/catalogue.go) attend simplement la fin de cette
// transaction avant de commencer la sienne. Aucune coordination applicative
// n'est nécessaire : c'est le verrou Postgres normal d'un ALTER TABLE qui
// sérialise les deux, pas un choix de ce paquet. Le prix est la
// parallélisation perdue, entre connecteurs qui partagent une table, le
// temps de CE chargement précis — un compromis correct au vu du gain
// mesuré, et limité à la durée du chargement, pas à celle du connecteur
// entier.
func SansContraintesFK(ctx context.Context, tx pgx.Tx, table string, fn func() error) error {
	rows, err := tx.Query(ctx, `
		SELECT conname, pg_get_constraintdef(oid)
		  FROM pg_constraint
		 WHERE conrelid = $1::regclass AND contype = 'f'`, table)
	if err != nil {
		return fmt.Errorf("contraintes FK de %s : %w", table, err)
	}
	type contrainte struct{ nom, def string }
	var contraintes []contrainte
	for rows.Next() {
		var c contrainte
		if err := rows.Scan(&c.nom, &c.def); err != nil {
			rows.Close()
			return err
		}
		contraintes = append(contraintes, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, c := range contraintes {
		stmt := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", table, pgx.Identifier{c.nom}.Sanitize())
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("suppression de %s sur %s : %w", c.nom, table, err)
		}
	}

	if err := fn(); err != nil {
		return err
	}

	for _, c := range contraintes {
		stmt := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s", table, pgx.Identifier{c.nom}.Sanitize(), c.def)
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("réinstallation de %s sur %s : %w", c.nom, table, err)
		}
	}
	return nil
}
