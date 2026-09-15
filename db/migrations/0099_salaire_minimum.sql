-- +goose Up
-- Chantier 11 (comparaisons internationales), premier volet : le salaire
-- minimum. Eurostat (earn_mw_cur) publie un montant mensuel semestriel pour
-- les pays européens ET les États-Unis — le seul jeu identifié à ce jour qui
-- réunit Europe et un pays du G8 hors UE dans la même nomenclature, sans
-- changer de définition en cours de route.
CREATE TABLE core.salaire_minimum (
  geo_code   text NOT NULL,
  geo_label  text NOT NULL,
  semestre   text NOT NULL,  -- ex. '2025-S1' : la source publie deux valeurs par an
  unite      text NOT NULL CHECK (unite IN ('EUR','PPS','NAC')),
  valeur     numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (geo_code, semestre, unite)
);

COMMENT ON TABLE core.salaire_minimum IS
  'Eurostat earn_mw_cur : salaire minimum mensuel brut, par pays et semestre, en euros '
  '(EUR, taux de change courant), en standard de pouvoir d''achat (PPS, corrige le '
  'niveau de prix) et en monnaie nationale (NAC, pour les pays hors zone euro). '
  'L''ABSENCE d''un pays pour un semestre n''est pas un zéro : plusieurs pays européens '
  '(Allemagne avant 2015, pays nordiques) n''ont pas de salaire minimum légal, fixé par '
  'la seule négociation collective — cette table ne le complète jamais par une '
  'estimation.';

-- +goose Down
DROP TABLE core.salaire_minimum;
