-- +goose Up

-- core.declaration_item n'a aucune clé naturelle : deux items du même bloc
-- (par exemple deux « activités professionnelles ») ne se distinguent que par
-- leur position dans le flux XML de la HATVP. rang fixe cette position à
-- l'écriture (l'ordre d'apparition dans le bloc, tel que lu par le
-- connecteur), pour permettre un MERGE au lieu du DELETE+COPY intégral
-- actuel.
ALTER TABLE core.declaration_item ADD COLUMN rang integer;

UPDATE core.declaration_item SET rang = sub.rang
  FROM (
    SELECT id, row_number() OVER (PARTITION BY declaration_id, bloc ORDER BY id) - 1 AS rang
      FROM core.declaration_item
  ) sub
 WHERE sub.id = core.declaration_item.id;

ALTER TABLE core.declaration_item ALTER COLUMN rang SET NOT NULL;

ALTER TABLE core.declaration_item
  ADD CONSTRAINT declaration_item_unique_par_position UNIQUE (declaration_id, bloc, rang);

-- +goose Down
ALTER TABLE core.declaration_item DROP CONSTRAINT declaration_item_unique_par_position;
ALTER TABLE core.declaration_item DROP COLUMN rang;
