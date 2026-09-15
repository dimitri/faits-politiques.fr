-- +goose Up
-- Décode BEN_RES_REG (région de résidence du bénéficiaire), la seule
-- nomenclature géographique d'Open Damir (docs/sante-donnees.md § 1.6). Codes
-- et libellés issus du lexique des variables publié par la Cnam (feuille
-- « MOD OPEN DAMIR », dictionnaire 2024) — pas une supposition à partir des
-- codes INSEE usuels : deux écarts réels qu'une correspondance devinée aurait
-- manqués.
--
--  1. Un seul code pour l'ensemble des DOM (5 « Régions et Départements
--     d'outre-mer ») — Guadeloupe, Martinique, Guyane, Réunion et Mayotte n'y
--     sont pas distingués, à l'inverse de core.medecin_secteur_effectif
--     (docs/sante-donnees.md § 2) qui les sépare.
--  2. Un code 99 « Inconnu » explicite, distinct de l'absence de ligne — ce
--     n'est pas un débordement de la nomenclature mais une valeur documentée
--     par la Cnam elle-même, qui porte le plus gros montant des quatorze
--     codes (26,9 Md€ sur 147,1 Md€ en 2025, vérifié).

CREATE TABLE ref.damir_region (
  code    text PRIMARY KEY,
  libelle text NOT NULL
);

COMMENT ON TABLE ref.damir_region IS
  'BEN_RES_REG : région de résidence du bénéficiaire, telle que la Cnam la '
  'code dans Open Damir depuis 2015 (lexique des variables, feuille MOD OPEN '
  'DAMIR). Un code par grande région post-2016, un code unique pour tous les '
  'DOM (5), un code Inconnu explicite (99) — pas les 18 régions '
  'administratives usuelles.';

INSERT INTO ref.damir_region (code, libelle) VALUES
  ('5',  'Régions et départements d''outre-mer'),
  ('11', 'Île-de-France'),
  ('24', 'Centre-Val de Loire'),
  ('27', 'Bourgogne-Franche-Comté'),
  ('28', 'Normandie'),
  ('32', 'Hauts-de-France'),
  ('44', 'Grand Est'),
  ('52', 'Pays de la Loire'),
  ('53', 'Bretagne'),
  ('75', 'Nouvelle-Aquitaine'),
  ('76', 'Occitanie'),
  ('84', 'Auvergne-Rhône-Alpes'),
  ('93', 'Provence-Alpes-Côte d''Azur et Corse'),
  ('99', 'Inconnu');

ALTER TABLE core.remboursement_region_prestation
  ADD CONSTRAINT remboursement_region_prestation_region_fkey
  FOREIGN KEY (region_code) REFERENCES ref.damir_region (code);

-- +goose Down
ALTER TABLE core.remboursement_region_prestation
  DROP CONSTRAINT remboursement_region_prestation_region_fkey;
DROP TABLE ref.damir_region;
