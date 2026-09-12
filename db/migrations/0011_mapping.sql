-- +goose Up
-- Normalisation des rattachements : successions de partis et les trois ponts.
--
-- Décision structurante : une carte de rattachement n'est pas une table globale mais
-- une LIGNÉE nommée, avec une tête modifiable et des RÉVISIONS gelées. Elle peut être
-- créée par n'importe qui, sans compte, et forkée. La carte de référence n'est qu'une
-- lignée parmi d'autres.
--
-- Ce que cela permet : un contradicteur publie sa propre carte et l'oppose à la vôtre
-- ligne à ligne, au lieu de récuser l'ensemble. Ce que cela impose : un résultat publié
-- ne peut citer qu'une révision gelée, sinon il devient irreproductible dès que la
-- carte évolue.
--
-- Une carte est de la donnée structurée sur deux vocabulaires fermés — codes nuance du
-- RNE d'un côté, identifiants CNCCFP de l'autre. Elle n'énonce rien sur personne. C'est
-- pour cette raison, et pour cette raison seulement, qu'elle peut être anonyme.

CREATE TYPE core.lineage_kind AS ENUM ('REFERENCE','COMMUNITY');

CREATE TABLE core.mapping_lineage (
  id                      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug                    text NOT NULL UNIQUE,
  label                   text NOT NULL,
  kind                    core.lineage_kind NOT NULL,
  -- Édition sans compte : qui détient le jeton peut modifier. Seul le HACHÉ est
  -- stocké : aucune identité, aucun profil, aucune donnée personnelle.
  edit_token_hash         bytea,
  -- Non listée par défaut. N'importe qui crée et partage son URL ; seule l'équipe
  -- référence une carte dans un index. Le vandalisme perd tout intérêt puisqu'il
  -- n'apporte aucune audience, sans rien retirer à l'usage de contestation.
  listed                  boolean NOT NULL DEFAULT false,
  forked_from_revision_id bigint,
  note                    text,
  created_at              timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT edit_token_is_32_bytes
    CHECK (edit_token_hash IS NULL OR octet_length(edit_token_hash) = 32),
  CONSTRAINT reference_listee_et_sans_jeton
    CHECK (kind <> 'REFERENCE' OR (listed AND edit_token_hash IS NULL))
);

-- Exactement une carte de référence, tenue par l'équipe.
CREATE UNIQUE INDEX mapping_lineage_une_seule_reference
  ON core.mapping_lineage ((kind)) WHERE kind = 'REFERENCE';

CREATE TABLE core.mapping_revision (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  lineage_id   bigint NOT NULL REFERENCES core.mapping_lineage(id) ON DELETE CASCADE,
  revision     int    NOT NULL,
  note         text,
  content_hash bytea,          -- NULL tant que la révision est la tête modifiable
  frozen_at    timestamptz,    -- idem
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (lineage_id, revision),
  CONSTRAINT gel_coherent CHECK ((frozen_at IS NULL) = (content_hash IS NULL)),
  CONSTRAINT hash_is_32_bytes CHECK (content_hash IS NULL OR octet_length(content_hash) = 32)
);

-- Une seule tête modifiable par lignée : « faire évoluer une carte » ne doit jamais
-- rendre irreproductible un résultat déjà publié.
CREATE UNIQUE INDEX mapping_revision_une_seule_tete
  ON core.mapping_revision (lineage_id) WHERE frozen_at IS NULL;

ALTER TABLE core.mapping_lineage
  ADD CONSTRAINT fork_pointe_une_revision
  FOREIGN KEY (forked_from_revision_id) REFERENCES core.mapping_revision(id);

-- Vocabulaire fermé de justification, à la place d'un champ de texte libre : c'était
-- la seule porte par laquelle de la prose pouvait entrer dans un objet censé n'en pas
-- contenir. Effet secondaire précieux : deux cartes deviennent comparables ligne à
-- ligne, et une divergence se lit d'un coup d'œil.
CREATE TABLE ref.rationale_code (
  code  text PRIMARY KEY,
  label text NOT NULL
);
INSERT INTO ref.rationale_code (code, label) VALUES
 ('LIBELLE_NOMME_LE_PARTI','Le libellé officiel de la nuance nomme ce parti'),
 ('USAGE_CONSTANT',        'Usage constant et documenté dans les sources officielles'),
 ('REVENDIQUE_PAR_PARTI',  'Rattachement revendiqué publiquement par le parti'),
 ('COALITION_DOMINANTE',   'Liste d''union dont ce parti est la composante principale'),
 ('FAMILLE_PLUS_LARGE',    'La nuance recouvre une famille plus large que ce parti'),
 ('NON_ATTRIBUABLE',       'Aucune attribution défendable'),
 ('ARBITRAIRE_ASSUME',     'Décision arbitraire, assumée comme telle par son auteur');

