-- +goose Up
-- Suite de 0054 : les clés étrangères non indexées qui pointent une organisation.
--
-- Même mécanique, même symptôme. `DELETE FROM core.organization` — exécuté à
-- chaque renormalisation de l'Assemblée, qui reconstruit ses groupes et ses
-- commissions — vérifie pour chaque ligne supprimée qu'aucune des six tables
-- ci-dessous n'y renvoie. Sans index, six parcours complets par ligne.
--
-- core.organization_identifier est le cas le plus coûteux : c'est la table par
-- laquelle passe tout rapprochement d'organisation, et elle n'avait d'index que
-- sur (scheme, value).
CREATE INDEX organization_identifier_org_idx ON core.organization_identifier (organization_id);
CREATE INDEX ballot_group_organization_idx ON core.ballot_group (organization_id);
CREATE INDEX amendement_author_organization_idx ON core.amendement_author (organization_id)
  WHERE organization_id IS NOT NULL;
CREATE INDEX amendement_attribution_organization_idx ON core.amendement_attribution (organization_id)
  WHERE organization_id IS NOT NULL;
CREATE INDEX dossier_author_organization_idx ON core.dossier_author (organization_id)
  WHERE organization_id IS NOT NULL;
CREATE INDEX texte_author_organization_idx ON core.texte_author (organization_id)
  WHERE organization_id IS NOT NULL;

-- +goose Down
DROP INDEX core.organization_identifier_org_idx, core.ballot_group_organization_idx,
           core.amendement_author_organization_idx, core.amendement_attribution_organization_idx,
           core.dossier_author_organization_idx, core.texte_author_organization_idx;
