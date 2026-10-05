-- +goose Up

-- core.topic_assignment n'avait aucune clé naturelle sur les lignes
-- attachées à un scrutin (internal/europe/europe.go, classification EuroVoc
-- des scrutins du Parlement européen) : seule la logique du connecteur
-- garantissait une ligne par (topic_code, scrutin_id), rien dans le schéma
-- ne l'imposait. Nécessaire pour passer du DELETE+COPY à un MERGE : sans
-- cette contrainte, MERGE ... ON tgt.topic_code = src.topic_code AND
-- tgt.scrutin_id = src.scrutin_id échouerait dès qu'une deuxième
-- correspondance existe côté cible. Un index partiel (scrutin_id IS NOT
-- NULL) plutôt qu'une contrainte pleine table : les lignes attachées à un
-- dossier/texte/amendement (ex. internal/senat/senat.go) ont scrutin_id
-- NULL et ne sont pas concernées par cette clé.
CREATE UNIQUE INDEX topic_assignment_scrutin_topic_key
  ON core.topic_assignment (topic_code, scrutin_id)
  WHERE scrutin_id IS NOT NULL;

-- +goose Down
DROP INDEX core.topic_assignment_scrutin_topic_key;
