-- +goose Up

-- mv.scrutin : miroir de core.scrutin, lu directement par huit fichiers de
-- internal/sitegen (dossiers, europe, groupes, load, main, scrutins,
-- senat, themes) pour son slug/objet/date/institution/résultat — jamais
-- une agrégation à épargner (38 042 lignes, déjà indexé pour ces mêmes
-- accès côté core), mais encore une table que « fpctl build site » ne
-- doit plus lire directement.
CREATE MATERIALIZED VIEW mv.scrutin AS
  SELECT id, slug, institution, source_uid, numero, date_seance, objet,
         type_vote, dossier_id, resultat, nb_votants, nb_pour, nb_contre, nb_abstentions
    FROM core.scrutin;

CREATE UNIQUE INDEX scrutin_pk ON mv.scrutin (id);
CREATE UNIQUE INDEX scrutin_slug_idx ON mv.scrutin (slug);
CREATE INDEX scrutin_institution_date_idx ON mv.scrutin (institution, date_seance DESC);
CREATE INDEX scrutin_dossier_idx ON mv.scrutin (dossier_id) WHERE dossier_id IS NOT NULL;

COMMENT ON MATERIALIZED VIEW mv.scrutin IS
  'Miroir de core.scrutin (colonnes lues par internal/sitegen) — jamais '
  'une agrégation, la table source est déjà de taille modeste ; le miroir '
  'sert à ce que internal/sitegen n''ait jamais à lire core directement.';

-- +goose Down
DROP MATERIALIZED VIEW mv.scrutin;
