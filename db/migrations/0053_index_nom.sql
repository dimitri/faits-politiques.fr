-- +goose Up
-- Le rapprochement d'un nom propre coûtait un parcours complet.
--
-- Le connecteur du Journal officiel cherche chaque personne citée par
-- `f_unaccent(lower(family_name)) = … AND f_unaccent(lower(given_name)) = …`.
-- Aucun index ne couvrait cette expression : chaque mention parcourait les
-- 620 000 personnes de core.person, et le chargement avançait à une cinquantaine
-- de mentions par minute.
--
-- L'index est posé sur l'EXPRESSION, seule forme utilisable ici : la comparaison
-- est faite sans accents ni casse, et un index sur les colonnes brutes ne serait
-- jamais consulté. core.f_unaccent existe précisément pour cela — c'est une
-- enveloppe IMMUTABLE autour de unaccent(), dont le dictionnaire n'est pas
-- réputé stable par PostgreSQL.
CREATE INDEX person_nom_norme_idx ON core.person (
  core.f_unaccent(lower(family_name)),
  core.f_unaccent(lower(given_name))
);

COMMENT ON INDEX core.person_nom_norme_idx IS
  'Rapprochement par nom normalisé : Journal officiel, fusion des sénateurs. '
  'Ne dispense pas de la règle — un nom seul ne prouve rien, il ouvre une '
  'candidature (core.acte_jo_mention.statut).';

-- +goose Down
DROP INDEX core.person_nom_norme_idx;
