-- +goose Up
-- Le répertoire des musées labellisés « Musée de France » (Muséofile,
-- ministère de la Culture) — vérifié directement : 1216 musées, mise à
-- jour hebdomadaire, géolocalisés (latitude/longitude), pour le dossier
-- docs/culture-donnees.md. Le label est un statut administratif (accès aux
-- aides de l'État, obligations de conservation) ; il ne dit rien de la
-- taille réelle ou de la fréquentation du musée — un très grand musée
-- national et un petit musée associatif comptent chacun pour un.
CREATE TABLE core.musee_france (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  identifiant         text NOT NULL UNIQUE,
  nom                 text NOT NULL,
  ville               text NOT NULL,
  code_postal         text,
  departement         text NOT NULL,
  region              text NOT NULL,
  domaine_thematique  text,
  geom                geometry(Point, 4326),
  source_id           bigint NOT NULL REFERENCES raw.source(id)
);

CREATE INDEX musee_france_geom_idx ON core.musee_france USING gist (geom);
CREATE INDEX musee_france_departement_idx ON core.musee_france (departement);

COMMENT ON TABLE core.musee_france IS
  'Musées labellisés "Musée de France" (Muséofile, ministère de la Culture, '
  'data.culture.gouv.fr). Le label est un statut administratif, pas une mesure '
  'de taille ou de fréquentation.';

-- +goose Down
DROP TABLE core.musee_france;
