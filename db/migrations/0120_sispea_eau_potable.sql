-- +goose Up
-- Les services publics d'eau potable, un par ligne (SISPEA) — qui les gère
-- (régie ou délégation, et à qui), combien d'habitants ils desservent, et le
-- prix réel du service. L'API Hub'Eau qui aurait pu servir cette donnée a
-- été décommissionnée le 10 septembre 2026 ; le remplacement est un export
-- en masse (XLS, parfois un .xls hérité livré sous une extension .xls
-- trompeuse — voir le commentaire du connecteur), pas une API.
--
-- Millésime 2023 seulement pour l'instant : l'export 2024 disponible au
-- moment du chargement est un binaire .xls hérité (format OLE2/CFBF), que
-- la bibliothèque Excel pure Go de ce dépôt ne sait pas lire — un problème
-- de format à résoudre séparément, pas une raison de deviner les chiffres.
CREATE TABLE core.service_eau_potable (
  id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                 integer NOT NULL,
  id_sispea             text NOT NULL,
  nom_service           text NOT NULL,
  code_departement      text,
  mode_gestion          text CHECK (mode_gestion IN ('REGIE', 'DELEGATION')),
  statut_operateur      text,
  nom_operateur         text,
  population_desservie  integer,
  prix_eur_m3           numeric,
  agence_de_leau        text,
  bassin_code           text,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  UNIQUE (annee, id_sispea)
);

CREATE INDEX service_eau_potable_dept_idx ON core.service_eau_potable (code_departement);

COMMENT ON TABLE core.service_eau_potable IS
  'Services publics d''eau potable (SISPEA, observatoire eaufrance.fr), un par service. '
  'mode_gestion/statut_operateur/nom_operateur : qui gère (régie directe ou délégation à '
  'un opérateur privé nommé). population_desservie et prix_eur_m3 : les deux seuls '
  'indicateurs descriptifs chargés pour l''instant (D101.0, D102.0 dans la nomenclature '
  'SISPEA — vérifié directement sur le Panorama Sispea 2020, annexe 1, avant chargement : '
  'un premier repérage par nom de colonne (p101_1) avait failli faire passer le taux de '
  'conformité microbiologique pour le prix de l''eau). Millésime 2023 seulement.';

-- +goose Down
DROP TABLE core.service_eau_potable;
