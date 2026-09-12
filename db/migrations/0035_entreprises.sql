-- +goose Up
-- Les comptes déposés des sociétés cotées.
--
-- La demande portait sur les dividendes et l'impôt sur les sociétés du CAC 40.
-- Ni l'un ni l'autre n'est disponible : les dividendes ne figurent que dans les
-- rapports annuels en PDF, société par société ; l'impôt payé est couvert par
-- le secret fiscal et n'apparaît que dans la liasse déposée, accessible par une
-- API de l'INPI qui exige un compte.
--
-- Ce qui est disponible, sous Licence Ouverte : les ratios financiers tirés par
-- l'INPI des comptes annuels déposés au greffe — chiffre d'affaires, marge
-- brute, EBE, EBIT, résultat net. 6,5 millions d'exercices, toutes sociétés
-- confondues.
--
-- LE PIÈGE, ET IL EST GRAVE : une même marque dépose parfois des comptes
-- SOCIAUX (la société mère seule) et parfois des comptes CONSOLIDÉS (le
-- groupe). TotalEnergies SE affiche 7 Md€ de chiffre d'affaires en social
-- contre environ 195 Md€ pour le groupe. Additionner ou classer des sociétés
-- dont la portée diffère produit un palmarès qui ne veut rien dire. La colonne
-- `portee` rend la distinction obligatoire à la lecture.
CREATE TABLE core.entreprise (
  siren       text PRIMARY KEY CHECK (siren ~ '^[0-9]{9}$'),
  nom         text NOT NULL,
  -- GROUPE : dépôt complet ou consolidé, les montants sont ceux du groupe.
  -- SOCIALE : comptes de la société mère seule.
  -- FILIALE_FRANCAISE : société cotée à l'étranger ; la ligne décrit sa
  --   filiale française, qui n'est PAS le groupe.
  portee      text NOT NULL CHECK (portee IN ('GROUPE','SOCIALE','FILIALE_FRANCAISE')),
  -- La preuve qui a servi à retenir ce SIREN plutôt qu'un autre. Trois
  -- résolutions automatiques ont échoué avant d'en arriver là : elle permet au
  -- lecteur de refaire la vérification au lieu de nous croire.
  verification text,
  indice      text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.entreprise IS
  'Sociétés suivies, identifiées par leur SIREN. Le rattachement d''une marque '
  'à un SIREN est une décision éditoriale vérifiée à la main (data/cac40.csv), '
  'jamais un appariement de noms.';

CREATE TABLE core.entreprise_compte (
  siren             text NOT NULL REFERENCES core.entreprise(siren) ON DELETE CASCADE,
  date_cloture      date NOT NULL,
  -- Code publié par l'INPI : K = complet ou consolidé, C et S = comptes
  -- sociaux. Conservé tel quel ; c'est lui qui dit ce que les montants couvrent.
  type_bilan        text,
  chiffre_affaires  numeric,
  marge_brute       numeric,
  ebe               numeric,
  ebit              numeric,
  resultat_net      numeric,
  -- Ce que la source appelle « confidentiality » : une société peut demander
  -- que ses comptes ne soient pas publics.
  confidentialite   text,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  provenance        core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (siren, date_cloture)
);

CREATE INDEX entreprise_compte_exercice_idx ON core.entreprise_compte (date_cloture);

COMMENT ON COLUMN core.entreprise_compte.chiffre_affaires IS
  'En euros. Ne comparer qu''entre sociétés de même portée : un chiffre social '
  'et un chiffre de groupe ne mesurent pas la même chose.';

-- +goose Down
DROP TABLE core.entreprise_compte;
DROP TABLE core.entreprise;
