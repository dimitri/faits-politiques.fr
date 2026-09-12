-- +goose Up
-- Couche RAW : archive scellée.
--
-- Règle absolue : aucun UPDATE, aucun DELETE sur ce schéma.
-- Un document est identifié par l'empreinte de ses octets. Récupérer deux fois la
-- même URL avec un contenu identique ne crée pas un second document, mais une
-- seconde récupération — ce qui permet d'affirmer « ce document était encore en
-- ligne le JJ/MM/AAAA » sans dupliquer les octets.

-- Un flux de données identifié, avec sa licence. Alimente le bandeau d'attribution
-- généré automatiquement : la Licence Ouverte impose de citer le producteur et la
-- date de dernière mise à jour.
CREATE TABLE raw.source (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug              text        NOT NULL UNIQUE,
  label             text        NOT NULL,
  publisher         text        NOT NULL,                -- organisme émetteur
  tier              core.source_tier NOT NULL,
  homepage_url      text,
  licence           text        NOT NULL,                -- ex. 'Licence Ouverte 2.0'
  licence_url       text,
  commercial_use    boolean     NOT NULL,                -- garde-fou : voir contrainte ci-dessous
  attribution_text  text        NOT NULL,
  expected_cadence  text,                                -- 'quotidienne', 'trimestrielle', 'par séance'...
  notes             text,                                -- pièges connus de la source
  active            boolean     NOT NULL DEFAULT true,
  created_at        timestamptz NOT NULL DEFAULT now()
);

-- Aucune source non commercialement réutilisable n'entre dans le pipeline : elle
-- contaminerait toutes les données dérivées. Concrètement, cela exclut les dumps
-- NosDéputés / NosSénateurs (CC BY-NC-SA) au profit des sources primaires AN et
-- Sénat, toutes deux en Licence Ouverte.
ALTER TABLE raw.source
  ADD CONSTRAINT source_must_allow_commercial_use CHECK (commercial_use);

-- Une exécution de connecteur.
CREATE TABLE raw.fetch_run (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id          bigint      NOT NULL REFERENCES raw.source(id),
  connector_version  text        NOT NULL,
  started_at         timestamptz NOT NULL DEFAULT now(),
  finished_at        timestamptz,
  status             text        NOT NULL DEFAULT 'RUNNING'
                     CHECK (status IN ('RUNNING','SUCCESS','FAILED','PARTIAL')),
  stats              jsonb       NOT NULL DEFAULT '{}'::jsonb,
  error              text
);
CREATE INDEX fetch_run_source_started_idx ON raw.fetch_run (source_id, started_at DESC);

-- Le document lui-même, identifié par ses octets. Immuable par construction.
CREATE TABLE raw.document (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  sha256        bytea       NOT NULL UNIQUE,
  storage_key   text        NOT NULL UNIQUE,  -- raw/{source}/{yyyy}/{mm}/{dd}/{sha256}.{ext}
  content_type  text        NOT NULL,
  byte_size     bigint      NOT NULL CHECK (byte_size > 0),
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sha256_is_32_bytes CHECK (octet_length(sha256) = 32)
);

COMMENT ON COLUMN raw.document.sha256 IS
  'Empreinte publiée sur la fiche vérifiable. Permet d''authentifier une citation ou une capture d''écran.';

-- Une récupération HTTP. Porte l'URL d'origine et la date de consultation, qui sont
-- des mentions obligatoires sur chaque fiche.
CREATE TABLE raw.retrieval (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id      bigint      NOT NULL REFERENCES raw.source(id),
  fetch_run_id   bigint      REFERENCES raw.fetch_run(id),
  url            text        NOT NULL,
  http_status    int         NOT NULL,
  fetched_at     timestamptz NOT NULL DEFAULT now(),
  document_id    bigint      REFERENCES raw.document(id),  -- NULL si échec
  etag           text,
  last_modified  timestamptz,
  CONSTRAINT document_present_iff_success
    CHECK ((http_status BETWEEN 200 AND 299) = (document_id IS NOT NULL))
);
CREATE INDEX retrieval_url_idx         ON raw.retrieval (url, fetched_at DESC);
CREATE INDEX retrieval_document_idx    ON raw.retrieval (document_id);
CREATE INDEX retrieval_source_time_idx ON raw.retrieval (source_id, fetched_at DESC);

-- Un enregistrement logique extrait d'un document, avant normalisation.
-- C'est le point de reprise : core est reconstruit à partir d'ici, sans re-télécharger.
-- L'unicité sur (document_id, record_type, natural_key) rend l'extraction idempotente.
CREATE TABLE raw.record (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  document_id  bigint      NOT NULL REFERENCES raw.document(id),
  record_type  text        NOT NULL,   -- 'an.scrutin', 'an.acteur', 'ofgl.compte'...
  natural_key  text        NOT NULL,   -- identifiant dans la source : 'VTANR5L17V1234'
  payload      jsonb       NOT NULL,
  extracted_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (document_id, record_type, natural_key)
);
CREATE INDEX record_type_key_idx ON raw.record (record_type, natural_key);
CREATE INDEX record_payload_idx  ON raw.record USING gin (payload jsonb_path_ops);

-- +goose Down
DROP TABLE raw.record;
DROP TABLE raw.retrieval;
DROP TABLE raw.document;
DROP TABLE raw.fetch_run;
DROP TABLE raw.source;
