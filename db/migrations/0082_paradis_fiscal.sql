-- +goose Up
-- « La France est-elle un paradis fiscal ? » Voir docs/paradis-fiscal-donnees.md.
--
-- Aucune définition internationale unique n'existe. Trois grilles officielles
-- et une mesure économique servent de référence :
--   1. les quatre facteurs de l'OCDE (1998) : imposition nulle ou symbolique,
--      absence d'échange d'informations, manque de transparence, absence
--      d'exigence d'activité substantielle ;
--   2. la liste européenne des juridictions non coopératives (critères du
--      Conseil, annexe V des conclusions de 2017) — qui par construction
--      n'examine pas les États membres ;
--   3. la liste française des États et territoires non coopératifs (ETNC,
--      article 238-0 A du CGI) ;
--   4. la mesure du transfert de bénéfices : où les multinationales déclarent
--      leurs profits par rapport à leurs salariés et à leur chiffre d'affaires
--      (déclarations pays par pays agrégées par l'OCDE).
-- Ces tables rassemblent les données qui permettent de situer la France
-- sur chacune, et de chiffrer ce que déclarent en France les filiales de
-- groupes étrangers.

-- ---------------------------------------------------------------------------
-- 1. Les listes de juridictions non coopératives
-- ---------------------------------------------------------------------------
CREATE TABLE ref.juridiction_non_cooperative (
  liste        text NOT NULL CHECK (liste IN ('ETNC_FR','UE_ANNEXE_I')),
  version      date NOT NULL,          -- date de l'arrêté (FR) ou des conclusions du Conseil (UE)
  juridiction  text NOT NULL,          -- nom tel que le texte l'écrit
  motif        text,                   -- fondement ou raison de l'inscription, repris du texte
  jo_texte_id  text REFERENCES jo.texte(id),
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  document_id  bigint REFERENCES raw.document(id),
  PRIMARY KEY (liste, version, juridiction),
  CHECK (jo_texte_id IS NOT NULL OR document_id IS NOT NULL)
);

COMMENT ON TABLE ref.juridiction_non_cooperative IS
  'Listes officielles des juridictions non coopératives, chaque version. ETNC_FR : arrêtés pris '
  'en application de l''article 238-0 A du CGI, lus dans le corpus du Journal officiel. '
  'UE_ANNEXE_I : annexe I des conclusions du Conseil, qui ne peut contenir aucun État membre.';

-- ---------------------------------------------------------------------------
-- 2. Déclarations pays par pays, agrégées par l'OCDE (CbCR)
-- ---------------------------------------------------------------------------
-- Pour chaque pays de la tête de groupe (siege) et chaque juridiction où le
-- groupe opère : bénéfice avant impôt, impôt dû, salariés, chiffre
-- d'affaires... C'est la mesure la plus directe du transfert de bénéfices :
-- beaucoup de profit et peu de salariés dans une juridiction.
CREATE TABLE core.cbcr_agregat (
  annee          smallint NOT NULL,
  siege          text NOT NULL,        -- ISO 3166 alpha-3 du pays de la société mère ultime
  juridiction    text NOT NULL,        -- ISO alpha-3, ou agrégat OCDE
  mesure         text NOT NULL,        -- code OCDE : PROFIT, TAX_ACCRUED, TAX_PAID, EMPLOYEES, TOT_REV...
  groupe_profit  text NOT NULL,        -- _T tous groupes ; PANELAI / PANELAII : sous-panels de l'OCDE
  unite          text NOT NULL,        -- USD, PS (personnes), PT_PRFT_BF_TAX...
  valeur         numeric NOT NULL,
  document_id    bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (annee, siege, juridiction, mesure, groupe_profit)
);

-- ---------------------------------------------------------------------------
-- 3. Indicateurs fiscaux par pays (OCDE, statistiques de l'impôt sur les sociétés)
-- ---------------------------------------------------------------------------
CREATE TABLE core.fiscalite_pays (
  pays         text NOT NULL,          -- ISO alpha-3
  annee        smallint NOT NULL,
  indicateur   text NOT NULL,          -- CIT_TAUX_LEGAL, EATR, EMTR, IP_TAUX, WHT_DIVIDENDES...
  variante     text NOT NULL DEFAULT '_T',  -- précision de l'indicateur (type d'actif, scénario)
  valeur       numeric,
  valeur_texte text,                   -- quand l'OCDE publie un libellé plutôt qu'un nombre
  unite        text,
  document_id  bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (pays, annee, indicateur, variante),
  CHECK (valeur IS NOT NULL OR valeur_texte IS NOT NULL)
);

-- ---------------------------------------------------------------------------
-- 4. Revenus des investissements directs, par pays de contrepartie (OCDE, BMD4)
-- ---------------------------------------------------------------------------
-- Ce que rapportent les filiales : dividendes versés, bénéfices réinvestis,
-- intérêts. Les entités à vocation spéciale (SPE) — sociétés sans activité
-- qui ne font que transiter des flux — sont publiées à part : leur poids dans
-- l'investissement d'un pays est un indicateur de paradis fiscal.
CREATE TABLE core.ide_revenu (
  pays_declarant  text NOT NULL,       -- ISO alpha-3
  annee           smallint NOT NULL,
  contrepartie    text NOT NULL,       -- ISO alpha-3 ou agrégat OCDE (W = monde)
  direction       text NOT NULL CHECK (direction IN ('ENTRANT','SORTANT')),
  composante      text NOT NULL,       -- TOTAL, DIVIDENDES, BENEFICES_REINVESTIS, REVENUS_ACTIONS, INTERETS
  type_entite     text NOT NULL CHECK (type_entite IN ('TOUTES','SPE','HORS_SPE')),
  unite           text NOT NULL,       -- USD_EXC (dollars), RC (monnaie nationale)
  valeur          numeric NOT NULL,
  document_id     bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (pays_declarant, annee, contrepartie, direction, composante, type_entite, unite)
);

-- ---------------------------------------------------------------------------
-- 5. Filiales sous contrôle étranger (Eurostat, statistiques FATS)
-- ---------------------------------------------------------------------------
CREATE TABLE core.fats_controle (
  pays_hote       text NOT NULL,       -- code Eurostat (FR)
  annee           smallint NOT NULL,
  pays_controle   text NOT NULL,       -- code Eurostat du pays de l'unité institutionnelle contrôlante
  activite        text NOT NULL,       -- NACE Rev. 2
  indicateur      text NOT NULL,       -- code Eurostat (ENT_NR, EMP_NR, AV_MEUR, GOS_MEUR, V12110...)
  serie           text NOT NULL,       -- jeu Eurostat : fats_g1b_08 (2008-2020) ou fats_ctrl (2021-)
  valeur          numeric NOT NULL,
  document_id     bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (pays_hote, annee, pays_controle, activite, indicateur, serie)
);

-- ---------------------------------------------------------------------------
-- 6. Filiales françaises de groupes étrangers, société par société
-- ---------------------------------------------------------------------------
-- Deux origines, jamais confondues :
--   GLEIF      la société déclare elle-même sa mère ultime dans le répertoire
--              mondial des LEI (niveau 2, licence CC0) : repérage systématique
--              mais partiel, les déclarations étant incomplètes ;
--   SELECTION  une sélection nommée (GAFAM, Disney...), dont le rattachement au
--              groupe est une information publique mais qu'aucun identifiant ne
--              relie : le fondement est écrit dans « justification ».
CREATE TABLE core.filiale_groupe_etranger (
  siren          text NOT NULL CHECK (siren ~ '^[0-9]{9}$'),
  origine        text NOT NULL CHECK (origine IN ('GLEIF','SELECTION')),
  groupe         text NOT NULL,        -- dénomination de la mère ultime
  pays_groupe    text NOT NULL,        -- ISO alpha-2 de la mère ultime
  lei            text,
  lei_groupe     text,
  justification  text,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  document_id    bigint REFERENCES raw.document(id),
  PRIMARY KEY (siren, origine),
  CHECK (origine = 'GLEIF' AND lei IS NOT NULL AND lei_groupe IS NOT NULL
         OR origine = 'SELECTION' AND justification IS NOT NULL)
);

-- Les comptes : ratios financiers publiés par l'INPI et la Banque centrale
-- européenne. Pas l'impôt sur les sociétés lui-même : le jeu donne le chiffre
-- d'affaires, le résultat courant avant impôt (en % du CA) et le résultat
-- net. La différence entre les deux mêle impôt, résultat exceptionnel et
-- participation des salariés : elle est publiée comme telle, jamais comme
-- « l'impôt payé ».
CREATE TABLE core.entreprise_comptes (
  siren                 text NOT NULL CHECK (siren ~ '^[0-9]{9}$'),
  date_cloture          date NOT NULL,
  type_bilan            text NOT NULL,         -- C complet, S simplifié, K consolidé...
  chiffre_affaires      numeric,
  ebe                   numeric,
  resultat_courant_ai   numeric,               -- CA × ratio publié ; NULL si l'un manque
  resultat_net          numeric,
  confidentialite       text,
  document_id           bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (siren, date_cloture, type_bilan)
);

-- ---------------------------------------------------------------------------
-- 7. Estimations du transfert de bénéfices (Tørsløv, Wier, Zucman)
-- ---------------------------------------------------------------------------
CREATE TABLE core.transfert_benefices_estimation (
  pays          text NOT NULL,          -- nom ou code tel que publié
  annee         smallint NOT NULL,
  indicateur    text NOT NULL,
  valeur        numeric NOT NULL,
  unite         text NOT NULL,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  document_id   bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (pays, annee, indicateur)
);

-- ---------------------------------------------------------------------------
-- Vues
-- ---------------------------------------------------------------------------

-- Le profit par salarié et l'impôt par dollar de profit, juridiction par
-- juridiction : la signature d'un centre de profits sans activité.
CREATE VIEW derived.cbcr_juridiction AS
WITH p AS (
  SELECT annee, siege, juridiction,
         max(valeur) FILTER (WHERE mesure = 'PROFIT')      AS benefice_usd,
         max(valeur) FILTER (WHERE mesure = 'TAX_ACCRUED') AS impot_du_usd,
         max(valeur) FILTER (WHERE mesure = 'TAX_PAID')    AS impot_paye_usd,
         max(valeur) FILTER (WHERE mesure = 'EMPLOYEES')   AS salaries,
         max(valeur) FILTER (WHERE mesure = 'TOT_REV')     AS chiffre_affaires_usd,
         max(valeur) FILTER (WHERE mesure = 'RPR')         AS ca_intragroupe_usd,
         max(valeur) FILTER (WHERE mesure = 'ASSETS')      AS actifs_corporels_usd
  FROM core.cbcr_agregat WHERE groupe_profit = '_T'
  GROUP BY annee, siege, juridiction
)
SELECT p.*,
       benefice_usd / nullif(salaries, 0)                          AS benefice_par_salarie_usd,
       round(100 * impot_du_usd / nullif(benefice_usd, 0), 1)      AS impot_sur_benefice_pct,
       round(100 * ca_intragroupe_usd / nullif(chiffre_affaires_usd, 0), 1) AS part_ca_intragroupe_pct,
       'cbcr-juridiction-v1'::text                                 AS method_version
FROM p;

COMMENT ON VIEW derived.cbcr_juridiction IS
  'Indicateurs de transfert de bénéfices par juridiction, depuis les déclarations pays par pays '
  'agrégées par l''OCDE. Tous groupes confondus, pertes comprises : un taux d''impôt sur bénéfice '
  'élevé peut refléter des filiales déficitaires qui paient un impôt minimum.';

-- Les filiales de groupes étrangers et leurs derniers comptes publiés.
CREATE VIEW derived.filiale_etrangere_comptes AS
WITH dernier AS (
  SELECT DISTINCT ON (siren) *
  FROM core.entreprise_comptes
  WHERE type_bilan IN ('C','S')
  ORDER BY siren, date_cloture DESC, type_bilan
)
SELECT f.siren, f.origine, f.groupe, f.pays_groupe, u.denomination, u.categorie_entreprise,
       d.date_cloture, d.chiffre_affaires, d.resultat_courant_ai, d.resultat_net,
       d.resultat_courant_ai - d.resultat_net AS ecart_rcai_resultat_net,
       round(100 * d.resultat_courant_ai / nullif(d.chiffre_affaires, 0), 2) AS marge_courante_pct,
       d.confidentialite,
       'filiale-etrangere-comptes-v1'::text AS method_version
FROM core.filiale_groupe_etranger f
LEFT JOIN ref.unite_legale u USING (siren)
LEFT JOIN dernier d USING (siren);

COMMENT ON COLUMN derived.filiale_etrangere_comptes.ecart_rcai_resultat_net IS
  'Résultat courant avant impôt moins résultat net : impôt sur les bénéfices, mais aussi résultat '
  'exceptionnel et participation des salariés. Ce n''est pas l''impôt payé.';

-- +goose Down
DROP VIEW derived.filiale_etrangere_comptes;
DROP VIEW derived.cbcr_juridiction;
DROP TABLE core.transfert_benefices_estimation;
DROP TABLE core.entreprise_comptes;
DROP TABLE core.filiale_groupe_etranger;
DROP TABLE core.fats_controle;
DROP TABLE core.ide_revenu;
DROP TABLE core.fiscalite_pays;
DROP TABLE core.cbcr_agregat;
DROP TABLE ref.juridiction_non_cooperative;
