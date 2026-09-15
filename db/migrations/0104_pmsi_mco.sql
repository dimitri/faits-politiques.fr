-- +goose Up
-- PMSI-MCO (Programme de médicalisation des systèmes d'information, champ
-- Médecine-Chirurgie-Obstétrique) : l'activité hospitalière, chargée depuis
-- le portail data-essentiel de l'ATIH (Opendatasoft, ODbL) — pas le portail
-- ScanSante, interactif et sans export stable, longtemps pris à tort pour la
-- seule porte d'entrée du PMSI.
--
-- Trois tables distinctes plutôt qu'un format long : les trois jeux ATIH ont
-- chacun leur propre grille de croisement (type d'hospitalisation ;
-- région × catégorie d'établissement ; région × âge × sexe), sans dimension
-- commune qui justifierait de les fusionner.
CREATE TABLE core.pmsi_mco_national (
  annee          smallint NOT NULL,
  typ_hospit     text NOT NULL,   -- 'Tous', 'Hospitalisation complète', 'Hospitalisation ambulatoire'
  nb_patients    integer NOT NULL,
  nb_sejours     integer NOT NULL,
  nb_jours       integer NOT NULL,
  duree_moy_sejour numeric NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, typ_hospit)
);

COMMENT ON TABLE core.pmsi_mco_national IS
  'ATIH, MCO - Chiffres clés (data-essentiel.atih.sante.fr). typ_hospit=''Tous'' est '
  'la somme EXACTE de ''Hospitalisation complète'' et ''Hospitalisation ambulatoire'' '
  '(vérifié ligne à ligne à l''écriture) — ne jamais additionner les trois lignes '
  'd''une même année.';

CREATE TABLE core.pmsi_mco_par_etablissement (
  annee            smallint NOT NULL,
  region           text NOT NULL,
  categ_etab       text NOT NULL,  -- public, privé lucratif, privé à but non lucratif
  nb_etablissements integer NOT NULL,
  nb_sejours       integer NOT NULL,
  nb_jours         integer NOT NULL,
  duree_moy_sejour numeric NOT NULL,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, region, categ_etab)
);

COMMENT ON TABLE core.pmsi_mco_par_etablissement IS
  'ATIH, MCO - Activité par type d''établissement. Partition plate (aucune ligne '
  '''Tous'' région ou catégorie dans ce jeu, vérifié à l''écriture) : sommer region '
  'ou categ_etab est valide ici, à la différence de core.pmsi_mco_national.';

CREATE TABLE core.pmsi_mco_par_patient (
  annee            smallint NOT NULL,
  region           text NOT NULL,  -- inclut 'Tous'
  age              text NOT NULL,  -- tranche d'âge, inclut 'Tous'
  sexe             text NOT NULL,  -- inclut 'Tous'
  nb_patients      integer NOT NULL,
  nb_sejours       integer NOT NULL,
  nb_jours         integer NOT NULL,
  duree_moy_sejour numeric NOT NULL,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, region, age, sexe)
);

COMMENT ON TABLE core.pmsi_mco_par_patient IS
  'ATIH, MCO - Caractéristique des patients hospitalisés. region=''Tous'', age=''Tous'' '
  'et sexe=''Tous'' sont chacun des agrégats des lignes nommées de leur propre '
  'dimension — ne jamais sommer une dimension qui porte déjà sa ligne ''Tous''.';

-- +goose Down
DROP TABLE core.pmsi_mco_par_patient;
DROP TABLE core.pmsi_mco_par_etablissement;
DROP TABLE core.pmsi_mco_national;
