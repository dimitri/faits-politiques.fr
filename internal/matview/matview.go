// Package matview tient le catalogue des matérialisations Postgres du
// schéma mv (voir db/migrations/0152_matview_ballot.sql) et sait décider,
// avant chaque REFRESH, s'il y a réellement quelque chose à refaire.
//
// La politique tient en trois étages, du disque à l'affichage :
//
//  1. octets archivés (raw.document, via raw.source.etape — internal/
//     archive.Archive.Etape) : ce qui a été téléchargé.
//  2. données en base (core/ref/geo) : ce que l'ingestion en a tiré —
//     mesuré par internal/checksum.Section, DÉJÀ utilisée pour le cache de
//     construction (core.section_checksum, internal/sitegen/cache.go).
//  3. matvue (mv.*) : ce qu'une agrégation en a calculé — mesurée ici,
//     mv.etat, par le même principe (une empreinte des tables source, plus
//     une empreinte du SELECT qui définit la matvue).
//
// internal/sitegen ne doit plus jamais recalculer lui-même une agrégation qu'une
// matvue de ce paquet couvre : un simple SELECT dans le schéma mv, jamais
// un GROUP BY sur une table brute à la construction (voir la revue qui a
// mené à ce paquet — internal/sitegen/scrutins.go, groupBreakdown, un GROUP BY sur
// la totalité de core.ballot À CHAQUE CONSTRUCTION).
package matview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/checksum"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Definition décrit une matvue du schéma mv : son nom, les tables dont son
// SELECT dépend (pour internal/checksum.Section — le signal « faut-il la
// refaire »), et ce SELECT lui-même (pour le sql_hash, et pour qu'il vive
// une seule fois dans le code Go — la migration qui CREATE la matvue en
// porte une copie littérale, à garder synchronisée : voir le commentaire
// de chaque migration mv.*).
type Definition struct {
	Nom    string   // "scrutin_groupe_vote" — sans le schéma
	Tables []string // tables qualifiées dont dépend le SELECT (checksum.Section)
	SQL    string   // le SELECT qui définit la matvue, pour le sql_hash
}

// QualifieNom : mv.<nom>, la forme utilisée dans REFRESH/SELECT.
func (d Definition) QualifieNom() string { return "mv." + d.Nom }

// Catalogue : les matvues connues. Une seule entrée aujourd'hui
// (scrutin_groupe_vote, le premier étage du chantier) — chaque nouvelle
// matvue s'y ajoute, jamais ailleurs : fpctl list matviews et
// ActualiserToutes n'ont besoin de connaître qu'elle.
var Catalogue = []Definition{
	{
		Nom:    "scrutin_groupe_vote",
		Tables: []string{"core.ballot", "core.organization"},
		SQL: `SELECT b.scrutin_id,
	     o.id                                             AS organization_id,
	     coalesce(o.short_name, o.name)                   AS organisation_nom,
	     o.slug                                            AS organisation_slug,
	     coalesce(b.position_rectifiee, b.position)::text AS position,
	     count(*)::int                                    AS n
	FROM core.ballot b
	JOIN core.organization o ON o.id = b.organization_id
	GROUP BY 1, 2, 3, 4, 5`,
	},
	{
		Nom:    "scrutin_vote_nominal",
		Tables: []string{"core.ballot", "core.person", "core.organization"},
		SQL: `SELECT b.scrutin_id,
	     p.slug                                              AS person_slug,
	     p.family_name || ', ' || p.given_name               AS person_nom,
	     p.family_name                                       AS person_family_name,
	     coalesce(o.short_name, o.name, '')                  AS organisation_nom,
	     coalesce(o.slug, '')                                AS organisation_slug,
	     coalesce(b.position_rectifiee, b.position)::text    AS position,
	     b.position_rectifiee IS NOT NULL                    AS rectifiee
	FROM core.ballot b
	JOIN core.person p ON p.id = b.person_id
	LEFT JOIN core.organization o ON o.id = b.organization_id`,
	},
}

func sqlHash(sql string) string {
	h := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(h[:])
}

// Etat : la dernière ligne connue de mv.etat pour une matvue — nil (pas
// d'erreur) si elle n'a jamais été actualisée depuis que cette ligne existe.
type Etat struct {
	DataHash     string
	SQLHash      string
	Lignes       int64
	ActualiseeLe time.Time
}

