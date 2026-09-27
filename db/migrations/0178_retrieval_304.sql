-- +goose Up

-- document_present_iff_success interdisait un document_id hors 200-299 —
-- correct tant qu'aucune requête conditionnelle n'existait : un GET normal
-- n'a que deux issues, un document reçu (2xx) ou rien (tout le reste).
-- Un 304 Not Modified (voir internal/archive.go, fetchOnce) en ajoute une
-- troisième, dont le sens est justement l'inverse d'une absence : « le
-- document que tu as DÉJÀ est toujours le bon ». Il porte donc, lui aussi,
-- un document_id — celui de la retrieval précédente — et la contrainte
-- s'élargit pour l'admettre sans rien relâcher d'autre : un 4xx/5xx reste
-- aussi strictement sans document qu'avant.
ALTER TABLE raw.retrieval DROP CONSTRAINT document_present_iff_success;
ALTER TABLE raw.retrieval ADD CONSTRAINT document_present_iff_success
  CHECK (((http_status BETWEEN 200 AND 299) OR http_status = 304) = (document_id IS NOT NULL));

-- +goose Down
ALTER TABLE raw.retrieval DROP CONSTRAINT document_present_iff_success;
ALTER TABLE raw.retrieval ADD CONSTRAINT document_present_iff_success
  CHECK ((http_status BETWEEN 200 AND 299) = (document_id IS NOT NULL));
