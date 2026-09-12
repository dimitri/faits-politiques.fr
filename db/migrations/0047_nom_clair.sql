-- +goose Up
-- Le nom « en clair » d'une commune, en capitales et sans article.
--
-- L'INSEE publie deux formes : LIBELLE (« L'Abergement-Clémenciat ») et NCC
-- (« ABERGEMENT CLEMENCIAT »), cette dernière en capitales, sans accents, et
-- avec l'article retiré. Les fichiers administratifs — dont le Répertoire
-- national des associations — emploient la seconde, parfois avec l'article
-- rejeté en fin de chaîne (« BUISSON DE CADOUIN L »).
--
-- Sans cette colonne, rattacher une association à sa commune supposait de
-- deviner la convention de nommage. Avec elle, c'est une égalité entre deux
-- chaînes publiées par le même producteur.
ALTER TABLE ref.commune ADD COLUMN IF NOT EXISTS nom_clair text;

CREATE INDEX IF NOT EXISTS commune_nom_clair_idx
  ON ref.commune (code_departement, nom_clair);

COMMENT ON COLUMN ref.commune.nom_clair IS
  'Colonne NCC du Code officiel géographique : capitales, sans accent, sans '
  'article. Clé de jointure des fichiers administratifs.';

-- +goose Down
DROP INDEX IF EXISTS commune_nom_clair_idx;
ALTER TABLE ref.commune DROP COLUMN IF EXISTS nom_clair;
