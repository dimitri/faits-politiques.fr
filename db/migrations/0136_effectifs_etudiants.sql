-- +goose Up
-- Les effectifs étudiants par commune (Atlas régional des effectifs
-- d'étudiants, SIES — service statistique du ministère de l'Enseignement
-- supérieur), agrégés par commune et par rentrée universitaire à partir
-- du détail par établissement — vérifié directement : 127 728 lignes
-- établissement × composante × filière × année dans le fichier source,
-- ici agrégées à la commune pour une carte lisible. Pour
-- docs/recherche-enseignement-superieur-donnees.md.
CREATE TABLE core.effectifs_etudiants_commune (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_insee  text NOT NULL,
  commune     text NOT NULL,
  rentree     int NOT NULL,
  effectif    bigint NOT NULL,
  geom        geometry(Point, 4326),
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (code_insee, rentree)
);

CREATE INDEX effectifs_etudiants_commune_geom_idx ON core.effectifs_etudiants_commune USING gist (geom);

COMMENT ON TABLE core.effectifs_etudiants_commune IS
  'Effectifs étudiants (hors CPGE comptés à part, "effectifhdccpge"), agrégés '
  'par commune et rentrée universitaire (SIES, Atlas régional). Un même '
  'étudiant en double inscription peut être compté deux fois, comme dans la '
  'source elle-même.';

-- +goose Down
DROP TABLE core.effectifs_etudiants_commune;
