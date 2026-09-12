-- +goose Up
-- La profession déclarée, qui est le plus proche de « ce que faisait cette
-- personne avant ».
--
-- Aucune source publique ne publie le PARCOURS D'ÉTUDES d'un élu : ni le RNE,
-- ni le Sénat, ni l'Assemblée, ni la HATVP. Ce n'est pas une lacune de
-- collecte, c'est une donnée que l'administration ne recueille pas.
--
-- Ce qui existe, et qui était lu puis jeté par les connecteurs :
--   RNE    catégorie socio-professionnelle, codée par l'INSEE
--   Sénat  catégorie et description libre de la profession
--
-- Les deux nomenclatures ne coïncident pas. Le code d'origine est donc conservé
-- à côté du libellé, et aucune fusion n'est tentée : « profession » au Sénat et
-- « CSP » au RNE ne mesurent pas la même chose.
ALTER TABLE core.person
  ADD COLUMN IF NOT EXISTS profession text,
  ADD COLUMN IF NOT EXISTS csp_code text,
  ADD COLUMN IF NOT EXISTS csp_libelle text;

COMMENT ON COLUMN core.person.profession IS
  'Profession en clair, telle que la source la publie. Déclarative.';
COMMENT ON COLUMN core.person.csp_code IS
  'Catégorie socio-professionnelle, nomenclature INSEE, publiée par le RNE.';

-- +goose Down
ALTER TABLE core.person
  DROP COLUMN IF EXISTS profession,
  DROP COLUMN IF EXISTS csp_code,
  DROP COLUMN IF EXISTS csp_libelle;
