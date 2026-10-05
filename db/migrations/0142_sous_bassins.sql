-- +goose Up
-- Les sous-bassins versants topographiques (BD Topage 2025, Sandre/IGN) —
-- une résolution bien plus fine que les 7 grands bassins hydrographiques
-- déjà chargés (geo.contour_bassin, migration 0088) : 6 190 polygones en
-- métropole, chacun rattaché à son grand bassin par CdBH. Comble le manque
-- documenté au § 7 du dossier bassins versants ("les tracés fins des
-- sous-bassins... non chargés").
CREATE TABLE geo.contour_sous_bassin (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_bassin  text NOT NULL,   -- CdBH : rattachement au grand bassin (geo.contour_bassin.code)
  nom          text,            -- TopoOH : souvent le nom du cours d'eau associé, pas toujours renseigné
  geom         geometry(MultiPolygon,4326) NOT NULL,
  source_id    bigint NOT NULL REFERENCES raw.source(id)
);
CREATE INDEX contour_sous_bassin_geom_idx ON geo.contour_sous_bassin USING gist (geom);
CREATE INDEX contour_sous_bassin_bassin_idx ON geo.contour_sous_bassin (code_bassin);

COMMENT ON TABLE geo.contour_sous_bassin IS
  'Sous-bassins versants topographiques de métropole (BD Topage 2025, thème '
  'BassinVersantTopographique, Sandre/IGN) — 6 190 polygones, une résolution bien plus fine que '
  'les 7 grands bassins hydrographiques (geo.contour_bassin). Le nom (TopoOH) est souvent celui '
  'du tronçon de cours d''eau associé, pas un nom de bassin à proprement parler ; certains '
  'polygones n''en ont aucun.';

-- +goose Down
DROP TABLE geo.contour_sous_bassin;
