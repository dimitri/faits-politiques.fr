-- +goose Up

-- Retour en arrière sur trois matvues (0163, 0164) : de purs miroirs, sans
-- agrégation ni jointure ni filtre — core.scrutin/core.macro_value/ref.
-- macro_serie recopiés à l'identique dans mv.*. Une matvue qui ne fait que
-- ça double le stockage et le coût de REFRESH pour zéro gain : dumper la
-- table BRUTE coûte exactement la même chose que dumper son miroir. La
-- bonne réponse (voir internal/matview.TablesDirectes) est d'inclure la
-- table brute elle-même dans le périmètre d'un dump -Fc pour la CI,
-- explicitement suivie comme telle, plutôt que de la déguiser en matvue.
--
-- La distinction qui compte : une matvue se justifie par une
-- TRANSFORMATION (agrégat, jointure, filtre qui réduit ou reforme les
-- données) — pas par le seul fait qu'internal/sitegen la lise directement.
DROP MATERIALIZED VIEW mv.scrutin;
DROP MATERIALIZED VIEW mv.macro_serie;
DROP MATERIALIZED VIEW mv.macro_value;

-- +goose Down
CREATE MATERIALIZED VIEW mv.macro_value AS
  SELECT serie_code, annee, valeur, statut FROM core.macro_value;
CREATE INDEX macro_value_serie_idx ON mv.macro_value (serie_code, annee);

CREATE MATERIALIZED VIEW mv.macro_serie AS
  SELECT code, label, unite, producteur, definition, famille, cofog FROM ref.macro_serie;
CREATE UNIQUE INDEX macro_serie_pk ON mv.macro_serie (code);

CREATE MATERIALIZED VIEW mv.scrutin AS
  SELECT id, slug, institution, source_uid, numero, date_seance, objet,
         type_vote, dossier_id, resultat, nb_votants, nb_pour, nb_contre, nb_abstentions
    FROM core.scrutin;
CREATE UNIQUE INDEX scrutin_pk ON mv.scrutin (id);
CREATE UNIQUE INDEX scrutin_slug_idx ON mv.scrutin (slug);
CREATE INDEX scrutin_institution_date_idx ON mv.scrutin (institution, date_seance DESC);
CREATE INDEX scrutin_dossier_idx ON mv.scrutin (dossier_id) WHERE dossier_id IS NOT NULL;
