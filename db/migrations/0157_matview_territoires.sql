-- +goose Up

-- Les cartes départementales de internal/sitegen/territoires.go
-- (loadTerritoires) recalculaient, à chaque construction, six agrégations
-- nationales sur core.commune_indicator (35k communes) — dont trois
-- répétaient LITTÉRALEMENT la même sous-requête (population par
-- département, 2023) séparément. mv.dept_population la porte une fois ;
-- les matvues suivantes la lisent PAR SELECT, une matvue construite sur une
-- autre — REFRESH ne cascade pas tout seul, l'ordre de création ci-dessous
-- (population d'abord) est aussi celui qu'internal/matview doit respecter
-- au rafraîchissement tant qu'un vrai graphe de dépendances n'existe pas
-- (voir la Definition Go, ordonnée à la main dans la même intention).
--
-- Toutes deux gardent l'année 2023 en dur pour la population — comme le
-- faisait déjà le code Go qu'elles remplacent — sauf mv.dept_population
-- elle-même, qui porte TOUTES les années disponibles : une matvue de base
-- n'a aucune raison de décider pour ses consommateurs quelle année leur
-- importe.
CREATE MATERIALIZED VIEW mv.dept_population AS
  SELECT c.code_departement,
         max(c.nom_clair)  AS nom_departement,
         d.period_year,
         sum(d.value)       AS population
    FROM core.commune_indicator d
    JOIN ref.commune c ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
   WHERE d.indicator_code = 'ofgl.population_totale'
   GROUP BY c.code_departement, d.period_year;

CREATE UNIQUE INDEX dept_population_pk ON mv.dept_population (code_departement, period_year);

COMMENT ON MATERIALIZED VIEW mv.dept_population IS
  'Population totale par département et par année (ofgl.population_totale, '
  'sommée sur les communes) — base commune à plusieurs autres matvues du '
  'schéma mv (voir leur définition), pour ne calculer ce dénominateur '
  'qu''une fois. Rafraîchie AVANT elles (voir internal/matview.Catalogue, '
  'ordonné à la main).';

-- Les quatre indicateurs financiers communaux moyennés par habitant, par
-- département — remplace la fermeture fin() de loadTerritoires, appelée
-- quatre fois avec un indicator_code différent à chaque construction.
CREATE MATERIALIZED VIEW mv.dept_indicateur_communal AS
  SELECT c.code_departement,
         max(c.nom_clair)                                       AS nom_departement,
         d.indicator_code,
         d.period_year,
         sum(d.value * p.value) / nullif(sum(p.value), 0)        AS valeur_par_hab
    FROM core.commune_indicator d
    JOIN core.commune_indicator p ON p.commune_code = d.commune_code
     AND p.period_year = d.period_year AND p.indicator_code = 'ofgl.population_totale'
    JOIN ref.commune c ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
   WHERE d.indicator_code IN ('ofgl.dette_par_hab', 'ofgl.investissement_par_hab',
                               'ofgl.epargne_brute_par_hab', 'ofgl.masse_salariale_par_hab')
   GROUP BY c.code_departement, d.indicator_code, d.period_year;

CREATE UNIQUE INDEX dept_indicateur_communal_pk
  ON mv.dept_indicateur_communal (code_departement, indicator_code, period_year);

COMMENT ON MATERIALIZED VIEW mv.dept_indicateur_communal IS
  'Dette, investissement, épargne brute et masse salariale communales, par '
  'habitant et par département — remplace fin() (internal/sitegen/'
  'territoires.go, loadTerritoires), rappelée une fois par indicateur à '
  'chaque construction.';

-- Densité associative (RNA) pour 1 000 habitants, par département — 2023
-- pour la population, comme le code Go qu'elle remplace.
CREATE MATERIALIZED VIEW mv.dept_association_densite AS
  SELECT c.code_departement,
         max(c.nom_clair)                                           AS nom_departement,
         1000.0 * count(a.rna_id) / nullif(max(pop.population), 0)  AS pour_mille
    FROM ref.commune c
    JOIN mv.dept_population pop ON pop.code_departement = c.code_departement AND pop.period_year = 2023
    LEFT JOIN core.association a ON a.commune_code = c.code_insee
   GROUP BY c.code_departement;

CREATE UNIQUE INDEX dept_association_densite_pk ON mv.dept_association_densite (code_departement);

COMMENT ON MATERIALIZED VIEW mv.dept_association_densite IS
  'Associations (RNA) déclarées pour 1 000 habitants, par département — '
  'remplace le bloc « densité associative » de loadTerritoires '
  '(internal/sitegen/territoires.go). Construite sur mv.dept_population.';

-- Médecins généralistes pour 100 000 habitants, par département — 2024
-- pour l'effectif (Cnam), 2023 pour la population, comme le code Go.
CREATE MATERIALIZED VIEW mv.dept_medecin_generaliste AS
  SELECT m.dep,
         max(m.libelle_departement)                                  AS nom_departement,
         100000.0 * sum(m.effectif) / nullif(max(pop.population), 0) AS pour_100k
    FROM (SELECT *, CASE WHEN code_departement ~ '^[0-9]$'
                     THEN '0' || code_departement ELSE code_departement END AS dep
            FROM core.medecin_secteur_effectif) m
    JOIN mv.dept_population pop ON pop.code_departement = m.dep AND pop.period_year = 2023
   WHERE m.annee = 2024 AND m.code_departement <> '999'
     AND m.profession_sante IN
       ('Médecins généralistes (hors médecins à expertise particulière - MEP)',
        'Médecins généralistes à expertise particulière (MEP)')
   GROUP BY m.dep;

CREATE UNIQUE INDEX dept_medecin_generaliste_pk ON mv.dept_medecin_generaliste (dep);

COMMENT ON MATERIALIZED VIEW mv.dept_medecin_generaliste IS
  'Médecins généralistes libéraux conventionnés pour 100 000 habitants, par '
  'département (Cnam 2024, population OFGL 2023) — remplace le bloc '
  '« médecins généralistes » de loadTerritoires (internal/sitegen/'
  'territoires.go). Construite sur mv.dept_population.';

-- Part des sièges municipaux 2026 dont la nuance nomme un parti, par
-- département — aucune dépendance à mv.dept_population (le dénominateur
-- est le nombre de sièges nuancés, pas la population).
CREATE MATERIALIZED VIEW mv.dept_part_partisane AS
  WITH s AS (
    SELECT c.code_departement AS dep, max(c.nom_clair) AS nom, ml.nuance_code AS nc, sum(ml.sieges_cm) AS sg
      FROM core.municipal_list ml
      JOIN ref.commune c ON c.code_insee = ml.commune_code AND c.cog_millesime = ml.cog_millesime
     WHERE ml.scrutin_annee = 2026 AND ml.sieges_cm > 0
       AND ml.nuance_code IS NOT NULL AND ml.nuance_code <> ''
     GROUP BY 1, 3)
  SELECT dep,
         max(nom)                                                      AS nom_departement,
         100.0 * coalesce(sum(sg) FILTER (WHERE nc IN
           ('LLR','LRN','LSOC','LFI','LCOM','LVEC','LUDR','LUXD','LEXD','LECO')), 0) / sum(sg) AS pct
    FROM s
   GROUP BY 1;

CREATE UNIQUE INDEX dept_part_partisane_pk ON mv.dept_part_partisane (dep);

COMMENT ON MATERIALIZED VIEW mv.dept_part_partisane IS
  'Part des sièges municipaux 2026 dont la nuance nomme un parti, par '
  'département — remplace le bloc « part-partisane » de loadTerritoires '
  '(internal/sitegen/territoires.go).';

-- Foyers au RSA pour 1 000 habitants, par département, au dernier mois
-- disponible — remplace le bloc RSA. « Dernier mois » se recalcule à
-- chaque REFRESH : jamais figé à une date que la migration aurait
-- connue au moment de son écriture.
CREATE MATERIALIZED VIEW mv.dept_rsa AS
  SELECT p.code_geo,
         p.nom_geo,
         p.mois,
         1000.0 * p.valeur / nullif(pop.population, 0) AS pour_mille
    FROM core.prestation_solidarite p
    JOIN mv.dept_population pop ON pop.code_departement = p.code_geo AND pop.period_year = 2023
   WHERE p.niveau = 'DEPARTEMENT' AND p.serie = 'RSA_beneficiaires'
     AND p.mois = (SELECT max(mois) FROM core.prestation_solidarite
                    WHERE serie = 'RSA_beneficiaires' AND niveau = 'DEPARTEMENT');

CREATE UNIQUE INDEX dept_rsa_pk ON mv.dept_rsa (code_geo);

COMMENT ON MATERIALIZED VIEW mv.dept_rsa IS
  'Foyers au RSA pour 1 000 habitants, par département, au dernier mois '
  'CNAF disponible — remplace le bloc RSA de loadTerritoires '
  '(internal/sitegen/territoires.go). Construite sur mv.dept_population.';

-- +goose Down
DROP MATERIALIZED VIEW mv.dept_rsa;
DROP MATERIALIZED VIEW mv.dept_part_partisane;
DROP MATERIALIZED VIEW mv.dept_medecin_generaliste;
DROP MATERIALIZED VIEW mv.dept_association_densite;
DROP MATERIALIZED VIEW mv.dept_indicateur_communal;
DROP MATERIALIZED VIEW mv.dept_population;
