-- +goose Up

-- core.rpps_professionnel_activite n'a aucune clé naturelle : une même
-- personne (identifiant_pp) porte une ligne par activité déclarée, et le
-- fichier RPPS contient de vrais doublons sur (identifiant_pp,
-- numero_finess_site, code_role, code_savoir_faire, code_mode_exercice) —
-- vérifié via GROUP BY, ex. identifiant_pp=10000005313. rang fixe la
-- position d'apparition dans le fichier source, à l'écriture, pour chaque
-- identifiant_pp — la même logique que la migration 0179 pour
-- core.declaration_item — et permet un MERGE au lieu du DELETE(table
-- entière)+COPY actuel.
ALTER TABLE core.rpps_professionnel_activite ADD COLUMN rang integer;

UPDATE core.rpps_professionnel_activite SET rang = sub.rang
  FROM (
    SELECT id, row_number() OVER (PARTITION BY identifiant_pp ORDER BY id) - 1 AS rang
      FROM core.rpps_professionnel_activite
  ) sub
 WHERE sub.id = core.rpps_professionnel_activite.id;

ALTER TABLE core.rpps_professionnel_activite ALTER COLUMN rang SET NOT NULL;

ALTER TABLE core.rpps_professionnel_activite
  ADD CONSTRAINT rpps_professionnel_activite_unique_par_position UNIQUE (identifiant_pp, rang);

-- +goose Down
ALTER TABLE core.rpps_professionnel_activite DROP CONSTRAINT rpps_professionnel_activite_unique_par_position;
ALTER TABLE core.rpps_professionnel_activite DROP COLUMN rang;
