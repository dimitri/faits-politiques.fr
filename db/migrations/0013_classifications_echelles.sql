-- +goose Up
-- Quatre ajouts liés :
--   A. Classes de réutilisation (le projet devient non commercial, mais la
--      restriction ne doit pas contaminer silencieusement les exports)
--   B. Classifications tierces de partis (PopuList, CHES) via Party Facts
--   C. Auteurs des textes — lacune : seuls les amendements en avaient
--   D. Échelles et codage de sens, forkables comme les cartes de rattachement

------------------------------------------------------------------------------
-- A. Classes de réutilisation
------------------------------------------------------------------------------
-- La contrainte binaire « commercial_use obligatoire » est remplacée par une classe
-- par source. Motif : ingérer une source non commerciale contamine les données qui
-- en dérivent, et un journaliste d'un média commercial ne pourrait plus les réutiliser
-- — or c'est un des publics visés. On garde donc la trace de la restriction plutôt
-- que de la dissoudre, et les exports se filtrent.

CREATE TYPE core.reuse_class AS ENUM (
  'OPEN',            -- domaine public ou équivalent
  'ATTRIBUTION',     -- Licence Ouverte, CC BY, ODbL : redistribuable, y compris commercialement
  'NON_COMMERCIAL',  -- CC BY-NC et assimilés
  'RESTRICTED'       -- usage scientifique, licence ad hoc, ou licence non explicitée
);

ALTER TABLE raw.source DROP CONSTRAINT source_must_allow_commercial_use;
ALTER TABLE raw.source ADD COLUMN reuse_class core.reuse_class NOT NULL DEFAULT 'ATTRIBUTION';
ALTER TABLE raw.source DROP COLUMN commercial_use;
ALTER TABLE raw.source
  ADD COLUMN commercial_use boolean
  GENERATED ALWAYS AS (reuse_class IN ('OPEN','ATTRIBUTION')) STORED;

COMMENT ON COLUMN raw.source.reuse_class IS
  'RESTRICTED couvre aussi les sources SANS licence explicite : une absence de licence '
  'n''est pas une autorisation. C''est le cas de CHES, qui n''en publie pas.';

-- Le sous-ensemble librement redistribuable. Tout export public passe par ici :
-- ce qui n'y figure pas peut être affiché sur le site, jamais reversé dans un export.
CREATE VIEW raw.source_redistribuable AS
SELECT * FROM raw.source WHERE reuse_class IN ('OPEN','ATTRIBUTION');

------------------------------------------------------------------------------
-- B. Classifications tierces de partis
------------------------------------------------------------------------------
-- Ces référentiels ne sont pas des faits : ce sont des jugements d'experts ou de
-- codeurs. Ils sont stockés AVEC leur vocabulaire et leur millésime, et ne deviennent
-- jamais un attribut du parti.

-- Party Facts est la table de correspondance entre tous ces jeux : c'est par elle
-- qu'on rapproche un parti français d'une ligne PopuList ou CHES, jamais par le nom.
ALTER TABLE core.organization_identifier DROP CONSTRAINT organization_identifier_scheme_check;
ALTER TABLE core.organization_identifier
  ADD CONSTRAINT organization_identifier_scheme_check
  CHECK (scheme IN ('AN_ORGANE','SENAT_GROUPE','EP_GROUP','RNE_NUANCE','WIKIDATA',
                    'CNCCFP','PARTYFACTS','POPULIST','CHES','PARLGOV','MANIFESTO'));

CREATE TABLE ref.classification_set (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug         text NOT NULL UNIQUE,
  provider     text NOT NULL,         -- 'PopuList', 'CHES'
  version      text NOT NULL,         -- 'v4.0', '2024'
  label        text NOT NULL,
  vocabulary   text NOT NULL,         -- vocabulaire propre, affiché tel quel
  published_on date,
  source_id    bigint REFERENCES raw.source(id),
  UNIQUE (provider, version)
);

COMMENT ON COLUMN ref.classification_set.vocabulary IS
  'Le vocabulaire de la source est affiché sans traduction. « far right » n''est pas '
  '« extrême droite » : la littérature distingue radical (rejette la démocratie '
  'libérale, accepte l''élection) et extrême (rejette la démocratie), distinction que '
  'l''usage français ignore.';

