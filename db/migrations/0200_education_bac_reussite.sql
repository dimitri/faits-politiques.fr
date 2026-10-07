-- +goose Up
-- Taux de réussite au baccalauréat, INSEE (série BDM 001769473, "Tous
-- baccalauréats — Taux de réussite — France"), pas RERS cette fois — même
-- indicateur, source différente, demandée nommément. La série INSEE
-- commence en 2011 (bien plus courte que les séries RERS ci-dessus) et
-- saute 2022 et 2023 (aucune observation publiée par l'INSEE pour ces deux
-- années au moment du chargement) : un vrai trou de la source, pas une
-- erreur de chargement.
CREATE TABLE core.education_bac_reussite (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee       integer NOT NULL UNIQUE,
  taux_pct    numeric NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE core.education_bac_reussite;
