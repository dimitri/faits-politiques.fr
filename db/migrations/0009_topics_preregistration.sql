-- +goose Up
-- Ferme les sept manques identifiés sur le cas d'usage « action municipale par
-- étiquette » et « votes par thème » :
--   1. périmètre budgétaire (piège CCAS)     5. rattachement indicateur -> mandat
--   2. tarifs municipaux collectés à la main 6. pré-enregistrement scellé
--   3. taxonomie thématique                  7. schéma des filtres -> voir 0012
--   4. Congrès de Versailles
--
-- ALTER TYPE ... ADD VALUE est autorisé dans une transaction depuis PG 12, à
-- condition que la valeur ajoutée ne soit pas UTILISÉE dans la même transaction.
-- Aucune insertion de cette migration n'emploie 'CONGRES' : c'est volontaire.

-- 1. Le Congrès n'est ni l'Assemblée ni le Sénat. La constitutionnalisation de l'IVG
--    y a été votée en mars 2024 : sans cette valeur, ce scrutin est inclassable.
ALTER TYPE core.institution ADD VALUE 'CONGRES';

-- 2. Périmètre budgétaire.
--    Le CCAS est un établissement public autonome : son budget n'est PAS celui de la
--    commune, qui n'en porte que la subvention d'équilibre. Sans cette colonne, une
--    baisse d'action sociale est soit invisible, soit comptée deux fois.
CREATE TYPE core.budget_scope AS ENUM (
  'COMMUNE',
  'CCAS',
  'CAISSE_DES_ECOLES',
  'BUDGET_ANNEXE',
  'EPCI'
);

ALTER TABLE core.commune_indicator
  ADD COLUMN budget_scope core.budget_scope NOT NULL DEFAULT 'COMMUNE';

ALTER TABLE core.commune_indicator
  DROP CONSTRAINT commune_indicator_commune_code_indicator_code_period_year_key;

ALTER TABLE core.commune_indicator
  ADD CONSTRAINT commune_indicator_unique_par_perimetre
  UNIQUE (commune_code, budget_scope, indicator_code, period_year);

COMMENT ON COLUMN core.commune_indicator.budget_scope IS
  'Entité dont relève réellement la dépense. Agréger COMMUNE et CCAS sans le savoir '
  'produit un chiffre faux dans les deux sens.';

-- 3. Tarifs municipaux.
--    Aucune source nationale n'existe : la cantine, exemple le plus cité du débat,
--    n'est publiée nulle part. Collecte manuelle, une délibération en preuve, sur un
--    petit nombre de communes. Volontairement séparé de commune_indicator : ce n'est
--    pas une donnée de producteur public.
CREATE TABLE core.municipal_tariff (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug           text    NOT NULL UNIQUE,
  commune_code   text    NOT NULL,
  cog_millesime  int     NOT NULL,
  service        text    NOT NULL CHECK (service IN
                   ('CANTINE','PERISCOLAIRE','CRECHE','CENTRE_LOISIRS',
                    'PISCINE','BIBLIOTHEQUE','CONSERVATOIRE','STATIONNEMENT','AUTRE')),
  tranche        text,                 -- tranche de quotient familial, ou NULL si tarif unique
  quotient_min   numeric,
  quotient_max   numeric,
  montant        numeric NOT NULL,
  unite          text    NOT NULL,     -- 'EUR_PAR_REPAS', 'EUR_PAR_MOIS'...
  applicable_from date   NOT NULL,
  applicable_to   date,
  deliberation_ref text,               -- numéro de délibération
  provenance     core.provenance NOT NULL DEFAULT 'HUMAN_VERIFIED',
  collected_by   text,
  created_at     timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (commune_code, cog_millesime) REFERENCES ref.commune(code_insee, cog_millesime),
  CONSTRAINT periode_coherente CHECK (applicable_to IS NULL OR applicable_to > applicable_from)
);
CREATE INDEX municipal_tariff_lookup_idx
  ON core.municipal_tariff (commune_code, service, applicable_from DESC);

