package matview

import (
	"context"
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaExport est le schéma jetable où ExporterCI recopie tout le
// périmètre CI (Perimetre()) sous forme de tables ordinaires. Jamais présent
// en dehors d'un export ou d'une restauration en cours — DROP SCHEMA
// CASCADE au début et à la fin de chaque opération.
const SchemaExport = "ci"

// ExporterCI construit, dans SchemaExport, une copie plate du périmètre CI
// entier : chaque matvue et chaque TableDirecte devient une TABLE ordinaire,
// sans aucune colonne d'un type énuméré propre au schéma core (mandate_type,
// organization_kind...) — converties en text au passage.
//
// Deux raisons y obligent, trouvées en testant pg_dump/pg_restore
// directement plutôt qu'en les supposant :
//
//  1. pg_restore restaure le CONTENU d'une matvue en rejouant
//     REFRESH MATERIALIZED VIEW, jamais par un simple COPY — dumper mv.*
//     directement exigerait donc au moment de la restauration exactement les
//     tables core/ref volumineuses que tout ce paquet existe pour éviter.
//     Une table ordinaire, elle, se restaure par COPY : le calcul a déjà eu
//     lieu ici, à l'export, plus jamais à la restauration.
//  2. pg_dump ne suit pas les types personnalisés d'une colonne quand la
//     sélection se fait par -t plutôt que par schéma entier (core.mandate_type
//     n'est alors dumpé nulle part, et CREATE TABLE échoue à la
//     restauration) — becoming text élimine la dépendance plutôt que de la
//     répliquer.
//
// Avec ça, SchemaExport ne dépend plus de rien en dehors de lui-même : un
// pg_dump -n ci suffit, aucun -t, aucune CREATE SCHEMA à deviner pour core/
// ref/mv à la restauration (RemettreEnPlace les crée explicitement).
func ExporterCI(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+SchemaExport+" CASCADE"); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+SchemaExport); err != nil {
		return err
	}
	for _, qualifie := range Perimetre() {
		colonnes, err := colonnesPortables(ctx, pool, qualifie)
		if err != nil {
			return fmt.Errorf("export CI de %s : %w", qualifie, err)
		}
		nomPlat := nomExport(qualifie)
		sql := fmt.Sprintf("CREATE TABLE %s.%s AS SELECT %s FROM %s",
			SchemaExport, nomPlat, colonnes, qualifie)
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("export CI de %s : %w", qualifie, err)
		}
		logs.Notice("export CI", "relation", qualifie)
	}
	return nil
}

// RemettreEnPlace défait ExporterCI après restauration : chaque table de
// SchemaExport migre vers le schéma et le nom qu'elle avait à l'export
// (core.mandate, mv.person_actif...), créant le schéma cible s'il n'existe
// pas encore — jamais mv/core/ref en entier, seulement ce que Perimetre()
// couvre. internal/sitegen ne fait ensuite aucune différence entre une vraie
// matvue et cette table ordinaire : ni REFRESH ni écriture n'y ont jamais
// lieu pendant fpctl build site.
func RemettreEnPlace(ctx context.Context, pool *pgxpool.Pool) error {
	schemas := map[string]bool{}
	for _, qualifie := range Perimetre() {
		schema, _, _ := strings.Cut(qualifie, ".")
		schemas[schema] = true
	}
	for schema := range schemas {
		if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
			return err
		}
	}
	for _, qualifie := range Perimetre() {
		schema, table, _ := strings.Cut(qualifie, ".")
		nomPlat := nomExport(qualifie)
		if _, err := pool.Exec(ctx, fmt.Sprintf("ALTER TABLE %s.%s SET SCHEMA %s",
			SchemaExport, nomPlat, schema)); err != nil {
			return fmt.Errorf("remise en place de %s : %w", qualifie, err)
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf("ALTER TABLE %s.%s RENAME TO %s",
			schema, nomPlat, table)); err != nil {
			return fmt.Errorf("remise en place de %s : %w", qualifie, err)
		}
		logs.Notice("relation CI remise en place", "relation", qualifie)
	}
	_, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+SchemaExport)
	return err
}

// nomExport : un nom de table sans point, unique dans SchemaExport — "." ne
// peut pas apparaître dans un identifiant Postgres non cité, donc
// core.mandate devient ci.core__mandate.
func nomExport(qualifie string) string {
	return strings.ReplaceAll(qualifie, ".", "__")
}

// colonnesPortables construit la liste SELECT de qualifie, convertissant en
// text toute colonne d'un type énuméré (pg_type.typtype = 'e') — les seuls
// types non intégrés à Postgres qu'une matvue ou une TableDirecte porte
// aujourd'hui. Un type composite ou un domaine futur qui échapperait à ce
// filtre échouerait à l'export (CREATE TABLE lèverait sur le type manquant
// une fois dans SchemaExport, plutôt qu'à la restauration) : l'échec reste
// au même endroit que l'écrit ce paquet, tout de suite, jamais silencieux.
func colonnesPortables(ctx context.Context, pool *pgxpool.Pool, qualifie string) (string, error) {
	schema, table, _ := strings.Cut(qualifie, ".")
	rows, err := pool.Query(ctx, `
		SELECT a.attname, t.typtype = 'e' AS est_enum
		FROM pg_attribute a
		JOIN pg_type t ON t.oid = a.atttypid
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, schema, table)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var nom string
		var enum bool
		if err := rows.Scan(&nom, &enum); err != nil {
			return "", err
		}
		if enum {
			cols = append(cols, fmt.Sprintf("%s::text AS %s", nom, nom))
		} else {
			cols = append(cols, nom)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "", fmt.Errorf("aucune colonne trouvée (relation inexistante ?)")
	}
	return strings.Join(cols, ", "), nil
}
