-- +goose Up

-- mv.scrutin_vote_nominal gagne organization_id et le prénom séparé du nom
-- de famille : sans le premier, impossible de rejoindre par la clé que le
-- reste du code utilise déjà pour identifier un groupe (core.organization.
-- id — voir groupes.go, byID map[int64]*Groupe) ; sans le second,
-- nominalVotes et loadMembres (qui veulent chacun un ordre différent,
-- « Nom, Prénom » et « Prénom Nom ») auraient dû défaire en SQL une
-- concaténation faite plus haut — plus fragile que de laisser chacun
-- composer le sien depuis les deux parties.
--
-- Recréée en entier (DROP puis CREATE) : PostgreSQL ne sait pas changer les
-- colonnes d'un SELECT de matvue sans la refaire. Sans conséquence ici,
-- aucun code ne dépend encore de son schéma exact hors internal/matview.
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

COMMENT ON MATERIALIZED VIEW mv.scrutin_vote_nominal IS
  'Relevé nominatif de chaque scrutin, une ligne par bulletin — remplace le '
  'JOIN ballot/person/organization que internal/sitegen refaisait à chaque '
  'construction (voir nominalVotes, loadMembres, loadPartisDeclares — '
  'internal/sitegen/scrutins.go, groupes.go). Rafraîchie par '
  'internal/matview, jamais par une requête applicative.';

-- +goose Down
DROP MATERIALIZED VIEW mv.scrutin_vote_nominal;

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