-- 4. Taxonomie thématique.
--    Il n'existe aucune classification officielle des scrutins. Ranger un texte sous
--    « libertés individuelles » plutôt que « sécurité » oriente le résultat : c'est le
--    principal point d'entrée du biais. D'où : taxonomie versionnée, publiée, et
--    provenance tracée sur chaque affectation.
CREATE TABLE ref.taxonomy (
  version      text PRIMARY KEY,
  label        text NOT NULL,
  published_at date NOT NULL,
  url          text,
  frozen       boolean NOT NULL DEFAULT false
);

CREATE TABLE ref.topic (
  code             text PRIMARY KEY,
  taxonomy_version text NOT NULL REFERENCES ref.taxonomy(version),
  label            text NOT NULL,
  parent_code      text REFERENCES ref.topic(code),
  definition       text NOT NULL,   -- critère d'inclusion, pour que l'affectation soit contestable
  CONSTRAINT pas_son_propre_parent CHECK (parent_code IS DISTINCT FROM code)
);

-- Affectation thématique. On classe de préférence le DOSSIER et non le scrutin :
-- classer scrutin par scrutin multiplie les décisions discutables.
CREATE TABLE core.topic_assignment (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  topic_code    text   NOT NULL REFERENCES ref.topic(code),
  dossier_id    bigint REFERENCES core.dossier(id) ON DELETE CASCADE,
  texte_id      bigint REFERENCES core.texte(id) ON DELETE CASCADE,
  amendement_id bigint REFERENCES core.amendement(id) ON DELETE CASCADE,
  scrutin_id    bigint REFERENCES core.scrutin(id) ON DELETE CASCADE,
  provenance    core.provenance NOT NULL,
  verification  core.verification_status NOT NULL DEFAULT 'UNVERIFIED',
  assigned_by   text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT topic_assignment_has_exactly_one_subject CHECK (
    num_nonnulls(dossier_id, texte_id, amendement_id, scrutin_id) = 1
  )
);
CREATE INDEX topic_assignment_topic_idx   ON core.topic_assignment (topic_code);
CREATE INDEX topic_assignment_dossier_idx ON core.topic_assignment (dossier_id) WHERE dossier_id IS NOT NULL;
CREATE INDEX topic_assignment_scrutin_idx ON core.topic_assignment (scrutin_id) WHERE scrutin_id IS NOT NULL;

-- 5. Rattachement d'une valeur annuelle au mandat en fonction.
--    Une année d'élection est partagée entre deux équipes : la fraction de couverture
--    empêche d'attribuer un exercice entier à qui n'en a exercé que trois mois.
CREATE VIEW core.commune_indicator_mandate AS
SELECT
  ci.id                AS commune_indicator_id,
  ci.commune_code,
  ci.indicator_code,
  ci.period_year,
  ci.budget_scope,
  ci.value,
  m.id                 AS mandate_id,
  m.person_id,
  (upper(range_intersect.r) - lower(range_intersect.r))::numeric
    / (make_date(ci.period_year, 12, 31) - make_date(ci.period_year, 1, 1) + 1)
                       AS couverture_annee
FROM core.commune_indicator ci
JOIN core.mandate m
  ON m.commune_code = ci.commune_code
 AND m.mandate_type = 'MAIRE'
CROSS JOIN LATERAL (
  SELECT m.validity * daterange(make_date(ci.period_year, 1, 1),
                                make_date(ci.period_year, 12, 31), '[]') AS r
) AS range_intersect
WHERE NOT isempty(range_intersect.r);

COMMENT ON VIEW core.commune_indicator_mandate IS
  'couverture_annee < 1 signale un exercice partagé entre deux mandats : ne jamais '
  'imputer l''année entière à l''un des deux.';