func lireEtat(ctx context.Context, pool *pgxpool.Pool, nom string) (*Etat, error) {
	var e Etat
	err := pool.QueryRow(ctx,
		`SELECT data_hash, sql_hash, lignes, actualisee_le FROM mv.etat WHERE nom = $1`, nom,
	).Scan(&e.DataHash, &e.SQLHash, &e.Lignes, &e.ActualiseeLe)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// Actualiser vérifie si def a besoin d'un REFRESH (empreinte des tables
// source ou du SELECT qui la définit différente de mv.etat) et, si oui, le
// fait — REFRESH et mise à jour de mv.etat dans LA MÊME TRANSACTION : un
// crash entre les deux ne laisse jamais mv.etat prétendre une fraîcheur que
// la matvue n'a pas atteinte, ni un REFRESH réel sans trace.
//
// Le calcul de l'empreinte des tables source (internal/checksum.Section)
// N'EST PAS dans cette transaction — il ne modifie rien, et pgcopydp-style
// hashtext() sur 4,9 millions de lignes (core.ballot) prend quelques
// secondes : l'exécuter hors transaction évite de tenir une transaction
// ouverte plus longtemps que nécessaire.
func Actualiser(ctx context.Context, pool *pgxpool.Pool, def Definition) (rafraichie bool, err error) {
	dataHash, err := checksum.Section(ctx, pool, def.Tables)
	if err != nil {
		return false, fmt.Errorf("empreinte des tables de %s : %w", def.Nom, err)
	}
	sqlH := sqlHash(def.SQL)

	actuel, err := lireEtat(ctx, pool, def.Nom)
	if err != nil {
		return false, fmt.Errorf("état de mv.%s : %w", def.Nom, err)
	}
	if actuel != nil && actuel.DataHash == dataHash && actuel.SQLHash == sqlH {
		return false, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "REFRESH MATERIALIZED VIEW "+def.QualifieNom()); err != nil {
		return false, fmt.Errorf("REFRESH %s : %w", def.QualifieNom(), err)
	}
	var lignes int64
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+def.QualifieNom()).Scan(&lignes); err != nil {
		return false, fmt.Errorf("comptage de %s : %w", def.QualifieNom(), err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mv.etat (nom, data_hash, sql_hash, lignes, actualisee_le)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (nom) DO UPDATE
		SET data_hash = excluded.data_hash, sql_hash = excluded.sql_hash,
		    lignes = excluded.lignes, actualisee_le = excluded.actualisee_le`,
		def.Nom, dataHash, sqlH, lignes); err != nil {
		return false, fmt.Errorf("mv.etat de %s : %w", def.Nom, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// ActualiserToutes actualise chaque matvue du Catalogue, dans l'ordre —
// aucune dépendance déclarée entre elles aujourd'hui (une seule), mais
// gardé en boucle simple plutôt qu'un pipeline.Registre tant qu'une
// matvue ne dépend pas du résultat d'une autre.
func ActualiserToutes(ctx context.Context, pool *pgxpool.Pool) error {
	for _, def := range Catalogue {
		rafraichie, err := Actualiser(ctx, pool, def)
		if err != nil {
			return fmt.Errorf("mv.%s : %w", def.Nom, err)
		}
		if rafraichie {
			logs.Notice("matvue actualisée", "nom", def.QualifieNom())
		} else {
			logs.Notice("matvue déjà à jour", "nom", def.QualifieNom())
		}
	}
	return nil
}

// EtatAffiche : une ligne du catalogue, avec son état connu (ou son
// absence) — pour fpctl list matviews.
type EtatAffiche struct {
	Nom          string
	Tables       []string
	Connue       bool
	Lignes       int64
	ActualiseeLe time.Time
}

// Lister renvoie l'état affiché de chaque matvue du Catalogue, dans l'ordre
// de déclaration — lit mv.etat tel quel, ne recalcule aucune empreinte
// (donc ne dit pas si une matvue est PÉRIMÉE, seulement quand et sur
// combien de lignes elle a tourné pour la dernière fois : le recalcul de
// l'empreinte des tables source coûte, sur core.ballot, plusieurs secondes
// — pas le prix d'un simple affichage).
func Lister(ctx context.Context, pool *pgxpool.Pool) ([]EtatAffiche, error) {
	out := make([]EtatAffiche, 0, len(Catalogue))
	for _, def := range Catalogue {
		e, err := lireEtat(ctx, pool, def.Nom)
		if err != nil {
			return nil, fmt.Errorf("état de mv.%s : %w", def.Nom, err)
		}
		ea := EtatAffiche{Nom: def.Nom, Tables: def.Tables}
		if e != nil {
			ea.Connue = true
			ea.Lignes = e.Lignes
			ea.ActualiseeLe = e.ActualiseeLe
		}
		out = append(out, ea)
	}
	return out, nil
}
