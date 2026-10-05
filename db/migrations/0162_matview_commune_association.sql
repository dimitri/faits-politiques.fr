-- +goose Up

-- mv.commune_association_count : le nombre d'associations déclarées par
-- commune (RNA) — ce que chargerPagesCommunes (internal/sitegen/
-- lieux_pages.go) recalculait par un GROUP BY sur la totalité de core.
-- association (1,18 million de lignes) à chaque construction, pour
-- alimenter les 34 875 pages communes.
CREATE MATERIALIZED VIEW mv.commune_association_count AS
  SELECT commune_code, count(*) AS n
    FROM core.association
   WHERE commune_code IS NOT NULL
   GROUP BY commune_code;

CREATE UNIQUE INDEX commune_association_count_pk ON mv.commune_association_count (commune_code);

COMMENT ON MATERIALIZED VIEW mv.commune_association_count IS
  'Associations déclarées (RNA) par commune — remplace le bloc '
  '« Associations déclarées » de chargerPagesCommunes (internal/sitegen/'
  'lieux_pages.go).';

-- +goose Down
DROP MATERIALIZED VIEW mv.commune_association_count;
