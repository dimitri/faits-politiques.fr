-- +goose Up
-- Population par département et grande tranche d'âge (Insee, estimations de
-- population au 1er janvier) : le dénominateur qui manquait à toute carte
-- rapportant un dispositif de la vieillesse (APA à domicile, aide sociale...)
-- à la population effectivement concernée plutôt qu'à la population totale.
-- Sans cette table, un département dense en bénéficiaires de l'APA pouvait
-- n'être qu'un département où les personnes de 75 ans ou plus sont
-- proportionnellement plus nombreuses — deux choses différentes.
--
-- Sexes non distingués (colonne « Ensemble » de la source, jamais Hommes ni
-- Femmes) : aucun usage de ce dépôt ne demande la répartition par sexe, et
-- la charger sans l'exploiter serait du volume gardé pour personne. La
-- tranche « Total » de la source n'est pas non plus stockée : elle se
-- reconstruit par une somme sur les cinq tranches, jamais une colonne à
-- tenir synchronisée avec elle-même.
CREATE TABLE core.population_age_departement (
  code_departement text NOT NULL,
  annee            smallint NOT NULL,
  tranche          text NOT NULL CHECK (tranche IN ('00_19', '20_39', '40_59', '60_74', '75_PLUS')),
  population       integer NOT NULL CHECK (population >= 0),
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (code_departement, annee, tranche)
);

COMMENT ON TABLE core.population_age_departement IS
  'Insee, estimations de population au 1er janvier par département et '
  'grande classe d''âge, 1975 à l''édition la plus récente. Ensemble des '
  'deux sexes uniquement. Métropole (2A/2B pour la Corse) et cinq DROM '
  '(Mayotte inclus, à la différence de core.apa_domicile) — jamais les '
  'lignes d''agrégat « France métropolitaine », « DOM » ou « France '
  'métropolitaine et DOM » de la source, qui doubleraient les totaux si '
  'elles étaient sommées avec les départements.';

-- +goose Down
DROP TABLE core.population_age_departement;
