-- +goose Up
-- Le budget de l'État par mission/programme/action, tel que voté au PLF —
-- la nomenclature LOLF (loi organique relative aux lois de finances, 2001).
-- Générique par construction : ce même jeu de données couvre Défense
-- (mission « Défense »), Sécurités (Police nationale, Gendarmerie nationale,
-- Sécurité civile, Sécurité et éducation routières), et plus tard Éducation
-- nationale (« Enseignement scolaire ») et Pouvoirs publics (chantiers
-- suivants) — une seule table plutôt qu'une par mission.
--
-- Limite posée dès l'ouverture : ce sont des montants VOTÉS au PLF (projet),
-- pas la loi de finances initiale adoptée ni l'exécution réelle — le même
-- piège voté/exécuté déjà documenté dans docs/budget-donnees.md § 2. Le champ
-- `loi` porte cette distinction pour ne jamais la perdre en aval.
CREATE TABLE core.budget_programme (
  id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  exercice               smallint NOT NULL,
  loi                    text NOT NULL,               -- 'PLF' pour l'instant ; LFI/exécution à ajouter séparément
  type_budget            text NOT NULL,                -- BG (budget général), CAS, BA, CCF
  ministere              text,
  mission_code           text NOT NULL,
  mission_libelle        text NOT NULL,
  programme_code         text NOT NULL,
  programme_libelle      text NOT NULL,
  action_code            text,
  action_libelle         text,
  sous_action_code       text,
  sous_action_libelle    text,
  categorie              smallint,
  titre                  smallint NOT NULL CHECK (titre BETWEEN 1 AND 7),
  autorisation_engagement numeric,
  credit_paiement        numeric,
  source_id              bigint NOT NULL REFERENCES raw.source(id),
  created_at             timestamptz NOT NULL DEFAULT now()
);

-- Deux exercices ont deux schémas source différents (voir le connecteur) mais
-- une seule clé naturelle : la ligne la plus fine que le PLF publie.
CREATE UNIQUE INDEX budget_programme_uniq ON core.budget_programme
  (exercice, loi, type_budget, mission_code, programme_code,
   coalesce(action_code,''), coalesce(sous_action_code,''), coalesce(categorie,-1), titre);

CREATE INDEX budget_programme_mission_idx ON core.budget_programme (mission_libelle, exercice);

COMMENT ON TABLE core.budget_programme IS
  'Dépenses budgétaires votées par mission/programme/action/titre (nomenclature LOLF), '
  'PLF 2024 et 2025 — source data.economie.gouv.fr. « titre » est la nature de la '
  'dépense (2 = personnel, 3 = fonctionnement, 5 = investissement, 6 = intervention, '
  '7 = opérations financières) : c''est ce qui permet de distinguer masse salariale '
  'et investissement au sein d''une même mission.';

-- +goose Down
DROP TABLE core.budget_programme;
