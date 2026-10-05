-- +goose Up
-- Les effectifs d'élèves sur longue période (RERS, tableaux 3.01 et 4.01) —
-- distincte de core.education_effectif_eleves (data.education.gouv.fr,
-- 2009-2025, ventilée par secteur public/privé) : cette table-ci ne
-- distingue pas le secteur, remonte beaucoup plus loin (1960 pour le
-- premier degré) et couvre aussi le second degré (collèges et lycées),
-- un jeu que docs/education-donnees.md § 6 listait explicitement comme
-- "non encore identifié précisément". Les deux tables ne se recouvrent
-- pas exactement (secteur contre niveau) et ne doivent jamais être
-- additionnées entre elles.
CREATE TABLE core.education_effectif_eleves_historique (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee           integer NOT NULL, -- année de rentrée scolaire
  degre           text NOT NULL,    -- premier | second
  niveau          text NOT NULL,    -- preelementaire | elementaire | college | lycee_general_technologique | lycee_professionnel
  effectif        numeric NOT NULL, -- milliers d'élèves, tel que publié
  edition_rers    integer NOT NULL,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (annee, niveau)
);

-- +goose Down
DROP TABLE core.education_effectif_eleves_historique;
