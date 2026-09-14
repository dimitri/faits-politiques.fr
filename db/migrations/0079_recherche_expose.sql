-- +goose Up
-- La recherche plein texte sur les exposés des motifs (core.texte_expose),
-- construite sur le même principe que jo.recherche_bloc et jo.recherche_texte
-- (0063_corpus_recherche.sql) : une VUE MATÉRIALISÉE, pas une colonne générée
-- — pg_dump n'en emporte que la définition, et PostgreSQL connaît la
-- dépendance. Voir 0063 pour la mesure qui justifie ce choix.
--
-- La configuration `fr` (public.fr, unaccent + french_stem) est la même que
-- pour le Journal officiel : une recherche « CICE » ou « competitivite »
-- doit trouver « crédit d'impôt pour la compétitivité et l'emploi » sans que
-- l'appelant ait à connaître l'accentuation exacte du texte source.
CREATE MATERIALIZED VIEW core.recherche_expose AS
  SELECT e.texte_id,
         t.titre,
         t.institution,
         t.kind,
         t.date_depot,
         to_tsvector('fr', e.integral) AS recherche
    FROM core.texte_expose e
    JOIN core.texte t ON t.id = e.texte_id;

CREATE UNIQUE INDEX recherche_expose_pk ON core.recherche_expose (texte_id);
CREATE INDEX recherche_expose_gin ON core.recherche_expose USING gin (recherche);
CREATE INDEX recherche_expose_date_idx ON core.recherche_expose (date_depot);

COMMENT ON MATERIALIZED VIEW core.recherche_expose IS
  'Vecteur de recherche des exposés des motifs (core.texte_expose), configuration '
  '`fr` (unaccent + french_stem). À rafraîchir après chaque ingestion des exposés '
  '(REFRESH MATERIALIZED VIEW CONCURRENTLY core.recherche_expose).';

-- +goose Down
DROP MATERIALIZED VIEW core.recherche_expose;
