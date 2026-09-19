-- +goose Up
-- L'inventaire SRU (loi Solidarité et renouvellement urbains, article 55) :
-- pour chaque commune soumise à l'obligation de logements sociaux, le
-- taux atteint, l'objectif cible et le statut de carence — DGALN/DHUP,
-- republié sur data.gouv.fr, vérifié directement (2 196 communes au
-- 1er janvier 2025, fichier CSV encodé en Latin-1 dans la source, converti
-- en UTF-8 à l'ingestion). Pour docs/logement-territoires-donnees.md.
CREATE TABLE core.sru_commune (
  id                       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_insee               text NOT NULL UNIQUE,
  commune                  text NOT NULL,
  departement              text NOT NULL,
  population               int,
  nombre_logements_sociaux int,
  taux_sru_pct             double precision,
  taux_cible_pct           double precision,
  deficitaire              boolean NOT NULL,
  carencee                 boolean NOT NULL,
  exemptee                 boolean NOT NULL,
  source_id                bigint NOT NULL REFERENCES raw.source(id)
);

CREATE INDEX sru_commune_code_idx ON core.sru_commune (code_insee);

COMMENT ON TABLE core.sru_commune IS
  'Inventaire SRU au 1er janvier 2025 (DGALN/DHUP) : taux de logements sociaux '
  'par commune soumise à l''article 55 de la loi SRU, statut de carence. '
  'Ne couvre que les communes dans le périmètre de la loi (2 196 sur ~35 000), '
  'pas l''ensemble des communes françaises.';

-- +goose Down
DROP TABLE core.sru_commune;
