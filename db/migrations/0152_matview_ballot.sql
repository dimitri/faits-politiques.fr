-- +goose Up

-- Schéma dédié aux VRAIES matérialisations Postgres (CREATE MATERIALIZED
-- VIEW), distinct de « derived » : derived contient déjà des vues à la
-- volée et des tables-reflets peuplées par du code Go bien à elles
-- (derived.coverage, derived.scrutin_topic — un INSERT explicite, jamais un
-- REFRESH), un mécanisme différent avec un cycle de vie différent. Tout ce
-- qui vit dans « mv » se rafraîchit UNIQUEMENT via REFRESH MATERIALIZED
-- VIEW, orchestré par internal/matview — jamais par un INSERT ou un UPDATE
-- direct. Un schéma à part rend « quelles matvues existent » une question
-- d'un SELECT sur pg_matviews WHERE schemaname='mv', sans avoir à trier
-- parmi les vues et tables ordinaires de derived.
CREATE SCHEMA mv;

COMMENT ON SCHEMA mv IS
  'Matérialisations Postgres pures (CREATE MATERIALIZED VIEW), rafraîchies '
  'par internal/matview — jamais peuplées par un INSERT/UPDATE direct. Voir '
  'mv.etat pour savoir laquelle est à jour.';

-- mv.etat : le dernier état connu de chaque matvue — l'empreinte des tables
-- source (internal/checksum.Section, déjà utilisée pour le cache de
-- construction, core.section_checksum) au moment du dernier REFRESH, et
-- l'empreinte du SELECT qui la définit (internal/matview.Definition.SQL) :
-- l'une capture « les données ont-elles changé », l'autre « la définition
-- a-t-elle changé » (une CREATE OR REPLACE / un ALTER dans une migration
-- ultérieure). Les deux doivent concorder avec l'état courant pour qu'un
-- REFRESH soit sauté — sous-couvrir l'une ou l'autre ne sert jamais une
-- page fausse, au pire un REFRESH inutile.
--
-- Écrite dans LA MÊME TRANSACTION que le REFRESH qu'elle décrit (voir
-- internal/matview.Actualiser) : jamais un REFRESH réussi sans que cette
-- ligne l'atteste, jamais cette ligne mise à jour sans REFRESH réel.
CREATE TABLE mv.etat (
  nom            text PRIMARY KEY,
  data_hash      text NOT NULL,
  sql_hash       text NOT NULL,
  lignes         bigint NOT NULL,
  actualisee_le  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE mv.etat IS
  'Un reflet, comme core.pipeline_etape : réécrit par internal/matview à '
  'chaque REFRESH, jamais tenu à la main. data_hash/sql_hash proviennent '
  'respectivement de internal/checksum.Section (les tables source) et du '
  'hachage du SELECT embarqué (internal/matview.Definition) — les deux '
  'doivent concorder avec l''état courant pour qu''un REFRESH soit sauté.';

-- mv.scrutin_groupe_vote : le dépouillement par groupe de chaque scrutin —
-- ce que cmd/build/scrutins.go (groupBreakdown) recalculait par un GROUP BY
-- sur la totalité de core.ballot (4,9 millions de lignes, quelques secondes)
-- À CHAQUE CONSTRUCTION, alors que core.ballot ne change qu'à l'ingestion.
-- Premier étage du chantier plus large (voir la revue de fpctl list deps/
-- matview) : nominalVotes, loadThemes, loadEurope, loadSenat et les
-- fonctions de groupes.go répètent la même sorte de passage complet sur
-- core.ballot — chacune sa propre matvue viendra ensuite.
--
-- INNER JOIN sur organization, comme le GROUP BY qu'elle remplace : un
-- bulletin sans organization_id (un vote hors groupe connu) disparaît déjà
-- aujourd'hui, pas une régression introduite ici.
CREATE MATERIALIZED VIEW mv.scrutin_groupe_vote AS
  SELECT b.scrutin_id,
         o.id                                                   AS organization_id,
         coalesce(o.short_name, o.name)                         AS organisation_nom,
         o.slug                                                 AS organisation_slug,
         coalesce(b.position_rectifiee, b.position)::text       AS position,
         count(*)::int                                          AS n
    FROM core.ballot b
    JOIN core.organization o ON o.id = b.organization_id
   GROUP BY 1, 2, 3, 4, 5;

CREATE UNIQUE INDEX scrutin_groupe_vote_pk
  ON mv.scrutin_groupe_vote (scrutin_id, organization_id, position);

COMMENT ON MATERIALIZED VIEW mv.scrutin_groupe_vote IS
  'Dépouillement par groupe de chaque scrutin (FOR/AGAINST/ABSTAIN/ABSENT/'
  'NON_VOTING), une ligne par (scrutin, organisation, position) — remplace '
  'le GROUP BY sur la totalité de core.ballot que cmd/build refaisait à '
  'chaque construction (voir groupBreakdown, cmd/build/scrutins.go). '
  'Rafraîchie par internal/matview, jamais par un GROUP BY applicatif.';

-- +goose Down
DROP MATERIALIZED VIEW mv.scrutin_groupe_vote;
DROP TABLE mv.etat;
DROP SCHEMA mv;
