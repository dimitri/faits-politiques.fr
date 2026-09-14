-- +goose Up
-- La recherche du corpus vit dans des VUES MATÉRIALISÉES, pas dans des colonnes.
--
-- CE QUI A ÉTÉ MESURÉ. Le vecteur de recherche doit être STOCKÉ : une recherche
-- de phrase sur index fonctionnel recalcule le vecteur de chaque candidat, et
-- coûte 50 057 ms là où la lecture d'un vecteur stocké en coûte 15 — GIN ne
-- range pas les positions des lexèmes. Restait à savoir SOUS QUELLE FORME le
-- stocker. Quatre formes, mesurées sur 200 000 blocs représentatifs :
--
--                                  construction  index    dump         restauration
--   colonne générée (0061)             89,8 s    13,5 s   55,8 Mo        105,1 s
--   colonne ordinaire, INSERT…SELECT   90,3 s    12,8 s  136,8 Mo         35,8 s
--   table CREATE TABLE AS              30,4 s    11,6 s  140,8 Mo         31,3 s
--   VUE MATÉRIALISÉE                   31,5 s    11,6 s    1,6 Ko         44,1 s
--
-- Trois enseignements, dans l'ordre où ils sont apparus :
--
--   1. pg_dump NE TRANSPORTE PAS une colonne générée : il la fait recalculer.
--      D'où les 105 s de restauration, trois fois le reste.
--   2. L'écart entre 90 s et 30 s à la construction n'oppose pas la vue à la
--      table : il oppose INSERT ... SELECT à CREATE TABLE AS. Une relation créée
--      dans la transaction courante évite une partie du travail d'écriture. Un
--      TRUNCATE suivi d'un INSERT dans la même transaction ne le retrouve pas
--      (86,6 s mesurés) : avec wal_level = replica, l'optimisation ne joue pas.
--   3. Le dump d'une vue matérialisée ne contient QUE SA DÉFINITION — 1,6 Ko
--      contre 140 Mo. pg_dump y émet un REFRESH, qui recalcule.
--
-- POURQUOI LA VUE MATÉRIALISÉE. Elle construit aussi vite que le CTAS, elle ne
-- pèse rien dans la sauvegarde, et sa restauration reste dans le même ordre de
-- grandeur. Sur le corpus entier cela donne un dump allégé de près de trois
-- gigaoctets pour environ quatre minutes de restauration en plus.
--
-- Et elle apporte ce qu'aucune colonne ne donne : PostgreSQL CONNAÎT la
-- dépendance. La vue ne peut pas dériver ligne à ligne — elle est fraîche ou
-- périmée, jamais incohérente — et la source ne peut pas être modifiée sans que
-- le moteur le signale. Une colonne ordinaire, elle, repose sur la discipline du
-- chargement.
DROP INDEX IF EXISTS jo.bloc_recherche_gin;
DROP INDEX IF EXISTS jo.texte_recherche_gin;
ALTER TABLE jo.bloc  DROP COLUMN IF EXISTS recherche;
ALTER TABLE jo.texte DROP COLUMN IF EXISTS recherche;

-- Les vues portent les colonnes qui servent à FILTRER, pas seulement le vecteur :
-- chercher un mot dans le dispositif d'un acte donné ne doit pas obliger à
-- revenir à la table.
CREATE MATERIALIZED VIEW jo.recherche_bloc AS
  SELECT b.id,
         b.texte_id,
         b.section,
         to_tsvector('fr', b.contenu) AS recherche
    FROM jo.bloc b;

-- L'index UNIQUE n'est pas décoratif : sans lui, REFRESH MATERIALIZED VIEW
-- CONCURRENTLY est refusé, et tout rafraîchissement prend un verrou exclusif sur
-- la vue — donc coupe la recherche pendant les quarante-quatre secondes qu'il
-- dure.
CREATE UNIQUE INDEX recherche_bloc_pk  ON jo.recherche_bloc (id);
CREATE INDEX recherche_bloc_gin ON jo.recherche_bloc USING gin (recherche);
CREATE INDEX recherche_bloc_texte_idx ON jo.recherche_bloc (texte_id);

COMMENT ON MATERIALIZED VIEW jo.recherche_bloc IS
  'Vecteur de recherche des blocs du Journal officiel. Vue matérialisée et non '
  'colonne : pg_dump n''en emporte que la définition (1,6 Ko contre 140 Mo), et '
  'PostgreSQL connaît la dépendance — la vue ne peut pas dériver ligne à ligne.';

CREATE MATERIALIZED VIEW jo.recherche_texte AS
  SELECT t.id,
         coalesce(t.date_texte, t.date_publi) AS date_acte,
         t.nature,
         to_tsvector('fr', coalesce(t.titre_complet, t.titre, '')) AS recherche
    FROM jo.texte t;

CREATE UNIQUE INDEX recherche_texte_pk  ON jo.recherche_texte (id);
CREATE INDEX recherche_texte_gin ON jo.recherche_texte USING gin (recherche);

-- +goose Down
DROP MATERIALIZED VIEW jo.recherche_texte;
DROP MATERIALIZED VIEW jo.recherche_bloc;
ALTER TABLE jo.texte ADD COLUMN recherche tsvector
  GENERATED ALWAYS AS (to_tsvector('fr', coalesce(titre_complet, titre, ''))) STORED;
ALTER TABLE jo.bloc ADD COLUMN recherche tsvector
  GENERATED ALWAYS AS (to_tsvector('fr', contenu)) STORED;
CREATE INDEX texte_recherche_gin ON jo.texte USING gin (recherche);
CREATE INDEX bloc_recherche_gin  ON jo.bloc  USING gin (recherche);
