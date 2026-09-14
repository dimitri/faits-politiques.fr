-- +goose Up
-- D'une fiche de paie aux caisses. Voir docs/cotisations-et-droits.md § 3 bis et D-060.
--
-- Trois tables de référence et un calcul :
--   ref.organisme_social   qui reçoit chaque prélèvement, et de quel budget il
--                          relève (État, Sécurité sociale votée en loi de
--                          financement, régimes paritaires, opérateur, fonds,
--                          hors administrations publiques) ;
--   ref.parametre_social   plafond, Smic, paramètres de la réduction générale,
--                          point Agirc-Arrco, par millésime ;
--   ref.taux_cotisation    chaque ligne du bulletin : taux, assiette, part,
--                          destinataire, nature du droit ouvert ;
--   ref.bulletin_cas       les hypothèses d'un bulletin d'exemple (salaire,
--                          effectif, taux accidents du travail, taux d'impôt).
-- Les vues recalculent le bulletin : aucune valeur de bulletin n'est stockée.

CREATE TABLE ref.organisme_social (
  code          text PRIMARY KEY,
  nom           text NOT NULL,
  statut        text NOT NULL,                 -- forme juridique, en clair
  -- Le budget dont relève la ressource, au sens de la loi qui l'arrête :
  --   ETAT             budget de l'État, voté en loi de finances ;
  --   SECU_LFSS        régimes obligatoires de base et organismes concourant à
  --                    leur financement, dont les recettes et objectifs sont
  --                    votés en loi de financement de la sécurité sociale ;
  --   SECU_PARITAIRE   régimes gérés par les partenaires sociaux, comptés en
  --                    administrations de sécurité sociale mais hors LFSS ;
  --   OPERATEUR_ETAT   établissement public de l'État, budget propre ;
  --   FONDS_ETAT       fonds sans personnalité morale placé sous l'autorité
  --                    d'un ministre ;
  --   COLLECTIVITE     budget d'une collectivité ou d'un groupement ;
  --   PRIVE            organisme de droit privé hors des budgets publics votés ;
  --   AUTRE            pas de budget unique (affectation choisie par l'employeur).
  budget        text NOT NULL CHECK (budget IN ('ETAT','SECU_LFSS','SECU_PARITAIRE','OPERATEUR_ETAT',
                                                'FONDS_ETAT','COLLECTIVITE','PRIVE','AUTRE')),
  -- Sous-secteur de comptabilité nationale quand une source l'établit ; NULL sinon.
  sous_secteur  text CHECK (sous_secteur IN ('S1311','S1313','S1314')),
  texte_budget  text NOT NULL,                 -- qui vote ou arrête ce budget
  fondement     text NOT NULL,                 -- d'où vient le classement
  jo_texte_id   text REFERENCES jo.texte(id),
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  document_id   bigint REFERENCES raw.document(id)
);

CREATE TABLE ref.parametre_social (
  millesime    date NOT NULL,
  code         text NOT NULL,
  libelle      text NOT NULL,
  valeur       numeric NOT NULL,
  unite        text NOT NULL,
  fondement    text NOT NULL,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  document_id  bigint REFERENCES raw.document(id),
  PRIMARY KEY (millesime, code)
);

CREATE TABLE ref.taux_cotisation (
  millesime      date NOT NULL,
  code           text NOT NULL,
  part           text NOT NULL CHECK (part IN ('SALARIE','EMPLOYEUR')),
  libelle        text NOT NULL,                -- tel qu'il figure sur le bulletin
  rubrique       text NOT NULL,                -- rubrique du bulletin simplifié
  ordre          smallint NOT NULL,
  assiette       text NOT NULL CHECK (assiette IN ('BRUT','TRANCHE_1','BRUT_ABATTU')),
  taux           numeric,                      -- en %, NULL si propre à chaque employeur
  variable       boolean NOT NULL DEFAULT false,
  effectif_min   integer NOT NULL DEFAULT 0,   -- taux dû à partir de cet effectif
  effectif_max   integer,                      -- … et en dessous de celui-ci
  organisme      text NOT NULL REFERENCES ref.organisme_social(code),
  -- La nature du droit que le prélèvement ouvre au salarié :
  --   DIFFERE      droit à son nom, proportionnel à ce qui est versé ;
  --   MIXTE        une partie contributive, une partie universelle ;
  --   SOLIDARITE   droit sans lien avec ce salaire ;
  --   IMPOT        impôt ou taxe, sans droit individuel.
  nature_droit   text NOT NULL CHECK (nature_droit IN ('DIFFERE','MIXTE','SOLIDARITE','IMPOT')),
  deductible_ir  boolean NOT NULL DEFAULT true, -- false : reste dans le net imposable
  -- Réduction générale : sur quelles cotisations employeur elle s'impute, et à
  -- quel taux (la part accidents du travail est limitée à 0,49 %).
  rgdu_groupe    text CHECK (rgdu_groupe IN ('URSSAF','IRC')),
  rgdu_taux      numeric,
  fondement      text NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  document_id    bigint REFERENCES raw.document(id),
  PRIMARY KEY (millesime, code, part, effectif_min),
  CHECK (taux IS NOT NULL OR variable),
  CHECK ((rgdu_groupe IS NULL) = (rgdu_taux IS NULL)),
  CHECK (rgdu_groupe IS NULL OR part = 'EMPLOYEUR')
);

CREATE TABLE ref.bulletin_cas (
  cas          text PRIMARY KEY,
  millesime    date NOT NULL,
  brut         numeric NOT NULL CHECK (brut > 0),
  effectif     integer NOT NULL CHECK (effectif > 0),
  taux_atmp    numeric NOT NULL,              -- taux notifié à l'établissement, en %
  taux_pas     numeric NOT NULL,              -- taux d'impôt transmis par la DGFiP, en %
  description  text NOT NULL
);

COMMENT ON TABLE ref.bulletin_cas IS
  'Bulletins d''exemple, factices : les personnes et l''entreprise n''existent pas. Les taux propres '
  'à un employeur (accidents du travail) ou à un foyer (impôt) sont des hypothèses.';

-- Chaque ligne du bulletin, recalculée.
CREATE VIEW derived.bulletin_ligne AS
WITH p AS (
  SELECT millesime,
         max(valeur) FILTER (WHERE code = 'PMSS')           AS pmss,
         max(valeur) FILTER (WHERE code = 'ABATTEMENT_CSG') AS abattement
  FROM ref.parametre_social GROUP BY millesime
)
SELECT b.cas, t.code, t.part, t.libelle, t.rubrique, t.ordre, t.organisme, t.nature_droit,
       t.deductible_ir, t.rgdu_groupe, t.rgdu_taux,
       CASE t.assiette WHEN 'BRUT' THEN b.brut
                       WHEN 'TRANCHE_1' THEN least(b.brut, p.pmss)
                       WHEN 'BRUT_ABATTU' THEN round(b.brut * (1 - p.abattement / 100), 2) END AS base,
       coalesce(t.taux, CASE WHEN t.code = 'ATMP' THEN b.taux_atmp END)      AS taux,
       round(CASE t.assiette WHEN 'BRUT' THEN b.brut
                             WHEN 'TRANCHE_1' THEN least(b.brut, p.pmss)
                             WHEN 'BRUT_ABATTU' THEN round(b.brut * (1 - p.abattement / 100), 2) END
             * coalesce(t.taux, CASE WHEN t.code = 'ATMP' THEN b.taux_atmp END) / 100, 2) AS montant,
       'bulletin-ligne-v1'::text AS method_version
FROM ref.bulletin_cas b
JOIN ref.taux_cotisation t ON t.millesime = b.millesime
                          AND b.effectif >= t.effectif_min
                          AND (t.effectif_max IS NULL OR b.effectif < t.effectif_max)
JOIN p ON p.millesime = b.millesime;

-- La réduction générale dégressive unique, et son imputation ligne par ligne.
-- Coefficient = Tmin + Tdelta × [½ × (3 × Smic / rémunération − 1)]^P, arrondi
-- au dix-millième ; nul à partir de trois Smic. La part imputée sur la retraite
-- complémentaire est plafonnée à 6,01 points de la valeur maximale du
-- coefficient ; au sein de chaque groupe, la répartition entre lignes suit les
-- taux (règle d'illustration : l'URSSAF ne publie pas de clé par branche).
CREATE VIEW derived.bulletin_reduction AS
WITH p AS (
  SELECT millesime,
         max(valeur) FILTER (WHERE code = 'SMIC_ANNUEL')            AS smic_an,
         max(valeur) FILTER (WHERE code = 'RGDU_TMIN')              AS tmin,
         max(valeur) FILTER (WHERE code = 'RGDU_TDELTA_MOINS_50')   AS tdelta_petit,
         max(valeur) FILTER (WHERE code = 'RGDU_TDELTA_50_ET_PLUS') AS tdelta_grand,
         max(valeur) FILTER (WHERE code = 'RGDU_P')                 AS p,
         max(valeur) FILTER (WHERE code = 'RGDU_PLAFOND_IRC')       AS plafond_irc
  FROM ref.parametre_social GROUP BY millesime
), c AS (
  SELECT b.cas, b.brut, p.plafond_irc,
         p.tmin + CASE WHEN b.effectif < 50 THEN p.tdelta_petit ELSE p.tdelta_grand END AS coef_max,
         CASE WHEN b.brut >= 3 * p.smic_an / 12 THEN 0
              ELSE round(p.tmin + CASE WHEN b.effectif < 50 THEN p.tdelta_petit ELSE p.tdelta_grand END
                         * power(0.5 * (3 * p.smic_an / 12 / b.brut - 1), p.p), 4) END AS coefficient
  FROM ref.bulletin_cas b JOIN p USING (millesime)
), m AS (
  SELECT cas, coefficient, coef_max, round(brut * coefficient, 2) AS montant,
         round(round(brut * coefficient, 2) * plafond_irc / (100 * coef_max), 2) AS part_irc
  FROM c
), l AS (
  SELECT m.cas, m.coefficient, m.montant, m.part_irc, l.code, l.organisme, l.rgdu_groupe,
         CASE l.rgdu_groupe WHEN 'IRC' THEN m.part_irc ELSE m.montant - m.part_irc END
           * l.rgdu_taux / sum(l.rgdu_taux) OVER (PARTITION BY m.cas, l.rgdu_groupe) AS brut_imputation,
         row_number() OVER (PARTITION BY m.cas, l.rgdu_groupe ORDER BY l.rgdu_taux DESC, l.code) AS rang
  FROM m JOIN derived.bulletin_ligne l ON l.cas = m.cas AND l.rgdu_groupe IS NOT NULL
)
SELECT cas, coefficient, montant, part_irc, code, organisme, rgdu_groupe,
       round(brut_imputation, 2)
         + CASE WHEN rang = 1 THEN
             CASE rgdu_groupe WHEN 'IRC' THEN part_irc ELSE montant - part_irc END
             - sum(round(brut_imputation, 2)) OVER (PARTITION BY cas, rgdu_groupe)
           ELSE 0 END AS imputation,
       'bulletin-reduction-rgdu-v1'::text AS method_version
FROM l;

-- Les totaux du bulletin.
CREATE VIEW derived.bulletin_synthese AS
WITH s AS (
  SELECT l.cas,
         sum(montant) FILTER (WHERE part = 'SALARIE')                        AS cotisations_salarie,
         sum(montant) FILTER (WHERE part = 'SALARIE' AND NOT deductible_ir)  AS non_deductible_ir,
         sum(montant) FILTER (WHERE part = 'EMPLOYEUR')                      AS cotisations_employeur_dues
  FROM derived.bulletin_ligne l GROUP BY l.cas
), r AS (
  SELECT DISTINCT cas, coefficient, montant AS reduction FROM derived.bulletin_reduction
)
SELECT b.cas, b.brut, s.cotisations_salarie, s.cotisations_employeur_dues,
       r.coefficient, coalesce(r.reduction, 0) AS reduction_generale,
       s.cotisations_employeur_dues - coalesce(r.reduction, 0)                    AS cotisations_employeur,
       b.brut - s.cotisations_salarie                                              AS net_avant_impot,
       b.brut - s.cotisations_salarie + s.non_deductible_ir                        AS net_imposable,
       round((b.brut - s.cotisations_salarie + s.non_deductible_ir) * b.taux_pas / 100, 2) AS impot_source,
       b.brut - s.cotisations_salarie
         - round((b.brut - s.cotisations_salarie + s.non_deductible_ir) * b.taux_pas / 100, 2) AS net_paye,
       b.brut + s.cotisations_employeur_dues - coalesce(r.reduction, 0)            AS cout_employeur,
       'bulletin-synthese-v1'::text AS method_version
FROM ref.bulletin_cas b
JOIN s USING (cas)
LEFT JOIN r USING (cas);

-- Où va chaque euro du coût employeur : un flux par destinataire, avec le budget
-- dont il relève. Le salaire net et l'impôt à la source y figurent : leur somme
-- avec les cotisations versées redonne le coût employeur.
CREATE VIEW derived.bulletin_flux AS
WITH lignes AS (
  SELECT l.cas, l.organisme, l.nature_droit,
         sum(l.montant) FILTER (WHERE l.part = 'SALARIE')   AS salarie,
         sum(l.montant) FILTER (WHERE l.part = 'EMPLOYEUR') AS employeur_du
  FROM derived.bulletin_ligne l GROUP BY 1, 2, 3
), red AS (
  SELECT cas, organisme, sum(imputation) AS reduction FROM derived.bulletin_reduction GROUP BY 1, 2
), f AS (
  SELECT g.cas, g.organisme, g.nature_droit, coalesce(g.salarie, 0) AS salarie,
         coalesce(g.employeur_du, 0) AS employeur_du, coalesce(r.reduction, 0) AS reduction
  FROM lignes g LEFT JOIN red r USING (cas, organisme)
  UNION ALL
  SELECT cas, 'SALARIE', 'SALAIRE', net_paye, 0, 0 FROM derived.bulletin_synthese
  UNION ALL
  SELECT cas, 'DGFIP', 'IMPOT', impot_source, 0, 0 FROM derived.bulletin_synthese
)
SELECT f.cas, f.organisme, coalesce(o.nom, 'Salarié') AS nom, o.budget, o.sous_secteur, f.nature_droit,
       f.salarie, f.employeur_du, f.reduction, f.salarie + f.employeur_du - f.reduction AS verse,
       'bulletin-flux-v1'::text AS method_version
FROM f LEFT JOIN ref.organisme_social o ON o.code = f.organisme;

-- +goose Down
DROP VIEW derived.bulletin_flux;
DROP VIEW derived.bulletin_synthese;
DROP VIEW derived.bulletin_reduction;
DROP VIEW derived.bulletin_ligne;
DROP TABLE ref.bulletin_cas;
DROP TABLE ref.taux_cotisation;
DROP TABLE ref.parametre_social;
DROP TABLE ref.organisme_social;
