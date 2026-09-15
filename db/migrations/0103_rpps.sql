-- +goose Up
-- RPPS (Répertoire partagé des professionnels de santé), Annuaire Santé de
-- l'ANS : la clé qui manquait au chantier Santé pour compter les
-- professionnels par spécialité, mode d'exercice et département, et les
-- relier aux établissements déjà chargés (ref.finess_etablissement).
--
-- Une ligne par (professionnel, activité) — un même professionnel peut
-- exercer sur plusieurs sites ou avec plusieurs rôles, donc identifiant_pp
-- N'EST PAS une clé unique de cette table ; il l'est de la personne, pas de
-- la ligne. Ne jamais compter les LIGNES comme un nombre de professionnels
-- sans dédoublonner sur identifiant_pp au préalable.
CREATE TABLE core.rpps_professionnel_activite (
  id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  identifiant_pp         text NOT NULL,
  nom                    text,
  prenom                 text,
  code_civilite          text,
  code_profession        text NOT NULL,
  libelle_profession     text NOT NULL,
  code_categorie_pro     text,
  libelle_categorie_pro  text,
  code_savoir_faire      text,   -- spécialité, quand elle existe
  libelle_savoir_faire   text,
  code_mode_exercice     text,
  libelle_mode_exercice  text,
  numero_finess_site     text,   -- pont vers ref.finess_etablissement.nofinesset, souvent absent
  code_departement       text,
  libelle_departement    text,
  code_commune           text,
  code_role              text,
  libelle_role           text,
  source_id              bigint NOT NULL REFERENCES raw.source(id),
  created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX rpps_pa_identifiant_pp_idx ON core.rpps_professionnel_activite (identifiant_pp);
CREATE INDEX rpps_pa_profession_dept_idx ON core.rpps_professionnel_activite (code_profession, code_departement);
CREATE INDEX rpps_pa_finess_idx ON core.rpps_professionnel_activite (numero_finess_site) WHERE numero_finess_site IS NOT NULL;

COMMENT ON TABLE core.rpps_professionnel_activite IS
  'Annuaire Santé (ANS), extraction RPPS en libre accès (ps-libreacces-personne-activite). '
  'Une ligne par activité déclarée, pas par professionnel : dédoublonner sur identifiant_pp '
  'avant de compter des personnes. numero_finess_site est vide pour une large part des '
  'lignes (exercice libéral hors structure) : un JOIN vers ref.finess_etablissement ne '
  'couvrira jamais l''ensemble des professionnels.';

-- +goose Down
DROP TABLE core.rpps_professionnel_activite;
