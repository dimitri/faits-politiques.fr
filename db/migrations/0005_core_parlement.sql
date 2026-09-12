-- +goose Up
-- Travaux parlementaires.
--
-- L'unité d'agrégation réelle est le DOSSIER, pas le texte : un dossier porte
-- plusieurs textes, plusieurs lectures et plusieurs chambres. Un même texte est voté
-- quatre fois avec des contenus différents ; sans la lecture, deux scrutins ne sont
-- pas comparables.

CREATE TABLE core.legislature (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  institution core.institution NOT NULL,
  numero      int    NOT NULL,
  validity    daterange NOT NULL,
  UNIQUE (institution, numero)
);

CREATE TABLE core.dossier (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug           text NOT NULL UNIQUE,
  institution    core.institution NOT NULL,
  source_uid     text NOT NULL,          -- identifiant officiel : DLR5L17N12345
  legislature_id bigint REFERENCES core.legislature(id),
  titre          text NOT NULL,
  date_ouverture date,
  UNIQUE (institution, source_uid)
);
CREATE INDEX dossier_titre_trgm_idx ON core.dossier
  USING gin (core.f_unaccent(titre) gin_trgm_ops);

CREATE TABLE core.texte (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug        text NOT NULL UNIQUE,
  dossier_id  bigint NOT NULL REFERENCES core.dossier(id),
  institution core.institution NOT NULL,
  source_uid  text NOT NULL,
  kind        text NOT NULL CHECK (kind IN (
                'PROJET_DE_LOI','PROPOSITION_DE_LOI','PROPOSITION_DE_RESOLUTION',
                'MOTION','RAPPORT','AUTRE')),
  numero      text,
  titre       text NOT NULL,
  date_depot  date,
  UNIQUE (institution, source_uid)
);

-- Étape de la navette. Deux scrutins sur « le même texte » à deux lectures
-- différentes portent sur des contenus différents.
CREATE TABLE core.lecture (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dossier_id  bigint NOT NULL REFERENCES core.dossier(id),
  institution core.institution NOT NULL,
  rang        int    NOT NULL,        -- 1re lecture, 2e lecture...
  phase       text   NOT NULL CHECK (phase IN ('COMMISSION','SEANCE','CMP','LECTURE_DEFINITIVE')),
  UNIQUE (dossier_id, institution, rang, phase)
);

CREATE TABLE core.amendement (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug                text NOT NULL UNIQUE,
  texte_id            bigint REFERENCES core.texte(id),
  lecture_id          bigint REFERENCES core.lecture(id),
  institution         core.institution NOT NULL,
  source_uid          text NOT NULL,
  numero              text NOT NULL,
  article_designation text,
  sort                core.amendement_sort,
  expose_sommaire     text,
  dispositif          text,
  date_depot          date,
  UNIQUE (institution, source_uid)
);
CREATE INDEX amendement_texte_idx   ON core.amendement (texte_id);
CREATE INDEX amendement_lecture_idx ON core.amendement (lecture_id);

-- « Qui propose » est une dimension distincte de « qui vote ». Aucun outil existant
-- ne l'exploite, et c'est elle qui révèle les cosignatures transpartisanes.
-- L'auteur est une personne OU une organisation (gouvernement, commission), jamais
-- les deux : arc exclusif.
CREATE TABLE core.amendement_author (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  amendement_id   bigint NOT NULL REFERENCES core.amendement(id) ON DELETE CASCADE,
  person_id       bigint REFERENCES core.person(id),
  organization_id bigint REFERENCES core.organization(id),
  role            text NOT NULL CHECK (role IN ('AUTEUR','COSIGNATAIRE','RAPPORTEUR','GOUVERNEMENT','COMMISSION')),
  rang            int,
  CONSTRAINT author_is_person_xor_organization
    CHECK (num_nonnulls(person_id, organization_id) = 1)
);
CREATE INDEX amendement_author_amdt_idx   ON core.amendement_author (amendement_id);
CREATE INDEX amendement_author_person_idx ON core.amendement_author (person_id);

