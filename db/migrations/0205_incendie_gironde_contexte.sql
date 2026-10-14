-- +goose Up
-- Fond géographique pour la carte locale de l'incendie de Gironde (voir
-- 0204_incendie_gironde_2026.sql) : sans eau, rivière ni ville repère, le
-- périmètre brûlé seul ne se lit pas — juste une forme sans échelle ni
-- orientation. Deux couches, chacune sa propre provenance :
--
-- geo.eau_gironde_2026 : le Bassin d'Arcachon et le littoral atlantique ne
-- sont pas des polygones OSM directs (seules les lignes de côte
-- natural=coastline le sont) ; on reprend donc le jeu pré-construit
-- osmdata.openstreetmap.de (dérivé des mêmes lignes de côte), découpé à la
-- zone qui nous intéresse.
--
-- geo.riviere_gironde_2026 : la Leyre n'est pas un « grand cours d'eau »
-- national au sens de geo.cours_eau (internal/hydro/cours_eau.go, une
-- liste choisie de 19 cours d'eau) — elle n'y figurerait jamais — donc une
-- table locale séparée, sur le même principe que zone_brulee_gironde_2026 :
-- un événement/contexte local, pas une couche nationale.
CREATE TABLE geo.eau_gironde_2026 (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  geom       geometry(MultiPolygon, 4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX eau_gironde_2026_geom_idx ON geo.eau_gironde_2026 USING gist(geom);

COMMENT ON TABLE geo.eau_gironde_2026 IS
  'Polygones d''eau (océan Atlantique, Bassin d''Arcachon) pour la carte '
  'locale de l''incendie de Gironde 2026 — osmdata.openstreetmap.de '
  '(water-polygons, dérivé des lignes de côte OSM), découpés à la zone '
  'd''intérêt.';

CREATE TABLE geo.riviere_gironde_2026 (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom        text NOT NULL,
  geom       geometry(MultiLineString, 4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX riviere_gironde_2026_geom_idx ON geo.riviere_gironde_2026 USING gist(geom);

COMMENT ON TABLE geo.riviere_gironde_2026 IS
  'Cours d''eau locaux (la Leyre) pour la carte de l''incendie de Gironde '
  '2026 — OpenStreetMap via Overpass, hors périmètre de geo.cours_eau '
  '(liste nationale des grands cours d''eau uniquement).';

-- +goose Down
DROP TABLE geo.riviere_gironde_2026;
DROP TABLE geo.eau_gironde_2026;
