-- +goose Up

-- mv.population_nationale_annee : la population totale par année, sommée
-- sur les 657 000 lignes de core.population_historique_commune — ce que
-- chargerPopulationGuerres (internal/sitegen/seconde_guerre_mondiale.go)
-- recalculait à chaque construction pour ne garder, au final, que quatre
-- années (1911, 1921, 1936, 1946).
CREATE MATERIALIZED VIEW mv.population_nationale_annee AS
  SELECT annee, sum(population) AS population
    FROM core.population_historique_commune
   GROUP BY annee;

CREATE UNIQUE INDEX population_nationale_annee_pk ON mv.population_nationale_annee (annee);

COMMENT ON MATERIALIZED VIEW mv.population_nationale_annee IS
  'Population nationale par année (sommée sur les communes) — remplace le '
  'GROUP BY sur la totalité de core.population_historique_commune que '
  'chargerPopulationGuerres (internal/sitegen/seconde_guerre_mondiale.go) '
  'refaisait à chaque construction.';

-- +goose Down
DROP MATERIALIZED VIEW mv.population_nationale_annee;
