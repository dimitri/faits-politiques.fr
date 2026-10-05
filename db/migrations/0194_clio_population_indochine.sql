-- +goose Up
-- Population du Viêt Nam, du Cambodge et du Laos, 1500-2000 — CLIO-INFRA
-- (clio-infra.eu, projet universitaire d'histoire économique, IISH Amsterdam),
-- fichier « Total Population », licence CC0-1.0 (domaine public, vérifié sur
-- la fiche Dataverse : hdl.handle.net/10622/SNETZV). Un fichier mondial (191
-- pays), chargé ici seulement pour les trois pays du dossier Indochine — pas
-- de raison de charger le reste tant qu'aucun autre dossier n'en a besoin.
--
-- Frontières MODERNES, pas coloniales : CLIO-INFRA compte la population du
-- territoire du Viêt Nam actuel (réunifié) à chaque date, y compris avant
-- 1954 et avant l'indépendance de 1945 — jamais le même périmètre que
-- geo.territoire_colonial (l'Indochine française incluait aussi des zones
-- aujourd'hui hors Viêt Nam/Cambodge/Laos actuels, et l'inverse). Une
-- population, pas un recensement colonial : à présenter comme un ordre de
-- grandeur démographique du pays actuel, jamais comme la population de la
-- colonie française elle-même.
CREATE TABLE core.population_indochine_historique (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  pays                text NOT NULL CHECK (pays IN ('Vietnam', 'Cambodge', 'Laos')),
  annee               smallint NOT NULL,
  population_milliers numeric NOT NULL CHECK (population_milliers > 0),
  source_id           bigint NOT NULL REFERENCES raw.source(id),
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX population_indochine_historique_uniq
  ON core.population_indochine_historique (pays, annee);

COMMENT ON TABLE core.population_indochine_historique IS
  'Population du Viêt Nam, Cambodge, Laos (frontières actuelles), 1500-2000 — CLIO-INFRA, '
  'CC0-1.0. Ne pas confondre avec la population de l''Indochine française coloniale '
  '(périmètre différent, non chargé) : voir le commentaire de cette migration.';

-- +goose Down
DROP TABLE core.population_indochine_historique;
