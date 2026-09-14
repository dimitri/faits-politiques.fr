-- +goose Up
-- La dette publique : combien, envers qui, à quel prix. Voir docs/dette-donnees.md.
--
-- core.macro_value porte déjà deux séries de dette (Eurostat, France, annuel).
-- Elles suffisent à tracer une courbe, pas à expliquer un mécanisme : on ne
-- peut pas y lire qui détient les titres, à quelle échéance, ni ce que coûte
-- réellement l'emprunt comparé au taux affiché par les marchés. Ce bloc est
-- donc un modèle à part, en format long, parce que les sources ne partagent
-- pas leurs dimensions : l'AFT découpe par instrument (OAT, BTF), la Banque de
-- France par secteur détenteur, Eurostat par échéance résiduelle, l'AFF suisse
-- par compte de bilan.
--
-- Une série = une combinaison de dimensions fixée ; une observation = une
-- valeur à une période. Chaque dimension non ventilée vaut '_T' (total),
-- convention SDMX que suivent déjà l'INSEE, la Banque de France et Eurostat :
-- NULL y signifierait « inconnu », ce qui n'est pas le cas.

CREATE TABLE ref.dette_serie (
  code              text PRIMARY KEY,   -- '<producteur>:<code chez le producteur>', stable
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  code_source       text NOT NULL,      -- IDBANK INSEE, clé SDMX BdF, requête Eurostat...
  libelle           text NOT NULL,      -- libellé du producteur, non réécrit
  pays              text NOT NULL,      -- ISO 3166-1 alpha-2 ; agrégats Eurostat tels quels (EU27_2020, EA20)
  frequence         text NOT NULL CHECK (frequence IN ('A','Q','M')),
  -- Les montants sont stockés à l'unité (euros, francs), multiplicateur du
  -- producteur appliqué au chargement : l'INSEE publie en millions ou en
  -- milliards selon la série, la Banque de France en milliers. Laisser le
  -- multiplicateur dans une colonne, c'est garantir qu'une requête l'oublie.
  unite             text NOT NULL CHECK (unite IN ('EUR','CHF','PCT','PCT_PIB','RATIO','NOMBRE')),
  concept           text NOT NULL CHECK (concept IN (
                      'DETTE_MAASTRICHT',        -- dette brute consolidée des APU, critère européen
                      'DETTE_NEGOCIABLE_ETAT',   -- titres émis par l'AFT (OAT, BTF), valeur nominale
                      'DETENTION_TITRES_ETAT',   -- ventilation par détenteur (Banque de France)
                      'DETTE_NETTE_APU',         -- dette brute moins certains actifs (INSEE)
                      'DETTE_BRUTE_FMI',         -- tous passifs sauf actions et dérivés
                      'DETTE_NETTE_FMI',
                      'ACTIFS_COTES_APU',        -- actions cotées et OPC détenus par les APU
                      'INTERETS_VERSES',         -- D41 payés, comptabilité nationale
                      'SOLDE_PUBLIC',            -- B9 : capacité (+) / besoin (−) de financement
                      'SOLDE_PRIMAIRE',          -- solde hors intérêts
                      'RECETTES_PUBLIQUES',
                      'DEPENSES_PUBLIQUES',
                      'TAUX_LONG_TERME',         -- rendement à 10 ans sur le marché secondaire
                      'BILAN_APU_CH',            -- compte de bilan, statistique financière suisse
                      'INDICATEUR_AFF',          -- ratios de l'AFF (quotient d'endettement...)
                      'ADJUDICATIONS_AFT')),     -- indicateurs de performance du programme 117
  mesure            text NOT NULL CHECK (mesure IN
                      ('ENCOURS','VARIATION_CUMULEE','FLUX','TAUX','PART','RATIO','NOMBRE')),
  secteur_emetteur  text NOT NULL,      -- S13 (APU), S1311 (État/administration centrale), S13111 (État)...
  zone_detenteur    text NOT NULL DEFAULT 'W0' CHECK (zone_detenteur IN ('W0','W1','W2')),
                                        -- W0 monde, W1 non-résidents, W2 résidents
  secteur_detenteur text NOT NULL DEFAULT '_T',
  echeance          text NOT NULL DEFAULT '_T',
  -- Deux conventions incompatibles : l'INSEE et l'AFT classent un titre selon
  -- sa durée À L'ÉMISSION (une OAT à 10 ans reste « long terme » la veille de
  -- son remboursement), Eurostat selon la durée QUI RESTE à courir. Additionner
  -- ou comparer les deux sans le savoir fabrique un écart qui n'existe pas.
  base_echeance     text CHECK (base_echeance IN ('INITIALE','RESIDUELLE')),
  instrument        text NOT NULL DEFAULT '_T',
  monnaie_emission  text NOT NULL DEFAULT '_T' CHECK (monnaie_emission IN ('_T','EUR','DEVISES')),
  notes             text,
  url               text,
  CHECK ((echeance = '_T') = (base_echeance IS NULL))
);

