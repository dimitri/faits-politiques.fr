-- +goose Up
-- Couche REF : nomenclatures externes. Toutes millésimées, parce qu'elles changent
-- et qu'un chiffre publié il y a deux ans ne se relit correctement qu'avec la
-- nomenclature de l'époque.

-- Code officiel géographique. Les communes fusionnent et se scindent : la clé est
-- (code, millésime), jamais le code seul.
CREATE TABLE ref.commune (
  code_insee            text    NOT NULL,
  cog_millesime         int     NOT NULL,
  nom                   text    NOT NULL,
  code_departement      text    NOT NULL,
  code_region           text    NOT NULL,
  population_municipale int,
  PRIMARY KEY (code_insee, cog_millesime)
);
CREATE INDEX commune_nom_trgm_idx ON ref.commune USING gin (nom gin_trgm_ops);

-- Fusions, scissions, changements de code. Sans cette table, une série temporelle
-- communale est fausse dès qu'une commune nouvelle apparaît.
CREATE TABLE ref.commune_change (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  effective_date date    NOT NULL,
  change_type    text    NOT NULL CHECK (change_type IN ('FUSION','SCISSION','RENOMMAGE','CHANGEMENT_CODE')),
  code_avant     text    NOT NULL,
  code_apres     text    NOT NULL,
  cog_millesime  int     NOT NULL
);
CREATE INDEX commune_change_avant_idx ON ref.commune_change (code_avant);
CREATE INDEX commune_change_apres_idx ON ref.commune_change (code_apres);

CREATE TABLE ref.epci (
  siren          text NOT NULL,
  cog_millesime  int  NOT NULL,
  nom            text NOT NULL,
  forme          text,               -- CC, CA, CU, METRO
  PRIMARY KEY (siren, cog_millesime)
);

CREATE TABLE ref.epci_membership (
  code_insee     text NOT NULL,
  siren_epci     text NOT NULL,
  cog_millesime  int  NOT NULL,
  PRIMARY KEY (code_insee, cog_millesime),
  FOREIGN KEY (siren_epci, cog_millesime) REFERENCES ref.epci(siren, cog_millesime)
);

-- BANATIC : compétences effectivement transférées à l'intercommunalité.
-- Sert à répondre « la commune ne décide pas de cela » (raison EPCI_COMPETENCE)
-- avant même de chercher un chiffre.
CREATE TABLE ref.epci_competence (
  siren_epci      text NOT NULL,
  annee           int  NOT NULL,
  competence_code text NOT NULL,
  competence_label text NOT NULL,
  PRIMARY KEY (siren_epci, annee, competence_code)
);

-- Nuances politiques du ministère de l'Intérieur. La nomenclature change à chaque
-- circulaire (la dernière date de février 2026), d'où le millésime obligatoire :
-- comparer des nuances de millésimes différents est une erreur.
CREATE TABLE ref.nuance_politique (
  code                 text NOT NULL,
  circulaire_millesime int  NOT NULL,
  libelle              text NOT NULL,
  famille              text,          -- regroupement, à n'utiliser qu'explicitement documenté
  PRIMARY KEY (code, circulaire_millesime)
);

-- Strates démographiques utilisées par l'OFGL et la DGFiP. Comparer hors strate
-- est la faute méthodologique la plus fréquente sur les finances locales.
CREATE TABLE ref.strate_demographique (
  code        text PRIMARY KEY,
  libelle     text NOT NULL,
  pop_min     int  NOT NULL,
  pop_max     int
);

