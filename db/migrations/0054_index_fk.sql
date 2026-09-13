-- +goose Up
-- Trois clés étrangères sans index rendaient la renormalisation interminable.
--
-- PostgreSQL n'indexe pas automatiquement le côté RÉFÉRENÇANT d'une clé
-- étrangère. À chaque ligne supprimée dans core.amendement, il doit vérifier
-- qu'aucune ligne n'y renvoie — et sans index, cette vérification est un
-- parcours complet de la table référençante, répété autant de fois qu'il y a
-- de lignes supprimées. `DELETE FROM core.amendement` tournait encore au bout
-- de sept minutes.
--
-- Le même oubli existe partout où une table pointe un objet parlementaire
-- reconstruit à chaque normalisation.
CREATE INDEX scrutin_amendement_idx ON core.scrutin (amendement_id)
  WHERE amendement_id IS NOT NULL;
CREATE INDEX topic_assignment_amendement_idx ON core.topic_assignment (amendement_id)
  WHERE amendement_id IS NOT NULL;
CREATE INDEX dossier_item_amendement_idx ON selection.dossier_item (amendement_id)
  WHERE amendement_id IS NOT NULL;

-- Les mêmes vérifications portent sur les textes et les dossiers, effacés eux
-- aussi à chaque renormalisation de l'Assemblée.
-- core.scrutin (dossier_id, texte_id), core.amendement (texte_id, lecture_id) et
-- core.topic_assignment (dossier_id) sont déjà indexés par les migrations qui
-- ont créé ces tables. Il ne manquait que ceux-ci.
CREATE INDEX scrutin_texte_idx ON core.scrutin (texte_id) WHERE texte_id IS NOT NULL;
CREATE INDEX topic_assignment_texte_idx ON core.topic_assignment (texte_id)
  WHERE texte_id IS NOT NULL;
CREATE INDEX intervention_dossier_idx ON core.intervention (dossier_id)
  WHERE dossier_id IS NOT NULL;
CREATE INDEX lecture_dossier_idx ON core.lecture (dossier_id);

-- +goose Down
DROP INDEX core.scrutin_amendement_idx, core.topic_assignment_amendement_idx,
           selection.dossier_item_amendement_idx, core.scrutin_texte_idx,
           core.topic_assignment_texte_idx, core.intervention_dossier_idx,
           core.lecture_dossier_idx;