CREATE INDEX dette_serie_source_idx ON ref.dette_serie (source_id);
CREATE INDEX dette_serie_concept_idx ON ref.dette_serie (concept, pays);

CREATE TABLE core.dette_observation (
  serie        text NOT NULL REFERENCES ref.dette_serie(code) ON DELETE CASCADE,
  periode      text NOT NULL CHECK (periode ~ '^[0-9]{4}(-Q[1-4]|-[0-9]{2})?$'),
  debut        date NOT NULL,           -- premier jour de la période, pour trier et joindre
  -- Pas de valeur, pas de ligne : une période absente chez le producteur
  -- reste absente ici. Rien n'est interpolé ni mis à zéro.
  valeur       numeric NOT NULL,
  statut       text,                    -- statut d'observation du producteur (A définitif, P provisoire...)
  document_id  bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (serie, periode)
);

COMMENT ON TABLE core.dette_observation IS
  'Observations des séries de dette, à l''unité (multiplicateur du producteur appliqué). '
  'Chaque valeur renvoie au document scellé dont elle provient.';

-- ---------------------------------------------------------------------------
-- Calculs. Des vues, pas des tables : chacune est une formule de quelques
-- lignes sur des séries déjà chargées, sans coût de calcul qui justifierait
-- de la figer. La method_version est dans la vue ; changer la formule, c'est
-- changer la version.
-- ---------------------------------------------------------------------------

-- Le taux APPARENT : intérêts versés dans l'année rapportés à la dette moyenne
-- de l'année (moyenne des encours de fin d'année t−1 et t). C'est ce que coûte
-- réellement le stock de dette, contracté à des dates et des taux différents.
-- Le taux à 10 ans du marché, lui, ne dit que le prix des emprunts NOUVEAUX :
-- l'écart entre les deux mesure le délai avec lequel une hausse des taux
-- se transmet à la charge d'intérêts.
CREATE VIEW derived.dette_taux_apparent AS
WITH annuel AS (
  SELECT s.pays, o.debut, s.concept, o.valeur
  FROM core.dette_observation o
  JOIN ref.dette_serie s ON s.code = o.serie
  WHERE s.frequence = 'A' AND s.unite = 'EUR' AND s.secteur_emetteur = 'S13'
    AND s.concept IN ('DETTE_MAASTRICHT','INTERETS_VERSES')
    AND s.zone_detenteur = 'W0' AND s.secteur_detenteur = '_T'
    AND s.echeance = '_T' AND s.instrument = '_T'
    AND (s.code LIKE 'eurostat:gov_10dd_edpt1:%' OR s.code LIKE 'eurostat:gov_10a_main:%')
), pivot AS (
  SELECT pays, debut,
         max(valeur) FILTER (WHERE concept = 'DETTE_MAASTRICHT') AS dette,
         max(valeur) FILTER (WHERE concept = 'INTERETS_VERSES')  AS interets
  FROM annuel GROUP BY pays, debut
), taux AS (
  SELECT s.pays, o.debut, o.valeur AS taux_10ans
  FROM core.dette_observation o JOIN ref.dette_serie s ON s.code = o.serie
  WHERE s.concept = 'TAUX_LONG_TERME' AND s.frequence = 'A' AND s.code LIKE 'eurostat:%'
)
SELECT p.pays,
       extract(year FROM p.debut)::int                         AS annee,
       p.interets,
       (p.dette + prec.dette) / 2                              AS dette_moyenne,
       round(100 * p.interets / ((p.dette + prec.dette) / 2), 3) AS taux_apparent_pct,
       t.taux_10ans                                            AS taux_10ans_pct,
       'dette-taux-apparent-v1'::text                          AS method_version