CREATE TABLE core.party_classification (
  id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  classification_set_id bigint NOT NULL REFERENCES ref.classification_set(id) ON DELETE CASCADE,
  party_id              bigint NOT NULL,
  party_kind            core.organization_kind NOT NULL DEFAULT 'PARTY' CHECK (party_kind = 'PARTY'),
  -- Catégoriel (PopuList : far-left, far-right, populist) OU numérique (CHES : lrgen,
  -- galtan), jamais les deux : ce ne sont pas les mêmes objets.
  category              text,
  dimension             text,
  value                 numeric,
  period_year           int,
  FOREIGN KEY (party_id, party_kind) REFERENCES core.organization(id, kind),
  CONSTRAINT categoriel_xor_numerique CHECK (
    (category IS NOT NULL AND dimension IS NULL AND value IS NULL) OR
    (category IS NULL AND dimension IS NOT NULL AND value IS NOT NULL)
  ),
  UNIQUE (classification_set_id, party_id, category, dimension, period_year)
);
CREATE INDEX party_classification_party_idx ON core.party_classification (party_id);

-- Quand deux référentiels divergent sur un parti, les deux sont affichés. Aucune
-- synthèse : moyenner deux classifications fabrique un jugement et le présente
-- comme un fait. Cette vue expose les divergences plutôt que de les masquer.
CREATE VIEW core.party_classification_divergence AS
SELECT party_id, category, count(DISTINCT classification_set_id) AS nb_referentiels,
       array_agg(DISTINCT classification_set_id) AS referentiels
FROM core.party_classification
WHERE category IS NOT NULL
GROUP BY party_id, category;

------------------------------------------------------------------------------
-- C. Auteurs des textes
------------------------------------------------------------------------------
-- Lacune : seuls les amendements avaient une table d'auteurs. Une proposition de loi
-- a pourtant des auteurs et des cosignataires, et « qui propose » est la dimension
-- que les outils existants n'exploitent pas.
CREATE TABLE core.texte_author (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  texte_id        bigint NOT NULL REFERENCES core.texte(id) ON DELETE CASCADE,
  person_id       bigint REFERENCES core.person(id),
  organization_id bigint REFERENCES core.organization(id),
  role            text NOT NULL CHECK (role IN ('AUTEUR','COSIGNATAIRE','RAPPORTEUR','GOUVERNEMENT','COMMISSION')),
  rang            int,
  CONSTRAINT author_is_person_xor_organization
    CHECK (num_nonnulls(person_id, organization_id) = 1)
);
CREATE INDEX texte_author_texte_idx  ON core.texte_author (texte_id);
CREATE INDEX texte_author_person_idx ON core.texte_author (person_id);

------------------------------------------------------------------------------
-- D. Échelles et codage de sens
------------------------------------------------------------------------------
-- Classer un objet sous un THÈME est une affectation (core.topic_assignment).
-- Le POSITIONNER sur une échelle suppose en plus de décider quel côté renforce et
-- quel côté affaiblit : c'est un acte éditorial, pas un fait.
-- Il vit donc dans une révision de carte, avec la même mécanique de gel et de fork.

CREATE TABLE ref.scale (
  code          text PRIMARY KEY,               -- 'INSTITUTIONS'
  topic_code    text NOT NULL REFERENCES ref.topic(code),
  label         text NOT NULL,
  -- Les pôles sont décrits LITTÉRALEMENT, jamais par une valeur. Un axe nommé par un
  -- jugement fait perdre le procès en neutralité ; nommé par son contenu, il le rend
  -- sans objet.
  pole_negatif  text NOT NULL,
  pole_positif  text NOT NULL,
  definition    text NOT NULL
);

CREATE TABLE core.scale_coding (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  -- Réutilise la machinerie des cartes : lignée nommée, révisions gelées, fork.
  -- Un codage de sens est exactement le même genre d'objet qu'un rattachement.
  mapping_revision_id bigint NOT NULL REFERENCES core.mapping_revision(id) ON DELETE CASCADE,
  scale_code          text   NOT NULL REFERENCES ref.scale(code),

  scrutin_id          bigint REFERENCES core.scrutin(id) ON DELETE CASCADE,
  amendement_id       bigint REFERENCES core.amendement(id) ON DELETE CASCADE,
  texte_id            bigint REFERENCES core.texte(id) ON DELETE CASCADE,

  direction           smallint NOT NULL CHECK (direction IN (-1, 1)),
  poids               numeric  NOT NULL DEFAULT 1 CHECK (poids > 0),
  -- Un objet dont les dispositions vont en sens contraires : affiché, jamais scoré.
  -- C'est le cas courant d'une loi, qui est un assemblage, là où un scrutin est un
  -- choix binaire sur un objet précis à un instant précis.
  composite           boolean NOT NULL DEFAULT false,
  scorable            boolean GENERATED ALWAYS AS (NOT composite) STORED,
  rationale_code      text REFERENCES ref.rationale_code(code),
  rationale_note      text,
  relu_par            text[],        -- relecture contradictoire : sensibilités opposées
  created_at          timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT coding_designe_exactement_un_objet CHECK (
    num_nonnulls(scrutin_id, amendement_id, texte_id) = 1
  ),
  CONSTRAINT justification_obligatoire CHECK (rationale_code IS NOT NULL)
);

