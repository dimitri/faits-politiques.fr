-- +goose Up
-- La table core.public_contract existait déjà (0044 ou proche), référencée
-- par selection.dossier_item et core.evidence, mais aucun connecteur ne
-- l'alimentait — voir docs/perimetre.md § 4.4. Cette migration ajoute trois
-- colonnes utiles que les DECP consolidées portent et que le schéma
-- d'origine ne capturait pas, plutôt que de créer une nouvelle table.
ALTER TABLE core.public_contract
  ADD COLUMN acheteur_region_code text,
  ADD COLUMN titulaire_region_code text,
  ADD COLUMN marche_type text,          -- fournitures, services ou travaux
  ADD COLUMN montant_anomalie text;     -- si renseigné, le montant a été jugé aberrant par la source et corrigé (montant_rationalise utilisé à la place)

COMMENT ON TABLE core.public_contract IS
  'DECP (données essentielles de la commande publique) consolidées par decp-processing '
  '(Colmo/data.gouv.fr), format Parquet. Une ligne par (marché, titulaire) sur l''état '
  'ACTUEL du marché (donneesActuelles=true dans la source) — les versions antérieures '
  '(avant avenant) ne sont pas conservées. Le montant est montant_rationalise (corrigé '
  'des valeurs aberrantes par la source), pas le montant brut déclaré ; montant_anomalie '
  'renseigné signale une correction. source_uid combine uid, titulaire et un numéro de '
  'modification car un même marché a une ligne par titulaire (jusqu''à 87 co-titulaires '
  'observés) et la source contient un petit nombre de lignes dupliquées à l''identique '
  '(~0,3 % des lignes retenues, dédoublonnées à l''ingestion plutôt que signalées comme '
  'des marchés distincts).';

-- +goose Down
ALTER TABLE core.public_contract
  DROP COLUMN acheteur_region_code,
  DROP COLUMN titulaire_region_code,
  DROP COLUMN marche_type,
  DROP COLUMN montant_anomalie;
