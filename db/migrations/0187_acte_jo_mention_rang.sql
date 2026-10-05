-- +goose Up

-- core.acte_jo_mention n'a aucune clé naturelle : chaque correspondance
-- regex dans le texte d'un acte devient sa propre ligne, sans déduplication,
-- et de vrais doublons existent (même acte_id/nom/prenom/origine/contexte
-- répété). rang fixe la position d'apparition dans l'acte, à l'écriture,
-- pour chaque acte_id — la même logique que la migration 0179 pour
-- core.declaration_item — et permet un MERGE au lieu du DELETE(scopé par
-- acte_id)+COPY actuel.
ALTER TABLE core.acte_jo_mention ADD COLUMN rang integer;

UPDATE core.acte_jo_mention SET rang = sub.rang
  FROM (
    SELECT id, row_number() OVER (PARTITION BY acte_id ORDER BY id) - 1 AS rang
      FROM core.acte_jo_mention
  ) sub
 WHERE sub.id = core.acte_jo_mention.id;

ALTER TABLE core.acte_jo_mention ALTER COLUMN rang SET NOT NULL;

ALTER TABLE core.acte_jo_mention
  ADD CONSTRAINT acte_jo_mention_unique_par_position UNIQUE (acte_id, rang);

-- +goose Down
ALTER TABLE core.acte_jo_mention DROP CONSTRAINT acte_jo_mention_unique_par_position;
ALTER TABLE core.acte_jo_mention DROP COLUMN rang;
