-- +goose Up

-- Les deux agrégations budgétaires par département que chargerFondSituation
-- (internal/sitegen/carte_situation.go) recalculait à chaque construction —
-- une fois pour les communes (pondérées par population, sur les 35k
-- communes), une fois pour les EPCI. Toutes les années disponibles : ces
-- deux requêtes sont déjà pariamétrées par exercice/period_year, la matvue
-- ne doit pas décider laquelle regarder.
CREATE MATERIALIZED VIEW mv.dept_budget_commune AS
  SELECT rc.code_departement,
         f.period_year,
         coalesce(sum(f.value*p.value) FILTER (WHERE f.indicator_code='ofgl.fonctionnement_par_hab'),0)::float8 AS fonctionnement,
         coalesce(sum(f.value*p.value) FILTER (WHERE f.indicator_code='ofgl.investissement_par_hab'),0)::float8 AS investissement
    FROM core.commune_indicator f
    JOIN core.commune_indicator p ON p.commune_code=f.commune_code AND p.period_year=f.period_year
     AND p.indicator_code='ofgl.population_totale'
    JOIN ref.commune rc ON rc.code_insee=f.commune_code AND rc.cog_millesime=f.cog_millesime
   WHERE f.indicator_code IN ('ofgl.fonctionnement_par_hab','ofgl.investissement_par_hab')
   GROUP BY rc.code_departement, f.period_year;

CREATE UNIQUE INDEX dept_budget_commune_pk ON mv.dept_budget_commune (code_departement, period_year);

COMMENT ON MATERIALIZED VIEW mv.dept_budget_commune IS
  'Fonctionnement et investissement communaux agrégés par département '
  '(montant par habitant × population), par année — remplace le premier '
  'bloc de chargerFondSituation (internal/sitegen/carte_situation.go).';

CREATE MATERIALIZED VIEW mv.dept_budget_epci AS
  SELECT e.code_departement,
         b.exercice,
         coalesce(sum(b.montant) FILTER (WHERE b.indicator_code='ofgl.fonctionnement_par_hab'),0)::float8 AS fonctionnement,
         coalesce(sum(b.montant) FILTER (WHERE b.indicator_code='ofgl.investissement_par_hab'),0)::float8 AS investissement
    FROM core.collectivite_budget b JOIN core.epci e ON e.siren=b.code
   WHERE b.niveau='GROUPEMENT' AND e.code_departement IS NOT NULL
   GROUP BY e.code_departement, b.exercice;

CREATE UNIQUE INDEX dept_budget_epci_pk ON mv.dept_budget_epci (code_departement, exercice);

COMMENT ON MATERIALIZED VIEW mv.dept_budget_epci IS
  'Fonctionnement et investissement des EPCI agrégés par département, par '
  'exercice — remplace le second bloc de chargerFondSituation (internal/'
  'sitegen/carte_situation.go).';

-- +goose Down
DROP MATERIALIZED VIEW mv.dept_budget_epci;
DROP MATERIALIZED VIEW mv.dept_budget_commune;
