-- +goose Up
-- Calories par hectare : la métrique demandée, et ce qu'elle vaut.
--
-- Elle rapporte les calories effectivement disponibles pour l'alimentation
-- humaine à la surface agricole du pays. C'est un rendement apparent du
-- territoire, pas un rendement agronomique.
--
-- QUATRE RÉSERVES, à afficher avec le chiffre :
--
--  1. Le numérateur est la DISPONIBILITÉ, pas la production : il inclut les
--     importations et exclut les exportations. Un pays peut afficher un bon
--     ratio en important sa nourriture.
--  2. Le dénominateur inclut les prairies permanentes, qui ne produisent pas de
--     calories végétales mais nourrissent le bétail. Rapporter des calories
--     d'assiette à cette surface mélange deux chaînes de production.
--  3. Les calories qui passent par le bétail sont comptées une fois dans
--     l'assiette, alors qu'elles ont consommé plusieurs fois leur valeur en
--     calories végétales. La vue expose donc aussi la part animale.
--  4. Une calorie n'est pas un régime : protéines, lipides et micronutriments
--     ne s'y réduisent pas.
CREATE VIEW derived.calories_par_hectare AS
  WITH kcal AS (
    SELECT b.annee, b.valeur AS kcal_hab_jour
      FROM core.bilan_alimentaire b
      JOIN ref.produit_alimentaire r ON r.code = b.produit_code
     WHERE r.libelle = 'Grand Total' AND b.element = 'Food supply (kcal/capita/day)'
  ), pop AS (
    SELECT b.annee, b.valeur * 1000 AS habitants
      FROM core.bilan_alimentaire b
      JOIN ref.produit_alimentaire r ON r.code = b.produit_code
     WHERE r.libelle = 'Population'
  ), surface AS (
    SELECT annee, valeur * 1000 AS hectares
      FROM core.agriculture_indicateur WHERE code = 'terres.agricoles'
  ), animal AS (
    SELECT b.annee, b.valeur AS kcal_animale
      FROM core.bilan_alimentaire b
      JOIN ref.produit_alimentaire r ON r.code = b.produit_code
     WHERE r.libelle = 'Animal Products' AND b.element = 'Food supply (kcal/capita/day)'
  )
  SELECT k.annee,
         k.kcal_hab_jour,
         p.habitants,
         s.hectares AS surface_agricole_ha,
         round(k.kcal_hab_jour * 365 * p.habitants / s.hectares) AS kcal_par_hectare_an,
         round(100 * a.kcal_animale / k.kcal_hab_jour, 1) AS part_animale_pct,
         -- Combien d'habitants la surface agricole nourrit, au niveau de
         -- disponibilité observé. Le chiffre est le même que le ratio
         -- ci-dessus, exprimé autrement — et plus lisible.
         round(p.habitants / (s.hectares / 10000.0)) AS habitants_nourris_par_10000_ha
    FROM kcal k
    JOIN pop p USING (annee)
    JOIN surface s USING (annee)
    LEFT JOIN animal a USING (annee);

COMMENT ON VIEW derived.calories_par_hectare IS
  'Rendement apparent du territoire : calories disponibles rapportées à la '
  'surface agricole. Le numérateur inclut les importations et exclut les '
  'exportations : ce n''est pas une mesure d''autonomie.';

-- +goose Down
DROP VIEW derived.calories_par_hectare;