-- Un objet ne peut porter qu'un codage par échelle et par révision.
CREATE UNIQUE INDEX scale_coding_unique_scrutin ON core.scale_coding
  (mapping_revision_id, scale_code, scrutin_id) WHERE scrutin_id IS NOT NULL;
CREATE UNIQUE INDEX scale_coding_unique_amendement ON core.scale_coding
  (mapping_revision_id, scale_code, amendement_id) WHERE amendement_id IS NOT NULL;
CREATE UNIQUE INDEX scale_coding_unique_texte ON core.scale_coding
  (mapping_revision_id, scale_code, texte_id) WHERE texte_id IS NOT NULL;
CREATE INDEX scale_coding_revision_idx ON core.scale_coding (mapping_revision_id, scale_code);

-- Le gel s'applique au codage comme aux rattachements : la fonction existante est
-- déclenchée par mapping_revision_id, donc elle s'applique telle quelle.
CREATE TRIGGER scale_coding_respecte_le_gel
  BEFORE INSERT OR UPDATE OR DELETE ON core.scale_coding
  FOR EACH ROW EXECUTE FUNCTION core.assert_revision_not_frozen();

-- Le seul chemin de calcul d'un score : les objets composites en sont exclus.
CREATE VIEW core.scale_coding_scorable AS
SELECT * FROM core.scale_coding WHERE scorable;

COMMENT ON VIEW core.scale_coding_scorable IS
  'Un texte est un assemblage de dispositions qui peuvent aller en sens contraires. '
  'Coder « la loi » plutôt que l''objet soumis au vote est la principale erreur à '
  'éviter : le marqueur composite la rend visible et exclut l''objet du score.';

-- Codes de justification propres au codage de sens.
INSERT INTO ref.rationale_code (code, label) VALUES
 ('OBJET_UNIQUE_EXPLICITE','L''objet soumis au vote porte une disposition unique et explicite'),
 ('DISPOSITIONS_CONTRAIRES','L''objet mêle des dispositions de sens opposés'),
 ('EXPOSE_DES_MOTIFS','Le sens ressort de l''exposé des motifs de l''objet lui-même'),
 ('AVIS_JURIDICTIONNEL','Le sens ressort d''un avis du Conseil d''État ou du Conseil constitutionnel');

-- Échelle INSTITUTIONS, adossée au protocole 002.
INSERT INTO ref.scale (code, topic_code, label, pole_negatif, pole_positif, definition) VALUES
 ('INSTITUTIONS','INSTITUTIONS','Contre-pouvoirs institutionnels',
  'A voté pour restreindre le champ, la saisine ou les moyens du contre-pouvoir concerné',
  'A voté pour étendre le champ, la saisine ou les moyens du contre-pouvoir concerné',
  'Portée : indépendance de la justice, liberté de la presse, contrôle constitutionnel, '
  'autorités administratives indépendantes, conventions relatives aux droits fondamentaux, '
  'révisions constitutionnelles. Les pôles sont décrits par leur contenu et ne portent '
  'aucun jugement de valeur.');

-- +goose Down
DELETE FROM ref.scale WHERE code = 'INSTITUTIONS';
DELETE FROM ref.rationale_code WHERE code IN
 ('OBJET_UNIQUE_EXPLICITE','DISPOSITIONS_CONTRAIRES','EXPOSE_DES_MOTIFS','AVIS_JURIDICTIONNEL');
DROP VIEW core.scale_coding_scorable;
DROP TRIGGER scale_coding_respecte_le_gel ON core.scale_coding;
DROP TABLE core.scale_coding;
DROP TABLE ref.scale;
DROP TABLE core.texte_author;
DROP VIEW core.party_classification_divergence;
DROP TABLE core.party_classification;
DROP TABLE ref.classification_set;
ALTER TABLE core.organization_identifier DROP CONSTRAINT organization_identifier_scheme_check;
ALTER TABLE core.organization_identifier
  ADD CONSTRAINT organization_identifier_scheme_check
  CHECK (scheme IN ('AN_ORGANE','SENAT_GROUPE','EP_GROUP','RNE_NUANCE','WIKIDATA'));
DROP VIEW raw.source_redistribuable;
ALTER TABLE raw.source DROP COLUMN commercial_use;
ALTER TABLE raw.source DROP COLUMN reuse_class;
DROP TYPE core.reuse_class;
ALTER TABLE raw.source ADD COLUMN commercial_use boolean NOT NULL DEFAULT true;
ALTER TABLE raw.source ADD CONSTRAINT source_must_allow_commercial_use CHECK (commercial_use);
