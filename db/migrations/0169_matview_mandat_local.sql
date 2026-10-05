-- +goose Up

-- Président/vice-présidents des conseils régionaux/départementaux/
-- communautaires : un rôle très spécifique parmi 617 196 mandats — 13 044
-- lignes en poste, moins de 3% de la table. Remplace le GROUP BY sur la
-- totalité de core.mandate que chargerCollectivites (internal/sitegen/
-- collectivites.go, fonction elus) refaisait à chaque construction.
CREATE MATERIALIZED VIEW mv.mandat_executif_local AS
  SELECT m.constituency, m.role, p.slug AS person_slug,
         p.given_name AS person_given_name, p.family_name AS person_family_name,
         lower(m.validity) AS depuis
    FROM core.mandate m JOIN core.person p ON p.id = m.person_id
   WHERE m.role LIKE '%résident%conseil%' AND m.constituency IS NOT NULL
     AND upper(m.validity) IS NULL;

CREATE INDEX mandat_executif_local_role_idx ON mv.mandat_executif_local (role);

COMMENT ON MATERIALIZED VIEW mv.mandat_executif_local IS
  'Présidents et vice-présidents en poste des conseils régionaux/'
  'départementaux/communautaires — remplace le GROUP BY sur la totalité de '
  'core.mandate que collectivites.go (fonction elus) refaisait à chaque '
  'construction.';

-- Effectifs des conseils régionaux/départementaux (par territoire) et
-- communautaires (total national) : trois GROUP BY que collectivites.go
-- refaisait sur la totalité de core.mandate à chaque construction.
CREATE MATERIALIZED VIEW mv.mandat_local_compte AS
  SELECT mandate_type::text AS mandate_type, left(constituency,2) AS code_territoire,
         count(*)::int AS nombre_elus
    FROM core.mandate
   WHERE mandate_type IN ('CONSEILLER_REGIONAL','CONSEILLER_DEPARTEMENTAL')
     AND constituency IS NOT NULL AND upper(validity) IS NULL
   GROUP BY 1, 2
  UNION ALL
  SELECT mandate_type::text, NULL, count(*)::int
    FROM core.mandate
   WHERE mandate_type = 'CONSEILLER_COMMUNAUTAIRE' AND upper(validity) IS NULL
   GROUP BY 1;

CREATE UNIQUE INDEX mandat_local_compte_pk
  ON mv.mandat_local_compte (mandate_type, coalesce(code_territoire, ''));

COMMENT ON MATERIALIZED VIEW mv.mandat_local_compte IS
  'Effectifs des conseils régionaux/départementaux (par territoire) et '
  'communautaires (total national) — remplace les GROUP BY/count(*) sur la '
  'totalité de core.mandate que collectivites.go (fonctions compte, '
  'chargerResumeEPCIBudget) refaisait à chaque construction.';

-- +goose Down
DROP MATERIALIZED VIEW mv.mandat_local_compte;
DROP MATERIALIZED VIEW mv.mandat_executif_local;
