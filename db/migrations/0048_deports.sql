-- +goose Up
-- Les déports : quand un député se retire d'un vote pour conflit d'intérêts.
--
-- C'est une donnée rare et précieuse. Elle ne dit pas qu'il y a eu faute : elle
-- dit l'inverse — que le député a signalé lui-même un lien qui aurait pu en
-- créer un, et s'est abstenu de participer. La publier comme un soupçon serait
-- un contresens ; la taire priverait le lecteur du seul endroit où un
-- parlementaire explique publiquement un intérêt personnel, dans ses propres
-- mots.
--
-- L'Assemblée les publie depuis 2017 dans son jeu de données ouvert, avec
-- l'explication rédigée par l'intéressé.
CREATE TABLE core.deport (
  source_uid    text PRIMARY KEY,
  person_id     bigint NOT NULL REFERENCES core.person(id) ON DELETE CASCADE,
  legislature   text,
  -- L'objet dont le député se retire : un texte, une audition, un vote.
  cible_type    text,
  cible_libelle text,
  -- COMPLET : absent de tout le processus ; PARTIEL : d'une partie seulement.
  portee_code   text,
  portee_libelle text,
  instance      text,
  -- L'explication, dans les mots du député. Transcrite sans reformulation.
  explication   text,
  date_creation timestamptz,
  date_publication timestamptz,
  source_id     bigint REFERENCES raw.source(id),
  provenance    core.provenance NOT NULL DEFAULT 'OFFICIAL'
);

CREATE INDEX deport_person_idx ON core.deport (person_id);

COMMENT ON TABLE core.deport IS
  'Déclarations de déport. Un déport n''est pas un manquement : c''est sa '
  'prévention, signalée par l''intéressé. L''explication est transcrite dans '
  'ses mots, jamais résumée.';

-- Les mandats de sénateurs, que le dump Dosleg porte depuis 1936 dans une table
-- d'auteurs et que nous n'exploitions pas.
--
-- C'était le seul niveau sans historique : le Sénat ne publiait ses membres
-- qu'au présent dans ODSEN_GENERAL, et le RNE ne couvre que la mandature en
-- cours. senat_raw.auteur porte datdeb et datfin par matricule — donc un
-- appariement par identifiant, sans aucun risque d'homonymie.
--
-- Attention : 2 496 lignes sur 5 873 portent une date de début. Les autres
-- restent sans mandat plutôt que d'en recevoir un inventé.

-- +goose Down
DROP TABLE core.deport;
