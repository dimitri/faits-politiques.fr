-- +goose Up
-- À quoi sert la dette, et que pèsent, à côté, les aides fiscales et sociales
-- et les dividendes. Voir docs/dette-donnees.md § 13.
--
-- Deux questions distinctes, deux traitements :
--
-- 1. « À quoi sert la dette » n'a pas de réponse au sens propre : l'argent
--    public est fongible, aucun euro emprunté n'est affecté à une dépense. La
--    comptabilité nationale donne en revanche une DÉCOMPOSITION exacte du besoin
--    de financement, par le compte de capital : épargne brute négative (les
--    recettes courantes ne couvrent pas les dépenses courantes), investissement,
--    transferts en capital nets. C'est la lecture de la « règle d'or », et elle
--    n'impute rien : elle dit ce qui resterait à financer si l'on finançait
--    d'abord le courant par le courant.
--
-- 2. Les aides fiscales (niches) et les exonérations de cotisations se mettent
--    EN REGARD du déficit et des dividendes, sans jamais s'additionner entre
--    elles : le CICE a été à la fois une dépense fiscale, une « exonération »
--    dans les séries de l'URSSAF et une subvention en comptabilité nationale.

-- Les opérations du compte des administrations publiques (SEC 2010), en
-- complément des concepts déjà présents. Le code de l'opération (P51G, B8G,
-- D9PAY...) est porté par la colonne instrument.
ALTER TABLE ref.dette_serie DROP CONSTRAINT dette_serie_concept_check;
ALTER TABLE ref.dette_serie ADD CONSTRAINT dette_serie_concept_check CHECK (concept IN (
  'DETTE_MAASTRICHT', 'DETTE_NEGOCIABLE_ETAT', 'DETENTION_TITRES_ETAT', 'DETTE_NETTE_APU',
  'DETTE_BRUTE_FMI', 'DETTE_NETTE_FMI', 'ACTIFS_COTES_APU', 'INTERETS_VERSES', 'SOLDE_PUBLIC',
  'SOLDE_PRIMAIRE', 'RECETTES_PUBLIQUES', 'DEPENSES_PUBLIQUES', 'TAUX_LONG_TERME', 'BILAN_APU_CH',
  'INDICATEUR_AFF', 'ADJUDICATIONS_AFT',
  'OPERATION_APU'));  -- opération du SEC, code dans instrument

-- ---------------------------------------------------------------------------
-- Les dépenses fiscales (« niches »)
-- ---------------------------------------------------------------------------

-- Un chiffrage par mesure, par millésime de loi de finances et par année
-- chiffrée. Le même montant change d'un millésime à l'autre (une prévision
-- devient une exécution, une exécution est révisée) : on garde tous les
-- millésimes, et la vue choisit.
CREATE TABLE core.depense_fiscale (
  millesime    smallint NOT NULL,            -- année du projet de loi de finances
  numero       text     NOT NULL CHECK (numero ~ '^[0-9]{5,6}$'),
  libelle      text     NOT NULL,
  impot        text,
  annee        smallint NOT NULL,
  stade        text     NOT NULL CHECK (stade IN ('EXECUTION','PREVISION')),
  -- Les annexes écrivent « ε » (moins de 0,5 M€), « nc » (non chiffré) ou
  -- « - » (sans objet) à la place d'un montant. Ce ne sont pas des zéros : un
  -- « nc » peut cacher un milliard. La mention est gardée, le montant reste NULL.
  montant_eur  numeric,
  mention      text CHECK (mention IN ('ε','nc','-')),
  source_id    bigint   NOT NULL REFERENCES raw.source(id),
  document_id  bigint   NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (millesime, numero, annee),
  CHECK ((montant_eur IS NULL) = (mention IS NOT NULL))
);

COMMENT ON TABLE core.depense_fiscale IS
  'Chiffrage des dépenses fiscales par mesure : annexe Voies et moyens tome II (PLF 2023) et '
  'budget vert (PLF 2024 à 2026). Montants en euros ; ε/nc/- conservés en mention.';

-- La nature du bénéficiaire, telle que l'annexe Voies et moyens la déclare.
-- Un seul millésime la publie en données ouvertes (PLF 2023) : les mesures
-- créées depuis n'en ont pas, et restent « non classées » plutôt que devinées.
CREATE TABLE ref.depense_fiscale_beneficiaire (
  numero       text PRIMARY KEY CHECK (numero ~ '^[0-9]{5,6}$'),
  nature       text NOT NULL CHECK (nature IN
                 ('ENTREPRISES','MENAGES','ENTREPRISES_ET_MENAGES','LOCAUX','PARCELLES')),
  nombre       bigint,                      -- nombre de bénéficiaires déclaré, NULL si non publié
  millesime    smallint NOT NULL,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  document_id  bigint NOT NULL REFERENCES raw.document(id)
);

