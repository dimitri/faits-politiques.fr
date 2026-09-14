-- +goose Up
-- Le taux de chômage au sens du BIT publié DIRECTEMENT par l'INSEE, au pas
-- trimestriel — la mesure de référence citée dans le débat public français.
--
-- Jusqu'ici, la seule série de taux en base (core.macro_value,
-- chomage.taux) venait d'Eurostat, au pas ANNUEL : une moyenne d'année qui
-- republie les chiffres transmis par l'INSEE, avec un an de retard de plus
-- et sans le détail trimestriel. La série BDM 001688527 est la source
-- elle-même, au pas auquel l'INSEE la publie (chaque trimestre, avec les
-- comptes nationaux) — pas une republication.
--
-- Champ : France hors Mayotte, données CVS (corrigées des variations
-- saisonnières), ensemble hommes et femmes, tous âges. C'est le champ le
-- plus large que l'INSEE publie sous cet identifiant, et celui qui se
-- rapproche le plus du geo=FR d'Eurostat utilisé ailleurs sur le site.
CREATE TABLE core.chomage_taux_trimestriel (
  trimestre      text NOT NULL PRIMARY KEY, -- 'AAAA-Qn', tel que l'INSEE le nomme
  annee          smallint NOT NULL,
  trimestre_num  smallint NOT NULL CHECK (trimestre_num BETWEEN 1 AND 4),
  taux           numeric NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id)
);

COMMENT ON TABLE core.chomage_taux_trimestriel IS
  'Taux de chômage au sens du BIT, France hors Mayotte, données CVS, '
  'trimestriel — INSEE, série BDM 001688527. Source directe, à distinguer de '
  'chomage.taux dans core.macro_value (Eurostat, annuel, republication).';

-- +goose Down
DROP TABLE core.chomage_taux_trimestriel;
