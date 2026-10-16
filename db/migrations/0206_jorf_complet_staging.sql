-- +goose Up
-- Zone d'atterrissage pour le chargement de la base complète du Journal
-- officiel (internal/jorf.IngestComplet, connecteur « jorf-complet »).
--
-- POURQUOI CE SCHÉMA. IngestComplet chargeait jusqu'ici directement dans
-- jo.sommaire/jo.lien/jo.texte/jo.bloc, après un TRUNCATE des quatre —
-- nécessaire parce que COPY ne sait pas fusionner, seulement ajouter, et que
-- rejouer le chargement sans vider d'abord violerait la clé primaire de
-- jo.texte/jo.sommaire dès la seconde ligne identique. Deux défauts réels à
-- ce TRUNCATE, découverts en essayant de le rejouer en octobre 2026 :
--
--   1. cinq tables hors du schéma jo portent désormais une clé étrangère vers
--      jo.texte (jo.acte_elu, core.dossier_promulgation, ref.fait_dossier,
--      ref.organisme_social, ref.juridiction_non_cooperative — aucune
--      n'existait quand ce TRUNCATE a été écrit) : Postgres refuse de vider
--      une table qu'une autre référence, sauf à vider les deux ensemble —
--      ERROR: cannot truncate a table referenced in a foreign key constraint.
--   2. même en réglant ce premier point, vider jo.texte avant de le recharger
--      depuis l'archive complète (un export figé au 13 juillet 2025, voir
--      fullArchiveURL) effacerait aussi tout ce que le connecteur « jorf »
--      (la fenêtre glissante des 60 derniers jours, lui dans la chaîne par
--      défaut, donc réellement à jour) y a déjà inséré depuis — soit, pour
--      toute base qui tourne après juillet 2025, PLUSIEURS MOIS de Journal
--      officiel réellement récent, perdus en échange d'une profondeur
--      historique qui s'arrête pile avant. Un connecteur « complet » qui
--      régresse la couverture récente de la base n'est pas complet, il est
--      juste différent.
--
-- D'où ce schéma de dépôt, sans contrainte ni clé primaire (symétrique de
-- jo.sommaire/jo.lien/jo.texte/jo.bloc, colonnes identiques) : les quatre
-- flux COPY en parallèle y écrivent exactement comme avant, puis le
-- connecteur fusionne dans les vraies tables par INSERT ... SELECT ...
-- WHERE NOT EXISTS — n'ajoute que ce qui manque, ne touche jamais une ligne
-- déjà là. Plus de TRUNCATE sur une table réelle, donc plus de conflit avec
-- une clé étrangère externe ; et la fenêtre récente du connecteur « jorf »
-- survit intacte à un chargement complet, quel que soit l'ordre dans lequel
-- les deux tournent.
CREATE SCHEMA IF NOT EXISTS jo_staging;
COMMENT ON SCHEMA jo_staging IS
  'Zone d''atterrissage jetable pour IngestComplet (jorf-complet) — vidée à '
  'chaque run, jamais lue ailleurs. Voir le commentaire de cette migration.';

CREATE TABLE jo_staging.sommaire (
  id          text,
  nature      text,
  titre       text,
  num         text,
  date_publi  date
);

CREATE TABLE jo_staging.texte (
  id            text,
  nature        text,
  num           text,
  nor           text,
  date_publi    date,
  date_texte    date,
  titre         text,
  titre_complet text,
  ministere     text,
  origine_publi text
);

CREATE TABLE jo_staging.lien (
  sommaire_id text,
  texte_id    text,
  titre       text,
  ordre       integer
);

CREATE TABLE jo_staging.bloc (
  texte_id    text,
  ordre       integer,
  section     text,
  article_id  text,
  article_num text,
  contenu     text
);

-- +goose Down
DROP SCHEMA jo_staging CASCADE;
