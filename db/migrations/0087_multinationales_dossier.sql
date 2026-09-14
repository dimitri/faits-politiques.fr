-- +goose Up
-- Évasion fiscale des multinationales : ce que la France ne perçoit pas, et ce
-- qu'elle verse. Voir docs/evasion-fiscale-multinationales.md et D-061.
--
-- Complète la migration 0082 (listes, OCDE, estimations, filiales et comptes)
-- par trois choses :
--   core.marche_public_cible   les marchés publics (données essentielles de la
--                              commande publique) dont le titulaire est une
--                              société d'un groupe suivi, ou dont l'objet nomme
--                              son produit (licences achetées via un revendeur) ;
--   ref.fait_multinationale    les faits établis hors données ouvertes :
--                              contrats révélés au Parlement, règlements
--                              fiscaux, constats d'enquêtes parlementaires ;
--   des vues par groupe        comptes, aides, marchés, faits, côte à côte.
-- Rien n'y additionne des natures différentes : un plafond d'accord-cadre n'est
-- pas une dépense, une aide ADEME n'est pas une ligne du TAM, un chiffre
-- d'affaires n'est pas un impôt.

CREATE TABLE core.marche_public_cible (
  uid                  text NOT NULL,        -- SIRET acheteur + identifiant du marché (DECP consolidées)
  titulaire_id         text NOT NULL,
  titulaire_type_id    text,                 -- SIRET, TVA, HORS-UE…
  titulaire_nom        text,
  siren                text CHECK (siren ~ '^[0-9]{9}$'),
  groupe               text NOT NULL,
  -- Comment le marché a été rattaché au groupe, du plus sûr au moins sûr :
  --   SIREN   le SIREN du titulaire est celui d'une société du groupe ;
  --   NOM     la dénomination du titulaire nomme le groupe ;
  --   OBJET   l'objet du marché nomme un produit du groupe : le titulaire est
  --           le plus souvent un revendeur, et le montant couvre souvent
  --           d'autres produits.
  correspondance       text NOT NULL CHECK (correspondance IN ('SIREN','NOM','OBJET')),
  acheteur_id          text,
  acheteur_nom         text,
  acheteur_categorie   text,
  objet                text,
  nature               text,                 -- Marché, Accord-cadre, Marché subséquent…
  techniques           text,                 -- Accord-cadre, Système d'acquisition dynamique…
  procedure            text,
  code_cpv             text,
  date_notification    date,
  duree_mois           numeric,
  -- Montant publié : le montant du marché, ou le MAXIMUM d'un accord-cadre ;
  -- répété à l'identique sur chaque ligne d'un marché à plusieurs titulaires.
  montant_eur          numeric,
  montant_rationalise  numeric,              -- corrigé des valeurs aberrantes par le consolidateur
  montant_anomalie     text,
  source_decp          text,
  document_id          bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (uid, titulaire_id, groupe)
);

CREATE INDEX marche_public_cible_groupe_idx ON core.marche_public_cible (groupe, correspondance);

COMMENT ON TABLE core.marche_public_cible IS
  'Marchés publics (DECP consolidées, dernière version de chaque marché) rattachés à un groupe suivi. '
  'Montants : maximum pour les accords-cadres, jamais une dépense constatée ; ne pas sommer les lignes '
  'd''un même uid.';

