-- +goose Up
-- Signature et ratification de l'Accord de Paris, pays par pays — la
-- collection dépositaire officielle de l'ONU (Traités multilatéraux
-- déposés auprès du Secrétaire général, chapitre XXVII.7.d), pas une
-- source secondaire. type_ratification distingue ratification, acceptation
-- (« A »), approbation (« AA ») et adhésion (« a », pour un pays qui
-- n'a jamais signé mais a rejoint directement) — quatre voies juridiques
-- différentes vers le même statut de partie au traité.
CREATE TABLE core.ratification_accord_paris (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  pays              text NOT NULL UNIQUE,
  date_signature    date,
  date_ratification date,
  type_ratification text,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ratification_accord_paris_date_idx ON core.ratification_accord_paris (date_ratification);

COMMENT ON TABLE core.ratification_accord_paris IS
  'Accord de Paris (COP21), signature et ratification par pays — Collection des traités des '
  'Nations unies, dépositaire officiel (chapitre XXVII.7.d). date_ratification NULL = signé '
  'mais jamais ratifié (le Yémen, notamment).';

-- +goose Down
DROP TABLE core.ratification_accord_paris;
