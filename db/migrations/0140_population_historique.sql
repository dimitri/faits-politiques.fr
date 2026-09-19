-- +goose Up
-- La population par commune sur longue période (1876-1999), Insee — la
-- seule série qui donne un ordre de grandeur direct des pertes de la
-- Première Guerre mondiale (1911→1921) et encadre la Seconde (1936→1954,
-- 1946 absent de la série). Bornée à 1999 : les millésimes 2006-2023 du
-- même fichier (population municipale récente) font doublon avec des
-- sources déjà chargées ailleurs dans ce dépôt pour la période courante —
-- non repris ici pour ne pas entretenir deux séries récentes concurrentes.
CREATE TABLE core.population_historique_commune (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_insee text NOT NULL,
  annee      int NOT NULL,
  population bigint NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (code_insee, annee)
);
CREATE INDEX population_historique_commune_annee_idx ON core.population_historique_commune (annee);

COMMENT ON TABLE core.population_historique_commune IS
  'Population par commune, recensements 1876 à 1999 (Insee, base-pop-historiques-1876-2023, '
  'géographie au 01/01/2025) — 1911 et 1921 encadrent la Première Guerre mondiale, 1936 et '
  '1954 encadrent la Seconde (1946 absent de la série source). Les codes communaux suivent la '
  'géographie 2025 : une commune fusionnée depuis porte le code de la commune nouvelle sur '
  'toute la série, pas son ancien code.';

-- +goose Down
DROP TABLE core.population_historique_commune;