CREATE TABLE ref.fait_multinationale (
  id            text PRIMARY KEY,
  groupe        text NOT NULL,
  type          text NOT NULL CHECK (type IN ('CONTRAT','REGULARISATION','CONSTAT_FISCAL','CONTROVERSE')),
  date_fait     date,
  periode       text,                        -- « 2013-2017 », quand le fait couvre une durée
  cocontractant text,                        -- entité du groupe partie au fait (société irlandaise…)
  acheteur      text,                        -- administration concernée
  intitule      text NOT NULL,
  montant_eur   numeric,
  -- PLAFOND (maximum d'un accord-cadre), ESTIME (montant prévisionnel),
  -- PAYE (somme versée), AMENDE, IMPOT (droits réclamés ou payés), CHIFFRE_AFFAIRES.
  nature_montant text CHECK (nature_montant IN ('PLAFOND','ESTIME','PAYE','AMENDE','IMPOT','CHIFFRE_AFFAIRES')),
  -- Qui établit le fait. OFFICIEL : texte ou réponse d'une institution ;
  -- PRESSE : révélé par un média, non confirmé officiellement ; ENTREPRISE :
  -- communiqué du groupe lui-même.
  qualite       text NOT NULL CHECK (qualite IN ('OFFICIEL','PRESSE','ENTREPRISE')),
  constat       text NOT NULL,               -- ce que la source établit, en une ou deux phrases
  source_url    text NOT NULL,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  document_id   bigint REFERENCES raw.document(id),
  CHECK (montant_eur IS NULL OR nature_montant IS NOT NULL),
  CHECK (qualite <> 'OFFICIEL' OR document_id IS NOT NULL)
);

-- Les marchés par groupe. Deux pièges des données essentielles, traités ici :
--   * un accord-cadre passé par une centrale d'achat ou un groupement est
--     republié par CHAQUE membre, avec le montant maximal du groupement
--     (160 M€ répétés chez des dizaines de communes) : les montants ne sont
--     comptés qu'une fois par triplet titulaire, date de notification, montant ;
--   * le consolidateur signale les montants suspects : ils sont écartés des
--     sommes, pas des comptes de marchés et d'acheteurs.
-- Les lignes où l'acheteur est la société titulaire elle-même (déclarations
-- erronées) sont écartées.
CREATE VIEW derived.multinationale_marches AS
WITH l AS (
  SELECT *, coalesce(montant_rationalise, montant_eur) AS montant,
         (coalesce(techniques, '') ILIKE '%accord-cadre%' OR coalesce(nature, '') ILIKE '%accord-cadre%') AS accord_cadre
  FROM core.marche_public_cible
  WHERE siren IS NULL OR left(acheteur_id, 9) <> siren
), u AS (   -- un marché compte une fois
  SELECT DISTINCT ON (groupe, uid) * FROM l
  ORDER BY groupe, uid, CASE correspondance WHEN 'SIREN' THEN 1 WHEN 'NOM' THEN 2 ELSE 3 END
), d AS (   -- un montant compte une fois
  SELECT DISTINCT ON (groupe, correspondance, titulaire_id, date_notification, montant) *
  FROM u WHERE coalesce(montant_anomalie, '') = '' AND montant IS NOT NULL
  ORDER BY groupe, correspondance, titulaire_id, date_notification, montant
)
SELECT u.groupe, u.correspondance,
       count(*)                                     AS marches,
       count(DISTINCT u.acheteur_id)                AS acheteurs,
       count(*) FILTER (WHERE u.accord_cadre)       AS dont_accords_cadres,
       (SELECT sum(montant) FROM d WHERE d.groupe = u.groupe AND d.correspondance = u.correspondance)
                                                    AS montants_dedoublonnes_eur,
       (SELECT sum(montant) FROM d WHERE d.groupe = u.groupe AND d.correspondance = u.correspondance AND NOT d.accord_cadre)
                                                    AS montants_hors_accords_cadres_eur,
       min(u.date_notification)                     AS premiere_notification,
       max(u.date_notification)                     AS derniere_notification,
       'multinationale-marches-v2'::text            AS method_version
FROM u
GROUP BY u.groupe, u.correspondance;

COMMENT ON VIEW derived.multinationale_marches IS
  'Marchés publics par groupe et mode de rattachement. montants_dedoublonnes_eur additionne des montants de '
  'marchés et des MAXIMA d''accords-cadres, dédoublonnés et hors montants suspects : un ordre de grandeur de '
  'l''engagement possible, pas une dépense. Rattachement OBJET : le montant couvre souvent d''autres produits.';

-- Les aides publiées par bénéficiaire, par groupe et par source (jamais sommées
-- entre sources), montants aberrants écartés.
CREATE VIEW derived.multinationale_aides AS
SELECT f.groupe, f.pays_groupe, a.source,
       count(*)                                   AS aides,
       count(DISTINCT a.siren)                    AS societes,
       sum(a.montant_nominal_eur)                 AS nominal_eur,
       sum(a.montant_esb_eur)                     AS esb_eur,
       min(a.date_octroi)                         AS premiere_aide,
       max(a.date_octroi)                         AS derniere_aide,
       'multinationale-aides-v1'::text            AS method_version
-- Un SIREN repéré à la fois par GLEIF et par la sélection garde le nom de groupe
-- de la sélection : les deux écritures (« THE WALT DISNEY COMPANY ») ne doivent
-- pas compter deux fois la même aide.
FROM (SELECT DISTINCT ON (siren) siren, groupe, pays_groupe FROM core.filiale_groupe_etranger
      ORDER BY siren, (origine = 'SELECTION') DESC) f
JOIN core.aide_nominative a USING (siren)
LEFT JOIN derived.aide_montant_suspect s ON s.source = a.source AND s.reference = a.reference
WHERE s.reference IS NULL
GROUP BY f.groupe, f.pays_groupe, a.source;

-- +goose Down
DROP VIEW derived.multinationale_aides;
DROP VIEW derived.multinationale_marches;
DROP TABLE ref.fait_multinationale;
DROP TABLE core.marche_public_cible;
