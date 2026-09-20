-- +goose Up

-- mv.scrutin_vote_nominal : le relevé nominatif de chaque scrutin — ce que
-- cmd/build/scrutins.go (nominalVotes) recalculait par un JOIN à trois
-- tables (ballot, scrutin, person) plus organization, TRIÉ, sur la
-- totalité de core.ballot, À CHAQUE CONSTRUCTION. Contrairement à
-- mv.scrutin_groupe_vote (0152), pas d'agrégation ici : même nombre de
-- lignes que core.ballot, le gain vient d'éviter de refaire le JOIN et le
-- tri à chaque fois, pas de réduire le volume.
--
-- LEFT JOIN organization, comme le SELECT qu'elle remplace : un bulletin
-- sans organization_id garde sa ligne (groupe/slug vides), pas une
-- régression introduite ici.
CREATE MATERIALIZED VIEW mv.scrutin_vote_nominal AS
  SELECT b.scrutin_id,
         p.slug                                              AS person_slug,
         p.family_name || ', ' || p.given_name                AS person_nom,
         p.family_name                                        AS person_family_name,
         coalesce(o.short_name, o.name, '')                    AS organisation_nom,
         coalesce(o.slug, '')                                  AS organisation_slug,
         coalesce(b.position_rectifiee, b.position)::text      AS position,
         b.position_rectifiee IS NOT NULL                      AS rectifiee
    FROM core.ballot b
    JOIN core.person p ON p.id = b.person_id
    LEFT JOIN core.organization o ON o.id = b.organization_id;

CREATE UNIQUE INDEX scrutin_vote_nominal_pk
  ON mv.scrutin_vote_nominal (scrutin_id, person_slug);
CREATE INDEX scrutin_vote_nominal_ordre_idx
  ON mv.scrutin_vote_nominal (scrutin_id, person_family_name);

COMMENT ON MATERIALIZED VIEW mv.scrutin_vote_nominal IS
  'Relevé nominatif de chaque scrutin, une ligne par bulletin — remplace le '
  'JOIN ballot/person/organization que cmd/build refaisait à chaque '
  'construction (voir nominalVotes, cmd/build/scrutins.go). Rafraîchie par '
  'internal/matview, jamais par une requête applicative.';

-- +goose Down
DROP MATERIALIZED VIEW mv.scrutin_vote_nominal;
