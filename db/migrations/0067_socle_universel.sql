-- +goose Up
-- Le socle universel articulé aux droits contributifs (docs/cotisations-et-
-- droits.md §7) chiffré au-delà d'un ordre de grandeur : une micro-simulation
-- par type de ménage, pas un forfait par personne.
--
-- Le seuil de pauvreté se lit PAR UNITÉ DE CONSOMMATION (UC), pas par personne
-- ni par ménage : un couple n'a pas besoin de deux fois le revenu d'une
-- personne seule pour vivre aussi bien. Un socle qui ignore ce fait soit
-- surpaie les couples (un forfait par adulte), soit sous-paie les familles
-- nombreuses (un forfait par ménage). Les cinq tables qui suivent portent les
-- données nécessaires pour distribuer le socle PAR UC et en simuler le
-- financement par une reprise fiscale ciblée sur les ménages aisés — pas en
-- ajoutant un forfait de plus, en le RÉCUPÉRANT là où le revenu avant
-- transferts dépasse déjà le seuil.

-- Série longue du seuil de pauvreté, chargée pour elle-même : c'est une donnée
-- qui intéresse au-delà de la simulation qui l'utilise (évolution du taux de
-- pauvreté, de son intensité, du seuil lui-même). La rupture de série 2019-2021
-- documentée par l'INSEE (refonte de l'enquête Revenus fiscaux et sociaux) est
-- laissée visible : 2020 EST chargée (l'Insee la publie) mais porte une valeur
-- que l'Insee signale elle-même comme fragile (collecte perturbée par le
-- confinement) — à afficher avec cette réserve, pas à escamoter ni à taire.
CREATE TABLE core.pauvrete_seuil_annuel (
  annee                  smallint NOT NULL CHECK (annee >= 1996),
  seuil_relatif          numeric NOT NULL CHECK (seuil_relatif IN (0.5, 0.6)),
  seuil_euros            numeric NOT NULL,
  nb_pauvres_milliers    integer NOT NULL,
  taux_pauvrete_pct      numeric NOT NULL,
  intensite_pauvrete_pct numeric NOT NULL,
  source_id              bigint NOT NULL REFERENCES raw.source(id),
  created_at             timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, seuil_relatif)
);

COMMENT ON TABLE core.pauvrete_seuil_annuel IS
  'Seuil de pauvreté, nombre de personnes pauvres, taux et intensité de la '
  'pauvreté, France métropolitaine, 1996-2023 (Insee, enquêtes Revenus fiscaux '
  'et sociaux). Euros constants de l''année de publication la plus récente : '
  'une série révisée à chaque publication, pas cumulable avec une édition '
  'antérieure de ce même tableau.';

-- Les grandes catégories de ménage retenues sont celles de la DREES (fiche 02
-- du Panorama « Minima sociaux et prestations de solidarité ») : elles seules
-- publient, pour chaque type, le revenu initial, les prestations non
-- contributives et les impôts directs — ce dont la simulation a besoin.
--
-- uc_empirique n'est PAS l'échelle d'équivalence théorique (1 / 0,5 / 0,3) : elle
-- est RECALCULÉE ici comme revenu_initial_menage / revenu_initial_uc, deux
-- chiffres que la DREES publie séparément pour les mêmes catégories (tableaux
-- 4a et 4b). Elle capture donc la composition RÉELLE de chaque catégorie —
-- « couple avec 2 enfants » n'est pas toujours exactement 2,0 enfants, et
-- « enfant » y inclut, sans limite d'âge, tout enfant célibataire du ménage.
-- Une échelle théorique appliquée à une catégorie hétérogène serait un chiffre
-- inventé maquillé en donnée.
CREATE TABLE core.menage_type_drees (
  type_menage                     text NOT NULL,
  annee                           smallint NOT NULL,
  uc_empirique                    numeric NOT NULL CHECK (uc_empirique > 0),
  revenu_initial_menage           numeric NOT NULL,
  prestations_non_contrib_menage  numeric NOT NULL,
  impots_directs_menage           numeric NOT NULL,
  revenu_disponible_menage        numeric NOT NULL,
  source_id                       bigint NOT NULL REFERENCES raw.source(id),
  created_at                      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (type_menage, annee)
);

