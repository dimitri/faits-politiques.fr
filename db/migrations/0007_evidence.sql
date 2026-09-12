-- +goose Up
-- Preuves, scellement des pages, contestations et corrections.
--
-- L'arc exclusif (colonnes typées nullables + CHECK num_nonnulls = 1) est préféré à
-- une référence polymorphe libre : il conserve l'intégrité référentielle, qui est
-- précisément ce qui rend l'auditabilité vérifiable plutôt que déclarative.

CREATE TABLE core.evidence (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

  -- Sujet : exactement une de ces colonnes est renseignée.
  scrutin_id          bigint REFERENCES core.scrutin(id) ON DELETE CASCADE,
  amendement_id       bigint REFERENCES core.amendement(id) ON DELETE CASCADE,
  mandate_id          bigint REFERENCES core.mandate(id) ON DELETE CASCADE,
  affiliation_id      bigint REFERENCES core.affiliation(id) ON DELETE CASCADE,
  commune_indicator_id bigint REFERENCES core.commune_indicator(id) ON DELETE CASCADE,
  public_contract_id  bigint REFERENCES core.public_contract(id) ON DELETE CASCADE,
  intervention_id     bigint REFERENCES core.intervention(id) ON DELETE CASCADE,

  -- Rattachement à l'archive scellée.
  document_id         bigint NOT NULL REFERENCES raw.document(id),
  retrieval_id        bigint NOT NULL REFERENCES raw.retrieval(id),
  tier                core.source_tier NOT NULL,
  page                int,
  anchor              text,          -- ancre, article, numéro de délibération
  quoted_excerpt      text,          -- extrait cité verbatim
  verification        core.verification_status NOT NULL DEFAULT 'AUTO_VERIFIED',
  provenance          core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at          timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT evidence_has_exactly_one_subject CHECK (
    num_nonnulls(scrutin_id, amendement_id, mandate_id, affiliation_id,
                 commune_indicator_id, public_contract_id, intervention_id) = 1
  )
);
CREATE INDEX evidence_scrutin_idx   ON core.evidence (scrutin_id)   WHERE scrutin_id IS NOT NULL;
CREATE INDEX evidence_amendement_idx ON core.evidence (amendement_id) WHERE amendement_id IS NOT NULL;
CREATE INDEX evidence_indicator_idx ON core.evidence (commune_indicator_id) WHERE commune_indicator_id IS NOT NULL;
CREATE INDEX evidence_document_idx  ON core.evidence (document_id);

COMMENT ON TABLE core.evidence IS
  'Le niveau de confiance et le statut de vérification appartiennent à la preuve, '
  'pas au fait : un fait n''a pas de confiance en soi, ses preuves en ont une.';

-- Scellement des pages publiées. Permet d'authentifier une capture d'écran :
-- « cette page au 11/09/2026, empreinte a3f... ». Sans cela, une page qui bouge
-- après citation détruit la confiance en une fois.
CREATE TABLE core.page_version (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  path         text        NOT NULL,     -- '/scrutin/an-17-1234'
  content_hash bytea       NOT NULL,
  published_at timestamptz NOT NULL DEFAULT now(),
  method_version text      NOT NULL,     -- version du gabarit et des calculs affichés
  UNIQUE (path, content_hash),
  CONSTRAINT content_hash_is_32_bytes CHECK (octet_length(content_hash) = 32)
);
CREATE INDEX page_version_path_idx ON core.page_version (path, published_at DESC);

-- Contestations publiques. C'est le mécanisme de neutralité : un élu ou un
-- journaliste conteste une fiche, la contestation ET la réponse sont visibles.
-- Beaucoup moins coûteux qu'un comité éditorial, et bien plus vérifiable.
CREATE TABLE core.contestation (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_ref     text        NOT NULL UNIQUE,
  page_path      text        NOT NULL,
  submitted_at   timestamptz NOT NULL DEFAULT now(),
  submitter_kind text        NOT NULL CHECK (submitter_kind IN
                   ('CITOYEN','JOURNALISTE','ELU','EQUIPE_PARLEMENTAIRE','ADMINISTRATION','AUTRE')),
  submitter_name text,
  body           text        NOT NULL,
  status         text        NOT NULL DEFAULT 'OPEN'
                 CHECK (status IN ('OPEN','ACCEPTED','REJECTED','PARTIAL')),
  response       text,
  responded_at   timestamptz,
  responded_by   text,
  CONSTRAINT reponse_obligatoire_si_close
    CHECK (status = 'OPEN' OR (response IS NOT NULL AND responded_at IS NOT NULL))
);
CREATE INDEX contestation_page_idx   ON core.contestation (page_path, submitted_at DESC);
CREATE INDEX contestation_status_idx ON core.contestation (status) WHERE status = 'OPEN';

-- Registre des corrections, lisible par machine, avec l'état antérieur conservé.
CREATE TABLE core.correction (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_ref      text        NOT NULL UNIQUE,
  contestation_id bigint      REFERENCES core.contestation(id),
  page_path       text        NOT NULL,
  applied_at      timestamptz NOT NULL DEFAULT now(),
  author          text        NOT NULL,
  description     text        NOT NULL,
  previous_value  jsonb       NOT NULL,
  new_value       jsonb       NOT NULL
);
CREATE INDEX correction_page_idx ON core.correction (page_path, applied_at DESC);

-- +goose Down
DROP TABLE core.correction;
DROP TABLE core.contestation;
DROP TABLE core.page_version;
DROP TABLE core.evidence;
