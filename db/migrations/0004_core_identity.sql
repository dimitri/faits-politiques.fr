-- +goose Up
-- Identités : personnes, organisations, mandats, appartenances.
--
-- Principe : une personne existe indépendamment de ses mandats et de son parti.
-- Ne jamais identifier quelqu'un par son appartenance courante — c'est précisément
-- l'erreur qui rend impossible de suivre une trajectoire.

CREATE TABLE core.person (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug        text NOT NULL UNIQUE,          -- permalien public, immuable
  family_name text NOT NULL,
  given_name  text NOT NULL,
  birth_date  date,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX person_name_trgm_idx ON core.person
  USING gin (core.f_unaccent(given_name || ' ' || family_name) gin_trgm_ops);

-- Le référentiel d'identités inter-institutions (« crosswalk »).
-- Chaque silo a ses propres identifiants et aucune table de correspondance ouverte
-- n'existe aujourd'hui. C'est ce qui permet de suivre une personne de conseiller
-- municipal à député puis à maire.
CREATE TABLE core.person_identifier (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  person_id  bigint NOT NULL REFERENCES core.person(id) ON DELETE CASCADE,
  scheme     text   NOT NULL CHECK (scheme IN (
               'AN_ACTEUR','AN_SYCOMORE','SENAT_MATRICULE','EP_MEP',
               'RNE','HATVP','WIKIDATA')),
  value      text   NOT NULL,
  provenance core.provenance NOT NULL DEFAULT 'OFFICIAL',
  UNIQUE (scheme, value)
);
CREATE INDEX person_identifier_person_idx ON core.person_identifier (person_id);

COMMENT ON TABLE core.person_identifier IS
  'Publié comme jeu de données ouvert : c''est une brique d''infrastructure réutilisable, '
  'pas seulement un détail interne.';

CREATE TABLE core.organization (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug       text NOT NULL UNIQUE,
  kind       core.organization_kind NOT NULL,
  name       text NOT NULL,
  short_name text,
  validity   daterange NOT NULL DEFAULT daterange(NULL, NULL),
  created_at timestamptz NOT NULL DEFAULT now(),
  -- Cible des clés étrangères composites qui garantissent la cohérence du type.
  UNIQUE (id, kind)
);

CREATE TABLE core.organization_identifier (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id bigint NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  scheme          text   NOT NULL CHECK (scheme IN ('AN_ORGANE','SENAT_GROUPE','EP_GROUP','RNE_NUANCE','WIKIDATA')),
  value           text   NOT NULL,
  UNIQUE (scheme, value)
);

-- Mandats. Un mandat est daté, pas courant : « X est député » est une affirmation
-- qui doit toujours porter une période.
CREATE TABLE core.mandate (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  person_id     bigint NOT NULL REFERENCES core.person(id),
  mandate_type  core.mandate_type NOT NULL,
  institution   core.institution,      -- NULL pour les mandats locaux
  commune_code  text,                  -- NULL hors mandat communal
  constituency  text,                  -- circonscription, département d'élection
  validity      daterange NOT NULL,
  cause_debut   text,
  cause_fin     text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT commune_code_iff_local CHECK (
    (mandate_type IN ('MAIRE','ADJOINT_AU_MAIRE','CONSEILLER_MUNICIPAL')) = (commune_code IS NOT NULL)
  ),
  -- Une personne ne peut pas détenir deux fois le même type de mandat en même temps.
  CONSTRAINT mandate_no_overlap EXCLUDE USING gist (
    person_id WITH =, mandate_type WITH =, validity WITH &&
  )
);
CREATE INDEX mandate_person_idx  ON core.mandate (person_id);
CREATE INDEX mandate_commune_idx ON core.mandate (commune_code) WHERE commune_code IS NOT NULL;
CREATE INDEX mandate_validity_idx ON core.mandate USING gist (validity);

-- Appartenances datées : groupe parlementaire, parti, coalition.
-- Le kind est dupliqué depuis l'organisation et verrouillé par une FK composite,
-- afin de pouvoir poser une contrainte d'exclusion qui ne vaut que pour certains
-- types (on ne peut pas appartenir à deux groupes parlementaires à la fois,
-- alors qu'une double appartenance partisane est possible).
CREATE TABLE core.affiliation (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  person_id         bigint NOT NULL REFERENCES core.person(id),
  organization_id   bigint NOT NULL,
  organization_kind core.organization_kind NOT NULL,
  role              text,               -- 'MEMBRE', 'APPARENTE', 'PRESIDENT'
  validity          daterange NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, organization_kind)
    REFERENCES core.organization(id, kind),
  CONSTRAINT one_parliamentary_group_at_a_time EXCLUDE USING gist (
    person_id WITH =, validity WITH &&
  ) WHERE (organization_kind = 'PARLIAMENTARY_GROUP')
);
CREATE INDEX affiliation_person_idx ON core.affiliation (person_id);
CREATE INDEX affiliation_org_idx    ON core.affiliation (organization_id);
CREATE INDEX affiliation_validity_idx ON core.affiliation USING gist (validity);

-- Étiquette politique d'un maire, telle que publiée par le RNE, avec son millésime
-- de nomenclature. Séparée de core.affiliation parce que la nuance est une
-- qualification administrative attribuée par la préfecture, pas une adhésion
-- déclarée par la personne. Les confondre serait une faute factuelle.
CREATE TABLE core.nuance_assignment (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  mandate_id           bigint NOT NULL REFERENCES core.mandate(id) ON DELETE CASCADE,
  nuance_code          text   NOT NULL,
  circulaire_millesime int    NOT NULL,
  FOREIGN KEY (nuance_code, circulaire_millesime)
    REFERENCES ref.nuance_politique(code, circulaire_millesime),
  UNIQUE (mandate_id, circulaire_millesime)
);

-- +goose Down
DROP TABLE core.nuance_assignment;
DROP TABLE core.affiliation;
DROP TABLE core.mandate;
DROP TABLE core.organization_identifier;
DROP TABLE core.organization;
DROP TABLE core.person_identifier;
DROP TABLE core.person;
