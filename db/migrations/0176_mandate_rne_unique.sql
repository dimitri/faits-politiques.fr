-- +goose Up

-- core.mandate n'a jusqu'ici aucune contrainte d'unicité sur (person_id,
-- mandate_type) — à raison : une même personne peut porter plusieurs mandats
-- successifs du même type (plusieurs mandats de DEPUTE au fil des
-- législatures, par exemple), chacun avec sa propre période de validité.
--
-- internal/communes/rne.go fait exception : sa propre requête d'insertion
-- porte déjà un DISTINCT ON (person_id, mandate_type) pour les cinq types
-- locaux qu'il gère (MAIRE, CONSEILLER_MUNICIPAL, CONSEILLER_COMMUNAUTAIRE,
-- CONSEILLER_DEPARTEMENTAL, CONSEILLER_REGIONAL) — l'invariant qu'IL respecte
-- est bien « une personne, un mandat actif de ce type », le RNE ne publiant
-- qu'un mandat en cours par élu et par fonction. Cet index rend cet
-- invariant, déjà vrai en pratique (vérifié : zéro doublon sur ces cinq
-- types avant cette migration), opposable à un MERGE — voir la conversion de
-- son DELETE+INSERT dans rne.go.
--
-- PARTIEL, jamais un index plein sur (person_id, mandate_type) : les autres
-- types (DEPUTE, SENATEUR...) n'ont pas cet invariant, et un index plein
-- romprait leur insertion dès qu'une personne cumule deux mandats du même
-- type au fil du temps.
CREATE UNIQUE INDEX mandate_rne_person_type_key ON core.mandate (person_id, mandate_type)
  WHERE mandate_type IN ('MAIRE', 'CONSEILLER_MUNICIPAL', 'CONSEILLER_COMMUNAUTAIRE',
                          'CONSEILLER_DEPARTEMENTAL', 'CONSEILLER_REGIONAL');

-- +goose Down
DROP INDEX core.mandate_rne_person_type_key;
