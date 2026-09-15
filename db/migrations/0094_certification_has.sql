-- +goose Up
-- La certification HAS (Haute Autorité de Santé) des établissements de
-- santé, 6ᵉ cycle (2025-) : la seule mesure de qualité normalisée et
-- comparable d'un établissement à l'autre publiée en open data — voir
-- docs/sante-donnees.md § 4. Rejoint ref.finess_etablissement par le même
-- numéro FINESS.
CREATE TABLE core.certification_has_demarche (
  code_demarche  text PRIMARY KEY,
  nofinesset     text,              -- FINESS_EG — rejoint ref.finess_etablissement, sans FK stricte
  nofinessej     text,
  raison_sociale text,
  cycle          text NOT NULL,
  version        text NOT NULL,
  annee_visite   smallint,
  mois_visite    smallint,
  date_decision  date,
  decision       text,              -- Certifié / Certifié avec mention / Certifié sous conditions / Non certifié
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX certification_has_demarche_finess_idx ON core.certification_has_demarche (nofinesset);

CREATE TABLE core.certification_has_chapitre (
  code_demarche    text NOT NULL REFERENCES core.certification_has_demarche(code_demarche),
  chapitre_num     smallint NOT NULL,
  chapitre_libelle text NOT NULL,
  score            numeric,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  PRIMARY KEY (code_demarche, chapitre_num)
);

COMMENT ON TABLE core.certification_has_demarche IS
  'HAS, certification des établissements de santé (6ᵉ cycle), une ligne par démarche de '
  'certification (une visite). Un établissement peut apparaître plusieurs fois s''il a été '
  'visité à plusieurs cycles — filtrer sur le cycle/l''année pour un état à une date donnée.';
COMMENT ON TABLE core.certification_has_chapitre IS
  'Score (0-100) par chapitre du référentiel de certification (Le patient, Les équipes de '
  'soins, L''établissement), par démarche. Trois chapitres au moment du chargement — un '
  'nombre fixé par le référentiel HAS, pas une convention de cette table.';

-- +goose Down
DROP TABLE core.certification_has_chapitre;
DROP TABLE core.certification_has_demarche;
