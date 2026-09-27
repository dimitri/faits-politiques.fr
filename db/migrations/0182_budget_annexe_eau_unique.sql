-- +goose Up

-- core.budget_annexe_eau n'avait aucune clé naturelle : seule la logique du
-- connecteur (internal/eau/budget_annexe.go) garantissait une ligne par
-- (collectivité, budget annexe, année, agrégat), rien dans le schéma ne
-- l'imposait. Vérifié sans doublon sur ces cinq colonnes avant ajout.
-- Nécessaire pour passer du DELETE+COPY à un MERGE : sans cette contrainte,
-- MERGE ... ON tgt.type_collectivite = src.type_collectivite AND ...
-- échouerait dès qu'une deuxième correspondance existe côté cible.
ALTER TABLE core.budget_annexe_eau
  ADD CONSTRAINT budget_annexe_eau_unique UNIQUE (type_collectivite, code, nom_budget, annee, agregat);

-- +goose Down
ALTER TABLE core.budget_annexe_eau DROP CONSTRAINT budget_annexe_eau_unique;
