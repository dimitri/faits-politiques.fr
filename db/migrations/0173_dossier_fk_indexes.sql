-- +goose Up

-- Deux clés étrangères vers core.dossier/core.texte sans aucun index, jugées
-- à tort sans conséquence lors du premier balayage de cette session (le
-- raisonnement s'arrêtait à la taille de la table PORTANT la clé — 4 806
-- lignes pour derived.scrutin_topic, 5 141 pour core.texte — sans compter
-- que chaque DELETE sur la table RÉFÉRENCÉE la scanne intégralement, une
-- fois par ligne supprimée : N suppressions × M lignes à vérifier, pas M
-- une fois. Mesuré en direct : DELETE FROM core.dossier (3 137 lignes,
-- renormalisation d'une législature) passait 7,6 s dans le seul déclencheur
-- scrutin_topic_via_dossier_id_fkey et 1,0 s dans texte_dossier_id_fkey, sur
-- une suppression qui ne devrait coûter que quelques dizaines de
-- millisecondes.
CREATE INDEX scrutin_topic_via_dossier_idx ON derived.scrutin_topic (via_dossier_id)
  WHERE via_dossier_id IS NOT NULL;
-- core.texte.dossier_id est NOT NULL : pas de clause partielle ici, chaque
-- ligne y a sa place.
CREATE INDEX texte_dossier_idx ON core.texte (dossier_id);

-- +goose Down
DROP INDEX derived.scrutin_topic_via_dossier_idx;
DROP INDEX core.texte_dossier_idx;