FROM pivot p
JOIN pivot prec ON prec.pays = p.pays AND prec.debut = p.debut - interval '1 year'
LEFT JOIN taux t ON t.pays = p.pays AND t.debut = p.debut
WHERE p.interets IS NOT NULL AND p.dette IS NOT NULL AND prec.dette IS NOT NULL;

COMMENT ON VIEW derived.dette_taux_apparent IS
  'Taux apparent = intérêts versés (D41) / moyenne des encours de dette Maastricht de fin '
  't−1 et t, Eurostat. Le taux à 10 ans est donné en regard, pas mêlé au calcul.';

-- Qui détient les titres négociables de l'État, trimestre par trimestre, en
-- regroupant les secteurs de la Banque de France en catégories lisibles.
--
-- Le total est la somme des détentions résidente et non résidente, toutes
-- échéances (W1 + W2, échéance '_T'). Il ne se reconstitue PAS depuis les
-- séries par échéance : jusqu'en 2013, les BTAN (2 à 5 ans) sont hors du
-- « long terme » comme du « court terme », et la Banque de France ne les
-- ventile pas par secteur résident. D'où une catégorie résiduelle, calculée
-- par différence et non inventée : nulle depuis le remboursement des derniers
-- BTAN (2017).
CREATE VIEW derived.dette_detention_etat AS
WITH encours AS (
  SELECT o.periode, o.debut, s.zone_detenteur, s.secteur_detenteur, s.echeance, o.valeur
  FROM core.dette_observation o
  JOIN ref.dette_serie s ON s.code = o.serie
  WHERE s.concept = 'DETENTION_TITRES_ETAT' AND s.mesure = 'ENCOURS' AND s.instrument = '_T'
), total AS (
  SELECT periode, debut,
         max(valeur) FILTER (WHERE zone_detenteur = 'W1') AS non_residents,
         max(valeur) FILTER (WHERE zone_detenteur = 'W2') AS residents
  FROM encours
  WHERE secteur_detenteur = '_T' AND echeance = '_T'
  GROUP BY periode, debut
), secteurs AS (
  -- Seuls les secteurs FEUILLES : les agrégats (S12, S12K, S12P, S12Q, S1M)
  -- compteraient deux fois.
  SELECT periode,
         CASE
           WHEN secteur_detenteur = 'S121' THEN 'BANQUE_DE_FRANCE'
           WHEN secteur_detenteur = 'S122' THEN 'BANQUES'
           WHEN secteur_detenteur = 'S128' THEN 'ASSURANCES'
           WHEN secteur_detenteur = 'S129' THEN 'FONDS_DE_PENSION'
           WHEN secteur_detenteur IN ('S123','S124') THEN 'FONDS_DE_PLACEMENT'
           WHEN secteur_detenteur IN ('S125','S126','S127') THEN 'AUTRES_FINANCIERES'
           WHEN secteur_detenteur IN ('S14','S15') THEN 'MENAGES'
           WHEN secteur_detenteur = 'S11' THEN 'ENTREPRISES'
           WHEN secteur_detenteur = 'S13' THEN 'ADMINISTRATIONS'
           WHEN secteur_detenteur = '_Z' THEN 'RESIDENTS_SECTEUR_INCONNU'
         END AS categorie,
         valeur
  FROM encours
  WHERE zone_detenteur = 'W2' AND echeance IN ('CT','LT')
    AND secteur_detenteur IN ('S121','S122','S123','S124','S125','S126','S127',
                              'S128','S129','S14','S15','S11','S13','_Z')
), lignes AS (
  SELECT periode, categorie, sum(valeur) AS valeur FROM secteurs GROUP BY periode, categorie
  UNION ALL
  SELECT periode, 'NON_RESIDENTS', non_residents FROM total
  UNION ALL
  SELECT t.periode, 'RESIDENTS_BTAN_NON_VENTILES', t.residents - sum(s.valeur)
  FROM total t JOIN secteurs s USING (periode)
  GROUP BY t.periode, t.residents
)
SELECT l.periode, t.debut, l.categorie,
       l.valeur                                                   AS encours_eur,
       round(100 * l.valeur / (t.non_residents + t.residents), 2) AS part_pct,
       'dette-detention-etat-v1'::text                            AS method_version