-- Successions de partis. Sans ce graphe, un parti « disparaît » à sa date de
-- renommage et toute série longue se scinde en deux.
CREATE TYPE core.succession_type AS ENUM ('RENOMMAGE','FUSION','SCISSION','ABSORPTION','DISSOLUTION');

CREATE TABLE core.organization_succession (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  predecessor_id  bigint NOT NULL REFERENCES core.organization(id),
  successor_id    bigint REFERENCES core.organization(id),  -- NULL si dissolution
  succession_type core.succession_type NOT NULL,
  effective_date  date   NOT NULL,
  note            text,
  CONSTRAINT successeur_sauf_dissolution
    CHECK ((succession_type = 'DISSOLUTION') = (successor_id IS NULL)),
  CONSTRAINT pas_sa_propre_suite
    CHECK (successor_id IS DISTINCT FROM predecessor_id),
  UNIQUE (predecessor_id, successor_id, effective_date)
);
CREATE INDEX succession_pred_idx ON core.organization_succession (predecessor_id);
CREATE INDEX succession_succ_idx ON core.organization_succession (successor_id);

-- Pont 1 — parti <-> groupe parlementaire. Relation n:n datée : un groupe agrège
-- plusieurs partis, et un apparenté n'est pas un membre.
CREATE TABLE core.party_group_link (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  mapping_revision_id bigint NOT NULL REFERENCES core.mapping_revision(id) ON DELETE CASCADE,
  party_id            bigint NOT NULL,
  party_kind          core.organization_kind NOT NULL DEFAULT 'PARTY' CHECK (party_kind = 'PARTY'),
  group_id            bigint NOT NULL,
  group_kind          core.organization_kind NOT NULL
                      CHECK (group_kind IN ('PARLIAMENTARY_GROUP','EP_GROUP')),
  relation            text   NOT NULL CHECK (relation IN ('COMPOSANTE','APPARENTE','MAJORITAIRE')),
  validity            daterange NOT NULL,
  rationale_code      text REFERENCES ref.rationale_code(code),
  created_at          timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (party_id, party_kind) REFERENCES core.organization(id, kind),
  FOREIGN KEY (group_id, group_kind) REFERENCES core.organization(id, kind)
);
CREATE INDEX party_group_link_revision_idx ON core.party_group_link (mapping_revision_id);

-- Pont 2 — parti national d'un eurodéputé tel que publié par le Parlement européen
-- (un libellé), rapproché de l'entrée CNCCFP correspondante.
-- Le CNCCFP est un registre de PARTIS, pas d'affiliations : il ne contient aucun
-- individu. Le lien personne -> parti national vient du Parlement européen lui-même.
CREATE TABLE core.ep_national_party_link (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  mapping_revision_id bigint NOT NULL REFERENCES core.mapping_revision(id) ON DELETE CASCADE,
  ep_party_label      text   NOT NULL,
  party_id            bigint NOT NULL,
  party_kind          core.organization_kind NOT NULL DEFAULT 'PARTY' CHECK (party_kind = 'PARTY'),
  rationale_code      text REFERENCES ref.rationale_code(code),
  FOREIGN KEY (party_id, party_kind) REFERENCES core.organization(id, kind),
  UNIQUE (mapping_revision_id, ep_party_label)
);

-- Pont 3 — nuance RNE <-> parti. Le point fragile de tout l'édifice.
-- Ce n'est pas une relation mais quatre, et les confondre ruine le volet local.
CREATE TYPE core.nuance_mapping_quality AS ENUM (
  'EXACT',         -- la nuance désigne ce parti et lui seul
  'COALITION',     -- la nuance désigne une liste d'union incluant le parti
  'BROADER',       -- la nuance est une famille plus large que le parti
  'NOT_MAPPABLE'   -- divers, sans étiquette, sous le seuil d'attribution
);

