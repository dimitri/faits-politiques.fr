-- +goose Up

-- core.service_assainissement n'avait aucune clé naturelle : seule la
-- logique du connecteur (internal/eau/assainissement.go) garantissait une
-- ligne par (compétence, année, commune, service SISPEA), rien dans le
-- schéma ne l'imposait. Vérifié sans doublon sur ces quatre colonnes avant
-- ajout. Nécessaire pour passer du DELETE+COPY à un MERGE : sans cette
-- contrainte, MERGE ... ON tgt.competence = src.competence AND ... échouerait
-- dès qu'une deuxième correspondance existe côté cible.
ALTER TABLE core.service_assainissement
  ADD CONSTRAINT service_assainissement_unique UNIQUE (competence, annee, code_insee, id_sispea_service);

-- +goose Down
ALTER TABLE core.service_assainissement DROP CONSTRAINT service_assainissement_unique;
