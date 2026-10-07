-- +goose Up

-- sujet_page.go (enBref, case "sci-holding") comptait les SCI actives par
-- un COUNT(*) direct sur ref.unite_legale (13 070 047 lignes, le répertoire
-- Sirene entier) À CHAQUE CONSTRUCTION DE LA PAGE /sujets/sci-holding/ — à
-- l'encontre du principe même de ce paquet (une agrégation récurrente passe
-- par une matvue, jamais un GROUP BY/COUNT sur une table brute à la
-- construction, voir le commentaire de tête de internal/matview/matview.go,
-- déjà la leçon tirée de groupBreakdown sur core.ballot). Ce COUNT(*) sur
-- la totalité de Sirene aurait aussi obligé le périmètre CI
-- (internal/matview.TablesDirectes, voir docs/ci-pipeline.md) à exporter
-- les 13 millions de lignes de ref.unite_legale pour une seule statistique
-- — là où une matvue à une ligne suffit et reste hors du périmètre.
CREATE MATERIALIZED VIEW mv.sci_holding_actives AS
  SELECT 'sci-holding' AS cle, count(*)::bigint AS actives
    FROM ref.unite_legale
   WHERE etat_administratif = 'A' AND categorie_juridique IN ('6540', '6541');

CREATE UNIQUE INDEX sci_holding_actives_pk ON mv.sci_holding_actives (cle);

COMMENT ON MATERIALIZED VIEW mv.sci_holding_actives IS
  'Nombre de sociétés civiles immobilières actives (Insee Sirene, '
  'catégories juridiques 6540/6541) — remplace un COUNT(*) direct sur '
  'ref.unite_legale dans sujet_page.go (enBref, case "sci-holding").';

-- +goose Down
DROP MATERIALIZED VIEW mv.sci_holding_actives;
