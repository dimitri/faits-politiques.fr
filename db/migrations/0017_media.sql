-- +goose Up
-- Portraits et logos.
--
-- Règle : ce site ne republie QUE des fichiers sous licence libre, et affiche
-- systématiquement leur licence et leur auteur. Les photographies officielles
-- des sites de campagne et la plupart des logos de partis sont protégés : les
-- reproduire sans autorisation serait une contrefaçon, et contredirait la
-- discipline de licences appliquée à toutes les autres données du projet.
-- Quand aucun fichier libre n'existe, l'absence est affichée comme telle.
CREATE TABLE core.media (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  person_id       bigint REFERENCES core.person(id) ON DELETE CASCADE,
  organization_id bigint REFERENCES core.organization(id) ON DELETE CASCADE,
  kind            text NOT NULL CHECK (kind IN ('PORTRAIT','LOGO')),
  fichier         text NOT NULL,            -- nom du fichier servi par le site
  source_url      text NOT NULL,            -- page de description du fichier d'origine
  licence         text NOT NULL,            -- « CC BY 4.0 », « Public domain »…
  licence_code    text NOT NULL,            -- « cc-by-4.0 », « pd »…
  auteur          text,
  largeur         int,
  hauteur         int,
  retrieval_id    bigint REFERENCES raw.retrieval(id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT media_a_exactement_un_sujet
    CHECK (num_nonnulls(person_id, organization_id) = 1),
  UNIQUE (person_id, organization_id, kind)
);
CREATE INDEX media_person_idx ON core.media (person_id) WHERE person_id IS NOT NULL;
CREATE INDEX media_org_idx    ON core.media (organization_id) WHERE organization_id IS NOT NULL;

COMMENT ON COLUMN core.media.licence_code IS
  'Seules les licences libres sont acceptées à l''ingestion. Un fichier sans '
  'licence libre identifiable n''est pas téléchargé : l''absence de portrait est '
  'une conséquence du droit d''auteur, pas un oubli.';

-- +goose Down
DROP TABLE core.media;