-- Pour chaque mesure et chaque année, le chiffrage le plus sûr : l'exécution
-- du millésime le plus récent, à défaut la prévision la plus récente.
CREATE VIEW derived.depense_fiscale_retenue AS
SELECT DISTINCT ON (d.numero, d.annee)
       d.numero, d.annee, d.stade, d.millesime, d.libelle, d.impot, d.montant_eur, d.mention,
       coalesce(b.nature, 'NON_CLASSEE') AS nature,
       'depense-fiscale-retenue-v1'::text AS method_version
FROM core.depense_fiscale d
LEFT JOIN ref.depense_fiscale_beneficiaire b USING (numero)
ORDER BY d.numero, d.annee, (d.stade = 'EXECUTION') DESC, d.millesime DESC;

-- ---------------------------------------------------------------------------
-- Le compte de capital : ce que « finance » le besoin de financement
-- ---------------------------------------------------------------------------

-- Identité du SEC : B9 = B8G + D9REC − D9PAY − P5 − NP.
-- Donc besoin de financement (−B9) = −B8G + (P5 + NP) + (D9PAY − D9REC).
-- Vérifiée au million près par cmd/verify.
CREATE VIEW derived.dette_compte_capital AS
WITH op AS (
  SELECT s.pays, split_part(s.code, ':', 5) AS secteur, extract(year FROM o.debut)::int AS annee,
         split_part(s.code, ':', 6) AS op, o.valeur
  FROM core.dette_observation o JOIN ref.dette_serie s ON s.code = o.serie
  WHERE s.code LIKE 'eurostat:gov_10a_main:%:MIO_EUR:%'
), p AS (
  SELECT pays, secteur, annee,
         max(valeur) FILTER (WHERE op = 'B9')     AS b9,
         max(valeur) FILTER (WHERE op = 'B8G')    AS b8g,
         max(valeur) FILTER (WHERE op = 'P5')     AS p5,
         max(valeur) FILTER (WHERE op = 'NP')     AS np,
         max(valeur) FILTER (WHERE op = 'D9PAY')  AS d9pay,
         max(valeur) FILTER (WHERE op = 'D9REC')  AS d9rec,
         max(valeur) FILTER (WHERE op = 'P51G')   AS p51g,
         max(valeur) FILTER (WHERE op = 'P51C')   AS p51c,
         max(valeur) FILTER (WHERE op = 'TE')     AS te
  FROM op GROUP BY pays, secteur, annee
)
SELECT pays, secteur, annee,
       -b9                                   AS besoin_financement,
       -b8g                                  AS epargne_brute_negative,  -- > 0 : le courant n'est pas couvert
       p5 + np                               AS investissement,
       d9pay - d9rec                         AS transferts_capital_nets,
       -b9 - (-b8g + p5 + np + d9pay - d9rec) AS ecart_identite,
       p51g - p51c                           AS investissement_net,
       te                                    AS depenses_totales,
       'dette-compte-capital-v1'::text       AS method_version
FROM p
WHERE b9 IS NOT NULL AND b8g IS NOT NULL AND p5 IS NOT NULL AND d9pay IS NOT NULL AND d9rec IS NOT NULL;

COMMENT ON VIEW derived.dette_compte_capital IS
  'Décomposition exacte du besoin de financement des administrations publiques (Eurostat '
  'gov_10a_main) : épargne brute négative + investissement + transferts en capital nets. '
  'Une identité comptable, pas une affectation de l''emprunt à une dépense.';

-- ---------------------------------------------------------------------------
-- En regard : déficit, aides, dividendes
-- ---------------------------------------------------------------------------

