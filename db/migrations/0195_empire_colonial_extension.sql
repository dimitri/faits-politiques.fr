-- +goose Up
-- L'empire colonial ne montrait que sa dernière extension avant chaque
-- indépendance — un instantané de son rétrécissement, jamais de sa
-- croissance. CShapes 2.0 (déjà téléchargé pour geo.territoire_colonial,
-- migration 0128) modélise chaque territoire comme une SUITE de périodes
-- (changements de frontière), pas un seul point : on peut donc situer
-- l'empire à des dates fixes choisies pour elles-mêmes, pas pour marquer
-- une fin.
--
-- Quatre repères choisis pour ce qu'ils marquent dans l'histoire de
-- l'empire, pas pour un pas de temps régulier : 1900 (avant la Grande
-- Guerre), 1920 (l'empire au sortir du conflit, mandats compris), 1938
-- (juste avant la Seconde Guerre mondiale), 1946 (juste après, avant la
-- première vague de décolonisation de 1953-1956).
CREATE TABLE geo.empire_colonial_extension (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  territoire    text NOT NULL,
  annee_repere  smallint NOT NULL CHECK (annee_repere IN (1900, 1920, 1938, 1946)),
  geom          geometry(MultiPolygon, 4326),
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX empire_colonial_extension_uniq
  ON geo.empire_colonial_extension (territoire, annee_repere);
CREATE INDEX empire_colonial_extension_geom_idx
  ON geo.empire_colonial_extension USING gist (geom);

COMMENT ON TABLE geo.empire_colonial_extension IS
  'L''empire colonial français à quatre dates fixes (1900/1920/1938/1946), pas à sa '
  'dernière extension avant chaque indépendance (geo.territoire_colonial) : montre la '
  'croissance de l''empire, pas seulement son rétrécissement. Une ligne par territoire '
  'qui était sous administration française à cette date précise (absent avant '
  'rattachement, absent après indépendance) — voir internal/geo/empire_colonial.go.';

-- +goose Down
DROP TABLE geo.empire_colonial_extension;
