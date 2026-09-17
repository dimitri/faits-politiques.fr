-- +goose Up
-- Les décisions d'aide des agences de l'eau : qui reçoit quoi, pour quel
-- projet — le premier maillon chargé de la chaîne redevances → aides →
-- investissements (voir docs/bassins-versants-donnees.md § 5). Périmètre
-- volontairement partiel, documenté comme tel plutôt que deviné :
--
-- - Loire-Bretagne : 11e programme (2019-2024) et 12e programme (2025-2030,
--   en cours), un fichier par programme, format stable. Le 10e programme
--   (2013-2018) n'est PAS chargé : chaque millésime y a son propre jeu de
--   colonnes (vérifié à l'inspection — 2013/2014/2015 partagent un format,
--   2016 et 2018 en ont chacun un autre), un travail de connecteur séparé.
-- - Artois-Picardie : les deux fichiers publiés au format « données
--   essentielles des conventions de subvention » (décret n° 2017-779),
--   qui couvrent 2017-2026.
-- - Adour-Garonne, Rhin-Meuse, Rhône-Méditerranée-Corse, Seine-Normandie :
--   aucun export en masse trouvé à l'inspection (portails de recherche par
--   critères, sans fichier téléchargeable) — non chargés, pas un oubli.
CREATE TABLE core.aide_agence_eau (
  id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  agence                text NOT NULL CHECK (agence IN ('LOIRE_BRETAGNE', 'ARTOIS_PICARDIE')),
  programme             text NOT NULL,
  annee                 integer NOT NULL,
  date_decision         text,
  reference_decision    text,
  nom_beneficiaire      text NOT NULL,
  siret_beneficiaire    text,
  code_departement      text,
  code_insee_commune    text,
  objet                 text,
  montant_eur           numeric NOT NULL,
  nature                text,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX aide_agence_eau_agence_annee_idx ON core.aide_agence_eau (agence, annee);
CREATE INDEX aide_agence_eau_dept_idx ON core.aide_agence_eau (code_departement);

COMMENT ON TABLE core.aide_agence_eau IS
  'Décisions d''aide accordées par les agences de l''eau, une ligne par aide. '
  'Périmètre partiel et assumé : Loire-Bretagne (programmes 11 et 12 '
  'seulement, 2019-2030) et Artois-Picardie (conventions publiées au format '
  'décret n° 2017-779, 2017-2026). Les quatre autres agences et le 10e '
  'programme Loire-Bretagne (2013-2018, format hétérogène par millésime) ne '
  'sont pas chargés. date_decision reste du texte : les formats de date '
  'diffèrent selon la source et le programme, jamais normalisés au prix '
  'd''une supposition.';

-- +goose Down
DROP TABLE core.aide_agence_eau;
