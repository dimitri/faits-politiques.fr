-- +goose Up

-- Deux pivots sur core.collectivite_budget (88 000 lignes) que
-- internal/sitegen/collectivites.go (loadCollectivites) recalculait à
-- chaque construction, une fois par niveau de collectivité :
-- jsonb_object_agg(indicator_code, ...) transforme les lignes en colonnes,
-- une opération qui coûte à chaque construction ce qu'elle coûterait de
-- toute façon une fois par ingestion.
--
-- Toutes les années présentes, jamais une seule figée : loadCollectivites
-- calcule son "exercice" par un max() à chaque construction (les données
-- OFGL arrivent avec des millésimes différents) — la matvue ne doit pas
-- décider pour lui laquelle regarder.

-- mv.collectivite_budget_pivot : régions et départements — nom et
-- population viennent directement de core.collectivite_budget (max(),
-- puisque chaque ligne indicateur les répète).
CREATE MATERIALIZED VIEW mv.collectivite_budget_pivot AS
  SELECT niveau, code, exercice,
         max(nom)                                    AS nom,
         max(population)                              AS population,
         jsonb_object_agg(indicator_code, montant)     AS totaux,
         jsonb_object_agg(indicator_code, euros_par_hab) AS par_hab
    FROM core.collectivite_budget
   WHERE niveau IN ('REGION', 'DEPARTEMENT')
   GROUP BY niveau, code, exercice;

CREATE UNIQUE INDEX collectivite_budget_pivot_pk
  ON mv.collectivite_budget_pivot (niveau, code, exercice);

COMMENT ON MATERIALIZED VIEW mv.collectivite_budget_pivot IS
  'Budgets régionaux et départementaux, un jsonb par indicateur (montant, '
  'euros_par_hab) — remplace la fermeture charger() de loadCollectivites '
  '(internal/sitegen/collectivites.go), rappelée une fois par niveau à '
  'chaque construction.';

-- mv.epci : un groupement à fiscalité propre par ligne, SANS dépendre d'un
-- exercice budgétaire — core.epci n'en a pas, une seule ligne par SIREN
-- existe. Séparée du budget (ci-dessous) précisément pour que CHAQUE EPCI
-- reste présent même pour un exercice où son budget n'a pas encore été
-- publié : un JOIN sur mv.epci_budget_exercice filtré à un exercice précis
-- ne doit jamais faire disparaître l'EPCI lui-même, seulement vider son
-- par_hab — un LEFT JOIN au moment de la lecture (comme le faisait déjà
-- loadCollectivites), pas une ligne par (siren, exercice) qui aurait
-- justement fait disparaître les exercices sans données.
CREATE MATERIALIZED VIEW mv.epci AS
  SELECT e.siren, e.nom, e.nature_juridique,
         coalesce(e.code_departement, '')                                          AS code_departement,
         coalesce(e.population_totale, 0)                                          AS population,
         coalesce(e.nb_membres, 0)                                                  AS nb_membres,
         trim(coalesce(e.president_prenom, '') || ' ' || coalesce(e.president_nom, '')) AS president,
         (SELECT count(*) FROM core.epci_competence x WHERE x.epci_siren = e.siren)  AS nb_competences
    FROM core.epci e
   WHERE e.nature_juridique = ANY(ARRAY['CC','CA','CU','METRO','MET69','EPT']);

CREATE UNIQUE INDEX epci_pk ON mv.epci (siren);
CREATE INDEX epci_departement_idx ON mv.epci (code_departement);

COMMENT ON MATERIALIZED VIEW mv.epci IS
  'Un groupement à fiscalité propre par ligne (nom, nature, département, '
  'population, compétences) — indépendant de tout exercice budgétaire, '
  'voir mv.epci_budget_exercice pour le budget. Remplace la partie '
  '« dimension » du bloc groupements de loadCollectivites (internal/'
  'sitegen/collectivites.go).';

-- mv.epci_budget_exercice : le budget par habitant de chaque EPCI, un
-- jsonb par indicateur, UNE LIGNE PAR EXERCICE où des données existent
-- réellement — jamais une ligne fabriquée pour un exercice sans donnée
-- (voir le commentaire de mv.epci ci-dessus pour pourquoi).
CREATE MATERIALIZED VIEW mv.epci_budget_exercice AS
  SELECT code AS siren, exercice,
         jsonb_object_agg(indicator_code, euros_par_hab) AS par_hab
    FROM core.collectivite_budget
   WHERE niveau = 'GROUPEMENT'
   GROUP BY code, exercice;

CREATE UNIQUE INDEX epci_budget_exercice_pk ON mv.epci_budget_exercice (siren, exercice);

COMMENT ON MATERIALIZED VIEW mv.epci_budget_exercice IS
  'Budget par habitant de chaque EPCI (jsonb par indicateur), par exercice '
  '— remplace la partie budgétaire du bloc groupements de loadCollectivites '
  '(internal/sitegen/collectivites.go). À joindre à mv.epci par un LEFT '
  'JOIN filtré sur l''exercice voulu, pour qu''un EPCI sans budget cette '
  'année-là reste présent, par_hab vide.';

-- +goose Down
DROP MATERIALIZED VIEW mv.epci_budget_exercice;
DROP MATERIALIZED VIEW mv.epci;
DROP MATERIALIZED VIEW mv.collectivite_budget_pivot;
