-- +goose Up

-- core.commune_delinquance (5,2 millions de lignes — 34 875 communes × 15
-- indicateurs × ~10 années, voir internal/communes/ssmsi.go) était lue
-- directement par frise.go, securite.go et lieux_pages.go. Découvert en
-- fermant le périmètre CI (fpctl dump ci / dump restore / build site sur
-- une base vide, cmd/fpctl/man/fpctl-dump.md § MÉTHODE). Deux formes de
-- réduction, pour deux usages distincts :
--
-- mv.commune_delinquance_dernier : la dernière année par commune et par
-- indicateur (lieux_pages.go, même DISTINCT ON déjà écrit à la main dans
-- le fichier) — 5,2M lignes toutes années confondues réduites à ~523 000.
CREATE MATERIALIZED VIEW mv.commune_delinquance_dernier AS
  SELECT DISTINCT ON (commune_code, indicateur_code)
         commune_code, indicateur_code, annee, nombre, taux_pour_mille, diffuse
    FROM core.commune_delinquance
   ORDER BY commune_code, indicateur_code, annee DESC;

CREATE UNIQUE INDEX commune_delinquance_dernier_pk
  ON mv.commune_delinquance_dernier (commune_code, indicateur_code);

COMMENT ON MATERIALIZED VIEW mv.commune_delinquance_dernier IS
  'Dernière année publiée, par commune et par indicateur (lieux_pages.go) — '
  'remplace un DISTINCT ON rejoué sur les 5,2M lignes de core.'
  'commune_delinquance à chaque construction du site.';

-- mv.commune_delinquance_national : agrégat national par indicateur et par
-- année (frise.go — deux courbes, __securite et delinquanceParGroupe —
-- et securite.go, le compte masqué/diffusé par indicateur-année). Toutes
-- les communes confondues : 15 indicateurs × ~10 années, quelques
-- centaines de lignes au lieu de 5,2 millions. Le même sum(population)
-- FILTER (WHERE diffuse), additionné à travers plusieurs indicateurs par
-- la requête qui en lit les lignes (frise.go, cas __securite) : la même
-- sur-addition de la population qu'un sum() direct sur core.
-- commune_delinquance aurait produite, préservée à l'identique plutôt que
-- « corrigée » en silence à l'occasion de cette fermeture.
CREATE MATERIALIZED VIEW mv.commune_delinquance_national AS
  SELECT indicateur_code, annee,
         sum(nombre) FILTER (WHERE diffuse)        AS nombre_diffuse,
         sum(population) FILTER (WHERE diffuse)    AS population_diffuse,
         count(*) FILTER (WHERE diffuse)           AS n_diffuse,
         count(*) FILTER (WHERE NOT diffuse)       AS n_masque
    FROM core.commune_delinquance
   GROUP BY indicateur_code, annee;

CREATE UNIQUE INDEX commune_delinquance_national_pk
  ON mv.commune_delinquance_national (indicateur_code, annee);

COMMENT ON MATERIALIZED VIEW mv.commune_delinquance_national IS
  'Agrégat national par indicateur et par année (frise.go, securite.go) — '
  'remplace un GROUP BY rejoué sur les 5,2M lignes de core.'
  'commune_delinquance à chaque construction du site.';

-- +goose Down
DROP MATERIALIZED VIEW mv.commune_delinquance_national;
DROP MATERIALIZED VIEW mv.commune_delinquance_dernier;
