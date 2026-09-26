-- +goose Up
-- SAE (Statistique annuelle des établissements de santé), bordereau
-- URGENCES2 : le nombre de passages aux urgences (PASSU), par établissement
-- et par type d'accueil (général/pédiatrique). Vérifié directement dans
-- l'archive déjà utilisée pour le personnel (Q24, migration 0089) : la
-- colonne PASSU existe sous ce nom depuis 2013 au moins, dans la même
-- archive 7z, extraite au même passage — voir docs/sante-donnees.md § 1.8.
--
-- Un établissement porte DEUX lignes s'il a un accueil général ET
-- pédiatrique distinct (vérifié : URG='GEN' et URG='PED' sur le même FI) —
-- jamais à additionner sans regarder le type, mais jamais un doublon non
-- plus : sommer PASSU sur toutes les lignes d'une année donne le bon total
-- national (21 427 875 en 2024, vérifié cohérent avec l'ordre de grandeur
-- publié par la Drees).
CREATE TABLE core.sae_urgences_passages (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee        smallint NOT NULL,
  nofinesset   text NOT NULL,
  nofinessej   text,
  type_urgence text NOT NULL,  -- GEN (général), PED (pédiatrique), AMU (rare, code non documenté distinctement)
  passages     integer,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sae_urgences_passages_uniq ON core.sae_urgences_passages (nofinesset, annee, type_urgence);
CREATE INDEX sae_urgences_passages_annee_idx ON core.sae_urgences_passages (annee);

COMMENT ON TABLE core.sae_urgences_passages IS
  'SAE (Drees), bordereau URGENCES2 : nombre de passages aux urgences (PASSU) par '
  'établissement, par type d''accueil (général/pédiatrique) et par année, 2013-2024. '
  'Mesure le VOLUME de passages, pas le temps d''attente ni la qualité de la prise en '
  'charge (voir ref.fait_dossier pour les deux points de comparaison ponctuels de '
  'l''Enquête Urgences Drees, 2013 et 2023, seule source sur ce second point).';

-- +goose Down
DROP TABLE core.sae_urgences_passages;