COMMENT ON TABLE core.menage_type_drees IS
  'Montants mensuels moyens par ménage, par grand type de ménage (Drees, '
  'enquête ERFS, tableau 4a de la fiche « composition du revenu »). '
  'uc_empirique = revenu_initial_menage / revenu_initial_uc (tableau 4b), pas '
  'l''échelle d''équivalence théorique : voir le commentaire de la table.';

-- Effectif de ménages par type — recensement de la population, PAS l'enquête
-- ERFS qui fournit les montants : deux sources, deux universi proches mais pas
-- identiques (l'ERFS exclut les ménages dont le revenu déclaré est négatif ou
-- dont la personne de référence est étudiante). L'écart mesuré à l'usage —
-- 29,5 M de ménages couverts par ces catégories sur 30,5 M au recensement,
-- soit 97 % — est documenté dans docs/, pas corrigé en silence.
CREATE TABLE core.menage_type_effectif (
  type_menage text NOT NULL,
  annee       smallint NOT NULL,
  nb_menages  bigint NOT NULL CHECK (nb_menages > 0),
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (type_menage, annee)
);

COMMENT ON TABLE core.menage_type_effectif IS
  'Nombre de ménages par grand type (Insee, recensement de la population, '
  'via l''API Melodi). Univers proche de celui de core.menage_type_drees mais '
  'non identique : voir le commentaire de la table pour l''écart de couverture.';

-- Distribution de la pension mensuelle BRUTE de droit direct (y compris
-- majoration pour trois enfants), par tranche de 100 €. Sert à vérifier, sur
-- une vraie distribution plutôt que sur une moyenne, ce que change une reprise
-- fiscale non linéaire : la moyenne masque l'hétérogénéité, et une fonction non
-- linéaire appliquée à une moyenne ne redonne PAS la moyenne de la fonction
-- (inégalité de Jensen). Voir docs/revenu-universel-microsimulation.md §3.
CREATE TABLE core.pension_tranche_eir (
  annee         smallint NOT NULL,
  tranche_min   integer NOT NULL CHECK (tranche_min >= 0),
  -- NULL : tranche ouverte (« supérieur à 4 500 euros »).
  tranche_max   integer,
  pct_femmes    numeric NOT NULL,
  pct_hommes    numeric NOT NULL,
  pct_ensemble  numeric NOT NULL,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, tranche_min)
);

COMMENT ON TABLE core.pension_tranche_eir IS
  'Distribution de la pension mensuelle brute de droit direct des retraités '
  '(Drees, Échantillon interrégimes de retraités), par tranche de 100 euros. '
  'Champ : bénéficiaires d''un avantage principal de droit direct d''un régime '
  'de base, résidant en France ou à l''étranger.';

