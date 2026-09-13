-- +goose Up
-- Géométries administratives, pour les cartes.
--
-- La source est OpenStreetMap, sous ODbL 1.0 : la licence impose d'attribuer
-- « © les contributeurs OpenStreetMap » partout où la donnée est affichée, et
-- de partager aux mêmes conditions toute base dérivée. C'est compatible avec la
-- Licence Ouverte des données produites par ce site, à condition que
-- l'attribution suive la carte — elle est donc rendue sur chaque page qui en
-- porte une, pas seulement dans les sources.
--
-- Pourquoi OSM et pas l'IGN : les deux conviendraient, mais OSM est déjà
-- redistribuable sans restriction supplémentaire, et son identifiant de
-- relation donne un permalien vérifiable pour chaque contour publié.
--
-- Les contours sont chargés en 4326 (degrés) parce que c'est ce que publie la
-- source, et projetés au rendu. Ils ne servent à AUCUN calcul de distance ni de
-- surface : uniquement à dessiner. Une aire calculée en degrés n'aurait pas de
-- sens, et ce site ne publie pas de chiffre qu'il ne peut pas défendre.
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE SCHEMA IF NOT EXISTS geo;
COMMENT ON SCHEMA geo IS
  'Contours administratifs OpenStreetMap (ODbL). Servent au dessin, jamais au calcul.';

-- Le contour tel que la source le publie. Rien n'est corrigé ici : c'est la
-- couche « raw » de la géométrie, et elle est adressée par l'identifiant de
-- relation OSM, qui est un permalien.
-- La clé est (niveau, code_insee) et non l'identifiant OSM : une même relation
-- sert à deux niveaux quand la collectivité est unique. La Martinique et la
-- Guyane ne sont plus des départements depuis 2015, et leur contour de
-- collectivité tient lieu des deux — un fait administratif, pas un doublon.
CREATE TABLE geo.contour (
  niveau          text        NOT NULL
                  CHECK (niveau IN ('REGION','DEPARTEMENT','COMMUNE','EPCI','PAYS')),
  -- Code officiel publié par la source dans ref:INSEE. Peut être NULL : OSM
  -- n'est pas tenu de le porter, et une absence se déclare.
  code_insee      text        NOT NULL,
  osm_relation_id bigint      NOT NULL,
  PRIMARY KEY (niveau, code_insee),
  nom             text        NOT NULL,
  -- Les tags OSM tels quels, pour que rien de ce qui a servi ne soit perdu.
  tags            jsonb       NOT NULL DEFAULT '{}'::jsonb,
  geom            geometry(MultiPolygon, 4326) NOT NULL,
  source_id       bigint      REFERENCES raw.source(id),
  provenance      bigint      REFERENCES raw.document(id),
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX contour_osm_idx ON geo.contour (osm_relation_id);
CREATE INDEX contour_geom_idx        ON geo.contour USING gist (geom);

COMMENT ON COLUMN geo.contour.code_insee IS
  'Code officiel lu dans le tag ref:INSEE, rattaché au COG par data/geo-rattachements.csv.';
COMMENT ON COLUMN geo.contour.geom IS
  'WGS 84. Sert à dessiner, jamais à mesurer : une aire en degrés n''a pas de sens.';

-- +goose Down
DROP SCHEMA IF EXISTS geo CASCADE;
