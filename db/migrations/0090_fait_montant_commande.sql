-- +goose Up
-- Faits sur les multinationales : une nature de montant de plus. Les rapports
-- parlementaires chiffrent souvent des COMMANDES passées sur un accord-cadre
-- (bons de commande émis), ni plafond du marché ni paiement constaté. Voir D-063.
ALTER TABLE ref.fait_multinationale DROP CONSTRAINT fait_multinationale_nature_montant_check;
ALTER TABLE ref.fait_multinationale ADD CONSTRAINT fait_multinationale_nature_montant_check
  CHECK (nature_montant IN ('PLAFOND','ESTIME','COMMANDE','PAYE','AMENDE','IMPOT','CHIFFRE_AFFAIRES'));

-- +goose Down
ALTER TABLE ref.fait_multinationale DROP CONSTRAINT fait_multinationale_nature_montant_check;
ALTER TABLE ref.fait_multinationale ADD CONSTRAINT fait_multinationale_nature_montant_check
  CHECK (nature_montant IN ('PLAFOND','ESTIME','PAYE','AMENDE','IMPOT','CHIFFRE_AFFAIRES'));
