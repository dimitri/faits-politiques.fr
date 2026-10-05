-- +goose Up

-- mv.securite_dept_annee : les faits et la population sommés par
-- (indicateur, département, année) — ce que loadSecurite (internal/
-- sitegen/securite.go) recalculait deux fois PAR INDICATEUR (15
-- indicateurs, donc 30 requêtes) sur la totalité de core.
-- commune_delinquance (5,2 millions de lignes) à chaque construction.
--
-- Les SOMMES BRUTES (nombre, population), pas le taux déjà divisé : la
-- carte départementale ET la série nationale en ont besoin, mais la série
-- nationale doit resommer ces deux nombres à travers les départements —
-- jamais moyenner des taux déjà calculés, qui donnerait le même poids à
-- une commune de 200 âmes et à Marseille (voir le commentaire de
-- loadSecurite). Une matvue par département permet aux deux consommateurs
-- de repartir du même endroit.
CREATE MATERIALIZED VIEW mv.securite_dept_annee AS
  SELECT d.indicateur_code,
         c.code_departement,
         max(c.nom_clair) AS nom_departement,
         d.annee,
         sum(d.nombre)     AS nombre,
         sum(d.population) AS population
    FROM core.commune_delinquance d
    JOIN ref.commune c ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
   WHERE d.diffuse
   GROUP BY d.indicateur_code, c.code_departement, d.annee;

CREATE UNIQUE INDEX securite_dept_annee_pk
  ON mv.securite_dept_annee (indicateur_code, code_departement, annee);

COMMENT ON MATERIALIZED VIEW mv.securite_dept_annee IS
  'Faits et population sommés par indicateur/département/année (SSMSI) — '
  'remplace les deux requêtes par indicateur (carte départementale, série '
  'nationale) que loadSecurite (internal/sitegen/securite.go) refaisait '
  'sur la totalité de core.commune_delinquance à chaque construction. Les '
  'deux consommateurs redivisent nombre/population à la lecture — jamais '
  'un taux déjà moyenné.';

-- +goose Down
DROP MATERIALIZED VIEW mv.securite_dept_annee;
