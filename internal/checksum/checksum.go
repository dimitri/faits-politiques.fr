// Package checksum calcule l'empreinte du contenu d'une table, et la combine
// pour une section du site — pour répondre à une seule question : « les
// tables qui alimentent cette section ont-elles changé depuis la dernière
// fois ? », sans jamais relire les données elles-mêmes à la construction.
//
// La requête par table vient de pgcopydb compare data (même auteur, même
// idée : détecter une divergence sans comparer ligne à ligne). hashtext(t::text)
// par ligne, sommé — donc indépendant de l'ordre de lecture — puis le compte de
// lignes est mêlé au résultat avant le MD5 final, pour qu'une table vidée puis
// réinsérée à l'identique ne soit pas confondue avec une table inchangée.
//
// Mesuré sur les tables réellement en jeu ici (le 15 septembre 2026) :
// commune_delinquance (5,2 M lignes) 3,5 s, ballot (4,9 M) 2,2 s,
// commune_indicator (2,5 M) 2,4 s, scrutin (38 042) 0,2 s. Négligeable une
// fois par ingestion ; répété à chaque construction, ça redeviendrait le
// problème que ce paquet existe pour éviter — d'où l'écriture dans
// core.section_checksum par cmd/ingest, jamais un calcul à la volée par
// internal/sitegen (voir internal/sitegen/cache.go, qui ne fait qu'une lecture de ligne).
package checksum

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Table renvoie l'empreinte du contenu d'une table qualifiée (schema.table),
// et son nombre de lignes. ONLY exclut les partitions/héritages éventuels :
// la question posée est « cette table a-t-elle changé », pas « et ses filles ».
func Table(ctx context.Context, pool *pgxpool.Pool, qualifiedName string) (hash string, rows int64, err error) {
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(1),
		       md5(format('%%s-%%s', sum(hashtext(t::text)::bigint), count(1)))::text
		FROM only %s t`, qualifiedName)).Scan(&rows, &hash)
	if err != nil {
		return "", 0, fmt.Errorf("empreinte de %s : %w", qualifiedName, err)
	}
	return hash, rows, nil
}

// Section combine l'empreinte de plusieurs tables en une seule, pour une
// section du site qui en dépend toutes. L'ordre des tables ne compte pas
// (triées avant combinaison) : seul l'ensemble des empreintes individuelles
// compte, comme chaque empreinte individuelle ignore déjà l'ordre des lignes.
func Section(ctx context.Context, pool *pgxpool.Pool, tables []string) (string, error) {
	trie := append([]string(nil), tables...)
	sort.Strings(trie)
	h := sha256.New()
	for _, t := range trie {
		th, rows, err := Table(ctx, pool, t)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%s:%d;", t, th, rows)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
