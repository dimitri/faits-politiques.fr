-- +goose Up

-- Deux clés étrangères COMPOSITES sans index sur leurs deux colonnes : seule
-- la première (scrutin_id, organization_id) était couverte, par
-- ballot_pkey/affiliation_org_idx — suffisant pour un SELECT filtré sur
-- cette seule colonne, mais pas pour que Postgres utilise pleinement
-- l'index lors de la vérification de la clé étrangère, qui porte sur LES
-- DEUX colonnes.
--
-- Mesuré en direct sur core.ballot (4,9 M lignes) : un DELETE FROM
-- core.scrutin WHERE institution = 'SENAT' (4 764 lignes, la remise à zéro
-- du connecteur du Sénat) passait 889 ms sur les 1 767 ms totaux dans le
-- seul déclencheur ballot_scrutin_id_granularite_fkey — la moitié du
-- temps de la requête. granularite y distingue deux populations réelles
-- (INDIVIDUAL / GROUP), donc l'index composite apporte une vraie
-- sélectivité en plus de scrutin_id seul.
--
-- core.ballot_group n'a PAS le même problème malgré la même forme de clé
-- étrangère : granularite y est contrainte à la seule valeur 'GROUP'
-- (ballot_group_granularite_check), un composite n'y ajouterait donc
-- aucune sélectivité sur ballot_group_pkey (scrutin_id, organization_id).
CREATE INDEX ballot_scrutin_granularite_idx ON core.ballot (scrutin_id, granularite);

-- core.affiliation.organization_kind distingue six populations réelles
-- (de 200 à 104 703 lignes selon le type d'organe) : même raisonnement,
-- affiliation_org_idx (organization_id seul) ne suffit pas à la
-- vérification de la clé étrangère composite vers core.organization(id, kind).
CREATE INDEX affiliation_org_kind_idx ON core.affiliation (organization_id, organization_kind);

-- +goose Down
DROP INDEX core.ballot_scrutin_granularite_idx;
DROP INDEX core.affiliation_org_kind_idx;