FROM lignes l JOIN total t USING (periode)
WHERE t.non_residents IS NOT NULL AND t.residents IS NOT NULL;

COMMENT ON VIEW derived.dette_detention_etat IS
  'Détention des titres négociables de l''État (OAT, BTAN, BTF, en valeur de marché) par '
  'catégorie de détenteur, Banque de France DET2. Non-résident = domicile du détenteur, pas '
  'nationalité. RESIDENTS_BTAN_NON_VENTILES est un résidu calculé, nul depuis 2017 (derniers BTAN remboursés).';

-- Pourquoi la dette n'augmente pas exactement du montant du déficit :
-- l'« ajustement stock-flux ». Un État peut emprunter plus que son déficit
-- pour gonfler sa trésorerie, prêter à d'autres (prêts à la Grèce en 2010),
-- ou émettre au-dessus du pair. L'écart est normal ; un écart durablement
-- important est une question à poser.
CREATE VIEW derived.dette_ajustement_stock_flux AS
WITH v AS (
  SELECT s.pays, extract(year FROM o.debut)::int AS annee,
         max(o.valeur) FILTER (WHERE s.concept = 'DETTE_MAASTRICHT') AS dette,
         max(o.valeur) FILTER (WHERE s.concept = 'SOLDE_PUBLIC')     AS solde
  FROM core.dette_observation o
  JOIN ref.dette_serie s ON s.code = o.serie
  WHERE s.frequence = 'A' AND s.unite = 'EUR' AND s.secteur_emetteur = 'S13'
    AND s.concept IN ('DETTE_MAASTRICHT','SOLDE_PUBLIC')
    AND s.zone_detenteur = 'W0' AND s.secteur_detenteur = '_T'
    AND s.echeance = '_T' AND s.instrument = '_T'
    AND (s.code LIKE 'eurostat:gov_10dd_edpt1:%' OR s.code LIKE 'eurostat:gov_10a_main:%')
  GROUP BY s.pays, extract(year FROM o.debut)
)
SELECT v.pays, v.annee,
       v.dette - p.dette                    AS variation_dette,
       -v.solde                             AS deficit,
       (v.dette - p.dette) - (-v.solde)     AS ajustement_stock_flux,
       'dette-asf-v1'::text                 AS method_version
FROM v JOIN v p ON p.pays = v.pays AND p.annee = v.annee - 1
WHERE v.dette IS NOT NULL AND p.dette IS NOT NULL AND v.solde IS NOT NULL;

-- +goose Down
DROP VIEW derived.dette_ajustement_stock_flux;
DROP VIEW derived.dette_detention_etat;
DROP VIEW derived.dette_taux_apparent;
DROP TABLE core.dette_observation;
DROP TABLE ref.dette_serie;
