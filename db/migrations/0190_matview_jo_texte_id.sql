-- +goose Up

-- jo.texte pèse 463 Mo (1,24M lignes, titre_complet compris) pour un seul
-- besoin côté sitegen : canaux.go vérifie qu'un identifiant précis EXISTE,
-- jamais son contenu. Découvert en fermant le périmètre CI (fpctl dump ci
-- / dump restore / build site sur une base vide, cmd/fpctl/man/
-- fpctl-dump.md § MÉTHODE) — jo.texte wholesale aurait fait exploser la
-- taille du dump pour une colonne sur dix.
CREATE MATERIALIZED VIEW mv.jo_texte_id AS
  SELECT id FROM jo.texte;

CREATE UNIQUE INDEX jo_texte_id_pk ON mv.jo_texte_id (id);

COMMENT ON MATERIALIZED VIEW mv.jo_texte_id IS
  'Les seuls identifiants de jo.texte (canaux.go : existence, jamais le '
  'contenu) — remplace jo.texte wholesale (463 Mo) dans le périmètre CI.';

-- +goose Down
DROP MATERIALIZED VIEW mv.jo_texte_id;
