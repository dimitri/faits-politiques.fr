-- +goose Up

-- core.commune_indicator (2,5 millions de lignes, toutes années et tous
-- indicateurs confondus, voir internal/communes/ofgl.go) était lue
-- directement par lieux.go et lieux_pages.go, qui n'en voulaient à chaque
-- fois que la DERNIÈRE année publiée par commune et par indicateur — un
-- DISTINCT ON déjà écrit à la main dans lieux_pages.go, rejoué à chaque
-- construction du site sur la table entière. Découvert en fermant le
-- périmètre CI (fpctl dump ci / dump restore / build site sur une base
-- vide, cmd/fpctl/man/fpctl-dump.md § MÉTHODE) : la table est trop grosse
-- pour y figurer telle quelle sans faire gonfler ce périmètre d'autant.
CREATE MATERIALIZED VIEW mv.commune_indicator_dernier AS
  SELECT DISTINCT ON (commune_code, indicator_code)
         commune_code, indicator_code, period_year, value
    FROM core.commune_indicator
   ORDER BY commune_code, indicator_code, period_year DESC;

CREATE UNIQUE INDEX commune_indicator_dernier_pk
  ON mv.commune_indicator_dernier (commune_code, indicator_code);

COMMENT ON MATERIALIZED VIEW mv.commune_indicator_dernier IS
  'Dernière année publiée, par commune et par indicateur (lieux.go, '
  'lieux_pages.go) — remplace un DISTINCT ON rejoué sur les 2,5M lignes '
  'de core.commune_indicator à chaque construction du site.';

-- +goose Down
DROP MATERIALIZED VIEW mv.commune_indicator_dernier;
