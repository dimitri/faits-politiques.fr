-- +goose Up
-- La carte d'Europe du dossier Seconde Guerre mondiale (§ 2) dessinait ses
-- voisins avec geo.contour_pays — des frontières ACTUELLES appliquées à une
-- situation de 1940, un anachronisme signalé directement par l'utilisateur
-- (la Pologne d'aujourd'hui n'est pas celle de 1940 ; l'Ukraine, la
-- Biélorussie et les pays baltes actuels n'existaient pas comme États
-- indépendants à cette date, absorbés ou partagés entre la Pologne d'avant-
-- guerre et l'URSS). CShapes 2.0 (déjà chargé pour l'empire colonial,
-- internal/geo/empire_colonial.go) publie un panel de frontières
-- historiques, une ligne par période de stabilité et par État du système
-- international : cette table fige une coupe à une seule date de référence
-- (1er septembre 1940 — voir internal/geo/europe_1940.go pour le choix de
-- cette date), pas un panel complet, pour cette seule carte.
CREATE TABLE geo.contour_europe_1940 (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom        text NOT NULL,
  nom_en     text NOT NULL,
  geom       geometry(MultiPolygon, 4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX contour_europe_1940_geom_idx ON geo.contour_europe_1940 USING gist(geom);

-- +goose Down
DROP TABLE geo.contour_europe_1940;
