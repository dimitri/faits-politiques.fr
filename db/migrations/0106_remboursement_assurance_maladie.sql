-- +goose Up
-- Open Damir (extraction ouverte du SNDS) : 36,6 millions de lignes pour le
-- seul mois de janvier 2025, ~970 Mo compressés par mois — bien trop fin
-- pour être chargé tel quel (voir docs/sante-donnees.md § 4, chargement
-- écarté une première fois pour cette raison précise). Ce qui suit charge un
-- AGRÉGAT calculé en flux pendant la lecture du fichier source, jamais le
-- détail ligne à ligne.
--
-- Agrégation par mois de TRAITEMENT (FLX_ANN_MOI), pas par mois de SOINS
-- (SOI_ANN/SOI_MOI) : un même fichier mensuel contient des soins de
-- centaines de mois différents (remboursements tardifs), si bien qu'un
-- agrégat par mois de soins serait incomplet tant que les fichiers
-- postérieurs n'ont pas été relus. Le mois de traitement, lui, est complet
-- dès la lecture d'un seul fichier — au prix de mesurer un flux
-- administratif (quand la Sécu a payé), pas la consommation de soins réelle
-- (quand le soin a eu lieu).
CREATE TABLE core.remboursement_national (
  annee              smallint NOT NULL,
  mois               smallint NOT NULL,
  montant_rembourse  numeric NOT NULL,
  base_remboursement numeric NOT NULL,
  nb_actes           bigint NOT NULL,
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, mois)
);

COMMENT ON TABLE core.remboursement_national IS
  'Open Damir (SNDS), agrégat national par mois de TRAITEMENT (pas de soins). '
  'Filtré sur PRS_REM_TYP=0 (voir le descriptif des variables Open Damir : sans ce '
  'filtre, les dénombrements et une partie des montants sont comptés en double). '
  'Ne couvre que les prestations remboursées en ville (nomenclature Open Damir) — '
  'ne reconstitue pas l''ONDAM en entier, qui inclut aussi l''hôpital (PMSI/T2A, '
  'core.pmsi_mco_national) et le médico-social, non couverts par cette source.';

CREATE TABLE core.remboursement_region_prestation (
  annee              smallint NOT NULL,
  mois               smallint NOT NULL,
  region_code        text NOT NULL,   -- BEN_RES_REG : région de résidence du bénéficiaire
  prs_nat_code       text NOT NULL,   -- nature de prestation, code brut (nomenclature non chargée)
  montant_rembourse  numeric NOT NULL,
  nb_actes           bigint NOT NULL,
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  created_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.remboursement_region_prestation IS
  'Open Damir (SNDS), agrégat par mois de traitement, région de résidence du '
  'bénéficiaire (BEN_RES_REG, 14 valeurs) et nature de prestation (PRS_NAT, code brut '
  'CNAM — 886 valeurs distinctes observées, AUCUNE nomenclature de libellés chargée : '
  'un code non traduit reste un code, pas une catégorie interprétée). Même filtre '
  'PRS_REM_TYP=0 que core.remboursement_national. Sans index à l''écriture : ajoutés '
  'après le chargement complet (voir internal/damir/damir.go) pour que le COPY en '
  'flux ne maintienne pas un index ligne à ligne.';

-- +goose Down
DROP TABLE core.remboursement_region_prestation;
DROP TABLE core.remboursement_national;
