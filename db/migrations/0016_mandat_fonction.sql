-- +goose Up
-- Fonction et portefeuille d'un mandat.
--
-- Un mandat ministériel n'a ni circonscription ni institution parlementaire :
-- ce qui le caractérise est la QUALITÉ (ministre, secrétaire d'État) et le
-- PORTEFEUILLE (l'intitulé du ministère). Les ranger dans « constituency »
-- aurait été commode et faux.
ALTER TABLE core.mandate
  ADD COLUMN role         text,
  ADD COLUMN portefeuille text;

COMMENT ON COLUMN core.mandate.role IS
  'Qualité telle que publiée par la source : « membre », « Ministre », '
  '« Secrétaire d''État », « Rapporteur »… Jamais normalisée ni traduite.';
COMMENT ON COLUMN core.mandate.portefeuille IS
  'Intitulé de l''organe de rattachement quand il en porte un — le ministère '
  'pour un mandat ministériel.';

-- +goose Down
ALTER TABLE core.mandate DROP COLUMN portefeuille;
ALTER TABLE core.mandate DROP COLUMN role;
