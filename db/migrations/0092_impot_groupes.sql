-- +goose Up
-- Ce que les multinationales paient en France : ce qu'on sait, groupe par
-- groupe. Voir docs/evasion-fiscale-multinationales.md § 5 et D-064.
--
--   ref.parametre_is              taux de l'impôt sur les sociétés et contributions,
--                                 pour calculer l'impôt « théorique » d'un résultat ;
--   core.cbcr_public              déclarations pays par pays PUBLIQUES (directive
--                                 (UE) 2021/2101) : impôt dû et payé par juridiction,
--                                 publié par le groupe lui-même ;
--   core.groupe_resultat_sec      résultats et impôts des groupes cotés aux
--                                 États-Unis (formulaires 10-K, API XBRL de la SEC) :
--                                 pas de ventilation par pays, mais la part étrangère ;
--   ref.groupe_statut_fiscal      ce que les sources officielles établissent, groupe
--                                 par groupe, et sur quel fait ;
--   derived.filiale_impot_theorique, derived.fiche_impot_groupe.

CREATE TABLE ref.parametre_is (
  code         text PRIMARY KEY,
  libelle      text NOT NULL,
  valeur       numeric NOT NULL,
  unite        text NOT NULL,
  fondement    text NOT NULL,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  document_id  bigint REFERENCES raw.document(id)
);

CREATE TABLE core.cbcr_public (
  groupe                    text NOT NULL,       -- nom de la sélection des filiales
  exercice_debut            date NOT NULL,
  exercice_fin              date NOT NULL,
  devise                    text NOT NULL,
  juridiction               text NOT NULL,       -- ISO 3166 alpha-2, ou 'AUTRES' (agrégat publié)
  chiffre_affaires          numeric,
  benefice_avant_impot      numeric,
  impot_paye                numeric,             -- décaissé dans l'exercice, remboursements déduits
  impot_du                  numeric,             -- charge d'impôt courant de l'exercice
  benefices_non_distribues  numeric,
  salaries                  numeric,
  source_id                 bigint NOT NULL REFERENCES raw.source(id),
  document_id               bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (groupe, exercice_fin, juridiction)
);

COMMENT ON TABLE core.cbcr_public IS
  'Déclarations pays par pays publiées par les groupes (directive (UE) 2021/2101). Toutes les entités '
  'du groupe dans la juridiction, chiffre d''affaires intragroupe compris ; impôt payé ≠ impôt dû '
  '(décalages, remboursements).';

CREATE TABLE core.groupe_resultat_sec (
  groupe        text NOT NULL,
  cik           text NOT NULL,
  exercice_fin  date NOT NULL,
  concept       text NOT NULL,     -- code normalisé : IMPOT, BENEFICE_AVANT_IMPOT, …_ETRANGER, …_DOMESTIQUE, IMPOT_COURANT_ETRANGER, CHIFFRE_AFFAIRES
  element_xbrl  text NOT NULL,     -- élément us-gaap retenu pour ce concept
  valeur        numeric NOT NULL,
  unite         text NOT NULL,
  formulaire    text NOT NULL,     -- 10-K, 20-F
  depot         text NOT NULL,     -- numéro d'enregistrement du dépôt (accession)
  document_id   bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (groupe, exercice_fin, concept)
);

-- Le classement éditorial de chaque groupe, écrit et fondé. Il ne dit jamais
-- « évasion » sans source officielle.
CREATE TABLE ref.groupe_statut_fiscal (
  groupe       text PRIMARY KEY,
  -- FRAUDE_TRANSIGEE      convention judiciaire d'intérêt public pour fraude fiscale ;
  -- IMPOT_NUL_CONSTATE    impôt sur les sociétés nul établi par une enquête officielle ;
  -- FACTURATION_ETRANGER  contrats ou clients français facturés depuis une société
  --                       étrangère du groupe, établi par une source officielle ;
  -- GROUPE_FRANCAIS       société de tête en France ;
  -- AUCUN_CONSTAT_PUBLIC  aucune source officielle ne documente d'évasion ni de fraude.
  statut       text NOT NULL CHECK (statut IN ('FRAUDE_TRANSIGEE','IMPOT_NUL_CONSTATE','FACTURATION_ETRANGER',
                                               'GROUPE_FRANCAIS','AUCUN_CONSTAT_PUBLIC')),
  resume       text NOT NULL,
  faits        text[] NOT NULL DEFAULT '{}',   -- identifiants de ref.fait_multinationale
  CHECK (statut IN ('AUCUN_CONSTAT_PUBLIC','GROUPE_FRANCAIS') OR cardinality(faits) > 0)
);

