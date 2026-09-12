-- +goose Up
-- Thème d'un scrutin : direct quand la source le publie, hérité sinon.
--
-- Deux taxonomies officielles coexistent, posées sur deux objets différents :
--
--   EuroVoc  posé par le Parlement européen sur le SCRUTIN
--   Sénat    posé par le Sénat sur la LOI (senat_raw.loithe)
--
-- Les scrutins du Sénat ne portent aucune référence de texte dans le dump
-- Dosleg (D-016) : les thèmes du Sénat ne s'appliquent donc pas aux votes du
-- Sénat. Ils atteignent en revanche les votes de l'Assemblée par la navette,
-- et uniquement par une égalité exacte de clés publiées (D-019) :
--
--   core.scrutin (AN) -> core.dossier (AN).senat_chemin
--                     -> signet « ppl24-125 » découpé dans l'URL
--                     -> senat_raw.loi.signet -> loicod
--                     -> core.dossier (SENAT).source_uid
--                     -> core.topic_assignment
--
-- Aucun rapprochement de titres, aucune heuristique. Si le Sénat n'est pas
-- cité par l'Assemblée dans son propre dossier, il n'y a pas de thème.
--
-- Le résultat est DÉRIVÉ, jamais réinjecté dans core.topic_assignment : une
-- assignation de core est une affirmation de la source, pas un calcul. La
-- table se recalcule intégralement, et porte son method_version pour qu'un
-- chiffre publié reste rattachable à la méthode qui l'a produit.
CREATE TABLE derived.scrutin_topic (
  scrutin_id      bigint NOT NULL REFERENCES core.scrutin(id) ON DELETE CASCADE,
  topic_code      text   NOT NULL REFERENCES ref.topic(code),
  -- DIRECT  : la source a posé le thème sur ce scrutin
  -- NAVETTE : hérité de la loi du Sénat que ce scrutin fait avancer
  origine         text   NOT NULL CHECK (origine IN ('DIRECT', 'NAVETTE')),
  -- Le dossier par lequel le thème est arrivé, pour que le lecteur puisse
  -- remonter la chaîne. NULL pour un thème direct.
  via_dossier_id  bigint REFERENCES core.dossier(id) ON DELETE CASCADE,
  method_version  text   NOT NULL,
  computed_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (scrutin_id, topic_code),
  CONSTRAINT scrutin_topic_via_coherent
    CHECK ((origine = 'NAVETTE') = (via_dossier_id IS NOT NULL))
);

CREATE INDEX scrutin_topic_topic_idx ON derived.scrutin_topic (topic_code);
CREATE INDEX scrutin_topic_origine_idx ON derived.scrutin_topic (origine);

COMMENT ON TABLE derived.scrutin_topic IS
  'Thème applicable à un scrutin. Recalculable intégralement depuis core et '
  'senat_raw. Un thème hérité est celui de la LOI, pas du scrutin : deux '
  'scrutins de sens opposés sur le même texte portent le même thème.';

COMMENT ON COLUMN derived.scrutin_topic.origine IS
  'DIRECT : publié par la source sur le scrutin lui-même. '
  'NAVETTE : hérité de la loi du Sénat citée par le dossier de l''Assemblée.';

-- +goose Down
DROP TABLE derived.scrutin_topic;
