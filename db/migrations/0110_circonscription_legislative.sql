-- +goose Up
-- Les circonscriptions législatives, telles que l'Insee les décrit.
--
-- Découpage issu de l'ordonnance n° 2009-935 du 29 juillet 2009 (ratifiée par
-- la loi n° 2010-165 du 23 février 2010), appliqué depuis les élections
-- législatives de juin 2012. Un député élu avant cette date représentait une
-- circonscription de même numéro mais d'un autre territoire : le lien entre un
-- mandat et ces tables ne vaut qu'à partir de la XIVe législature.
--
-- Trois fichiers de l'Insee, « Portraits des circonscriptions législatives » :
--   - le fond cartographique (558 circonscriptions : métropole et DROM ; ni
--     collectivités d'outre-mer, ni Français établis hors de France) ;
--   - la table de correspondance communes → circonscriptions, en géographie
--     communale au 1er janvier 2021 ; une commune peut appartenir à plusieurs
--     circonscriptions (Paris, Marseille, Toulouse…) ;
--   - les indicateurs statistiques (population légale 2019 et 2013, inscrits
--     d'avril 2022, recensement 2018, Filosofi 2019, BPE 2020).
--
-- Le code est celui de l'Insee et du code électoral, cinq caractères :
-- département sur deux caractères et numéro sur trois en métropole (« 24001 »,
-- « 2A002 »), département sur trois et numéro sur deux outre-mer (« 97302 »).

CREATE TABLE ref.circonscription_legislative (
  code             text PRIMARY KEY,
  nom              text NOT NULL,              -- « Dordogne - 1re circonscription », libellé Insee
  code_departement text NOT NULL,              -- 24, 2A, 973, 987…
  source_id        bigint NOT NULL REFERENCES raw.source(id)
);

CREATE TABLE ref.circonscription_commune (
  circonscription text NOT NULL REFERENCES ref.circonscription_legislative(code) ON DELETE CASCADE,
  commune_code    text NOT NULL,               -- code commune, géographie au 1er janvier 2021
  commune_nom     text NOT NULL,
  cog_millesime   integer NOT NULL,
  entiere         boolean NOT NULL,            -- false : la commune est partagée entre plusieurs circonscriptions
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  PRIMARY KEY (circonscription, commune_code)
);
CREATE INDEX circonscription_commune_commune_idx ON ref.circonscription_commune (commune_code);

-- Les variables statistiques, avec le libellé et la source publiés par l'Insee.
-- La circonscription « 00000 » est la ligne de référence du fichier : moyenne
-- France hors Mayotte pour le recensement, France métropolitaine pour la
-- pauvreté. Les valeurs « nd » (non déterminé) ne sont pas chargées.
CREATE TABLE ref.circonscription_variable (
  variable  text PRIMARY KEY,
  libelle   text NOT NULL,
  source    text NOT NULL
);

CREATE TABLE core.circonscription_indicateur (
  circonscription text NOT NULL,               -- code, ou « 00000 » pour la référence nationale
  variable        text NOT NULL REFERENCES ref.circonscription_variable(variable),
  valeur          numeric NOT NULL,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  PRIMARY KEY (circonscription, variable)
);

-- Le contour : géométrie simplifiée publiée par l'Insee, « officielle à défaut
-- d'être précise ». Aucune superficie n'en est tirée sans le dire.
CREATE TABLE geo.contour_circonscription (
  code        text PRIMARY KEY REFERENCES ref.circonscription_legislative(code) ON DELETE CASCADE,
  geom        geometry(MultiPolygon, 4326) NOT NULL,
  srid_rendu  integer NOT NULL DEFAULT 2154,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX contour_circonscription_geom_idx ON geo.contour_circonscription USING gist (geom);

-- +goose Down
DROP TABLE geo.contour_circonscription;
DROP TABLE core.circonscription_indicateur;
DROP TABLE ref.circonscription_variable;
DROP TABLE ref.circonscription_commune;
DROP TABLE ref.circonscription_legislative;
