-- +goose Up
-- Qui reçoit les aides : la taille des entreprises. Voir docs/dette-donnees.md § 14.
--
-- Trois ajouts :
--   1. la nature du bénéficiaire des dépenses fiscales sur quatre millésimes
--      (PLF 2020 à 2023) au lieu d'un : la clé devient (millésime, numéro) ;
--   2. les exonérations de cotisations et la masse salariale PAR TRANCHE
--      D'EFFECTIF de l'entreprise (URSSAF), de quoi rapporter une part des
--      aides à une part de l'emploi ;
--   3. la catégorie d'entreprise de l'INSEE (PME, ETI, grande entreprise) pour
--      chaque personne morale du répertoire SIRENE, qui servira à croiser les
--      aides publiées bénéficiaire par bénéficiaire.

-- ---------------------------------------------------------------------------
-- 1. Bénéficiaires des dépenses fiscales, par millésime
-- ---------------------------------------------------------------------------

DROP VIEW derived.dette_aides_dividendes;
DROP VIEW derived.depense_fiscale_retenue;

-- Les lignes chargées sous l'ancienne source (PLF 2023 seul) sont rechargées
-- sous la source « voies-et-moyens-t2 », qui couvre les PLF 2020 à 2023.
DELETE FROM ref.depense_fiscale_beneficiaire
 WHERE source_id IN (SELECT id FROM raw.source WHERE slug = 'plf2023-voies-et-moyens-t2');
DELETE FROM core.depense_fiscale
 WHERE source_id IN (SELECT id FROM raw.source WHERE slug = 'plf2023-voies-et-moyens-t2');

ALTER TABLE ref.depense_fiscale_beneficiaire DROP CONSTRAINT depense_fiscale_beneficiaire_pkey;
ALTER TABLE ref.depense_fiscale_beneficiaire ADD PRIMARY KEY (millesime, numero);

-- Pour chaque ANNÉE, un seul millésime : le plus récent qui publie une
-- exécution pour cette année, à défaut le plus récent tout court. Choisir
-- mesure par mesure (v1) mêlait deux millésimes dans un même total : une
-- mesure renumérotée d'une annexe à l'autre était comptée sous ses deux
-- numéros (2019 : 103,0 Md€ au lieu des 99,9 exécutés).
--
-- La nature d'une mesure peut changer d'un millésime à l'autre (les
-- exonérations de taxe foncière passent de « ménages » à « locaux » en 2022) :
-- on retient la déclaration du millésime le plus récent qui la classe.
CREATE VIEW derived.depense_fiscale_retenue AS
WITH choix AS (
  SELECT annee,
         coalesce(max(millesime) FILTER (WHERE stade = 'EXECUTION'), max(millesime)) AS millesime
  FROM core.depense_fiscale GROUP BY annee
)
SELECT d.numero, d.annee, d.stade, d.millesime, d.libelle, d.impot, d.montant_eur, d.mention,
       coalesce((SELECT b.nature FROM ref.depense_fiscale_beneficiaire b
                  WHERE b.numero = d.numero ORDER BY b.millesime DESC LIMIT 1), 'NON_CLASSEE') AS nature,
       'depense-fiscale-retenue-v2'::text AS method_version
FROM core.depense_fiscale d
JOIN choix c USING (annee, millesime);

-- Recréée à l'identique de 0075, sur la nouvelle vue.
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

-- ---------------------------------------------------------------------------
-- 2. Exonérations et emploi par tranche d'effectif de l'entreprise (URSSAF)
-- ---------------------------------------------------------------------------

