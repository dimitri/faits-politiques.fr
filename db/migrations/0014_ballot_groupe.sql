-- +goose Up
-- Le groupe du votant, tel que publié par la source avec le scrutin.
--
-- L'Assemblée ventile chaque scrutin par groupe : le relevé indique donc, pour
-- chaque votant, le groupe sous lequel son vote a été enregistré ce jour-là.
-- C'est une TRANSCRIPTION, pas une inférence — et c'est plus fiable que de
-- reconstituer l'appartenance à la date du scrutin à partir des mandats, dont
-- les fichiers publiés ne portent pas les groupes de la 17e législature.
ALTER TABLE core.ballot
  ADD COLUMN organization_id bigint REFERENCES core.organization(id);

CREATE INDEX ballot_organization_idx ON core.ballot (organization_id)
  WHERE organization_id IS NOT NULL;

COMMENT ON COLUMN core.ballot.organization_id IS
  'Groupe sous lequel la source a enregistré ce vote, à la date du scrutin. '
  'Ne pas confondre avec core.affiliation, qui décrit une appartenance déclarée '
  'sur une période : ici il s''agit du relevé d''un jour précis.';

-- +goose Down
DROP INDEX core.ballot_organization_idx;
ALTER TABLE core.ballot DROP COLUMN organization_id;
