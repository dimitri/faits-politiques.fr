-- +goose Up

-- Décompte par personne de la position D'ORIGINE (pas position_rectifiee),
-- pour loadPersons (internal/sitegen/load.go) — remplace le GROUP BY sur la
-- totalité de core.ballot (4,9 millions de lignes) que cette fonction
-- refaisait à chaque construction. Distinct à dessein de
-- mv.scrutin_vote_nominal, qui porte la position corrigée.
CREATE MATERIALIZED VIEW mv.person_position_brute AS
  SELECT person_id, position::text AS position, count(*)::int AS nombre_votes
    FROM core.ballot
   GROUP BY 1, 2;

CREATE UNIQUE INDEX person_position_brute_pk
  ON mv.person_position_brute (person_id, position);

COMMENT ON MATERIALIZED VIEW mv.person_position_brute IS
  'Décompte par (personne, position d''origine) sur core.ballot — remplace '
  'le GROUP BY sur la totalité de core.ballot que loadPersons refaisait à '
  'chaque construction (internal/sitegen/load.go). Rafraîchie par '
  'internal/matview, jamais par un GROUP BY applicatif.';

-- +goose Down
DROP MATERIALIZED VIEW mv.person_position_brute;
