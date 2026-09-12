-- +goose Up
-- Arrêts de la Cour européenne des droits de l'homme concernant la France.
--
-- La Cour condamne un ÉTAT, jamais une personne. Ce schéma ne comporte donc
-- aucun rattachement à un responsable politique : ce serait une imputation, pas
-- une transcription, et la Cour ne la formule nulle part.
CREATE TABLE core.cedh_arret (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  itemid      text NOT NULL UNIQUE,        -- identifiant HUDOC
  appno       text NOT NULL,               -- numéro(s) de requête
  slug        text NOT NULL UNIQUE,
  titre       text NOT NULL,
  date_arret  date NOT NULL,
  formation   text,                        -- CHAMBER, GRANDCHAMBER, COMMITTEE
  importance  text,
  articles    text[] NOT NULL DEFAULT '{}',
  conclusion  text NOT NULL,
  violation   boolean NOT NULL,
  url         text NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cedh_date_idx      ON core.cedh_arret (date_arret DESC);
CREATE INDEX cedh_violation_idx ON core.cedh_arret (violation) WHERE violation;
CREATE INDEX cedh_articles_idx  ON core.cedh_arret USING gin (articles);

COMMENT ON COLUMN core.cedh_arret.violation IS
  'Vrai lorsque le dispositif retient au moins une violation. Déduit du champ '
  '« conclusion » publié par HUDOC, jamais d''une lecture de l''arrêt : un arrêt '
  'peut retenir une violation sur un article et une non-violation sur un autre.';

-- Regroupements thématiques d'articles, publiés et contestables.
CREATE TABLE ref.cedh_theme (
  code       text PRIMARY KEY,
  libelle    text NOT NULL,
  articles   text[] NOT NULL,
  definition text NOT NULL
);

-- +goose Down
DROP TABLE ref.cedh_theme;
DROP TABLE core.cedh_arret;
