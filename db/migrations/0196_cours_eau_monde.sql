-- +goose Up
-- geo.cours_eau (migration antérieure) ne couvre que la France : sur toute
-- carte qui dépasse les frontières nationales (Europe de l'Ouest, Indochine
-- et ses voisins...), le Rhin ou le Danube s'arrêtaient net à la frontière
-- française, comme s'ils n'existaient qu'en France — signalé directement à
-- la vue d'une carte publiée. Natural Earth publie un fond mondial des
-- grands cours d'eau à la même échelle (1:50m) et sous la même licence que
-- geo.contour_pays (déjà chargé, migration correspondante dans
-- internal/geo/pays.go) : une table séparée plutôt qu'une fusion avec
-- geo.cours_eau, dont la source, la résolution et la couverture restent
-- différentes (IGN, France seule, bien plus fin).
CREATE TABLE geo.cours_eau_monde (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom        text,
  nom_en     text,
  geom       geometry(MultiLineString, 4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cours_eau_monde_geom_idx ON geo.cours_eau_monde USING gist(geom);

-- +goose Down
DROP TABLE geo.cours_eau_monde;
