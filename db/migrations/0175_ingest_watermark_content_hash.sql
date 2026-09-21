-- +goose Up

-- core.ingest_watermark portait un seul mécanisme (record_count/high_water_id
-- sur raw.record — voir 0172) parce que seul internal/an s'y adossait. Les
-- connecteurs qui n'ont pas de raw.record à observer (internal/europe,
-- internal/senat, internal/campagne, internal/partis : ils parsent un
-- fichier téléchargé directement, sans étage raw.record intermédiaire) ont
-- besoin d'un signal équivalent tiré d'autre chose — le sha256 déjà calculé
-- par internal/archive.Fetch pour chaque fichier (Fetched.SHA256).
--
-- content_hash porte ce second mécanisme, dans la même table plutôt qu'une
-- nouvelle : la question posée (« faut-il refaire ce travail ? ») et sa
-- réponse (oui/non, avec raison) sont identiques, seule l'empreinte change
-- de nature. record_count/high_water_id passent NULL le temps de cette
-- migration à NOT NULL pour ne pas obliger un scope basé sur un fichier à
-- écrire deux zéros qui ne veulent rien dire — un scope ne renseigne que le
-- couple de colonnes qui correspond à son propre mécanisme, jamais les deux.
ALTER TABLE core.ingest_watermark ALTER COLUMN record_count DROP NOT NULL;
ALTER TABLE core.ingest_watermark ALTER COLUMN high_water_id DROP NOT NULL;
ALTER TABLE core.ingest_watermark ADD COLUMN content_hash text;

COMMENT ON COLUMN core.ingest_watermark.content_hash IS
  'sha256 (internal/archive.Fetched.SHA256) du dernier fichier téléchargé '
  'qu''un scope a traité avec succès — le pendant de record_count/'
  'high_water_id pour un connecteur sans étage raw.record (europe, senat, '
  'campagne, partis). Un scope ne renseigne jamais les deux mécanismes.';

-- +goose Down
ALTER TABLE core.ingest_watermark DROP COLUMN content_hash;
ALTER TABLE core.ingest_watermark ALTER COLUMN record_count SET NOT NULL;
ALTER TABLE core.ingest_watermark ALTER COLUMN high_water_id SET NOT NULL;
