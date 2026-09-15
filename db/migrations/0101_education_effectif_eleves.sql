-- +goose Up
-- Chantier 6 (Éducation), une donnée citée mais jamais chargée : les
-- effectifs d'élèves du premier degré, pour confirmer ou infirmer si la
-- légère baisse d'ETP enseignants déjà chargée (core.education_personnel_etablissement,
-- -0,7 % entre les rentrées 2024 et 2025) suit la démographie scolaire ou
-- s'en écarte — voir docs/education-donnees.md § 2.
CREATE TABLE core.education_effectif_eleves (
  annee          smallint NOT NULL,  -- année de rentrée scolaire
  secteur        text NOT NULL,      -- libellé Depp brut : PUBLIC, PRIVE SOUS CONTRAT, et une fois PRIVE (2022, seule apparition)
  nombre_ecoles  integer NOT NULL,
  nombre_eleves  integer NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, secteur)
);

COMMENT ON TABLE core.education_effectif_eleves IS
  'Depp, fr-en-ecoles-effectifs-nb_classes : effectifs d''élèves du premier degré '
  '(écoles maternelles et élémentaires), agrégat national par secteur et rentrée scolaire, '
  '2009-2025. PREMIER DEGRÉ SEULEMENT — ne couvre pas les collèges et lycées, à la '
  'différence de core.education_personnel_etablissement qui a les deux degrés. Le secteur '
  '« PRIVE » (sans précision) n''apparaît qu''en 2022 (98 écoles, 13 150 élèves) — probable '
  'variante de libellé pour le privé hors contrat, non confirmée, gardée telle quelle plutôt '
  'que reclassée.';

-- +goose Down
DROP TABLE core.education_effectif_eleves;