-- Une ligne par année, colonnes JUXTAPOSÉES et jamais sommées. Les colonnes
-- cice_* rendent visible le recouvrement : le CICE figure dans les
-- exonérations de l'URSSAF (2013-2018) ET dans les dépenses fiscales.
CREATE VIEW derived.dette_aides_dividendes AS
WITH annees AS (SELECT generate_series(2004, extract(year FROM now())::int) AS annee),
eu AS (
  SELECT extract(year FROM o.debut)::int AS annee, split_part(s.code, ':', 6) AS op, o.valeur
  FROM core.dette_observation o JOIN ref.dette_serie s ON s.code = o.serie
  WHERE s.code LIKE 'eurostat:gov_10a_main:FR:MIO_EUR:S13:%'
     OR s.code = 'eurostat:gov_10dd_edpt1:FR:MIO_EUR:S13:GD'
), df AS (
  SELECT annee,
         sum(montant_eur)                                                AS total,
         sum(montant_eur) FILTER (WHERE nature = 'ENTREPRISES')          AS entreprises,
         sum(montant_eur) FILTER (WHERE nature = 'MENAGES')              AS menages,
         sum(montant_eur) FILTER (WHERE nature NOT IN ('ENTREPRISES','MENAGES')) AS autres,
         sum(montant_eur) FILTER (WHERE numero = '210324')               AS cice,
         count(*) FILTER (WHERE mention = 'nc')                          AS n_non_chiffrees,
         bool_and(stade = 'EXECUTION')                                   AS execution
  FROM derived.depense_fiscale_retenue GROUP BY annee
), exo AS (
  SELECT annee, sum(montant_eur) AS total,
         sum(montant_eur) FILTER (WHERE code_mesure = '141') AS cice
  FROM core.exoneration_cotisation GROUP BY annee
), mv AS (
  SELECT annee,
         max(valeur) FILTER (WHERE serie_code = 'dividendes.verses.snf')   * 1e6 AS div_snf,
         max(valeur) FILTER (WHERE serie_code = 'dividendes.verses.sf')    * 1e6 AS div_sf,
         max(valeur) FILTER (WHERE serie_code = 'dividendes.recus.menages')* 1e6 AS div_menages,
         max(valeur) FILTER (WHERE serie_code = 'impot.societes.encaisse') * 1e6 AS is_encaisse
  FROM core.macro_value GROUP BY annee
)
SELECT a.annee,
       -(SELECT valeur FROM eu WHERE eu.annee = a.annee AND op = 'B9')          AS deficit,
       (SELECT valeur FROM eu WHERE eu.annee = a.annee AND op = 'GD')
         - (SELECT valeur FROM eu WHERE eu.annee = a.annee - 1 AND op = 'GD')  AS variation_dette,
       (SELECT valeur FROM eu WHERE eu.annee = a.annee AND op = 'D41PAY')      AS interets,
       exo.total        AS exonerations_cotisations,
       exo.cice         AS cice_dans_exonerations,
       df.total         AS depenses_fiscales,
       df.entreprises   AS depenses_fiscales_entreprises,
       df.menages       AS depenses_fiscales_menages,
       df.autres        AS depenses_fiscales_autres_ou_non_classees,
       df.cice          AS cice_dans_depenses_fiscales,
       df.n_non_chiffrees,
       df.execution     AS depenses_fiscales_executees,
       (SELECT valeur FROM eu WHERE eu.annee = a.annee AND op = 'D3PAY')       AS subventions,
       (SELECT valeur FROM eu WHERE eu.annee = a.annee AND op = 'PTC')         AS credits_impot_payables,
       mv.div_snf       AS dividendes_verses_snf,
       mv.div_sf        AS dividendes_verses_sf,
       mv.div_menages   AS dividendes_recus_menages,
       mv.is_encaisse   AS impot_societes,
       'dette-aides-dividendes-v1'::text AS method_version
FROM annees a
LEFT JOIN df  USING (annee)
LEFT JOIN exo USING (annee)
LEFT JOIN mv  USING (annee);

COMMENT ON VIEW derived.dette_aides_dividendes IS
  'Déficit, variation de la dette, exonérations de cotisations (URSSAF), dépenses fiscales par '
  'nature de bénéficiaire, subventions, dividendes et impôt sur les sociétés, par année. Colonnes '
  'juxtaposées : elles se recouvrent (CICE) et ne doivent pas être additionnées.';

-- +goose Down
DROP VIEW derived.dette_aides_dividendes;
DROP VIEW derived.dette_compte_capital;
DROP VIEW derived.depense_fiscale_retenue;
DROP TABLE ref.depense_fiscale_beneficiaire;
DROP TABLE core.depense_fiscale;
DELETE FROM ref.dette_serie WHERE concept = 'OPERATION_APU';
ALTER TABLE ref.dette_serie DROP CONSTRAINT dette_serie_concept_check;
ALTER TABLE ref.dette_serie ADD CONSTRAINT dette_serie_concept_check CHECK (concept IN (
  'DETTE_MAASTRICHT', 'DETTE_NEGOCIABLE_ETAT', 'DETENTION_TITRES_ETAT', 'DETTE_NETTE_APU',
  'DETTE_BRUTE_FMI', 'DETTE_NETTE_FMI', 'ACTIFS_COTES_APU', 'INTERETS_VERSES', 'SOLDE_PUBLIC',
  'SOLDE_PRIMAIRE', 'RECETTES_PUBLIQUES', 'DEPENSES_PUBLIQUES', 'TAUX_LONG_TERME', 'BILAN_APU_CH',
  'INDICATEUR_AFF', 'ADJUDICATIONS_AFT'));
