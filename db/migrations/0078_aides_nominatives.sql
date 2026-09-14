-- +goose Up
-- Les aides publiées bénéficiaire par bénéficiaire, croisées avec la catégorie
-- d'entreprise de l'INSEE. Voir docs/dette-donnees.md § 15.
--
-- Trois sources, un format :
--   TAM     registre européen de transparence des aides d'État : les aides
--           importantes (au-delà de 500 k€, puis 100 k€), toutes autorités
--           françaises confondues ;
--   ADEME   les aides financières de l'ADEME, sans seuil ;
--   MINIMIS le registre public des aides de minimis (depuis 2026), plafonnées
--           à 300 k€ sur trois ans.
-- Elles se RECOUVRENT (une aide ADEME notifiée figure aussi au TAM ; une aide
-- de minimis peut venir de l'ADEME) : on ne somme jamais deux sources.

CREATE TABLE core.aide_nominative (
  source               text NOT NULL CHECK (source IN ('TAM','ADEME','MINIMIS')),
  reference            text NOT NULL,   -- identifiant de la ligne chez le producteur
  -- L'identifiant publié (SIREN, SIRET, parfois mal formé) et le SIREN qu'on en
  -- tire. Pour un bénéficiaire qui n'est pas une personne morale du répertoire
  -- (entrepreneur individuel, identifiant étranger ou illisible), le nom et
  -- l'identifiant ne sont PAS conservés : un SIREN de personne physique est une
  -- donnée personnelle, et aucun usage du projet ne demande de la nommer.
  siren                text CHECK (siren ~ '^[0-9]{9}$'),
  identifiant_publie   text,
  personne_morale      boolean NOT NULL,  -- SIREN présent dans ref.unite_legale
  nom_beneficiaire     text,
  -- TAM seulement. PETITE_ETI : « small mid-caps », catégorie européenne
  -- apparue en 2025 (moins de 750 salariés), entre PME et grande entreprise.
  type_declare         text CHECK (type_declare IN ('PME','PETITE_ETI','GRANDE_ENTREPRISE')),
  regime               text,              -- numéro SA, dispositif, régime
  intitule             text,              -- titre de la mesure ou objet de l'aide
  instrument           text,
  objectif             text,
  secteur              text,
  region               text,
  -- Trois montants, jamais confondus : le NOMINAL (somme accordée), l'ESB
  -- (équivalent-subvention brut : ce que vaut l'avantage, un prêt pesant moins
  -- que son montant) et, pour les avantages fiscaux que le TAM publie par
  -- tranches (« > 1 000 000 - 2 000 000 »), les bornes de la tranche.
  montant_nominal_eur  numeric,
  montant_esb_eur      numeric,
  tranche_min_eur      numeric,
  tranche_max_eur      numeric,
  date_octroi          date,
  date_publication     date,
  autorite             text,
  operateur            text,
  source_id            bigint NOT NULL REFERENCES raw.source(id),
  document_id          bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (source, reference),
  CHECK (personne_morale OR (nom_beneficiaire IS NULL AND identifiant_publie IS NULL))
);

CREATE INDEX aide_nominative_siren_idx ON core.aide_nominative (siren);

COMMENT ON TABLE core.aide_nominative IS
  'Aides publiques publiées par bénéficiaire (TAM, ADEME, minimis). Nom et identifiant conservés '
  'pour les seules personnes morales de SIRENE. Sources recouvrantes : ne pas les additionner.';

-- Les montants aberrants. Les registres reprennent ce que saisissent les
-- autorités, erreurs comprises : 1 061 M€ d'aide à l'investissement à une
-- exploitation agricole, dans un régime dont l'aide médiane est de 21 000 €.
-- On ne corrige pas une source ; on isole. Règle : plus de 10 M€ ET plus de
-- 10 000 fois la médiane du même régime, calculée sur au moins 20 aides. Un
-- seuil à 1 000 fois retenait aussi des aides réelles (le projet de réacteur
-- Nuward, 300 M€ ; le renouvellement forestier confié à l'ONF, 40 M€).
CREATE VIEW derived.aide_montant_suspect AS
WITH r AS (
  SELECT source, regime,
         percentile_cont(0.5) WITHIN GROUP (ORDER BY coalesce(montant_esb_eur, montant_nominal_eur)) AS mediane,
         count(*) AS n
  FROM core.aide_nominative
  WHERE coalesce(montant_esb_eur, montant_nominal_eur) IS NOT NULL
  GROUP BY source, regime
)
SELECT a.source, a.reference, a.nom_beneficiaire, a.regime,
       coalesce(a.montant_esb_eur, a.montant_nominal_eur) AS montant_eur,
       r.mediane AS mediane_regime_eur, r.n AS aides_du_regime,
       'aide-montant-suspect-v1'::text AS method_version
FROM core.aide_nominative a
JOIN r USING (source, regime)
WHERE coalesce(a.montant_esb_eur, a.montant_nominal_eur) > 10e6
  AND r.n >= 20
  AND coalesce(a.montant_esb_eur, a.montant_nominal_eur) > 10000 * r.mediane;

-- Répartition par catégorie d'entreprise INSEE, source par source et année par
-- année. Les bénéficiaires publics (catégorie juridique 4 et 7 : établissements
-- publics, collectivités) et les associations (92) sont isolés : l'argument
-- porte sur les entreprises, et l'ADEME verse des centaines de millions à des
-- intermédiaires publics (ASP) qui les reversent. Les montants suspects sont
-- comptés à part, hors des sommes.
CREATE VIEW derived.aide_par_categorie AS
WITH a AS (
  SELECT n.source, extract(year FROM n.date_octroi)::int AS annee, n.siren,
         CASE
           WHEN NOT n.personne_morale THEN 'HORS_PERSONNES_MORALES'
           WHEN u.categorie_juridique LIKE '7%' OR u.categorie_juridique LIKE '4%' THEN 'PUBLIC'
           WHEN u.categorie_juridique LIKE '92%' THEN 'ASSOCIATION'
           ELSE coalesce(u.categorie_entreprise, 'NON_CATEGORISEE')
         END AS categorie,
         s.reference IS NOT NULL AS suspect,
         n.montant_nominal_eur, n.montant_esb_eur, n.tranche_min_eur, n.tranche_max_eur
  FROM core.aide_nominative n
  LEFT JOIN ref.unite_legale u ON u.siren = n.siren
  LEFT JOIN derived.aide_montant_suspect s ON s.source = n.source AND s.reference = n.reference
)
SELECT source, annee, categorie,
       count(*)                                                    AS aides,
       count(DISTINCT siren)                                       AS beneficiaires,
       sum(montant_nominal_eur) FILTER (WHERE NOT suspect)         AS nominal_eur,
       sum(montant_esb_eur) FILTER (WHERE NOT suspect)             AS esb_eur,
       count(*) FILTER (WHERE suspect)                             AS aides_suspectes,
       count(*) FILTER (WHERE tranche_min_eur IS NOT NULL OR tranche_max_eur IS NOT NULL) AS aides_en_tranche,
       sum(tranche_min_eur)                                        AS tranches_min_eur,
       sum(tranche_max_eur)                                        AS tranches_max_eur,
       'aide-par-categorie-v1'::text                               AS method_version
FROM a
GROUP BY source, annee, categorie;

-- Le TAM demande à l'autorité de déclarer « PME » ou « grande entreprise ».
-- Confronter cette déclaration à la catégorie de l'INSEE mesure sa fiabilité.
CREATE VIEW derived.aide_tam_type_declare AS
SELECT n.type_declare,
       coalesce(u.categorie_entreprise, CASE WHEN n.personne_morale THEN 'NON_CATEGORISEE' ELSE 'HORS_PERSONNES_MORALES' END) AS categorie_insee,
       count(*)              AS aides,
       sum(n.montant_esb_eur) AS esb_eur,
       'aide-tam-type-declare-v1'::text AS method_version
FROM core.aide_nominative n
LEFT JOIN ref.unite_legale u ON u.siren = n.siren
WHERE n.source = 'TAM'
GROUP BY 1, 2;

-- +goose Down
DROP VIEW derived.aide_tam_type_declare;
DROP VIEW derived.aide_par_categorie;
DROP VIEW derived.aide_montant_suspect;
DROP TABLE core.aide_nominative;
