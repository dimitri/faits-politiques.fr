-- +goose Up
-- derived.dossier_mentions_an / _an_total (0098_faits_dossiers.sql) comptent les
-- interventions de l'Assemblée qui emploient les mots de chaque dossier via
-- `i.contenu ~* t.motif` — une expression régulière quelconque par dossier
-- (72 aujourd'hui), pas un simple mot-clé : « cloud act|extraterritorial »,
-- « logiciels? libres? ». Le FTS déjà posé sur cette table
-- (intervention_fts_idx, 0034 ou proche) suppose une requête `@@
-- to_tsquery(...)`, pas une regex arbitraire — il ne sert à rien ici, et
-- convertir 72 regex en tsquery équivalentes changerait la sémantique de la
-- recherche (jointure de lexèmes normalisés/dérivés, pas correspondance de
-- sous-chaîne) pour un gain non mesuré.
--
-- MESURÉ avant/après (EXPLAIN ANALYZE, base de développement) :
--
--   sans index   balaie 260 778 interventions x 72 motifs en boucle imbriquée
--                (18,8 millions d'évaluations de regex) — 5 à 20 minutes selon
--                la charge concurrente de la base au moment de la mesure
--   avec l'index bitmap index scan sur intervention_contenu_trgm_idx, ~1 400
--                lignes candidates par motif au lieu de 260 778 — 4,9 s
--
-- pg_trgm est déjà installée (voir les migrations antérieures). Ce trigramme
-- accélère directement `~*` sans changer une ligne de dossier_terme ni de vue.
CREATE INDEX intervention_contenu_trgm_idx ON core.intervention USING gin (contenu gin_trgm_ops);

COMMENT ON INDEX core.intervention_contenu_trgm_idx IS
  'Accélère `contenu ~* motif` (derived.dossier_mentions_an, dossier_mentions_an_total) : '
  'sans lui, chaque régénération des dossiers balaie 260k interventions x 72 motifs en '
  'boucle imbriquée. Ne remplace pas intervention_fts_idx (@@ to_tsquery), qui répond à '
  'une question différente (mots-clés normalisés, pas motif regex).';

-- +goose Down
DROP INDEX core.intervention_contenu_trgm_idx;
