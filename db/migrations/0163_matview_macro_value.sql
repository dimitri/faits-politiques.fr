-- +goose Up

-- mv.macro_value / mv.macro_serie : deux petites tables de référence
-- (1 988 et 61 lignes) lues DIRECTEMENT par onze fichiers de internal/
-- sitegen (budget, canaux, chomage, dette, dividendes, emploi, fonctions,
-- frise, sujet_page, sujets, vieillesse), chacun filtrant par son propre
-- serie_code. Pas une agrégation à épargner (les tables sont minuscules) :
-- un simple miroir, motivé par le nombre de consommateurs qui liraient
-- sinon core/ref directement — ce que « fpctl build site » ne doit plus
-- jamais faire, pour que le schéma mv seul suffise à reconstruire le site
-- (voir le dump -Fc de mv envisagé pour la CI).
CREATE MATERIALIZED VIEW mv.macro_value AS
  SELECT serie_code, annee, valeur, statut FROM core.macro_value;

CREATE INDEX macro_value_serie_idx ON mv.macro_value (serie_code, annee);

COMMENT ON MATERIALIZED VIEW mv.macro_value IS
  'Miroir de core.macro_value, lu directement par onze fichiers de '
  'internal/sitegen (budget, canaux, chomage, dette, dividendes, emploi, '
  'fonctions, frise, sujet_page, sujets, vieillesse) — jamais une '
  'agrégation, la table source est déjà minuscule (1 988 lignes) ; le '
  'miroir sert à ce que internal/sitegen n''ait jamais à lire core '
  'directement.';

CREATE MATERIALIZED VIEW mv.macro_serie AS
  SELECT code, label, unite, producteur, definition, famille, cofog FROM ref.macro_serie;

CREATE UNIQUE INDEX macro_serie_pk ON mv.macro_serie (code);

COMMENT ON MATERIALIZED VIEW mv.macro_serie IS
  'Miroir de ref.macro_serie (61 lignes), même raison que mv.macro_value.';

-- +goose Down
DROP MATERIALIZED VIEW mv.macro_serie;
DROP MATERIALIZED VIEW mv.macro_value;
