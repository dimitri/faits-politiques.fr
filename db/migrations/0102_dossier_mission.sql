-- +goose Up
-- Les missions budgétaires suivies par chaque dossier.
--
-- Un dossier « vertical » (justice, culture, recherche…) suit une ou plusieurs
-- missions de l'État. Le lien est déclaré dans internal/dossiers et la section
-- « Situation chiffrée » du dossier en est tirée par cmd/sections-dossiers : le
-- tableau par programme ne peut pas diverger de core.budget_programme.
--
-- Le libellé d'une mission change d'un projet de loi de finances à l'autre
-- (« Travail et emploi » en 2024, « Travail, emploi et administration des
-- ministères sociaux » en 2025) : le lien est donc un motif, pas un code.

CREATE TABLE ref.dossier_mission (
  dossier text NOT NULL,
  libelle text NOT NULL,                     -- le nom de la mission tel qu'affiché dans le dossier
  motif   text NOT NULL,                     -- expression régulière PostgreSQL ancrée sur core.budget_programme.mission_libelle
  PRIMARY KEY (dossier, libelle)
);

-- Crédits demandés par programme, pour chaque exercice chargé. Ce sont des
-- montants du PROJET de loi de finances (champ `loi`), ni la loi votée ni
-- l'exécution. Le libellé retenu est celui du dernier exercice où le programme
-- existe.
CREATE VIEW derived.dossier_budget_programme AS
SELECT m.dossier, m.libelle AS mission, b.exercice, b.loi, b.programme_code,
       (array_agg(b.programme_libelle ORDER BY b.exercice DESC))[1] AS programme_libelle,
       sum(b.autorisation_engagement) AS autorisation_engagement,
       sum(b.credit_paiement)         AS credit_paiement,
       'dossier-budget-programme-v1'::text AS method_version
FROM ref.dossier_mission m
JOIN core.budget_programme b ON b.mission_libelle ~ m.motif
GROUP BY m.dossier, m.libelle, b.exercice, b.loi, b.programme_code;

COMMENT ON VIEW derived.dossier_budget_programme IS
  'Crédits par programme des missions suivies par un dossier (projet de loi de finances, budget général et comptes spéciaux). '
  'Montants demandés, pas votés ni exécutés.';

-- +goose Down
DROP VIEW IF EXISTS derived.dossier_budget_programme;
DROP TABLE IF EXISTS ref.dossier_mission;
