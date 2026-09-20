-- +goose Up

-- Deux colonnes nommées "n" (0152, 0162) — clair dans le SELECT qui les
-- calcule, opaque partout où elles sont lues ensuite (sum(n), n::int, sans
-- rien qui rappelle ce qui est compté). Recréées avec un nom qui porte le
-- sens jusqu'au bout de la requête qui les consomme.
DROP MATERIALIZED VIEW mv.scrutin_groupe_vote;

CREATE MATERIALIZED VIEW mv.scrutin_groupe_vote AS
  SELECT b.scrutin_id,
         o.id                                             AS organization_id,
         coalesce(o.short_name, o.name)                   AS organisation_nom,
         o.slug                                            AS organisation_slug,
         coalesce(b.position_rectifiee, b.position)::text AS position,
         count(*)::int                                    AS nombre_votes
    FROM core.ballot b
    JOIN core.organization o ON o.id = b.organization_id
   GROUP BY 1, 2, 3, 4, 5;

CREATE UNIQUE INDEX scrutin_groupe_vote_pk
  ON mv.scrutin_groupe_vote (scrutin_id, organization_id, position);

COMMENT ON MATERIALIZED VIEW mv.scrutin_groupe_vote IS
  'Dépouillement par groupe de chaque scrutin (FOR/AGAINST/ABSTAIN/ABSENT/'
  'NON_VOTING), une ligne par (scrutin, organisation, position) — remplace '
  'le GROUP BY sur la totalité de core.ballot que cmd/build refaisait à '
  'chaque construction (voir groupBreakdown, internal/sitegen/scrutins.go). '
  'Rafraîchie par internal/matview, jamais par un GROUP BY applicatif.';

DROP MATERIALIZED VIEW mv.commune_association_count;

CREATE MATERIALIZED VIEW mv.commune_association_count AS
  SELECT commune_code, count(*) AS nombre_associations
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
CREATE MATERIALIZED VIEW mv.commune_association_count AS
  SELECT commune_code, count(*) AS n FROM core.association
   WHERE commune_code IS NOT NULL GROUP BY commune_code;
CREATE UNIQUE INDEX commune_association_count_pk ON mv.commune_association_count (commune_code);

DROP MATERIALIZED VIEW mv.scrutin_groupe_vote;
CREATE MATERIALIZED VIEW mv.scrutin_groupe_vote AS
  SELECT b.scrutin_id,
         o.id                                             AS organization_id,
         coalesce(o.short_name, o.name)                   AS organisation_nom,
         o.slug                                            AS organisation_slug,
         coalesce(b.position_rectifiee, b.position)::text AS position,
         count(*)::int                                    AS n
    FROM core.ballot b
    JOIN core.organization o ON o.id = b.organization_id
   GROUP BY 1, 2, 3, 4, 5;
CREATE UNIQUE INDEX scrutin_groupe_vote_pk
  ON mv.scrutin_groupe_vote (scrutin_id, organization_id, position);