-- La tranche est celle de l'ENTREPRISE au sens de l'URSSAF (base Sequoia,
-- effectifs moyens de l'année) : une société juridique, pas un groupe. Les
-- filiales de 300 salariés d'un groupe de 50 000 sont comptées « 250 à 499 » :
-- la part des grands groupes est donc MINORÉE par cette source, dans une
-- proportion qu'elle ne permet pas de mesurer.
CREATE TABLE ref.tranche_effectif_urssaf (
  code     text PRIMARY KEY,              -- lettre de tri de l'URSSAF : 'a' ... 'h'
  libelle  text NOT NULL UNIQUE,          -- '0 à 9', ..., '2000 et plus'
  borne_min integer NOT NULL,
  borne_max integer                       -- NULL : pas de borne haute
);
INSERT INTO ref.tranche_effectif_urssaf VALUES
  ('a','0 à 9',0,9), ('b','10 à 19',10,19), ('c','20 à 49',20,49), ('d','50 à 99',50,99),
  ('e','100 à 249',100,249), ('f','250 à 499',250,499), ('g','500 à 1999',500,1999),
  ('h','2000 et plus',2000,NULL);

CREATE TABLE core.exoneration_tranche (
  annee                 smallint NOT NULL,
  tranche               text     NOT NULL REFERENCES ref.tranche_effectif_urssaf(code),
  code_grande_categorie text     NOT NULL,
  grande_categorie      text     NOT NULL,
  code_categorie        text     NOT NULL,
  categorie             text     NOT NULL,
  montant_eur           numeric,             -- NULL si non publié ; jamais 0 par défaut
  source_id             bigint   NOT NULL REFERENCES raw.source(id),
  document_id           bigint   NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (annee, tranche, code_categorie)
);

CREATE TABLE core.emploi_prive_tranche (
  annee              smallint NOT NULL,
  tranche            text     NOT NULL REFERENCES ref.tranche_effectif_urssaf(code),
  nombre_entreprises integer,
  nombre_etablissements integer,
  effectifs_moyens   numeric,
  masse_salariale_eur numeric,
  source_id          bigint   NOT NULL REFERENCES raw.source(id),
  document_id        bigint   NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (annee, tranche)
);

-- Part des exonérations et part de la masse salariale, par tranche. Le taux
-- d'exonération (exonérations / masse salariale) dit l'intensité de l'aide
-- pour un euro de salaire versé ; la part dit qui reçoit la masse.
CREATE VIEW derived.exoneration_par_taille AS
WITH e AS (
  SELECT annee, tranche, sum(montant_eur) AS exonerations,
         sum(montant_eur) FILTER (WHERE code_categorie = '14') AS dont_cice
  FROM core.exoneration_tranche GROUP BY annee, tranche
)
SELECT e.annee, e.tranche, t.libelle, e.exonerations, e.dont_cice,
       round(100 * e.exonerations / sum(e.exonerations) OVER (PARTITION BY e.annee), 2) AS part_exonerations_pct,
       m.masse_salariale_eur,
       round(100 * m.masse_salariale_eur / sum(m.masse_salariale_eur) OVER (PARTITION BY e.annee), 2) AS part_masse_salariale_pct,
       round(100 * e.exonerations / m.masse_salariale_eur, 2) AS taux_exoneration_pct,
       m.nombre_entreprises, m.effectifs_moyens,
       'exoneration-par-taille-v1'::text AS method_version
FROM e
JOIN ref.tranche_effectif_urssaf t ON t.code = e.tranche
LEFT JOIN core.emploi_prive_tranche m USING (annee, tranche);

-- ---------------------------------------------------------------------------
-- 3. Catégorie d'entreprise (SIRENE)
-- ---------------------------------------------------------------------------

-- Une ligne par PERSONNE MORALE du répertoire SIRENE. Les entrepreneurs
-- individuels (catégorie juridique 1000) sont exclus : leur SIREN désigne une
-- personne physique, et le projet n'en a pas besoin pour croiser des aides aux
-- entreprises — minimisation des données personnelles.
--
-- La catégorie d'entreprise (loi LME de 2008, décret 2008-1354) est calculée
-- par l'INSEE au niveau de l'ENTREPRISE au sens économique, c'est-à-dire du
-- groupe profilé : une filiale de 40 salariés d'un groupe du CAC 40 est « GE ».
-- C'est la bonne notion pour « grandes entreprises » ; ce n'est PAS la même que
-- la tranche d'effectif de l'URSSAF, qui porte sur la seule société.
CREATE TABLE ref.unite_legale (
  siren                 text PRIMARY KEY CHECK (siren ~ '^[0-9]{9}$'),
  denomination          text,
  categorie_juridique   text NOT NULL,
  activite_principale   text,              -- code NAF
  nomenclature_activite text,
  etat_administratif    text NOT NULL CHECK (etat_administratif IN ('A','C')),
  categorie_entreprise  text CHECK (categorie_entreprise IN ('PME','ETI','GE')),
  annee_categorie       smallint,
  tranche_effectifs     text,              -- code INSEE tel quel ('NN' = non employeuse ou inconnu, '00', '01'...'53')
  annee_effectifs       smallint,
  caractere_employeur   text CHECK (caractere_employeur IN ('O','N')),
  date_creation         date,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  document_id           bigint NOT NULL REFERENCES raw.document(id),
  CHECK (categorie_juridique <> '1000')
);

-- Un seul index secondaire : la clé primaire sert aux jointures par SIREN,
-- celui-ci aux répartitions par catégorie des unités actives.
CREATE INDEX unite_legale_categorie_idx ON ref.unite_legale (categorie_entreprise) WHERE etat_administratif = 'A';

COMMENT ON TABLE ref.unite_legale IS
  'Personnes morales du répertoire SIRENE (stock INSEE) : catégorie d''entreprise PME/ETI/GE '
  'calculée au niveau du groupe, tranche d''effectif de l''unité légale. Entrepreneurs '
  'individuels exclus.';

CREATE VIEW derived.unite_legale_repartition AS
SELECT coalesce(categorie_entreprise, 'NON_CATEGORISEE') AS categorie_entreprise,
       etat_administratif,
       count(*)                                          AS unites_legales,
       -- caractereEmployeurUniteLegale est vide dans tout le stock : la
       -- présence de salariés se lit dans la tranche d'effectif.
       count(*) FILTER (WHERE tranche_effectifs NOT IN ('NN','00')) AS avec_salaries,
       max(annee_categorie)                              AS annee_categorie,
       'unite-legale-repartition-v1'::text               AS method_version
FROM ref.unite_legale
GROUP BY 1, 2;

-- +goose Down
DROP VIEW derived.unite_legale_repartition;
DROP TABLE ref.unite_legale;
DROP VIEW derived.exoneration_par_taille;
DROP TABLE core.emploi_prive_tranche;
DROP TABLE core.exoneration_tranche;
DROP TABLE ref.tranche_effectif_urssaf;
DROP VIEW derived.dette_aides_dividendes;
DROP VIEW derived.depense_fiscale_retenue;
DELETE FROM ref.depense_fiscale_beneficiaire WHERE millesime <> 2023;
ALTER TABLE ref.depense_fiscale_beneficiaire DROP CONSTRAINT depense_fiscale_beneficiaire_pkey;
ALTER TABLE ref.depense_fiscale_beneficiaire ADD PRIMARY KEY (numero);
