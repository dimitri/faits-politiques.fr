-- +goose Up

-- Un count(*) par source, page /sources — remplace un JOIN à quatre tables
-- filtré par slug, rejoué une fois par source (166 fois), sur la totalité
-- de raw.record (489 Mo, 177 026 lignes).
CREATE MATERIALIZED VIEW mv.source_enregistrements AS
  SELECT s.slug AS source_slug, count(*)::int AS nombre_enregistrements
    FROM raw.record rec
    JOIN raw.document d ON d.id = rec.document_id
    JOIN raw.retrieval r ON r.document_id = d.id
    JOIN raw.source s ON s.id = r.source_id
   GROUP BY s.slug;

CREATE UNIQUE INDEX source_enregistrements_pk ON mv.source_enregistrements (source_slug);

COMMENT ON MATERIALIZED VIEW mv.source_enregistrements IS
  'Nombre d''enregistrements par source (page /sources) — remplace le JOIN '
  'à quatre tables sur la totalité de raw.record que sources.go rejouait '
  'une fois par source.';

-- +goose Down
DROP MATERIALIZED VIEW mv.source_enregistrements;