-- L'impôt « théorique » d'une filiale : le taux légal appliqué à son résultat
-- courant avant impôt, avec la contribution sociale (3,3 % de l'IS au-delà de
-- 763 000 €) et, pour les exercices clos à compter du 31 décembre 2025, la
-- contribution exceptionnelle des entreprises de plus de 1 Md€ de chiffre
-- d'affaires. Ce n'est pas l'impôt dû : le résultat fiscal diffère du résultat
-- courant (réintégrations, déficits, crédits d'impôt dont le CIR), et l'écart
-- publié mêle impôt, exceptionnel et participation. Exercices ouverts avant 2022
-- (taux supérieurs) : non calculés. Le seuil de chiffre d'affaires est apprécié
-- ici société par société, pas au niveau d'un groupe fiscalement intégré.
CREATE VIEW derived.filiale_impot_theorique AS
WITH p AS (
  SELECT max(valeur) FILTER (WHERE code = 'IS_TAUX_NORMAL')          / 100 AS is_taux,
         max(valeur) FILTER (WHERE code = 'CSB_TAUX')                / 100 AS csb_taux,
         max(valeur) FILTER (WHERE code = 'CSB_ABATTEMENT')                AS csb_abattement,
         max(valeur) FILTER (WHERE code = 'CEBGE_TAUX_1_3_MD')       / 100 AS ce_taux_bas,
         max(valeur) FILTER (WHERE code = 'CEBGE_TAUX_3_MD')         / 100 AS ce_taux_haut,
         max(valeur) FILTER (WHERE code = 'CEBGE_SEUIL_BAS')               AS ce_seuil_bas,
         max(valeur) FILTER (WHERE code = 'CEBGE_SEUIL_HAUT')              AS ce_seuil_haut
  FROM ref.parametre_is
), f AS (
  SELECT c.*, (SELECT max(chiffre_affaires) FROM core.entreprise_comptes x
               WHERE x.siren = c.siren AND x.type_bilan IN ('C','S')
                 AND x.date_cloture < c.date_cloture AND x.date_cloture >= c.date_cloture - 400) AS ca_precedent
  FROM derived.filiale_etrangere_comptes c
  WHERE c.date_cloture >= '2022-12-31'
), t AS (
  SELECT f.*, p.*, greatest(f.resultat_courant_ai, 0) * p.is_taux AS is_theorique
  FROM f CROSS JOIN p
)
SELECT siren, origine, groupe, pays_groupe, denomination, date_cloture, chiffre_affaires,
       resultat_courant_ai, resultat_net, ecart_rcai_resultat_net,
       round(is_theorique
             + csb_taux * greatest(0, is_theorique - csb_abattement)
             + CASE WHEN date_cloture >= '2025-12-31'
                         AND greatest(chiffre_affaires, coalesce(ca_precedent, 0)) >= ce_seuil_bas
                    THEN CASE WHEN greatest(chiffre_affaires, coalesce(ca_precedent, 0)) >= ce_seuil_haut
                              THEN ce_taux_haut ELSE ce_taux_bas END * is_theorique
                    ELSE 0 END, 0) AS impot_theorique,
       round(100 * ecart_rcai_resultat_net / nullif(resultat_courant_ai, 0), 1) AS ecart_pct_rcai,
       'filiale-impot-theorique-v1'::text AS method_version
FROM t
WHERE resultat_courant_ai IS NOT NULL;

COMMENT ON COLUMN derived.filiale_impot_theorique.impot_theorique IS
  'Taux légal (IS, contribution sociale, contribution exceptionnelle) appliqué au résultat courant avant '
  'impôt. Point de comparaison, pas l''impôt dû.';

-- La fiche d'un groupe : ce que ses filiales déclarent en France, l'impôt que le
-- taux légal y appliquerait, ce que le groupe publie lui-même pour la France,
-- sa part de bénéfice étranger, ses marchés publics, et le statut établi.
CREATE VIEW derived.fiche_impot_groupe AS
WITH fil AS (
  SELECT groupe, count(*) AS societes, sum(chiffre_affaires) AS ca, sum(resultat_courant_ai) AS rcai,
         sum(resultat_net) AS rn, sum(ecart_rcai_resultat_net) AS ecart, sum(impot_theorique) AS impot_theorique,
         min(date_cloture) AS premiere_cloture, max(date_cloture) AS derniere_cloture
  FROM (SELECT DISTINCT ON (siren) * FROM derived.filiale_impot_theorique ORDER BY siren, (origine = 'SELECTION') DESC) x
  WHERE origine = 'SELECTION'
  GROUP BY groupe
), cb AS (
  SELECT DISTINCT ON (groupe) groupe, exercice_fin AS cbcr_exercice, devise AS cbcr_devise,
         chiffre_affaires AS cbcr_ca_fr, benefice_avant_impot AS cbcr_benefice_fr, impot_du AS cbcr_impot_du_fr,
         impot_paye AS cbcr_impot_paye_fr, salaries AS cbcr_salaries_fr
  FROM core.cbcr_public WHERE juridiction = 'FR'
  ORDER BY groupe, exercice_fin DESC
), sec AS (
  SELECT DISTINCT ON (groupe) groupe, exercice_fin AS sec_exercice,
         max(valeur) FILTER (WHERE concept = 'IMPOT') OVER w AS sec_impot,
         coalesce(max(valeur) FILTER (WHERE concept = 'BENEFICE_AVANT_IMPOT') OVER w,
                  max(valeur) FILTER (WHERE concept = 'BENEFICE_AVANT_IMPOT_DOMESTIQUE') OVER w
                  + max(valeur) FILTER (WHERE concept = 'BENEFICE_AVANT_IMPOT_ETRANGER') OVER w) AS sec_benefice,
         max(valeur) FILTER (WHERE concept = 'BENEFICE_AVANT_IMPOT_ETRANGER') OVER w AS sec_benefice_etranger,
         max(valeur) FILTER (WHERE concept = 'IMPOT_COURANT_ETRANGER') OVER w AS sec_impot_courant_etranger
  FROM core.groupe_resultat_sec
  WINDOW w AS (PARTITION BY groupe, exercice_fin)
  ORDER BY groupe, exercice_fin DESC
), mp AS (
  SELECT groupe, sum(marches) FILTER (WHERE correspondance IN ('SIREN','NOM')) AS marches_titulaire,
         sum(marches) FILTER (WHERE correspondance = 'OBJET') AS marches_objet
  FROM derived.multinationale_marches GROUP BY groupe
)
SELECT s.groupe, s.statut, s.resume, s.faits,
       fil.societes, fil.ca, fil.rcai, fil.rn, fil.ecart, fil.impot_theorique, fil.derniere_cloture,
       cb.cbcr_exercice, cb.cbcr_devise, cb.cbcr_ca_fr, cb.cbcr_benefice_fr, cb.cbcr_impot_du_fr, cb.cbcr_impot_paye_fr, cb.cbcr_salaries_fr,
       round(100 * cb.cbcr_impot_du_fr / nullif(cb.cbcr_benefice_fr, 0), 1) AS cbcr_taux_fr_pct,
       sec.sec_exercice, sec.sec_impot, sec.sec_benefice, sec.sec_benefice_etranger, sec.sec_impot_courant_etranger,
       round(100 * sec.sec_impot / nullif(sec.sec_benefice, 0), 1) AS sec_taux_effectif_pct,
       round(100 * sec.sec_impot_courant_etranger / nullif(sec.sec_benefice_etranger, 0), 1) AS sec_taux_courant_etranger_pct,
       mp.marches_titulaire, mp.marches_objet,
       'fiche-impot-groupe-v1'::text AS method_version
FROM ref.groupe_statut_fiscal s
LEFT JOIN fil USING (groupe)
LEFT JOIN cb USING (groupe)
LEFT JOIN sec USING (groupe)
LEFT JOIN mp USING (groupe);

-- +goose Down
DROP VIEW derived.fiche_impot_groupe;
DROP VIEW derived.filiale_impot_theorique;
DROP TABLE ref.groupe_statut_fiscal;
DROP TABLE core.groupe_resultat_sec;
DROP TABLE core.cbcr_public;
DROP TABLE ref.parametre_is;
