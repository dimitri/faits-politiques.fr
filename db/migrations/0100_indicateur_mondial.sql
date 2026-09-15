-- +goose Up
-- Chantier 11, second volet : le PIB et sa critique la plus concrète —
-- l'épargne nette ajustée de la Banque mondiale, qui retranche du revenu
-- national l'épuisement des ressources naturelles (entre autres correctifs),
-- ce que le PIB ne fait jamais. Format long (pays, indicateur, année,
-- valeur) : plusieurs indicateurs hétérogènes de la Banque mondiale, sur le
-- modèle déjà utilisé par core.prestation_solidarite et core.aide_alimentaire.
CREATE TABLE core.indicateur_mondial (
  pays_code  text NOT NULL,      -- code ISO 3166-1 alpha-2 (nomenclature Banque mondiale)
  pays_label text NOT NULL,
  indicateur text NOT NULL,      -- code Banque mondiale, ex. 'NY.GDP.MKTP.CD'
  annee      smallint NOT NULL,
  valeur     numeric,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pays_code, indicateur, annee)
);

COMMENT ON TABLE core.indicateur_mondial IS
  'Banque mondiale, indicateurs de comparaison internationale (PIB courant en dollars, '
  'épargne nette ajustée en % du RNB, épuisement des ressources naturelles en % du RNB). '
  'NY.ADJ.SVNG.GN.ZS et NY.ADJ.DRES.GN.ZS ne se somment PAS avec NY.GDP.MKTP.CD : ce sont '
  'des ratios en pourcentage du revenu national brut, pas des montants en dollars — voir '
  'docs/international-donnees.md § 4.';

-- +goose Down
DROP TABLE core.indicateur_mondial;
