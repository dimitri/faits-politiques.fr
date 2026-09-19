-- +goose Up
-- Les grands cours d'eau de métropole (BD Topage 2025, Sandre/IGN), un
-- calque de repère à superposer aux cartes existantes — même producteur et
-- même licence que geo.contour_bassin, mais des lignes (le tracé du cours
-- d'eau) et non des polygones (le bassin versant qui l'entoure), d'où une
-- table séparée. Le fichier source ne porte ni ordre de Strahler ni débit :
-- la sélection des cours d'eau "majeurs" se fait par une liste de noms
-- choisie à l'exécution du connecteur, pas par un seuil dans cette table.
-- Un cours d'eau arrive fragmenté en plusieurs tronçons (Sandre coupe aux
-- confluences) : une ligne par tronçon plutôt qu'une géométrie fusionnée,
-- les tronçons partageant leurs extrémités se raccordent visuellement au
-- tracé sans qu'une fusion serveur soit nécessaire.
CREATE TABLE geo.cours_eau (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom        text NOT NULL,          -- ex. 'la Seine', tel que publié par Sandre (TopoOH)
  geom       geometry(MultiLineString,4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cours_eau_geom_idx ON geo.cours_eau USING gist (geom);
CREATE INDEX cours_eau_nom_idx ON geo.cours_eau (nom);

COMMENT ON TABLE geo.cours_eau IS
  'Tracé des grands cours d''eau de métropole (BD Topage 2025, Sandre/IGN), filtré à une '
  'liste de noms choisie à l''ingestion (internal/hydro/cours_eau.go) parmi les 134 739 '
  'tronçons du fichier source complet, qui couvre un réseau bien plus fin que ce que ce '
  'calque de repère nécessite.';

-- +goose Down
DROP TABLE geo.cours_eau;
