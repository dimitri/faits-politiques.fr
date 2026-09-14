-- +goose Up
-- Les 7 grands bassins hydrographiques de métropole (BD Topage, Sandre/IGN) —
-- la couche géographique du dossier « pour aller plus loin » sur la
-- gouvernance de l'eau (docs/bassins-versants-donnees.md). Table séparée de
-- geo.contour plutôt qu'un niveau ajouté à sa contrainte CHECK : un bassin
-- hydrographique n'est pas un échelon administratif, et sa géométrie ne vient
-- pas de la même chaîne (Sandre/IGN, pas OSM).
CREATE TABLE geo.contour_bassin (
  code       text PRIMARY KEY,      -- code Sandre du bassin (ex. '04' = Loire-Bretagne)
  nom        text NOT NULL,
  geom       geometry(MultiPolygon,4326) NOT NULL,
  srid_source integer NOT NULL DEFAULT 2154,  -- Lambert-93, la projection native du fichier Sandre
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contour_bassin_geom_idx ON geo.contour_bassin USING gist (geom);

COMMENT ON TABLE geo.contour_bassin IS
  'Les 7 bassins hydrographiques de France métropolitaine (BD Topage 2025, Sandre/IGN) : '
  'les 6 comités de bassin classiques (Artois-Picardie, Rhin-Meuse, Seine-Normandie, '
  'Loire-Bretagne, Adour-Garonne, Rhône-Méditerranée) plus la Corse. N''inclut PAS les '
  'bassins d''outre-mer : le fichier source est scopé métropole (suffixe FXX) — à compléter '
  'séparément si besoin, pas silencieusement absent.';

-- +goose Down
DROP TABLE geo.contour_bassin;