-- 6. Pré-enregistrement.
--    Fige la population, les indicateurs et la méthode AVANT le premier calcul.
--    Sans cela, tout écart observé est attribuable au choix des indicateurs, et
--    l'objection est irréfutable même quand elle est fausse.
CREATE TABLE core.preregistration (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug           text        NOT NULL UNIQUE,
  title          text        NOT NULL,
  document_path  text        NOT NULL,     -- docs/pre-enregistrement-001.md
  content_hash   bytea       NOT NULL,     -- empreinte du protocole scellé
  sealed_at      timestamptz,              -- NULL tant que non scellé
  method_version text        NOT NULL,
  population_def jsonb       NOT NULL,
  supersedes_id  bigint      REFERENCES core.preregistration(id),
  amendment_note text,
  CONSTRAINT content_hash_is_32_bytes CHECK (octet_length(content_hash) = 32)
);

-- La liste des indicateurs est close au scellement. Ajouter un indicateur après avoir
-- vu un résultat invalide le protocole.
CREATE TABLE core.preregistration_indicator (
  preregistration_id bigint NOT NULL REFERENCES core.preregistration(id) ON DELETE CASCADE,
  indicator_code     text   NOT NULL REFERENCES ref.indicator(code),
  role               text   NOT NULL CHECK (role IN ('PRIMARY','CONTEXT','MATCHING')),
  PRIMARY KEY (preregistration_id, indicator_code)
);

-- Un calcul publié doit désigner le protocole scellé dont il relève.
ALTER TABLE derived.comparison_run
  ADD COLUMN preregistration_id bigint REFERENCES core.preregistration(id);

-- +goose StatementBegin
CREATE FUNCTION core.assert_preregistration_sealed() RETURNS trigger
LANGUAGE plpgsql AS $fn$
DECLARE
  v_sealed timestamptz;
BEGIN
  IF NEW.preregistration_id IS NULL THEN
    RETURN NEW;
  END IF;
  SELECT sealed_at INTO v_sealed
    FROM core.preregistration WHERE id = NEW.preregistration_id;
  IF v_sealed IS NULL THEN
    RAISE EXCEPTION
      'Protocole % non scellé : aucun calcul ne peut s''y rattacher.', NEW.preregistration_id;
  END IF;
  IF NEW.computed_at < v_sealed THEN
    RAISE EXCEPTION
      'Calcul antérieur au scellement du protocole % : le pré-enregistrement serait sans effet.',
      NEW.preregistration_id;
  END IF;
  RETURN NEW;
END;
$fn$;
-- +goose StatementEnd

CREATE TRIGGER comparison_run_requires_sealed_protocol
  BEFORE INSERT OR UPDATE ON derived.comparison_run
  FOR EACH ROW EXECUTE FUNCTION core.assert_preregistration_sealed();

-- +goose Down
DROP TRIGGER comparison_run_requires_sealed_protocol ON derived.comparison_run;
DROP FUNCTION core.assert_preregistration_sealed();
ALTER TABLE derived.comparison_run DROP COLUMN preregistration_id;
DROP TABLE core.preregistration_indicator;
DROP TABLE core.preregistration;
DROP VIEW core.commune_indicator_mandate;
DROP TABLE core.topic_assignment;
DROP TABLE ref.topic;
DROP TABLE ref.taxonomy;
DROP TABLE core.municipal_tariff;
ALTER TABLE core.commune_indicator DROP CONSTRAINT commune_indicator_unique_par_perimetre;
ALTER TABLE core.commune_indicator ADD CONSTRAINT commune_indicator_commune_code_indicator_code_period_year_key
  UNIQUE (commune_code, indicator_code, period_year);
ALTER TABLE core.commune_indicator DROP COLUMN budget_scope;
DROP TYPE core.budget_scope;
-- core.institution conserve la valeur CONGRES : PostgreSQL ne sait pas retirer
-- une valeur d'énumération. Sans effet de bord.
