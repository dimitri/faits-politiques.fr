-- +goose Up
-- Les prises de parole en séance publique.
--
-- La table core.intervention existait, vide, depuis le schéma initial. Elle
-- porte l'essentiel — qui, quand, quoi — mais il manquait de quoi situer une
-- intervention dans sa séance et mesurer ce qu'elle pèse.
--
-- Ce que le compte rendu de l'Assemblée apporte et qui est rare : chaque
-- paragraphe porte `id_acteur`, l'identifiant du député. Le rattachement à une
-- personne est donc un APPARIEMENT PAR IDENTIFIANT, sans la moindre homonymie
-- à arbitrer — à l'inverse du Journal officiel, où les noms sont en prose.
ALTER TABLE core.intervention
  ADD COLUMN IF NOT EXISTS legislature   text,
  ADD COLUMN IF NOT EXISTS session       text,
  ADD COLUMN IF NOT EXISTS numero_seance text,
  -- Le rang du paragraphe dans la séance : sans lui, l'ordre du débat est perdu.
  ADD COLUMN IF NOT EXISTS ordre         integer,
  -- Le rôle au moment de la parole : président de séance, orateur, rapporteur.
  ADD COLUMN IF NOT EXISTS role_debat    text,
  -- La durée de la prise de parole, en secondes, publiée par l'Assemblée.
  -- C'est la seule mesure objective du temps de parole disponible.
  ADD COLUMN IF NOT EXISTS duree_s       numeric,
  ADD COLUMN IF NOT EXISTS source_id     bigint REFERENCES raw.source(id);

CREATE INDEX IF NOT EXISTS intervention_seance_idx
  ON core.intervention (date_seance, ordre);

COMMENT ON COLUMN core.intervention.duree_s IS
  'Durée publiée par l''Assemblée. Un long temps de parole n''est pas un '
  'indicateur de travail : un président de séance parle beaucoup et ne défend '
  'rien.';

-- Le temps de parole par personne et par session. Vue et non table : elle se
-- déduit, et le calcul doit suivre toute correction du chargement.
CREATE VIEW derived.temps_de_parole AS
  SELECT i.person_id,
         i.legislature,
         i.session,
         count(*) AS interventions,
         sum(i.duree_s) AS duree_totale_s,
         count(*) FILTER (WHERE i.role_debat = 'president') AS dont_presidence,
         min(i.date_seance) AS premiere,
         max(i.date_seance) AS derniere
    FROM core.intervention i
   WHERE i.person_id IS NOT NULL
   GROUP BY 1, 2, 3;

COMMENT ON VIEW derived.temps_de_parole IS
  'Temps de parole par personne et par session. La colonne dont_presidence '
  'isole les prises de parole de conduite de séance, qui ne relèvent pas du '
  'débat et gonfleraient le total d''un président.';

-- +goose Down
DROP VIEW derived.temps_de_parole;
ALTER TABLE core.intervention
  DROP COLUMN IF EXISTS legislature, DROP COLUMN IF EXISTS session,
  DROP COLUMN IF EXISTS numero_seance, DROP COLUMN IF EXISTS ordre,
  DROP COLUMN IF EXISTS role_debat, DROP COLUMN IF EXISTS duree_s,
  DROP COLUMN IF EXISTS source_id;
