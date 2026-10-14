-- +goose Up
-- Le périmètre brûlé de l'incendie de Gironde (Saumos/Le Porge, juillet-août
-- 2026), pour la carte locale de docs/securite-civile-donnees.md § « L'été
-- 2026 ». Source : Copernicus EMS, cartographie rapide, activation EMSR899
-- (« Wildfire in Saumos, France »), déclenchée le 22 juillet 2026 à la
-- demande du C.O.G.I.C. — gratuite et ouverte (politique de données
-- Copernicus), ni une estimation ni un tracé refait à la main : 1 832
-- polygones détectés par télédétection (produit DEL, delineation map,
-- deuxième passage de suivi), réunis ici en une seule géométrie — le détail
-- polygone par polygone n'ajoute rien à une carte qui montre où le feu est
-- passé, pas un inventaire parcelle par parcelle.
--
-- Une seule ligne, pas une table par événement à venir : si un autre
-- incendie mérite la même carte un jour, ce sera une autre table nommée
-- pour lui, sur ce modèle — comme geo.contour_europe_1940 pour une autre
-- coupe historique, jamais une table générique "événements" qui inviterait
-- à mélanger des géométries sans rapport entre elles.
CREATE TABLE geo.zone_brulee_gironde_2026 (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  notation   text NOT NULL,
  surface_ha numeric NOT NULL,
  geom       geometry(MultiPolygon, 4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX zone_brulee_gironde_2026_geom_idx ON geo.zone_brulee_gironde_2026 USING gist(geom);

COMMENT ON TABLE geo.zone_brulee_gironde_2026 IS
  'Périmètre brûlé de l''incendie de Gironde, juillet-août 2026 (Copernicus '
  'EMS, activation EMSR899, produit de délinéation). Une seule géométrie '
  'unioned à partir des 1 832 polygones sources ; surface_ha est leur somme, '
  'pas une mesure recalculée sur la géométrie unioned.';

-- +goose Down
DROP TABLE geo.zone_brulee_gironde_2026;
