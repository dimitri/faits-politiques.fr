-- +goose Up
-- Comble un manque signalé explicitement : le second degré (Depp, déjà
-- chargé dans core.education_personnel_etablissement) ne publie qu'un
-- résidu « ETP autre » (etp_total - etp_enseignants - etp_vie_scolaire),
-- qui mélange direction, administratif et technique sans les distinguer —
-- voir docs/education-donnees.md § 3. La Depp publie en réalité
-- cette décomposition, par catégorie précise (personnels de direction,
-- d'inspection, encadrement supérieur, CPE, psychologues de l'Éducation
-- nationale, AESH, AED, filières administrative/santé-sociale/technique,
-- ITRF), dans son Panorama statistique des personnels de l'enseignement
-- scolaire — un document trouvé et vérifié directement (le fichier PDF,
-- pas de mémoire), à un niveau national (pas par établissement).
CREATE TABLE core.education_personnel_categorie (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee      smallint NOT NULL,  -- rentrée scolaire
  categorie  text NOT NULL,
  effectif   integer NOT NULL,
  etp        numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX education_personnel_categorie_uniq ON core.education_personnel_categorie (annee, categorie);

COMMENT ON TABLE core.education_personnel_categorie IS
  'Personnels non enseignants du secteur public, par catégorie précise (Depp, Panorama '
  'statistique des personnels de l''enseignement scolaire, Figures 3.1 et 3.10), France '
  'entière, rentrée 2024. Champ : personnels rémunérés au titre de l''éducation '
  'nationale, hors apprentis, en activité au 30 novembre. Les catégories ENCADREMENT_TOTAL, '
  'EDUCATION_TOTAL, ASSISTANCE_EDUCATIVE_TOTAL, VIE_SCOLAIRE_TOTAL, ASS_TOTAL et '
  'NON_ENSEIGNANTS_TOTAL sont des sous-totaux déjà calculés par la source, pas une somme '
  'refaite ici — chaque sous-total est vérifié à l''ingestion contre la somme de ses '
  'composantes, à l''arrondi près.';

-- +goose Down
DROP TABLE core.education_personnel_categorie;
