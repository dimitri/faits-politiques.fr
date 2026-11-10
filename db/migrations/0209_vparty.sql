-- +goose Up
-- V-Party (Varieties of Party Identity and Organization, V-Dem) : un troisième
-- référentiel de classification de partis, après PopuList et CHES, qui couvre
-- 178 pays avec les mêmes questions. Un parti y a son identifiant propre
-- (v2paid), comme CHES a le sien : le schéma 'VPARTY' le nomme, sur
-- l'identifiant d'une organisation comme sur le lien entre un parti français et
-- son entrée dans le référentiel (data/organisations.csv, colonne vparty_nom).

ALTER TABLE core.organization_identifier DROP CONSTRAINT organization_identifier_scheme_check;
ALTER TABLE core.organization_identifier
  ADD CONSTRAINT organization_identifier_scheme_check
  CHECK (scheme IN ('AN_ORGANE','SENAT_GROUPE','EP_GROUP','RNE_NUANCE','WIKIDATA',
                    'CNCCFP','PARTYFACTS','POPULIST','CHES','PARLGOV','MANIFESTO','VPARTY'));

ALTER TABLE core.party_referential_link DROP CONSTRAINT party_referential_link_scheme_check;
ALTER TABLE core.party_referential_link
  ADD CONSTRAINT party_referential_link_scheme_check
  CHECK (scheme IN ('CHES','POPULIST','PARTYFACTS','PARLGOV','MANIFESTO','AN_ORGANE','VPARTY'));

-- +goose Down
ALTER TABLE core.party_referential_link DROP CONSTRAINT party_referential_link_scheme_check;
ALTER TABLE core.party_referential_link
  ADD CONSTRAINT party_referential_link_scheme_check
  CHECK (scheme IN ('CHES','POPULIST','PARTYFACTS','PARLGOV','MANIFESTO','AN_ORGANE'));

ALTER TABLE core.organization_identifier DROP CONSTRAINT organization_identifier_scheme_check;
ALTER TABLE core.organization_identifier
  ADD CONSTRAINT organization_identifier_scheme_check
  CHECK (scheme IN ('AN_ORGANE','SENAT_GROUPE','EP_GROUP','RNE_NUANCE','WIKIDATA',
                    'CNCCFP','PARTYFACTS','POPULIST','CHES','PARLGOV','MANIFESTO'));
