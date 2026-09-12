-- +goose Up
-- Une contrainte UNIQUE portant sur des colonnes NULL ne contraint rien : en
-- PostgreSQL deux NULL sont distincts, donc (NULL, 42, 'LOGO') n'entrait jamais
-- en conflit avec lui-même. Le piège s'était déjà présenté sur
-- core.party_classification ; il est corrigé ici par des index partiels, qui
-- disent exactement ce qu'on veut dire.
ALTER TABLE core.media DROP CONSTRAINT media_a_exactement_un_sujet;
ALTER TABLE core.media
  ADD CONSTRAINT media_a_exactement_un_sujet
  CHECK (num_nonnulls(person_id, organization_id) = 1);

ALTER TABLE core.media DROP CONSTRAINT media_person_id_organization_id_kind_key;

CREATE UNIQUE INDEX media_un_par_personne ON core.media (person_id, kind)
  WHERE person_id IS NOT NULL;
CREATE UNIQUE INDEX media_un_par_organisation ON core.media (organization_id, kind)
  WHERE organization_id IS NOT NULL;

-- +goose Down
DROP INDEX core.media_un_par_organisation;
DROP INDEX core.media_un_par_personne;
ALTER TABLE core.media
  ADD CONSTRAINT media_person_id_organization_id_kind_key
  UNIQUE (person_id, organization_id, kind);
