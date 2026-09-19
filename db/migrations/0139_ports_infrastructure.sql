-- +goose Up
-- L'accès terrestre aux quatre grands ports maritimes suivis par le
-- dossier (docs/ports-donnees.md) : autoroutes (OpenStreetMap, filtrées à
-- un rayon de 80 km autour de chaque port — pas le réseau national entier)
-- et voies ferrées portuaires (SNCF Réseau, type_ligne='Vport' — un
-- raccordement au port, pas une preuve d'usage fret : aucune donnée
-- ouverte ne distingue fret et voyageurs sur le réseau ferré français).
CREATE TABLE geo.autoroute_portuaire (
  id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ref       text,                 -- ex. 'A 16' ; peut être vide (bretelles sans numéro)
  highway   text NOT NULL,        -- 'motorway' ou 'motorway_link'
  geom      geometry(LineString,2154) NOT NULL,
  source_id bigint NOT NULL REFERENCES raw.source(id)
);
CREATE INDEX autoroute_portuaire_geom_idx ON geo.autoroute_portuaire USING gist (geom);

CREATE TABLE geo.voie_ferree_portuaire (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_ligne text NOT NULL,
  geom       geometry(LineString,4326) NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id)
);
CREATE INDEX voie_ferree_portuaire_geom_idx ON geo.voie_ferree_portuaire USING gist (geom);

COMMENT ON TABLE geo.autoroute_portuaire IS
  'Tronçons autoroutiers (OpenStreetMap via data.gouv.fr) situés à moins de 80 km d''un des '
  'quatre grands ports du dossier ports-donnees.md — une sélection géographique pour ce '
  'dossier, pas une couche autoroutière nationale.';
COMMENT ON TABLE geo.voie_ferree_portuaire IS
  'Tronçons ferroviaires classés "voie portuaire" (SNCF Réseau, type_ligne=Vport) sur tout '
  'le territoire — un raccordement physique au port, jamais une mesure de trafic fret : '
  'aucune donnée ouverte ne distingue fret et voyageurs sur le réseau ferré national.';

-- Le report modal (fer/fleuve/route) dans le pré- et post-acheminement des
-- marchandises, tel que publié par l'Observatoire de la performance
-- portuaire (DGITM, édition 2024, données 2023) — une seule année, un
-- chiffre agrégé par port, pas une série. Certains ports ne publient
-- qu'un plafond ("moins de X %") plutôt qu'un chiffre exact :
-- est_plafond le signale, pour ne jamais afficher "20 %" quand la source
-- dit "moins de 20 %".
CREATE TABLE core.report_modal_port (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  port               text NOT NULL UNIQUE,
  annee              int NOT NULL,
  part_massifiee_pct double precision,  -- fer + fleuve, tous trafics confondus
  part_fer_pct       double precision,
  part_fleuve_pct    double precision,
  est_plafond        boolean NOT NULL DEFAULT false,
  note               text NOT NULL,
  source_id          bigint NOT NULL REFERENCES raw.source(id)
);

-- La comparaison avec les ports nord-européens n'est honnête que restreinte
-- aux conteneurs : les chiffres français ci-dessus portent sur TOUS les
-- trafics, ceux publiés par Anvers/Rotterdam ne portent que sur les
-- conteneurs — les deux ne sont jamais comparés dans le même tableau.
-- HAROPA et Hambourg ne publient pas de répartition conteneurs comparable
-- vérifiée : absents plutôt que devinés (voir le dossier, § « ce que les
-- données ne disent pas »).
CREATE TABLE core.report_modal_conteneurs (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  port            text NOT NULL,
  pays            text NOT NULL,
  annee           int NOT NULL,
  part_fer_pct    double precision NOT NULL,
  part_fleuve_pct double precision NOT NULL,
  part_route_pct  double precision NOT NULL,
  route_calculee  boolean NOT NULL DEFAULT false,  -- route = 100 - fer - fleuve, non publiée telle quelle
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (port, annee)
);

-- +goose Down
DROP TABLE core.report_modal_conteneurs;
DROP TABLE core.report_modal_port;
DROP TABLE geo.voie_ferree_portuaire;
DROP TABLE geo.autoroute_portuaire;
