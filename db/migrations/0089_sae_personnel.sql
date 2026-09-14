-- +goose Up
-- SAE (Statistique annuelle des établissements de santé), bordereau Q24 :
-- le personnel non médical PAR FONCTION (direction, soins, administratif,
-- technique...), pas seulement par statut. Rendu chargeable par la
-- disponibilité de 7-zip (les bases SAE sont distribuées en une archive .7z
-- contenant une cinquantaine de bordereaux thématiques ; Q24 est celui dont
-- les colonnes sont directement lisibles, sans dictionnaire de codes à
-- résoudre séparément — voir docs/sante-donnees.md § 4 pour les bordereaux
-- laissés de côté et pourquoi).
--
-- DISCI (discipline) est un code HIÉRARCHIQUE dans le fichier source, pas une
-- partition : chaque établissement publie une ligne par discipline PLUS une
-- ligne 9999 qui en est déjà la somme (vérifié : établissement 010000024,
-- discipline 1000 + discipline 2000 = discipline 9999, à l'ETP près). Charger
-- toutes les lignes aurait compté chaque agent deux à trois fois. Seule la
-- ligne 9999 (le total déjà calculé par la Drees) est chargée — d'où
-- l'absence de colonne « discipline » : il n'y en a plus qu'une par
-- établissement.
CREATE TABLE core.sae_personnel_fonction (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee        smallint NOT NULL,
  nofinesset   text NOT NULL,   -- rejoint ref.finess_etablissement.nofinesset (pas de FK stricte : millésimes différents)
  nofinessej   text,
  etp_direction         numeric,  -- DIREC
  etp_direction_soins   numeric,  -- DIRSOIN
  etp_administratif     numeric,  -- ADMIN
  etp_admin_technique_ouvrier numeric, -- ADMTO
  etp_cadre             numeric,  -- CADRE
  etp_infirmier         numeric,  -- INFNS (infirmiers sans spécialisation)
  etp_infirmier_specialise numeric, -- INFSP
  etp_aide_soignant     numeric,  -- AIDES
  etp_agent_service_hospitalier numeric, -- ASHAU
  etp_psychologue       numeric,  -- PSYCH
  etp_sage_femme        numeric,  -- SAGFE
  etp_reeducation       numeric,  -- REEDU
  etp_social_educatif   numeric,  -- SOITO (à confirmer précisément au dictionnaire Depp/Drees)
  etp_educateur_specialise numeric, -- EDUCS
  etp_assistant_service_social numeric, -- ASSIS
  etp_autre_educatif    numeric,  -- EDUTO
  etp_pharmacie_labo    numeric,  -- PHLAB
  etp_technique         numeric,  -- TECHN
  etp_total_pnm         numeric,  -- ETPPNM : total personnel non médical de la ligne
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sae_personnel_fonction_uniq ON core.sae_personnel_fonction (nofinesset, annee);

COMMENT ON TABLE core.sae_personnel_fonction IS
  'SAE (Drees), bordereau Q24 : personnel non médical par fonction, une ligne par '
  'établissement (le total « discipline 9999 » du fichier source, voir le commentaire '
  'ci-dessus), 2024. Les libellés de colonnes suivent le dictionnaire des variables SAE ; '
  'certains (SOITO notamment) restent une meilleure approximation faute d''avoir recoupé '
  'le dictionnaire XLSX complet ligne à ligne — à vérifier avant tout usage fin. '
  'nofinesset se rejoint à ref.finess_etablissement mais SANS contrainte de clé étrangère : '
  'les deux sources ne partagent pas le même millésime de référence.';

-- +goose Down
DROP TABLE core.sae_personnel_fonction;
