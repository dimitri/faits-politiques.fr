-- +goose Up
-- Initiateur d'un dossier législatif.
--
-- L'open data de l'Assemblée ne publie pas les auteurs dans le document lui-même
-- — le champ « auteurs » d'une proposition de loi ne contient que l'organe
-- « Assemblée nationale ». L'auteur réel figure dans l'« initiateur » du DOSSIER.
-- On le stocke donc là où la source le met, plutôt que de le rattacher par
-- déduction à l'un des textes du dossier.
CREATE TABLE core.dossier_author (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dossier_id      bigint NOT NULL REFERENCES core.dossier(id) ON DELETE CASCADE,
  person_id       bigint REFERENCES core.person(id) ON DELETE CASCADE,
  organization_id bigint REFERENCES core.organization(id) ON DELETE CASCADE,
  role            text NOT NULL CHECK (role IN ('INITIATEUR','GOUVERNEMENT','COMMISSION')),
  rang            int,
  CONSTRAINT auteur_personne_xor_organisation
    CHECK (num_nonnulls(person_id, organization_id) = 1)
);
CREATE INDEX dossier_author_dossier_idx ON core.dossier_author (dossier_id);
CREATE INDEX dossier_author_person_idx  ON core.dossier_author (person_id) WHERE person_id IS NOT NULL;

COMMENT ON TABLE core.dossier_author IS
  '« Qui propose » est une dimension distincte de « qui vote », et la seule que '
  'les outils français existants n''exploitent pas. Transcrite telle que publiée.';

-- +goose Down
DROP TABLE core.dossier_author;
