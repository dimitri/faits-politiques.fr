-- +goose Up
-- Correction : `stime` n'est pas une durée, c'est un INSTANT.
--
-- La colonne avait été nommée `duree_s` et présentée comme le temps de parole.
-- Le total sorti de la base l'a démenti : 539 310 heures de parole pour deux
-- années de séance, et 5 940 heures pour une seule ministre — soit deux heures
-- et demie par prise de parole. Impossible.
--
-- `stime` est la position de la prise de parole dans la séance, en secondes
-- depuis son ouverture : « La séance est ouverte » y vaut 26,68. La durée d'une
-- intervention est donc l'écart avec la suivante, et c'est une valeur DÉRIVÉE,
-- pas une donnée publiée.
--
-- Le nom d'une colonne est une affirmation. Celui-là en faisait une fausse.
ALTER TABLE core.intervention RENAME COLUMN duree_s TO instant_s;

COMMENT ON COLUMN core.intervention.instant_s IS
  'Position de la prise de parole dans la séance, en secondes depuis son '
  'ouverture. CE N''EST PAS UNE DURÉE : la durée se déduit de l''écart avec '
  'l''intervention suivante de la même séance.';

DROP VIEW IF EXISTS derived.temps_de_parole;

CREATE VIEW derived.temps_de_parole AS
  WITH ordonne AS (
    SELECT i.person_id, i.legislature, i.session, i.date_seance,
           i.numero_seance, i.role_debat, i.instant_s,
           -- L'instant de la prise de parole suivante, dans la MÊME séance.
           lead(i.instant_s) OVER (
             PARTITION BY i.date_seance, i.numero_seance ORDER BY i.ordre
           ) AS instant_suivant
      FROM core.intervention i
     WHERE i.instant_s IS NOT NULL
  ), durees AS (
    SELECT *,
           -- Un écart négatif ou aberrant signale une séance mal ordonnée : il
           -- est écarté plutôt que compté. Une prise de parole de plus d'un
           -- quart d'heure existe, au-delà c'est une anomalie de séquence.
           CASE WHEN instant_suivant > instant_s
                 AND instant_suivant - instant_s <= 900
                THEN instant_suivant - instant_s END AS duree_s
      FROM ordonne
  )
  SELECT person_id, legislature, session,
         count(*) AS interventions,
         sum(duree_s) FILTER (WHERE coalesce(role_debat,'') <> 'president') AS duree_debat_s,
         count(*) FILTER (WHERE role_debat = 'president') AS dont_presidence,
         count(*) FILTER (WHERE duree_s IS NULL) AS duree_indeterminee,
         min(date_seance) AS premiere,
         max(date_seance) AS derniere
    FROM durees
   WHERE person_id IS NOT NULL
   GROUP BY 1, 2, 3;

COMMENT ON VIEW derived.temps_de_parole IS
  'Temps de parole DÉRIVÉ de l''écart entre interventions successives d''une '
  'même séance. Les écarts supérieurs à quinze minutes sont écartés comme '
  'anomalies de séquence, et leur nombre est publié dans duree_indeterminee.';

-- +goose Down
DROP VIEW derived.temps_de_parole;
ALTER TABLE core.intervention RENAME COLUMN instant_s TO duree_s;
