-- +goose Up
-- La frise de carrière : un mandat, et la présidence qui l'encadre.
--
-- Vue et non table : tout se déduit de core.mandate, et une copie pourrait
-- diverger de ce qu'elle prétend résumer. Un mandat qui chevauche deux
-- présidences produit deux lignes — c'est le fait, pas un défaut : un ministre
-- entré en fonction sous une présidence et sorti sous une autre a bien exercé
-- sous les deux.
--
-- La période commune est calculée par l'intersection des intervalles. Elle
-- permet d'afficher « sous la présidence de X (du … au …) » sans que personne
-- ait à refaire le calcul, et sans laisser croire que le mandat a duré aussi
-- longtemps que la présidence.
--
-- Rappel qui doit accompagner tout affichage : la concomitance est un repère
-- chronologique, pas une imputation. Un ministre n'est pas responsable des
-- actes du président, ni l'inverse.
CREATE VIEW core.mandat_contexte AS
  SELECT m.id              AS mandate_id,
         m.person_id,
         m.mandate_type,
         m.validity        AS mandat_periode,
         p.id              AS presidence_mandate_id,
         pp.id             AS president_person_id,
         pp.given_name || ' ' || pp.family_name AS president,
         p.role            AS president_qualite,
         p.validity        AS presidence_periode,
         m.validity * p.validity AS periode_commune
    FROM core.mandate m
    JOIN core.mandate p
      ON p.mandate_type = 'PRESIDENT_REPUBLIQUE'
     AND p.validity && m.validity
    JOIN core.person pp ON pp.id = p.person_id
   WHERE m.mandate_type <> 'PRESIDENT_REPUBLIQUE';

COMMENT ON VIEW core.mandat_contexte IS
  'Chaque mandat, replacé sous la ou les présidences qu''il traverse. '
  'Repère chronologique uniquement : jamais une imputation de responsabilité.';

-- Les mandats en cours, tous niveaux confondus. « En cours » veut dire : dont
-- l'intervalle contient aujourd'hui — pas « dont la date de fin est nulle »,
-- qui confondrait un mandat courant avec un mandat dont la fin n'a pas été
-- publiée. Ici les deux coïncident parce que le RNE ne publie pas de fin, et
-- c'est une raison de plus pour que le critère soit explicite.
CREATE VIEW core.mandat_actuel AS
  SELECT m.id AS mandate_id, m.person_id, m.mandate_type, m.role,
         m.commune_code, m.constituency, m.validity,
         lower(m.validity) AS depuis
    FROM core.mandate m
   WHERE m.validity @> CURRENT_DATE;

COMMENT ON VIEW core.mandat_actuel IS
  'Mandats dont la période contient la date du jour, tous niveaux confondus.';

-- +goose Down
DROP VIEW core.mandat_actuel;
DROP VIEW core.mandat_contexte;
