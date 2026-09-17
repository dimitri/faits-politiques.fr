-- +goose Up
-- Les EPTB et EPAGE (établissements publics territoriaux de bassin / établis-
-- sements publics d'aménagement et de gestion des eaux) — le mécanisme par
-- lequel un EPCI délègue tout ou partie de la GEMAPI à une structure dont le
-- périmètre suit le bassin versant plutôt que les limites administratives
-- (voir docs/bassins-versants-donnees.md § 1.2). Aucune agence ni ministère
-- ne publie de périmètre géographique national unique pour ces structures :
-- chaque DREAL régionale publie, ou non, sa propre couche, dans des formats
-- disparates (GML sans GeoJSON, portails de téléchargement interactifs sans
-- export direct) et incomplets (à peine deux régions sur treize couvertes à
-- l'inspection). Le contour est reconstruit ici par un autre chemin, déjà
-- disponible dans ce dépôt : BANATIC (la base nationale sur l'intercommu-
-- nalité, DGCL) déclare pour chaque groupement s'il est EPTB et/ou EPAGE, et
-- liste ses membres (communes ou EPCI) — le même mécanisme qui construit déjà
-- les contours d'EPCI (core.epci_membre, geo.contour_cog) sert donc aussi ici.
CREATE TABLE core.eptb_epage (
  siren               text PRIMARY KEY,
  nom                 text NOT NULL,
  type                text NOT NULL CHECK (type IN ('EPTB', 'EPAGE', 'EPTB_EPAGE')),
  nature_juridique    text,
  code_departement    text,
  population_totale   integer,
  nb_membres          integer,
  source_id           bigint NOT NULL REFERENCES raw.source(id),
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.eptb_epage_membre (
  eptb_siren          text NOT NULL REFERENCES core.eptb_epage(siren) ON DELETE CASCADE,
  membre_siren        text NOT NULL,
  membre_nom          text,
  categorie           text,
  commune_code        text,
  population_membre   integer,
  PRIMARY KEY (eptb_siren, membre_siren)
);

-- Le contour de chaque EPTB/EPAGE, reconstruit par union des géométries déjà
-- chargées (communes ou EPCI membres, geo.contour_cog) — jamais téléchargé
-- comme tel. nb_membres_resolus peut être inférieur à nb_membres_total : un
-- membre qui n'est ni une commune ni un EPCI déjà chargé (un autre syndicat
-- mixte, par exemple) reste hors du contour, plutôt que deviné.
CREATE TABLE geo.contour_eptb_epage (
  siren               text PRIMARY KEY REFERENCES core.eptb_epage(siren),
  geom                geometry(MultiPolygon, 4326) NOT NULL,
  nb_membres_resolus  integer NOT NULL,
  nb_membres_total    integer NOT NULL,
  source_id           bigint NOT NULL REFERENCES raw.source(id),
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contour_eptb_epage_geom_idx ON geo.contour_eptb_epage USING gist (geom);

COMMENT ON TABLE core.eptb_epage IS
  'EPTB et EPAGE déclarés dans BANATIC (registre national des groupements de '
  'collectivités, DGCL) via ses colonnes EPAGE/EPTB — un registre déclaratif, '
  'pas un décompte associatif ou ministériel (l''ANEB, association des '
  'établissements publics de bassin, en recense un nombre légèrement '
  'différent par ses propres moyens). type = EPTB_EPAGE quand les deux '
  'colonnes BANATIC valent OUI, un cas réel (double statut), pas un doublon.';

COMMENT ON TABLE geo.contour_eptb_epage IS
  'Contour reconstruit par union des communes et EPCI membres déjà chargés '
  '(geo.contour_cog) — jamais un périmètre officiel téléchargé tel quel. Voir '
  'nb_membres_resolus/nb_membres_total pour la part non couverte par membre.';

-- +goose Down
DROP TABLE geo.contour_eptb_epage;
DROP TABLE core.eptb_epage_membre;
DROP TABLE core.eptb_epage;