-- Attribution politique d'une proposition, calculée puis figée.
-- Seul UNAMBIGUOUS autorise à écrire « le groupe X a proposé ceci ».
CREATE TABLE core.amendement_attribution (
  amendement_id    bigint PRIMARY KEY REFERENCES core.amendement(id) ON DELETE CASCADE,
  organization_id  bigint REFERENCES core.organization(id),
  attribution      core.author_attribution NOT NULL,
  method_version   text NOT NULL,
  computed_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT org_required_unless_unresolved
    CHECK ((attribution = 'UNRESOLVED') = (organization_id IS NULL))
);

-- Un scrutin est un objet indépendant. granularite décrit ce que la SOURCE publie
-- réellement, pas ce qu'on aimerait avoir.
CREATE TABLE core.scrutin (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug            text NOT NULL UNIQUE,          -- permalien public et immuable
  institution     core.institution NOT NULL,
  legislature_id  bigint REFERENCES core.legislature(id),
  source_uid      text NOT NULL,
  numero          text,
  date_seance     date NOT NULL,
  granularite     core.scrutin_granularite NOT NULL,
  objet           text NOT NULL,                 -- intitulé officiel, transcrit tel quel
  type_vote       text,
  dossier_id      bigint REFERENCES core.dossier(id),
  texte_id        bigint REFERENCES core.texte(id),
  lecture_id      bigint REFERENCES core.lecture(id),
  amendement_id   bigint REFERENCES core.amendement(id),
  resultat        text CHECK (resultat IN ('ADOPTE','REJETE')),
  nb_votants      int,
  nb_pour         int,
  nb_contre       int,
  nb_abstentions  int,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (institution, source_uid),
  -- Cible des FK composites qui empêchent de mélanger les granularités.
  UNIQUE (id, granularite)
);
CREATE INDEX scrutin_date_idx     ON core.scrutin (date_seance DESC);
CREATE INDEX scrutin_dossier_idx  ON core.scrutin (dossier_id);
CREATE INDEX scrutin_objet_trgm_idx ON core.scrutin
  USING gin (core.f_unaccent(objet) gin_trgm_ops);

COMMENT ON COLUMN core.scrutin.objet IS
  'Intitulé officiel, transcrit sans reformulation. Toute reformulation destinée à '
  'l''affichage vit dans core.scrutin_resume, avec sa provenance et son relecteur.';

-- Positions nominatives. N'existe que pour les scrutins de granularité INDIVIDUAL :
-- la FK composite le garantit au niveau du schéma, pas par convention.
CREATE TABLE core.ballot (
  scrutin_id         bigint NOT NULL,
  granularite        core.scrutin_granularite NOT NULL DEFAULT 'INDIVIDUAL'
                     CHECK (granularite = 'INDIVIDUAL'),
  person_id          bigint NOT NULL REFERENCES core.person(id),
  position           core.vote_position NOT NULL,   -- position au relevé officiel
  par_delegation     boolean NOT NULL DEFAULT false,
  -- Mise au point : à l'AN, un député peut corriger son vote APRÈS le scrutin.
  -- Cela modifie le relevé nominatif sans modifier le résultat officiel.
  -- Stocker les deux est obligatoire, sinon on publie soit un vote faux,
  -- soit un résultat faux.
  position_rectifiee core.vote_position,
  rectifiee_le       date,
  PRIMARY KEY (scrutin_id, person_id),
  FOREIGN KEY (scrutin_id, granularite) REFERENCES core.scrutin(id, granularite),
  CONSTRAINT rectification_coherente
    CHECK ((position_rectifiee IS NULL) = (rectifiee_le IS NULL))
);
CREATE INDEX ballot_person_idx ON core.ballot (person_id);

-- Vue de lecture : la position telle qu'elle doit être affichée, plus le drapeau
-- qui impose de mentionner la mise au point sur la fiche.
CREATE VIEW core.ballot_effective AS
SELECT
  b.scrutin_id,
  b.person_id,
  b.position                                       AS position_officielle,
  COALESCE(b.position_rectifiee, b.position)       AS position_affichee,
  b.position_rectifiee IS NOT NULL                 AS a_fait_mise_au_point,
  b.rectifiee_le,
  b.par_delegation
FROM core.ballot b;

