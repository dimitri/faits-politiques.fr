-- +goose Up
-- Les budgets des autres niveaux de collectivité.
--
-- Les communes ne sont qu'un étage. Régions, départements et groupements à
-- fiscalité propre décident de masses comparables ou supérieures, et sur des
-- compétences qui expliquent une partie de ce que les communes ne font plus :
-- les collèges et le social au département, les lycées et les transports à la
-- région, l'eau et les déchets à l'intercommunalité (D-024).
--
-- Même source que les comptes communaux — l'OFGL, Licence Ouverte — donc mêmes
-- conventions et mêmes agrégats. C'est ce qui rend les niveaux comparables
-- entre eux, à condition de ne jamais les additionner : une dépense portée par
-- un groupement l'est POUR ses communes membres, et la sommer avec la leur
-- compterait deux fois le même euro.
CREATE TABLE core.collectivite_budget (
  -- REGION, DEPARTEMENT, GROUPEMENT. Les communes restent dans
  -- core.commune_indicator, qui leur ajoute le millésime du COG.
  niveau          text NOT NULL CHECK (niveau IN ('REGION','DEPARTEMENT','GROUPEMENT')),
  -- Code INSEE pour les régions et départements, SIREN pour les groupements.
  code            text NOT NULL,
  nom             text NOT NULL,
  exercice        integer NOT NULL,
  indicator_code  text NOT NULL REFERENCES ref.indicator(code),
  montant         numeric,
  euros_par_hab   numeric,
  population      integer,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  provenance      core.provenance NOT NULL DEFAULT 'OFFICIAL',
  PRIMARY KEY (niveau, code, exercice, indicator_code)
);

CREATE INDEX collectivite_budget_niveau_idx ON core.collectivite_budget (niveau, exercice);
CREATE INDEX collectivite_budget_code_idx ON core.collectivite_budget (code);

COMMENT ON TABLE core.collectivite_budget IS
  'Comptes des régions, départements et groupements à fiscalité propre. '
  'NE JAMAIS ADDITIONNER LES NIVEAUX : une dépense portée par un groupement '
  'l''est pour ses communes membres.';

-- Ce que chaque niveau pèse, année par année. Vue plutôt que table : elle se
-- déduit entièrement, et une copie divergerait au premier rechargement.
CREATE VIEW derived.poids_des_niveaux AS
  SELECT b.exercice, b.niveau, i.label AS indicateur,
         count(DISTINCT b.code) AS collectivites,
         sum(b.montant) AS total
    FROM core.collectivite_budget b
    JOIN ref.indicator i ON i.code = b.indicator_code
   GROUP BY 1, 2, 3
  UNION ALL
  SELECT ci.period_year, 'COMMUNE', i.label,
         count(DISTINCT ci.commune_code), sum(ci.value * pop.population)
    FROM core.commune_indicator ci
    JOIN ref.indicator i ON i.code = ci.indicator_code
    LEFT JOIN LATERAL (
      SELECT value::integer AS population FROM core.commune_indicator p
       WHERE p.commune_code = ci.commune_code AND p.period_year = ci.period_year
         AND p.indicator_code = 'ofgl.population_totale' LIMIT 1
    ) pop ON true
   WHERE ci.indicator_code <> 'ofgl.population_totale'
   GROUP BY 1, 2, 3;

COMMENT ON VIEW derived.poids_des_niveaux IS
  'Masse par niveau de collectivité et par indicateur. Les communes sont '
  'converties du par-habitant au total ; les niveaux ne s''additionnent pas.';

-- +goose Down
DROP VIEW derived.poids_des_niveaux;
DROP TABLE core.collectivite_budget;
