-- +goose Up
-- InserJeunes (DEPP/ministère du Travail, data.education.gouv.fr) : le
-- devenir des apprentis après leur formation, par CFA et niveau de
-- diplôme — le volet « la formation par apprentissage mène-t-elle à un
-- emploi » du dossier jeunesse (docs/jeunesse-donnees.md).
--
-- Ce jeu ne publie AUCUN effectif par CFA, seulement des taux — impossible
-- d'en tirer un taux national pondéré par la taille réelle des CFA. Ce
-- dépôt n'en calcule donc aucun : seules des statistiques descriptives
-- (médiane, quartiles) sur les taux publiés sont exploitables, jamais une
-- « moyenne nationale » qui laisserait croire à une pondération absente.
CREATE TABLE core.insertion_apprentissage (
  annee_cumul                  text NOT NULL,  -- ex. 'cumul 2023-2024' : deux promotions consécutives, tel que publié
  uai                           text NOT NULL,  -- identifiant de l'établissement (CFA)
  libelle_etablissement         text NOT NULL,
  region                        text NOT NULL,
  niveau_formation              text NOT NULL,  -- CAP, BAC PRO, BTS, BP, MC3/4/5, autres niveaux 3/4/5
  taux_poursuite_etudes         numeric,
  taux_emploi_6_mois            numeric,
  taux_interruption_formation   numeric,
  taux_contrats_interrompus     numeric,
  taux_emploi_12_mois           numeric,
  taux_emploi_18_mois           numeric,
  taux_emploi_24_mois           numeric,
  source_id                     bigint NOT NULL REFERENCES raw.source(id),
  created_at                    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee_cumul, uai, niveau_formation)
);

COMMENT ON TABLE core.insertion_apprentissage IS
  'DEPP, enquête InserJeunes — devenir des apprentis par CFA et niveau de '
  'formation, six promotions cumulées 2018-2019 à 2023-2024. Aucun effectif '
  'publié par CFA : ne jamais calculer une moyenne nationale pondérée à '
  'partir de cette table, seules des statistiques descriptives (médiane, '
  'quartiles) sur les taux sont fondées.';

-- +goose Down
DROP TABLE core.insertion_apprentissage;
