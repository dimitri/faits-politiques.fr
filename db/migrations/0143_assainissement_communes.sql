-- +goose Up
-- Les services d'assainissement (collectif et non collectif), au niveau de
-- la commune — SISPEA 2023, jeux "exploités pour les rapports nationaux"
-- (data.gouv.fr/OFB). Comble deux manques documentés au § 7 du dossier
-- bassins versants : l'assainissement (jusqu'ici seule l'eau potable était
-- chargée) et la composition communale des services (chaque ligne relie
-- une commune à son service, ce que le seul export "eau potable" ne
-- permettait pas).
CREATE TABLE core.service_assainissement (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  competence           text NOT NULL CHECK (competence IN ('COLLECTIF','NON_COLLECTIF')),
  annee                int NOT NULL,
  code_insee           text NOT NULL,
  id_sispea_collectivite text,
  nom_collectivite     text,
  id_sispea_service    text,
  nom_service          text,
  mode_gestion         text,       -- 'REGIE' ou 'DELEGATION', comme core.service_eau_potable
  nom_operateur        text,
  population_desservie int,
  agence_de_leau       text,
  source_id            bigint NOT NULL REFERENCES raw.source(id)
);
CREATE INDEX service_assainissement_commune_idx ON core.service_assainissement (code_insee);
CREATE INDEX service_assainissement_competence_idx ON core.service_assainissement (competence, annee);

COMMENT ON TABLE core.service_assainissement IS
  'Composition communale des services d''assainissement collectif et non collectif, 2023 '
  '(SISPEA, jeux "exploités pour les rapports nationaux", OFB/data.gouv.fr). Les colonnes '
  'd''indicateur (prix, taux de conformité...) ne sont volontairement pas chargées : leur '
  'signification exacte par compétence (d201_0, d301_0...) n''a pas été vérifiée au moment du '
  'chargement, à la différence de d101_0/d102_0 déjà vérifiés pour l''eau potable — seuls les '
  'champs d''identification et de gestion, sans ambiguïté, sont repris ici.';

-- +goose Down
DROP TABLE core.service_assainissement;
