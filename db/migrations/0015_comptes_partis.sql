-- +goose Up
-- Comptes publics des partis (CNCCFP) et périodes des classifications tierces.

-- Catalogue des postes retenus. Les comptes CNCCFP comptent 166 colonnes ;
-- n'en exposer qu'une sélection est un choix, donc il est explicite, ordonné et
-- relié à la colonne d'origine — un lecteur peut retrouver la donnée brute.
CREATE TABLE ref.party_account_poste (
  code           text PRIMARY KEY,
  label          text NOT NULL,
  categorie      text NOT NULL CHECK (categorie IN ('PRODUIT','CHARGE','SOLDE')),
  colonne_source text NOT NULL,
  ordre          int  NOT NULL,
  caveat         text
);

INSERT INTO ref.party_account_poste (code, label, categorie, colonne_source, ordre, caveat) VALUES
 ('cotisations_adherents','Cotisations des adhérents','PRODUIT','Cotisations_des_adherents',10,NULL),
 ('cotisations_elus','Cotisations des élus','PRODUIT','Cotisations_des_elus',20,
  'Reversement par les élus d''une part de leur indemnité ; distinct d''une adhésion.'),
 ('aide_publique_1','Aide publique, 1re fraction','PRODUIT','Aide_publique_1ere_fraction',30,
  'Répartie selon les suffrages obtenus aux dernières élections législatives.'),
 ('aide_publique_2','Aide publique, 2nde fraction','PRODUIT','Aide_publique_2nde_fraction',40,
  'Répartie selon le nombre de parlementaires s''étant rattachés au parti, déclaré en novembre et publié au JO en décembre.'),
 ('dons_personnes_physiques','Dons de personnes physiques','PRODUIT','Dons_de_personne_physique',50,
  'Plafonnés par personne et par an. Les dons de personnes morales sont interdits depuis 1995.'),
 ('contributions_recues','Contributions d''autres partis','PRODUIT','Contributions_financieres_de_partis_ou_groupements_politiques',60,NULL),
 ('total_produits_activite','Total des produits d''activité','PRODUIT','Total_I',90,NULL),
 ('contributions_candidats','Contributions versées aux candidats','CHARGE','Contributions_versees_aux_candidats',110,NULL),
 ('communication','Communication, presse, publicité, réseaux sociaux','CHARGE','Communication_presse_publications_televisions_publicite_sites_internet_reseaux_sociaux',120,NULL),
 ('salaires','Salaires et traitements','CHARGE','Salaires_et_traitements',130,NULL),
 ('charges_sociales','Charges sociales','CHARGE','Charges_sociales',140,NULL),
 ('total_charges_activite','Total des charges d''activité','CHARGE','Total_II',190,NULL),
 ('total_produits','Total des produits','SOLDE','Total_des_produits_I_+_III_+_V',210,NULL),
 ('total_charges','Total des charges','SOLDE','Total_des_charges_II_+_IV_+VI_+_VII_+_VIII_+_IX',220,NULL),
 ('resultat','Excédent ou déficit d''ensemble','SOLDE','EXCEDENT_OU_DEFICIT_D_ENSEMBLE',230,NULL);

CREATE TABLE core.party_account_line (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id bigint NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  exercice        int    NOT NULL,
  poste           text   NOT NULL REFERENCES ref.party_account_poste(code),
  montant         numeric NOT NULL,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  provenance      core.provenance NOT NULL DEFAULT 'OFFICIAL',
  UNIQUE (organization_id, exercice, poste)
);
CREATE INDEX party_account_line_org_idx ON core.party_account_line (organization_id, exercice);

COMMENT ON TABLE core.party_account_line IS
  'Comptes d''ensemble déposés à la CNCCFP. Un parti absent d''un exercice n''a pas '
  'nécessairement cessé d''exister : il peut ne pas avoir déposé, ou ne pas y être tenu. '
  'L''absence est une absence de données, jamais un zéro.';

-- Les classifications tierces sont DATÉES : PopuList indique depuis quand un
-- parti relève d'une catégorie. Afficher la catégorie sans sa période
-- laisserait croire qu'elle a toujours valu.
ALTER TABLE core.party_classification
  ADD COLUMN periode_debut int,
  ADD COLUMN periode_fin   int;

COMMENT ON COLUMN core.party_classification.periode_debut IS
  'Année de début telle que publiée par le référentiel. NULL = non précisée par la source.';

-- +goose Down
ALTER TABLE core.party_classification DROP COLUMN periode_fin;
ALTER TABLE core.party_classification DROP COLUMN periode_debut;
DROP TABLE core.party_account_line;
DROP TABLE ref.party_account_poste;
