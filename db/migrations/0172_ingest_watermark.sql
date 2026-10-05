-- +goose Up

-- core.ingest_watermark : « le lot de raw.record qu'une étape de
-- normalisation a déjà digéré », pas les données elles-mêmes. Sert à
-- répondre vite à « faut-il refaire ce travail » sans le refaire pour le
-- savoir — le même principe que core.section_checksum (cmd/ingest /
-- cmd/build), mais à un autre étage : ici, une étape de normalisation
-- décide de sauter SA PROPRE reconstruction ; section_checksum décide si
-- cmd/build peut recopier une section déjà construite. Les deux caches ne
-- se recouvrent pas et ne doivent pas être confondus.
--
-- record_count et high_water_id, pas un hachage du contenu : raw.record
-- n'est jamais réécrit en place, seulement complété (une nouvelle ligne
-- par nouvelle version d'une fiche) — comparer ces deux nombres suffit
-- donc à détecter tout ajout, et coûte un balayage d'index plutôt qu'un
-- hachage de chaque ligne (voir internal/checksum, qui lui doit hacher
-- parce qu'il compare des tables mutables).
CREATE TABLE core.ingest_watermark (
  scope         text PRIMARY KEY,
  record_count  bigint NOT NULL,
  high_water_id bigint NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.ingest_watermark IS
  'Dernier état de raw.record (count, max(id)) qu''une étape de normalisation '
  'a traité avec succès pour un scope donné — permet à cette étape de sauter '
  'sa reconstruction quand rien de neuf n''est arrivé depuis. Jamais lu ni '
  'écrit par cmd/build (voir core.section_checksum pour ce cache-là).';

-- +goose Down
DROP TABLE core.ingest_watermark;
