-- +goose Up
-- Empreinte des données d'une section du site construit — pas la construction
-- elle-même, seulement « qu'est-ce qui a changé depuis la dernière fois ».
--
-- Calculée par cmd/ingest, une fois par exécution, jamais par cmd/build : la
-- requête qui la produit (voir internal/checksum) parcourt chaque table en
-- entier, et refaire ce parcours à chaque construction coûterait, mesuré sur
-- les tables réellement en jeu (commune_delinquance, 5,2 M lignes ; ballot,
-- 4,9 M ; commune_indicator, 2,5 M), plusieurs secondes par table à chaque
-- lancement — négligeable une fois par ingestion, gênant si répété à chaque
-- construction pendant qu'on ajuste un seul gabarit.
--
-- data_hash ne dit rien sur LE CODE qui lit ces tables : cmd/build compare
-- séparément, à la volée (lecture de fichiers, pas de requête), un hachage
-- des .go et .gohtml dont dépend la section. Les deux doivent concorder avec
-- la dernière construction réussie pour qu'une section soit recopiée plutôt
-- que reconstruite — voir cmd/build/cache.go.
CREATE TABLE core.section_checksum (
  section     text PRIMARY KEY,
  data_hash   text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.section_checksum IS
  'Empreinte des tables sources d''une section du site, recalculée par cmd/ingest '
  'après chaque exécution. Sert à décider si cmd/build peut recopier la section '
  'depuis la construction précédente plutôt que la refaire — jamais une donnée '
  'publiée elle-même.';

-- +goose Down
DROP TABLE core.section_checksum;
