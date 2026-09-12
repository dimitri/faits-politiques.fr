-- +goose Up
-- Identifiants du Sénat.
--
-- Correction d'une affirmation erronée portée jusqu'ici par ce projet : le
-- Sénat publie bien les votes INDIVIDUELS de ses membres. La base Dosleg
-- contient 1,98 million de positions nominatives depuis 2006. La granularité
-- GROUP du schéma reste utile pour d'autres cas, mais elle ne s'applique pas au
-- Sénat tel qu'il publie aujourd'hui.
ALTER TABLE core.person_identifier DROP CONSTRAINT person_identifier_scheme_check;
ALTER TABLE core.person_identifier
  ADD CONSTRAINT person_identifier_scheme_check
  CHECK (scheme IN ('AN_ACTEUR','AN_SYCOMORE','SENAT_MATRICULE','EP_MEP',
                    'RNE','HATVP','WIKIDATA'));

-- +goose Down
ALTER TABLE core.person_identifier DROP CONSTRAINT person_identifier_scheme_check;
ALTER TABLE core.person_identifier
  ADD CONSTRAINT person_identifier_scheme_check
  CHECK (scheme IN ('AN_ACTEUR','AN_SYCOMORE','SENAT_MATRICULE','EP_MEP',
                    'RNE','HATVP','WIKIDATA'));
