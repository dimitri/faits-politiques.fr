-- +goose Up

-- core.aide_agence_eau n'a aucune clé naturelle : les fichiers eux-mêmes
-- portent de vrais doublons — deux lignes identiques sur toutes les
-- colonnes publiées (vérifié via GROUP BY sur l'ensemble des colonnes
-- publiées, ex. ARTOIS_PICARDIE/10e-11e/20-I-035/2020, deux occurrences
-- strictement identiques). rang fixe la position d'apparition dans le
-- fichier source, à l'écriture, pour chaque (agence, programme) — la même
-- logique que la migration 0179 pour core.declaration_item — et permet un
-- MERGE au lieu du DELETE(scopé par agence+programme)+COPY actuel.
ALTER TABLE core.aide_agence_eau ADD COLUMN rang integer;

UPDATE core.aide_agence_eau SET rang = sub.rang
  FROM (
    SELECT id, row_number() OVER (PARTITION BY agence, programme ORDER BY id) - 1 AS rang
      FROM core.aide_agence_eau
  ) sub
 WHERE sub.id = core.aide_agence_eau.id;

ALTER TABLE core.aide_agence_eau ALTER COLUMN rang SET NOT NULL;

ALTER TABLE core.aide_agence_eau
  ADD CONSTRAINT aide_agence_eau_unique_par_position UNIQUE (agence, programme, rang);

-- +goose Down
ALTER TABLE core.aide_agence_eau DROP CONSTRAINT aide_agence_eau_unique_par_position;
ALTER TABLE core.aide_agence_eau DROP COLUMN rang;
