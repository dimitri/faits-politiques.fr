-- +goose Up
-- Les résultats du contrôle fiscal, France entière — montants notifiés
-- (droits et pénalités mis en recouvrement, avant tout recours ou
-- négociation) et montants effectivement encaissés, vérifiés directement
-- dans trois rapports du Sénat (commission des finances). Le notifié
-- n'est pas connu pour 2022 et 2023 dans une source primaire au moment du
-- chargement : colonne nullable, jamais comblée par une estimation.
CREATE TABLE core.controle_fiscal_resultats (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee               int NOT NULL UNIQUE,
  montant_notifie_m   double precision,        -- droits et pénalités mis en recouvrement, M€
  notifie_calcule     boolean NOT NULL DEFAULT false,  -- déduit par différence, non publié tel quel
  montant_encaisse_m  double precision NOT NULL,       -- effectivement recouvré, M€
  source_id           bigint NOT NULL REFERENCES raw.source(id)
);

COMMENT ON TABLE core.controle_fiscal_resultats IS
  'Résultats annuels du contrôle fiscal (montants notifiés et encaissés, France entière), '
  '2015-2024 — Sénat, commission des finances (rapports n° r22-072, l24-034-215-1, '
  'l25-139-314). Le notifié 2022 et 2023 n''a pas été retrouvé dans une source primaire ; '
  'le notifié 2024 est déduit de l''écart notifié/encaissé publié (11,4 + 5,2 Md€), signalé '
  'par notifie_calcule.';

-- +goose Down
DROP TABLE core.controle_fiscal_resultats;
