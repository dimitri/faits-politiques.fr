-- +goose Up
-- Les personnels des établissements scolaires, par statut — DEPP, granularité
-- établissement (comme FINESS pour la santé ou OFGL pour les communes : la
-- clé la plus fine que la source publie). Le budget de la mission
-- « Enseignement scolaire » n'a pas besoin d'une table dédiée : il vit déjà
-- dans core.budget_programme (docs/securite-police-donnees.md,
-- docs/defense-donnees.md), qui couvre toutes les missions du budget général.
CREATE TABLE core.education_personnel_etablissement (
  id                       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                    smallint NOT NULL,
  degre                    text NOT NULL CHECK (degre IN ('PREMIER','SECOND')),
  identifiant_etablissement text NOT NULL,
  nom_etablissement        text NOT NULL,
  nature_etablissement     text,
  code_departement         text,
  code_academie            text,
  secteur                  text NOT NULL CHECK (secteur IN ('PUBLIC','PRIVE')),
  etp_total                numeric,
  etp_enseignants          numeric,
  etp_vie_scolaire         numeric,          -- second degré seulement (AED notamment)
  proportion_non_titulaires numeric,          -- second degré seulement, en %
  proportion_agreges        numeric,          -- second degré seulement, en %
  proportion_certifies      numeric,          -- second degré seulement, en %
  source_id                bigint NOT NULL REFERENCES raw.source(id),
  created_at               timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX education_personnel_uniq ON core.education_personnel_etablissement
  (annee, degre, identifiant_etablissement);
CREATE INDEX education_personnel_secteur_idx ON core.education_personnel_etablissement (degre, secteur, annee);

COMMENT ON TABLE core.education_personnel_etablissement IS
  'Effectifs enseignants et personnels (en ETP) par établissement, premier et '
  'second degré, Depp (data.education.gouv.fr). etp_total - etp_enseignants - '
  'etp_vie_scolaire donne le personnel hors face-à-face pédagogique et hors '
  'vie scolaire (direction, administratif, technique) pour le second degré — '
  'voir docs/education-donnees.md. Premier degré : un seul champ etp_enseignants, '
  'pas de décomposition par statut publiée à ce niveau de détail. Second degré : '
  'un seul millésime publié (2024) au moment du chargement ; premier degré en a deux '
  '(2024, 2025) — asymétrie de la source, pas un choix de ce connecteur.';

-- +goose Down
DROP TABLE core.education_personnel_etablissement;