-- Catalogue des indicateurs communaux. La formule et la réserve sont stockées avec
-- l'indicateur : un indicateur dérivé qui a perdu sa formule n'est plus auditable.
CREATE TABLE ref.indicator (
  code              text PRIMARY KEY,          -- 'ofgl.dette_par_habitant'
  label             text NOT NULL,
  unit              text NOT NULL,             -- 'EUR_PAR_HABITANT', 'TAUX_PCT', 'NOMBRE'
  producer          text NOT NULL,             -- 'OFGL', 'DGFiP', 'SSMSI'
  formula           text,                      -- 'encours_dette / population_municipale'
  caveat            text NOT NULL,             -- affiché sur toute fiche portant cet indicateur
  competence_code   text,                      -- si non NULL : vérifier ref.epci_competence avant d'afficher
  comparable        boolean NOT NULL DEFAULT true,
  created_at        timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN ref.indicator.caveat IS
  'Réserve obligatoire et non dissimulable. Ex. pour SSMSI : « faits enregistrés par les '
  'services, pas faits commis ; ne mesure pas l''action municipale ».';

-- Raisons typées de non-vérifiabilité. Répondre « je ne sais pas » avec sa raison
-- est une réponse de plein droit (perimetre.md §2.2).
CREATE TABLE ref.unverifiable_reason (
  code        text PRIMARY KEY,
  label       text NOT NULL,
  explanation text NOT NULL
);

INSERT INTO ref.unverifiable_reason (code, label, explanation) VALUES
 ('NO_ROLL_CALL',     'Aucun scrutin public',
  'Le vote a eu lieu à main levée. Aucune trace nominative n''existe : ni cet outil ni aucun autre ne peut dire qui a voté quoi.'),
 ('GROUP_LEVEL_ONLY', 'Position connue au niveau du groupe seulement',
  'La source publie la position du groupe et la liste nominative des exceptions. La position des autres membres ne peut pas en être déduite.'),
 ('NOT_PRODUCED',     'Donnée non produite',
  'Aucun producteur public ne mesure cette donnée à ce niveau géographique ou temporel.'),
 ('EPCI_COMPETENCE',  'Compétence intercommunale',
  'Cette compétence a été transférée à l''intercommunalité. La commune n''en décide pas.'),
 ('OUT_OF_CORPUS',    'Déclaration hors corpus',
  'Les propos tenus en plateau, en meeting ou sur les réseaux sociaux ne sont pas couverts. Seules les interventions en séance le sont.'),
 ('OUT_OF_PERIOD',    'Hors période couverte',
  'La période visée est antérieure à la profondeur historique des sources ingérées.'),
 ('AMBIGUOUS_SUBJECT','Objet non identifiable',
  'L''affirmation ne se rattache pas à un objet parlementaire ou à un indicateur identifiable.');

-- Table d'alias : « la loi Duplomb », « la loi immigration », « le budget 2025 »
-- vers l'objet réel. Maintenue à la main, quelques centaines d'entrées, très fort
-- effet de levier. La cible est désignée par son slug public (kind, target_key) et non
-- par une clé technique : un alias survit à une réingestion complète de core.
CREATE TABLE ref.alias (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  alias_text  text    NOT NULL,
  normalized  text    NOT NULL,   -- unaccent(lower(alias_text)), rempli à l'écriture
  kind        text    NOT NULL CHECK (kind IN ('DOSSIER','TEXTE','PERSON','ORGANIZATION','COMMUNE','INDICATOR')),
  target_key  text    NOT NULL,   -- slug de la cible, résolu à l'usage
  provenance  core.provenance NOT NULL DEFAULT 'HUMAN_VERIFIED',
  added_by    text,
  note        text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (normalized, kind, target_key)
);
CREATE INDEX alias_normalized_trgm_idx ON ref.alias USING gin (normalized gin_trgm_ops);

-- +goose Down
DROP TABLE ref.alias;
DROP TABLE ref.unverifiable_reason;
DROP TABLE ref.indicator;
DROP TABLE ref.strate_demographique;
DROP TABLE ref.nuance_politique;
DROP TABLE ref.epci_competence;
DROP TABLE ref.epci_membership;
DROP TABLE ref.epci;
DROP TABLE ref.commune_change;
DROP TABLE ref.commune;
