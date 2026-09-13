-- +goose Up
-- Les mandats répétés, vus comme UNE carrière et non comme N lignes.
--
-- Un sénateur élu en 2001, battu en 2011, réélu en 2014 occupe trois lignes de
-- core.mandate. C'est juste — chaque période a sa circonscription, sa qualité,
-- son institution — mais ce n'est pas ainsi qu'on lit une carrière. La question
-- « depuis combien de temps siège-t-il ? » n'a pas de réponse sur une ligne.
--
-- PostgreSQL sait dire cela en un seul type. `datemultirange` est un ensemble
-- d'intervalles disjoints, avec ses trous : {[2001-10-01,2011-10-01),
-- [2014-10-01,)} DIT l'interruption, là où trois lignes obligent le lecteur à
-- la reconstituer. (Le type est daterange/datemultirange et non
-- tstzrange/tstzmultirange : un mandat commence un jour, pas à une heure. Le
-- fuseau horaire n'a rien à faire dans une date d'élection.)
--
-- Pourquoi une vue et non une colonne : core.mandate porte des faits PROPRES À
-- CHAQUE PÉRIODE — la circonscription, la qualité, la commune. Les fondre dans
-- un multirange les perdrait. La série est donc DÉRIVÉE de ces lignes, ce qui
-- est le sens même du schéma `derived`.
-- Le regroupement se fait sur (personne, type de mandat) SANS l'institution.
-- Grouper par institution recoupait la carrière selon la SOURCE qui l'a
-- renseignée : le président du Sénat apparaissait comme sénateur jusqu'en 2023
-- d'après le dump Dosleg, et comme sénateur depuis 2023 d'après le RNE, en deux
-- lignes — soit exactement le morcellement que cette vue existe pour réparer.
-- Les institutions présentes sont rendues côte à côte.
CREATE VIEW derived.mandat_serie AS
  SELECT m.person_id,
         m.mandate_type,
         array_remove(array_agg(DISTINCT m.institution), NULL) AS institutions,
         range_agg(m.validity) AS periodes,
         count(*) AS nb_periodes,
         lower(range_agg(m.validity)) AS debut,
         upper(range_agg(m.validity)) AS fin,
         -- Le temps RÉELLEMENT passé sous mandat, trous déduits. Une somme de
         -- durées ligne à ligne donnerait le même chiffre ; le multirange le
         -- donne sans qu'on ait à se demander si deux lignes se chevauchent.
         (SELECT sum(upper(p) - lower(p))
            FROM unnest(range_agg(m.validity)) AS p
           WHERE NOT upper_inf(p) AND NOT lower_inf(p)) AS jours_clos,
         bool_or(upper_inf(m.validity) OR upper(m.validity) > CURRENT_DATE) AS en_cours
    FROM core.mandate m
   WHERE NOT isempty(m.validity)
   GROUP BY 1, 2;

COMMENT ON VIEW derived.mandat_serie IS
  'Une ligne par carrière (personne × type de mandat). `periodes` '
  'est un datemultirange : il porte les interruptions, que N lignes obligeaient '
  'à reconstituer. `jours_clos` ignore les périodes ouvertes — un mandat en '
  'cours n''a pas de durée, il a un début.';

-- +goose Down
DROP VIEW derived.mandat_serie;