COMMENT ON VIEW core.ballot_effective IS
  'Les agrégats de résultat (nb_pour, etc.) se calculent sur position_officielle ; '
  'l''affichage du vote d''une personne utilise position_affichee et signale la mise au point.';

-- Positions par groupe. C'est le cas du Sénat : la source publie la position du
-- groupe et la liste nominative des exceptions. On ne déduit RIEN sur les autres
-- membres du groupe.
CREATE TABLE core.ballot_group (
  scrutin_id      bigint NOT NULL,
  granularite     core.scrutin_granularite NOT NULL DEFAULT 'GROUP'
                  CHECK (granularite = 'GROUP'),
  organization_id bigint NOT NULL REFERENCES core.organization(id),
  position        core.vote_position NOT NULL,
  effectif        int,
  nb_pour         int,
  nb_contre       int,
  nb_abstentions  int,
  nb_non_votants  int,
  PRIMARY KEY (scrutin_id, organization_id),
  FOREIGN KEY (scrutin_id, granularite) REFERENCES core.scrutin(id, granularite)
);

CREATE TABLE core.ballot_group_exception (
  scrutin_id      bigint NOT NULL,
  organization_id bigint NOT NULL,
  person_id       bigint NOT NULL REFERENCES core.person(id),
  position        core.vote_position NOT NULL,
  PRIMARY KEY (scrutin_id, organization_id, person_id),
  FOREIGN KEY (scrutin_id, organization_id)
    REFERENCES core.ballot_group(scrutin_id, organization_id) ON DELETE CASCADE
);

COMMENT ON TABLE core.ballot_group_exception IS
  'Les seules positions individuelles connaissables sur un scrutin de granularité GROUP. '
  'Pour tout autre membre du groupe, la réponse est GROUP_LEVEL_ONLY.';

-- Interventions en séance, pour vérifier « X a dit Y à l'Assemblée ».
-- Les propos tenus hors Parlement restent hors corpus (OUT_OF_CORPUS).
CREATE TABLE core.intervention (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug         text NOT NULL UNIQUE,
  institution  core.institution NOT NULL,
  source_uid   text NOT NULL,
  person_id    bigint REFERENCES core.person(id),
  dossier_id   bigint REFERENCES core.dossier(id),
  date_seance  date NOT NULL,
  contenu      text NOT NULL,
  UNIQUE (institution, source_uid)
);
CREATE INDEX intervention_person_idx ON core.intervention (person_id, date_seance DESC);
CREATE INDEX intervention_fts_idx ON core.intervention
  USING gin (to_tsvector('french', contenu));

-- Reformulation destinée à l'affichage, tenue à l'écart de l'objet officiel.
-- Séparée parce que c'est le principal point d'entrée du biais éditorial : elle
-- porte sa provenance, son rédacteur et ses relecteurs.
CREATE TABLE core.scrutin_resume (
  scrutin_id      bigint PRIMARY KEY REFERENCES core.scrutin(id) ON DELETE CASCADE,
  titre_neutre    text NOT NULL,
  description     text NOT NULL,
  ne_dit_pas      text NOT NULL,   -- section « ce que cette fiche ne dit pas »
  provenance      core.provenance NOT NULL,
  redige_par      text,
  relu_par        text[],          -- relecture contradictoire : au moins deux sensibilités
  verification    core.verification_status NOT NULL DEFAULT 'UNVERIFIED',
  updated_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT publie_seulement_si_verifie_humainement
    CHECK (verification <> 'HUMAN_VERIFIED' OR provenance = 'HUMAN_VERIFIED')
);

-- +goose Down
DROP TABLE core.scrutin_resume;
DROP TABLE core.intervention;
DROP TABLE core.ballot_group_exception;
DROP TABLE core.ballot_group;
DROP VIEW core.ballot_effective;
DROP TABLE core.ballot;
DROP TABLE core.scrutin;
DROP TABLE core.amendement_attribution;
DROP TABLE core.amendement_author;
DROP TABLE core.amendement;
DROP TABLE core.lecture;
DROP TABLE core.texte;
DROP TABLE core.dossier;
DROP TABLE core.legislature;
