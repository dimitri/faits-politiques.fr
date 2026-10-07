-- +goose Up

-- Quelles tables (core./ref./geo./derived./jo., JAMAIS mv. — voir plus bas)
-- chaque nœud du graphe de construction du site (internal/sitegen, voir
-- graphe.go) lit RÉELLEMENT. internal/pipeline.Registre.Publier reste
-- réservé au socle parlementaire audité (core.pipeline_etape/
-- pipeline_dependance, migration 0150) : le graphe de sitegen n'y est
-- jamais publié, donc ce reflet-ci vit dans sa propre table plutôt que
-- d'étendre celle du socle.
--
-- Pas un instantané tenu à la main (comme l'était internal/matview.
-- TablesDirectes avant ce chantier) : réécrit en entier à la fin de chaque
-- « fpctl build site » réussi, à partir de ce qu'un pgx.QueryTracer a
-- RÉELLEMENT observé pendant cette construction (internal/sitegen/
-- table_trace.go) — jamais deviné par relecture du code, qui dérive
-- silencieusement dès qu'une page change sa requête (voir l'incident
-- core.medecin_secteur_effectif, PR « Prochaines étapes », 6 octobre 2026).
--
-- mv.* est délibérément absent : une matvue est déjà son propre nœud du
-- catalogue internal/matview (Definition.Tables y joue exactement ce rôle),
-- la retracer ici serait une troisième source de vérité pour la même
-- question. Ce que ce traceur voit, c'est la lecture DIRECTE d'une table
-- core/ref/geo brute par une page — précisément ce qu'internal/matview.
-- TablesDirectes doit couvrir pour que build-pr reconstruise le site sans
-- base complète.
-- table_qualifiee couvre nom ET tout ce dont nom dépend, transitivement
-- (internal/sitegen.transitiveTables) : lire cette table pour un seul
-- nœud répond déjà à « de quoi ai-je besoin pour reconstruire CETTE page »,
-- sans redérouler le graphe de dépendances à la lecture.
CREATE TABLE core.sitegen_table_usage (
  etape           text NOT NULL,
  table_qualifiee text NOT NULL,
  PRIMARY KEY (etape, table_qualifiee)
);

COMMENT ON TABLE core.sitegen_table_usage IS
  'Tables core/ref/geo/derived/jo nécessaires à chaque nœud du graphe de '
  'construction du site (internal/sitegen), lui ET sa fermeture '
  'transitive de dépendances — mesuré à l''exécution (pgx.QueryTracer), '
  'réécrit en entier à chaque « fpctl build site » réussi. Jamais la '
  'source de vérité (le code Go l''est), jamais mv.* (voir internal/'
  'matview.Definition.Tables pour ça).';

-- +goose Down
DROP TABLE core.sitegen_table_usage;
