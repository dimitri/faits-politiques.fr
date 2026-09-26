-- +goose Up

-- core.evidence porte sept colonnes « sujet » optionnelles (une seule
-- renseignée par ligne), chacune une clé étrangère ON DELETE CASCADE. Trois
-- d'entre elles (scrutin_id, amendement_id, commune_indicator_id) ont déjà un
-- index partiel ; les quatre autres n'en avaient aucun. Sans index, chaque
-- DELETE sur la table référencée (core.affiliation, core.mandate,
-- core.intervention, core.public_contract) déclenche pour chaque ligne
-- supprimée un balayage complet de core.evidence pour vérifier l'absence de
-- ligne dépendante — un coût qui grandit avec core.evidence, alors que ces
-- suppressions massives ont lieu à chaque renormalisation (constaté : 1,8 s
-- passées dans le seul déclencheur evidence_affiliation_id_fkey pour
-- 163 724 lignes d'affiliation supprimées, sur une table core.evidence
-- pourtant encore vide).
CREATE INDEX evidence_affiliation_idx ON core.evidence (affiliation_id)
  WHERE affiliation_id IS NOT NULL;
CREATE INDEX evidence_mandate_idx ON core.evidence (mandate_id)
  WHERE mandate_id IS NOT NULL;
CREATE INDEX evidence_intervention_idx ON core.evidence (intervention_id)
  WHERE intervention_id IS NOT NULL;
CREATE INDEX evidence_public_contract_idx ON core.evidence (public_contract_id)
  WHERE public_contract_id IS NOT NULL;

-- +goose Down
DROP INDEX core.evidence_affiliation_idx;
DROP INDEX core.evidence_mandate_idx;
DROP INDEX core.evidence_intervention_idx;
DROP INDEX core.evidence_public_contract_idx;