-- Même logique pour l'assurance chômage : répartition des allocataires
-- indemnisés par tranche de montant mensuel d'allocation. Le classeur Unédic
-- porte une feuille par trimestre depuis juin 2014 ; toutes celles reconnues
-- sont chargées (le connecteur les retrouve par leur titre, pas par un nom de
-- feuille attendu à l'avance — voir internal/macro/chomage_unedic.go).
CREATE TABLE core.chomage_tranche_unedic (
  date_reference date    NOT NULL,
  tranche_min    integer NOT NULL CHECK (tranche_min >= 0),
  tranche_max    integer, -- NULL : tranche ouverte (« 3 000 et plus »)
  effectif       integer NOT NULL,
  pct            numeric NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (date_reference, tranche_min)
);

COMMENT ON TABLE core.chomage_tranche_unedic IS
  'Répartition des allocataires de l''Assurance chômage (ARE, AREF, CSP) par '
  'tranche de montant mensuel d''indemnisation (Unédic, FNA). Allocataires de '
  'la solidarité-État (ASS, ATS, AER) exclus : leur montant dépend des '
  'ressources, pas d''un salaire de référence, la source ne les inclut pas.';

-- La simulation elle-même : une VUE, comme derived.pdr_corps_electoral
-- (0038_population_age.sql) — elle ne peut pas diverger de ses entrées, et une
-- table figerait un résultat que la moindre correction des tables sources
-- devrait pourtant répercuter.
--
-- MÉTHODE (method_version 'socle-uc-v1-reprise-lineaire') :
--   1. Le socle vaut le seuil de pauvreté (60 % de la médiane) multiplié par le
--      nombre d'UC du ménage — chaque ménage touche EXACTEMENT son propre seuil
--      de pauvreté si le socle était son seul revenu. Ni plus (un forfait par
--      adulte surpaierait les couples), ni moins (un forfait par ménage
--      sous-paierait les familles).
--   2. Une reprise fiscale récupère le socle À PROPORTION de l'écart entre le
--      revenu initial par UC du ménage et le seuil : 0 % de reprise au seuil,
--      100 % (recouvrement intégral) à deux fois le seuil. C'est une pente
--      unique sur le revenu initial TOTAL — salaires, pensions, allocation
--      chômage, patrimoine confondus — qui remplace à la fois les prestations
--      non contributives actuelles (supprimées, cf. prestations_non_contrib_
--      menage) et les règles différentes qu'exigerait, branche par branche,
--      l'architecture « dégressive » de docs/cotisations-et-droits.md §7.2.
--   3. Le delta mensuel par ménage = socle net de reprise − anciennes
--      prestations non contributives supprimées. Il NE COMPTE PAS les impôts
--      directs actuels, laissés inchangés : la reprise sur le socle est un
--      mécanisme ADDITIONNEL, pas une refonte du barème de l'impôt sur le
--      revenu.
--
-- LIMITE ASSUMÉE : les colonnes de core.menage_type_drees sont des MOYENNES par
-- catégorie. Appliquer une fonction non linéaire (la reprise, plafonnée à 0 et
-- 1) à une moyenne ne donne pas la moyenne de la fonction — voir le §3 de
-- docs/revenu-universel-microsimulation.md, qui chiffre le biais sur la
-- distribution réelle des pensions et de l'allocation chômage. Le total net
-- de cette vue SOUS-ESTIME donc la reprise véritable, et par construction
-- SURESTIME le coût net du socle.
CREATE VIEW derived.socle_universel_simulation AS
WITH seuil AS (
  SELECT seuil_euros FROM core.pauvrete_seuil_annuel
   WHERE seuil_relatif = 0.6 ORDER BY annee DESC LIMIT 1
)
SELECT d.type_menage,
       d.annee,
       e.nb_menages,
       d.uc_empirique,
       s.seuil_euros AS seuil_mensuel_uc,
       round(d.revenu_initial_menage / d.uc_empirique / s.seuil_euros, 3) AS ratio_revenu_initial_seuil,
       round(least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)), 3) AS taux_reprise,
       round(s.seuil_euros * d.uc_empirique) AS socle_brut_menage,
       round(s.seuil_euros * d.uc_empirique
             * (1 - least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)))) AS socle_net_menage,
       d.prestations_non_contrib_menage,
       round(s.seuil_euros * d.uc_empirique
             * (1 - least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)))
             - d.prestations_non_contrib_menage) AS delta_mensuel_menage,
       round(e.nb_menages
             * (s.seuil_euros * d.uc_empirique
                * (1 - least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)))
                - d.prestations_non_contrib_menage)
             * 12 / 1e9, 2) AS delta_annuel_md_euros,
       'socle-uc-v1-reprise-lineaire'::text AS method_version
  FROM core.menage_type_drees d
  JOIN core.menage_type_effectif e ON e.type_menage = d.type_menage AND e.annee = d.annee
 CROSS JOIN seuil s
 WHERE d.type_menage <> 'ensemble';

COMMENT ON VIEW derived.socle_universel_simulation IS
  'Micro-simulation du socle universel par unité de consommation, financé par '
  'une reprise fiscale linéaire entre 1 et 2 fois le seuil de pauvreté. '
  'Méthode et limites : voir le commentaire de la vue et '
  'docs/revenu-universel-microsimulation.md.';

-- +goose Down
DROP VIEW derived.socle_universel_simulation;
DROP TABLE core.chomage_tranche_unedic;
DROP TABLE core.pension_tranche_eir;
DROP TABLE core.menage_type_effectif;
DROP TABLE core.menage_type_drees;
DROP TABLE core.pauvrete_seuil_annuel;
