-- +goose Up
-- Les intercommunalités, et surtout : qui exerce quoi.
--
-- Un EPCI — établissement public de coopération intercommunale — est une
-- structure à laquelle des communes transfèrent des compétences : l'eau, les
-- déchets, les transports, l'urbanisme, l'action sociale. Communauté de
-- communes, communauté d'agglomération, communauté urbaine, métropole et
-- syndicats en sont les formes.
--
-- Pourquoi cette table existe. Sans elle, un montant communal est illisible :
-- deux communes voisines peuvent afficher des « dépenses de fonctionnement »
-- écartées d'un facteur deux simplement parce que l'une a transféré la collecte
-- des déchets et l'autre non. Comparer leurs budgets sans le savoir, c'est
-- comparer deux périmètres et croire comparer deux politiques.
--
-- Le schéma prévoyait déjà un code EPCI_COMPETENCE pour écarter un indicateur
-- quand la compétence n'est pas communale. Il lui manquait la donnée.
CREATE TABLE ref.competence (
  code           text PRIMARY KEY,
  libelle        text NOT NULL,
  categorie_code text NOT NULL,
  categorie      text NOT NULL,
  ordre          integer NOT NULL DEFAULT 0
);

COMMENT ON TABLE ref.competence IS
  'Nomenclature des compétences transférables, publiée par la DGCL (BANATIC).';

CREATE TABLE core.epci (
  siren            text PRIMARY KEY,
  nom              text NOT NULL,
  nature_juridique text NOT NULL,
  code_departement text,
  date_creation    date,
  population_totale integer,
  nb_membres       integer,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  provenance       core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX epci_departement_idx ON core.epci (code_departement);
CREATE INDEX epci_nature_idx ON core.epci (nature_juridique);

COMMENT ON COLUMN core.epci.nature_juridique IS
  'CC, CA, CU, METRO, SIVU, SIVOM, SMF, SMO… — code publié par la DGCL, '
  'conservé tel quel : le regrouper serait une décision éditoriale.';

-- Une commune peut appartenir à plusieurs groupements : un EPCI à fiscalité
-- propre et plusieurs syndicats. Ce n'est pas une anomalie, c'est la règle.
CREATE TABLE core.epci_membre (
  epci_siren    text NOT NULL REFERENCES core.epci(siren) ON DELETE CASCADE,
  commune_code  text NOT NULL,
  cog_millesime integer NOT NULL,
  membre_siren  text,
  categorie     text,
  PRIMARY KEY (epci_siren, commune_code),
  FOREIGN KEY (commune_code, cog_millesime)
    REFERENCES ref.commune(code_insee, cog_millesime)
);

CREATE INDEX epci_membre_commune_idx ON core.epci_membre (commune_code);

CREATE TABLE core.epci_competence (
  epci_siren      text NOT NULL REFERENCES core.epci(siren) ON DELETE CASCADE,
  competence_code text NOT NULL REFERENCES ref.competence(code),
  PRIMARY KEY (epci_siren, competence_code)
);

CREATE INDEX epci_competence_code_idx ON core.epci_competence (competence_code);

-- Ce qu'une commune a effectivement transféré : la réunion des compétences de
-- tous les groupements auxquels elle adhère. Vue et non table : elle se déduit
-- entièrement des trois tables ci-dessus, et une copie pourrait diverger.
CREATE VIEW core.commune_competence AS
  SELECT DISTINCT m.commune_code, c.competence_code, m.epci_siren
    FROM core.epci_membre m
    JOIN core.epci_competence c ON c.epci_siren = m.epci_siren;

COMMENT ON VIEW core.commune_competence IS
  'Compétences exercées à la place d''une commune par l''un de ses groupements. '
  'Une ligne ici veut dire : sur ce sujet, le budget communal ne mesure pas la '
  'politique menée.';

-- +goose Down
DROP VIEW core.commune_competence;
DROP TABLE core.epci_competence;
DROP TABLE core.epci_membre;
DROP TABLE core.epci;
DROP TABLE ref.competence;
