-- +goose Up
-- Contours des communes et des intercommunalités, par millésime du COG.
--
-- Pourquoi une table à côté de geo.contour plutôt que dedans. geo.contour porte
-- les régions et les départements tirés d'OpenStreetMap, par un choix documenté
-- dans 0058 : OSM est redistribuable et chaque contour a un permalien. Mais pour
-- les COMMUNES, deux exigences font pencher vers l'IGN :
--
--   1. Le MILLÉSIME. Une donnée communale ne vaut que pour une géographie datée :
--      le recensement agricole est publié en communes 2025, la base est en
--      communes 2026, et les communes nouvelles de l'année séparent les deux.
--      L'IGN publie Admin Express COG CARTO aligné sur chaque millésime du COG ;
--      OSM décrit la géographie du jour, sans millésime.
--   2. La COMPLÉTUDE. Admin Express couvre les 34 877 communes, sans trou.
--
-- La clé inclut donc le millésime, ce que geo.contour n'a pas besoin de faire
-- pour des régions et des départements qui ne changent presque jamais.
--
-- Même règle que geo.contour : la géométrie sert à DESSINER, jamais à mesurer.
-- Une aire calculée en degrés n'a pas de sens. La superficie publiée ici est
-- celle que l'IGN diffuse — cadastrale, en hectares —, pas un calcul maison.
CREATE TABLE geo.contour_cog (
  -- EPCI : intercommunalités à fiscalité propre, qui PARTITIONNENT le territoire.
  -- EPT  : établissements publics territoriaux du Grand Paris, qui subdivisent
  -- la Métropole. Les ranger au même niveau superposerait deux contours sur les
  -- mêmes 131 communes : une carte colorierait deux fois Boulogne-Billancourt.
  niveau        text     NOT NULL CHECK (niveau IN ('COMMUNE', 'EPCI', 'EPT')),
  -- Code INSEE pour une commune, SIREN pour une intercommunalité.
  code          text     NOT NULL,
  cog_millesime integer  NOT NULL,
  nom           text     NOT NULL,
  geom          geometry(MultiPolygon, 4326) NOT NULL,
  -- Projection de rendu propre au territoire, déduite de data/geo-projections.csv
  -- par le département. Même principe que geo.contour.srid_rendu : un outre-mer
  -- ne se dessine jamais dans le repère de l'hexagone.
  srid_rendu    integer  NOT NULL DEFAULT 2154,
  code_departement text,
  code_region      text,
  -- Superficie cadastrale publiée par l'IGN, en hectares. NULL pour une EPCI :
  -- additionner des superficies cadastrales serait défendable, mais ce serait
  -- une valeur que la source ne publie pas.
  superficie_cadastrale_ha numeric,
  population    integer,
  -- SIREN des intercommunalités de la commune, tels que l'IGN les publie. C'est
  -- une seconde source d'appartenance, indépendante de BANATIC : cmd/verify
  -- confronte les deux.
  codes_siren_epci text[],
  source_id     bigint   NOT NULL REFERENCES raw.source(id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (niveau, code, cog_millesime)
);

CREATE INDEX contour_cog_geom_idx ON geo.contour_cog USING gist (geom);
CREATE INDEX contour_cog_millesime_idx ON geo.contour_cog (niveau, cog_millesime);

COMMENT ON TABLE geo.contour_cog IS
  'Contours communaux (IGN Admin Express COG CARTO, petite échelle) et '
  'intercommunaux, un jeu par millésime du COG. Les contours d''EPCI sont '
  'l''union des communes membres selon l''IGN pour le même millésime : ils '
  'correspondent donc toujours à la composition publiée à cette date.';
COMMENT ON COLUMN geo.contour_cog.superficie_cadastrale_ha IS
  'Superficie publiée par l''IGN. Seule surface utilisable pour un calcul : la '
  'géométrie en WGS 84 ne se mesure pas.';

-- +goose Down
DROP TABLE geo.contour_cog;