CREATE TABLE core.nuance_party_link (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  mapping_revision_id  bigint NOT NULL REFERENCES core.mapping_revision(id) ON DELETE CASCADE,
  nuance_code          text   NOT NULL,
  circulaire_millesime int    NOT NULL,
  party_id             bigint,           -- NULL obligatoire si NOT_MAPPABLE
  party_kind           core.organization_kind NOT NULL DEFAULT 'PARTY' CHECK (party_kind = 'PARTY'),
  qualification        core.nuance_mapping_quality NOT NULL,
  -- Colonne calculée : l'agrégeabilité devient déclarative et indexable, au lieu de
  -- dépendre de la vigilance de qui écrit la requête.
  aggregatable         boolean GENERATED ALWAYS AS (qualification = 'EXACT') STORED,
  rationale_code       text REFERENCES ref.rationale_code(code),
  rationale_note       text,
  created_at           timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (nuance_code, circulaire_millesime)
    REFERENCES ref.nuance_politique(code, circulaire_millesime),
  FOREIGN KEY (party_id, party_kind) REFERENCES core.organization(id, kind),
  CONSTRAINT parti_absent_ssi_non_mappable
    CHECK ((qualification = 'NOT_MAPPABLE') = (party_id IS NULL)),
  CONSTRAINT justification_codee_si_exact
    CHECK (qualification <> 'EXACT' OR rationale_code IS NOT NULL)
);

-- Dans une révision donnée, une nuance ne peut désigner exactement qu'UN parti.
-- Les qualifications plus faibles peuvent être multiples : une nuance de coalition
-- recouvre légitimement plusieurs partis.
CREATE UNIQUE INDEX nuance_party_one_exact_per_revision
  ON core.nuance_party_link (mapping_revision_id, nuance_code, circulaire_millesime)
  WHERE qualification = 'EXACT';
CREATE INDEX nuance_party_link_revision_idx ON core.nuance_party_link (mapping_revision_id);

-- Une révision gelée est en lecture seule : c'est ce qui rend une comparaison
-- reproductible. On crée une nouvelle révision, on ne réécrit pas l'ancienne.
-- +goose StatementBegin
CREATE FUNCTION core.assert_revision_not_frozen() RETURNS trigger
LANGUAGE plpgsql AS $fn$
DECLARE
  v_rev bigint;
  v_frozen timestamptz;
BEGIN
  v_rev := COALESCE(NEW.mapping_revision_id, OLD.mapping_revision_id);
  SELECT frozen_at INTO v_frozen FROM core.mapping_revision WHERE id = v_rev;
  IF v_frozen IS NOT NULL THEN
    RAISE EXCEPTION
      'Révision % gelée le % : créer une nouvelle révision plutôt que la modifier.', v_rev, v_frozen;
  END IF;
  RETURN COALESCE(NEW, OLD);
END;
$fn$;
-- +goose StatementEnd

CREATE TRIGGER nuance_party_link_respecte_le_gel
  BEFORE INSERT OR UPDATE OR DELETE ON core.nuance_party_link
  FOR EACH ROW EXECUTE FUNCTION core.assert_revision_not_frozen();
CREATE TRIGGER party_group_link_respecte_le_gel
  BEFORE INSERT OR UPDATE OR DELETE ON core.party_group_link
  FOR EACH ROW EXECUTE FUNCTION core.assert_revision_not_frozen();
CREATE TRIGGER ep_national_party_link_respecte_le_gel
  BEFORE INSERT OR UPDATE OR DELETE ON core.ep_national_party_link
  FOR EACH ROW EXECUTE FUNCTION core.assert_revision_not_frozen();

-- Comment on sait qu'une personne appartient à un parti. Trois canaux au statut très
-- différent : le rattachement JO est déclaré par l'élu, la nuance RNE est attribuée
-- par la préfecture — elle ne produit JAMAIS d'affiliation.
CREATE TYPE core.affiliation_source AS ENUM (
  'JO_RATTACHEMENT',    -- seconde fraction de l'aide publique, publié au JO en décembre
  'EP_DECLARATION',     -- parti national publié par le Parlement européen
  'INSTITUTION',        -- appartenance à un groupe, publiée par l'assemblée
  'PARTY_DECLARATION'   -- déclaratif : niveau 3 de la hiérarchie des sources
);

ALTER TABLE core.affiliation
  ADD COLUMN declared_via core.affiliation_source NOT NULL DEFAULT 'INSTITUTION';

COMMENT ON COLUMN core.affiliation.declared_via IS
  'La nuance RNE ne figure pas dans cette énumération : c''est délibéré. Une '
  'qualification préfectorale ne crée pas une appartenance partisane.';

-- Le seul chemin d'agrégation nuance -> parti.
CREATE VIEW core.nuance_party_exact AS
SELECT mapping_revision_id, nuance_code, circulaire_millesime, party_id,
       rationale_code, rationale_note
