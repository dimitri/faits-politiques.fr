-- +goose Up

-- mv.scrutin_vote_nominal gagne person_id : internal/sitegen/load.go
-- (loadPersons, loadVotesBulk) indexe ses personnes par core.person.id
-- (map[int64]*Person), pas par slug — sans cette colonne, chaque
-- consommateur aurait dû rejoindre core.person une seconde fois pour
-- retrouver l'identifiant qu'il connaît déjà.
DROP MATERIALIZED VIEW mv.scrutin_vote_nominal;

CREATE MATERIALIZED VIEW mv.scrutin_vote_nominal AS
  SELECT b.scrutin_id,
         p.id                                                  AS person_id,
         p.slug                                              AS person_slug,
         p.family_name                                        AS person_family_name,
         p.given_name                                         AS person_given_name,
         o.id                                                  AS organization_id,
         coalesce(o.short_name, o.name, '')                    AS organisation_nom,
         coalesce(o.slug, '')                                  AS organisation_slug,
         coalesce(b.position_rectifiee, b.position)::text      AS position,
         b.position_rectifiee IS NOT NULL                      AS rectifiee
    FROM core.ballot b
    JOIN core.person p ON p.id = b.person_id
    LEFT JOIN core.organization o ON o.id = b.organization_id;

CREATE UNIQUE INDEX scrutin_vote_nominal_pk
  ON mv.scrutin_vote_nominal (scrutin_id, person_id);
CREATE INDEX scrutin_vote_nominal_ordre_idx
  ON mv.scrutin_vote_nominal (scrutin_id, person_family_name);
CREATE INDEX scrutin_vote_nominal_organization_idx
  ON mv.scrutin_vote_nominal (organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX scrutin_vote_nominal_person_idx
  ON mv.scrutin_vote_nominal (person_id);

COMMENT ON MATERIALIZED VIEW mv.scrutin_vote_nominal IS
  'Relevé nominatif de chaque scrutin, une ligne par bulletin — remplace le '
  'JOIN ballot/person/organization que internal/sitegen refaisait à chaque '
  'construction (voir nominalVotes, loadMembres, loadPartisDeclares, '
  'loadPersons, loadVotesBulk — internal/sitegen/scrutins.go, groupes.go, '
  'load.go). Rafraîchie par internal/matview, jamais par une requête '
  'applicative.';

-- +goose Down
DROP MATERIALIZED VIEW mv.scrutin_vote_nominal;

CREATE MATERIALIZED VIEW mv.scrutin_vote_nominal AS
  SELECT b.scrutin_id,
         p.slug                                              AS person_slug,
         p.family_name                                        AS person_family_name,
         p.given_name                                         AS person_given_name,
         o.id                                                  AS organization_id,
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
CREATE INDEX scrutin_vote_nominal_organization_idx
  ON mv.scrutin_vote_nominal (organization_id) WHERE organization_id IS NOT NULL;
