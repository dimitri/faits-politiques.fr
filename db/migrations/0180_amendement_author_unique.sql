-- +goose Up

-- core.amendement_author n'avait aucune clé naturelle : seule la logique du
-- connecteur (internal/an/amendements.go) garantit une ligne par amendement,
-- rien dans le schéma ne l'imposait. Nécessaire pour passer du DELETE+INSERT
-- à un MERGE : sans cette contrainte, MERGE ... ON tgt.amendement_id =
-- src.amendement_id échouerait dès qu'une deuxième correspondance existe côté
-- cible.
ALTER TABLE core.amendement_author
  ADD CONSTRAINT amendement_author_amendement_id_key UNIQUE (amendement_id);

-- +goose Down
ALTER TABLE core.amendement_author DROP CONSTRAINT amendement_author_amendement_id_key;