FROM core.nuance_party_link
WHERE aggregatable;

COMMENT ON VIEW core.nuance_party_exact IS
  'Toute agrégation par parti au niveau local passe par ici et nulle part ailleurs. '
  'COALITION, BROADER et NOT_MAPPABLE sont affichées sur les fiches mais comptées '
  'dans le taux d''exclusion.';

CREATE VIEW core.commune_party AS
SELECT
  npe.mapping_revision_id,
  m.commune_code,
  npe.party_id,
  m.person_id,
  m.validity,
  na.nuance_code,
  na.circulaire_millesime
FROM core.mandate m
JOIN core.nuance_assignment na ON na.mandate_id = m.id
JOIN core.nuance_party_exact npe
  ON npe.nuance_code = na.nuance_code
 AND npe.circulaire_millesime = na.circulaire_millesime
WHERE m.mandate_type = 'MAIRE';

-- Affiliation partisane effective d'une personne, avec sa source et sa confiance.
-- Précédence : déclaration de l'élu > déclaration de parti. La nuance n'apparaît pas.
CREATE VIEW core.person_party_effective AS
SELECT
  a.person_id,
  a.organization_id AS party_id,
  a.validity,
  a.declared_via,
  CASE a.declared_via
    WHEN 'JO_RATTACHEMENT'   THEN 'HAUTE'
    WHEN 'EP_DECLARATION'    THEN 'HAUTE'
    WHEN 'INSTITUTION'       THEN 'MOYENNE'
    WHEN 'PARTY_DECLARATION' THEN 'FAIBLE'
  END AS confiance
FROM core.affiliation a
WHERE a.organization_kind = 'PARTY';

ALTER TABLE derived.comparison_run
  ADD COLUMN mapping_revision_id bigint REFERENCES core.mapping_revision(id);

-- Un résultat publié ne peut citer qu'une révision GELÉE, jamais une tête modifiable.
-- +goose StatementBegin
CREATE FUNCTION core.assert_revision_frozen_for_citation() RETURNS trigger
LANGUAGE plpgsql AS $fn$
DECLARE v_frozen timestamptz;
BEGIN
  IF NEW.mapping_revision_id IS NULL THEN RETURN NEW; END IF;
  SELECT frozen_at INTO v_frozen FROM core.mapping_revision WHERE id = NEW.mapping_revision_id;
  IF v_frozen IS NULL THEN
    RAISE EXCEPTION
      'Révision % non gelée : un résultat publié ne peut pas citer une carte encore modifiable.',
      NEW.mapping_revision_id;
  END IF;
  RETURN NEW;
END;
$fn$;
-- +goose StatementEnd

CREATE TRIGGER comparison_run_cite_une_revision_gelee
  BEFORE INSERT OR UPDATE ON derived.comparison_run
  FOR EACH ROW EXECUTE FUNCTION core.assert_revision_frozen_for_citation();

-- +goose Down
DROP TRIGGER comparison_run_cite_une_revision_gelee ON derived.comparison_run;
DROP FUNCTION core.assert_revision_frozen_for_citation();
ALTER TABLE derived.comparison_run DROP COLUMN mapping_revision_id;
DROP VIEW core.person_party_effective;
DROP VIEW core.commune_party;
DROP VIEW core.nuance_party_exact;
ALTER TABLE core.affiliation DROP COLUMN declared_via;
DROP TYPE core.affiliation_source;
DROP TRIGGER ep_national_party_link_respecte_le_gel ON core.ep_national_party_link;
DROP TRIGGER party_group_link_respecte_le_gel ON core.party_group_link;
DROP TRIGGER nuance_party_link_respecte_le_gel ON core.nuance_party_link;
DROP FUNCTION core.assert_revision_not_frozen();
DROP TABLE core.nuance_party_link;
DROP TYPE core.nuance_mapping_quality;
DROP TABLE core.ep_national_party_link;
DROP TABLE core.party_group_link;
DROP TABLE core.organization_succession;
DROP TYPE core.succession_type;
-- Les deux tables se référencent mutuellement (une lignée peut être le fork d'une
-- révision) : lever la contrainte avant de démonter.
ALTER TABLE core.mapping_lineage DROP CONSTRAINT fork_pointe_une_revision;
DROP TABLE core.mapping_revision;
DROP TABLE core.mapping_lineage;
DROP TYPE core.lineage_kind;
DROP TABLE ref.rationale_code;
