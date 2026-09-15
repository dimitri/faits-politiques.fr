-- +goose Up
-- Le taux de pauvreté au sens Insee (core.pauvrete_seuil_annuel, seuil à 60 %
-- de la médiane) n'est comparable qu'à une mesure construite de la même façon
-- ailleurs : Eurostat publie ce même indicateur (« taux de risque de
-- pauvreté », seuil à 60 % du revenu médian équivalent), avec la même
-- définition pour tous les pays européens — la seule comparaison
-- internationale que ce projet peut afficher sans changer la définition en
-- cours de route. Elle NE couvre PAS les États-Unis, le Japon ni le Canada
-- (absents du géocode Eurostat) : voir docs/pauvrete-donnees.md § 5 pour la
-- réserve méthodologique sur ces pays (mesurés par l'OCDE à un seuil de 50 %,
-- non directement comparable).
CREATE TABLE core.pauvrete_taux_eu (
  geo_code   text NOT NULL,
  geo_label  text NOT NULL,
  annee      smallint NOT NULL,
  taux_pct   numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (geo_code, annee)
);

COMMENT ON TABLE core.pauvrete_taux_eu IS
  'Eurostat tps00184 : taux de risque de pauvreté (seuil à 60 % du revenu médian '
  'équivalent), population totale, par pays européen et année. Ne couvre pas les '
  'États-Unis, le Japon ni le Canada — absents de la nomenclature géographique '
  'Eurostat. Ne pas comparer directement à une mesure OCDE (seuil à 50 %) sans '
  'le signaler.';

-- +goose Down
DROP TABLE core.pauvrete_taux_eu;
