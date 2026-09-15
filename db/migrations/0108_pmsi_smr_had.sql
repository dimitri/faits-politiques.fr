-- +goose Up
-- PMSI-SMR (Soins médicaux et de réadaptation) et PMSI-HAD (Hospitalisation à
-- domicile), les deux champs que docs/sante-donnees.md § 1.5 signalait
-- « publiés séparément par l'ATIH sur le même portail que le MCO, non
-- explorés ». Même portail (data-essentiel.atih.sante.fr), mais AUCUN des
-- deux ne partage le schéma de MCO — chaque table ci-dessous suit
-- exactement les colonnes réelles du jeu source, jamais le gabarit MCO forcé
-- dessus :
--
--  1. SMR distingue HC (hospitalisation complète) et HP (hospitalisation
--     partielle), pas HC/ambulatoire comme MCO ; HP n'a pas de notion de
--     « séjour » (nb_sejours et duree_moy_sejour valent NULL pour HP dans la
--     source elle-même, pas une valeur manquante à l'ingestion).
--  2. SMR porte une deuxième durée, la durée moyenne de PRISE EN CHARGE
--     (duree_moy_prise_charge), un concept distinct de la durée de séjour
--     que MCO ne publie pas — ne jamais les confondre.
--  3. HAD n'a aucune dimension type d'hospitalisation : une seule
--     hospitalisation à domicile, pas de sous-catégorie.
--  4. Ni SMR ni HAD ne publient de ligne 'Tous' pour âge ou sexe dans leur
--     jeu par patient (contrairement à MCO) : ces deux tables n'ont donc pas
--     de contrôle de cohérence interne du même type, seulement une
--     comparaison à `cmd/verify` que les patients par tranche ne dépassent
--     jamais le total national.
CREATE TABLE core.pmsi_smr_regional (
  annee                smallint NOT NULL,
  region               text NOT NULL,  -- inclut 'Tous' (total national)
  type_hosp            text NOT NULL,  -- 'Tous', 'HC', 'HP'
  nb_etablissements    integer NOT NULL,
  nb_patients          integer NOT NULL,
  nb_sejours           integer,        -- NULL pour HP : pas de notion de séjour
  nb_jours             integer NOT NULL,
  duree_moy_sejour     numeric,        -- NULL pour HP, même raison
  duree_moy_prise_charge numeric NOT NULL,
  source_id            bigint NOT NULL REFERENCES raw.source(id),
  created_at           timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, region, type_hosp)
);

COMMENT ON TABLE core.pmsi_smr_regional IS
  'ATIH, SMR - Chiffres clés. nb_jours s''additionne exactement HC+HP=Tous '
  '(vérifié à l''écriture) ; nb_patients NE s''additionne PAS (un patient peut '
  'cumuler HC et HP dans l''année, compté une fois dans Tous mais dans chacune '
  'des deux sous-catégories) ; nb_sejours est NULL pour HP par construction de '
  'la source, pas une donnée manquante.';

CREATE TABLE core.pmsi_smr_par_etablissement (
  annee             smallint NOT NULL,
  region            text NOT NULL,
  categ_etab        text NOT NULL,
  nb_etablissements integer NOT NULL,
  nb_jours          integer NOT NULL,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  created_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, region, categ_etab)
);

COMMENT ON TABLE core.pmsi_smr_par_etablissement IS
  'ATIH, SMR - Activité par type d''établissement. Ne publie que nb_jours : '
  'ni séjours ni durée moyenne à ce niveau de croisement, à la différence de '
  'core.pmsi_mco_par_etablissement — une limite de la source, pas de ce '
  'connecteur.';

CREATE TABLE core.pmsi_smr_par_patient (
  annee            smallint NOT NULL,
  age              text NOT NULL,
  sexe             text NOT NULL,
  type_hosp        text NOT NULL,  -- 'Tous', 'HC', 'HP'
  nb_patients      integer NOT NULL,
  nb_sejours       integer,
  nb_jours         integer NOT NULL,
  duree_moy_sejour numeric,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, age, sexe, type_hosp)
);

COMMENT ON TABLE core.pmsi_smr_par_patient IS
  'ATIH, SMR - Caractéristique des patients hospitalisés. Aucune dimension '
  'région ici (à la différence de core.pmsi_mco_par_patient) ; aucune ligne '
  '''Tous'' pour âge ou sexe dans la source.';

CREATE TABLE core.pmsi_had_regional (
  annee              smallint NOT NULL,
  region             text NOT NULL,  -- inclut 'Tous'
  nb_patients        integer NOT NULL,
  nb_sejours         integer NOT NULL,
  nb_jours           integer NOT NULL,
  duree_moyenne_sejour numeric NOT NULL,
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, region)
);

COMMENT ON TABLE core.pmsi_had_regional IS
  'ATIH, HAD - Chiffres clés. Pas de dimension type d''hospitalisation : une '
  'hospitalisation à domicile n''a pas de sous-catégorie comme MCO ou SMR.';

CREATE TABLE core.pmsi_had_par_etablissement (
  annee             smallint NOT NULL,
  region            text NOT NULL,
  categ_etab        text NOT NULL,
  nb_etablissements integer NOT NULL,
  nb_sejours        integer NOT NULL,
  nb_jours          integer NOT NULL,
  duree_moy_sejour  numeric NOT NULL,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  created_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, region, categ_etab)
);

COMMENT ON TABLE core.pmsi_had_par_etablissement IS
  'ATIH, HAD - Activité par type d''établissement.';

CREATE TABLE core.pmsi_had_par_patient (
  annee       smallint NOT NULL,
  age         text NOT NULL,
  sexe_label  text NOT NULL,  -- 'Homme'/'Femme' ; la source code aussi un entier (1/2), non repris
  nb_patients integer NOT NULL,
  nb_sejours  integer NOT NULL,
  nb_jours    integer NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, age, sexe_label)
);

COMMENT ON TABLE core.pmsi_had_par_patient IS
  'ATIH, HAD - Caractéristique des patients hospitalisés. Ne publie aucune '
  'durée de séjour à ce niveau de croisement, à la différence de SMR et MCO.';

-- +goose Down
DROP TABLE core.pmsi_had_par_patient;
DROP TABLE core.pmsi_had_par_etablissement;
DROP TABLE core.pmsi_had_regional;
DROP TABLE core.pmsi_smr_par_patient;
DROP TABLE core.pmsi_smr_par_etablissement;
DROP TABLE core.pmsi_smr_regional;
