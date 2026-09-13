-- +goose Up
-- Les prestations de solidarité, mois par mois et département par département.
--
-- Pourquoi cette table à côté de core.protection_sociale : la DREES publie deux
-- objets qui ne répondent pas à la même question. Les comptes de la protection
-- sociale (1959-2024) donnent des MONTANTS annuels par risque, au niveau
-- national — ils disent combien la solidarité coûte. Ce suivi-ci donne des
-- EFFECTIFS mensuels par territoire — il dit combien de gens en dépendent, et
-- où. Les fusionner ferait perdre la géographie d'un côté, les montants de
-- l'autre.
--
-- La colonne `maturite` n'est pas décorative : la DREES publie des valeurs
-- provisoires qu'elle révise ensuite. Une page qui affiche un chiffre provisoire
-- sans le dire annonce comme acquis ce que la source présente comme estimé.
CREATE TABLE core.prestation_solidarite (
  serie        text NOT NULL,
  nom_serie    text NOT NULL,
  mois         date NOT NULL,
  -- Le même mois existe à trois échelles dans la source. Les additionner
  -- compterait chaque allocataire trois fois : le niveau est donc dans la clé,
  -- et toute requête doit en choisir un.
  niveau       text NOT NULL CHECK (niveau IN ('NATIONAL', 'REGION', 'DEPARTEMENT')),
  code_geo     text NOT NULL,
  nom_geo      text NOT NULL,
  -- numeric, pas double precision : ces valeurs sont des effectifs et des
  -- montants, et de l'argent en virgule flottante ne s'additionne pas
  -- exactement (voir docs/monnaie-et-inflation.md § 5).
  valeur       numeric NOT NULL,
  unite        text,
  maturite     text,
  commentaire  text,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  provenance   core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (serie, mois, niveau, code_geo)
);

CREATE INDEX prestation_solidarite_mois_idx ON core.prestation_solidarite (mois DESC);
CREATE INDEX prestation_solidarite_geo_idx ON core.prestation_solidarite (niveau, code_geo);

COMMENT ON TABLE core.prestation_solidarite IS
  'Effectifs mensuels d''allocataires des prestations de solidarité (RSA, AAH, '
  'prime d''activité, ASS…), par département, région et France entière. '
  'Source DREES. Les trois niveaux géographiques coexistent : ne jamais sommer '
  'sans filtrer sur `niveau`.';

COMMENT ON COLUMN core.prestation_solidarite.maturite IS
  'Ce que la DREES dit de la valeur : définitive, provisoire, estimée. Une '
  'valeur provisoire sera révisée ; l''afficher sans mention est une erreur.';

-- +goose Down
DROP TABLE core.prestation_solidarite;
