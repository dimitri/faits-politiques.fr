-- +goose Up
-- Comment les professionnels de santé libéraux sont rémunérés : le secteur
-- conventionnel (secteur 1 tarifs fixés, secteur 2 honoraires libres, avec ou
-- sans adhésion à l'Optam, non conventionné) — la question posée
-- explicitement pour les médecins, mais la source d'Ameli couvre toutes les
-- professions libérales de santé avec la même nomenclature ; chargée
-- intégralement plutôt que filtrée aux seuls médecins.
CREATE TABLE core.medecin_secteur_effectif (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee               smallint NOT NULL,
  profession_sante    text NOT NULL,
  code_region         text NOT NULL,
  libelle_region      text NOT NULL,       -- « FRANCE » = agrégat national, pas une région
  code_departement    text NOT NULL,
  libelle_departement text NOT NULL,       -- « Tout département » (code 999) = agrégat régional
  secteur_code        text NOT NULL,
  secteur_libelle     text NOT NULL,
  effectif            integer NOT NULL,
  source_id           bigint NOT NULL REFERENCES raw.source(id),
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX medecin_secteur_effectif_uniq ON core.medecin_secteur_effectif
  (annee, profession_sante, code_region, code_departement, secteur_code);
CREATE INDEX medecin_secteur_effectif_profession_idx ON core.medecin_secteur_effectif (profession_sante, annee);

COMMENT ON TABLE core.medecin_secteur_effectif IS
  'Effectifs de professionnels de santé libéraux par secteur conventionnel (Ameli/'
  'data.ameli.fr, « Démographie secteurs conventionnels »). ATTENTION agrégats mêlés aux '
  'détails, comme systématiquement dans les sources DREES/Cnam de ce dépôt : '
  'libelle_region = ''FRANCE'' est le total national, code_departement = ''999'' '
  '(libelle ''Tout département'') est le total régional — ne jamais les sommer avec les '
  'lignes détaillées sous peine de doubler les effectifs.';

-- +goose Down
DROP TABLE core.medecin_secteur_effectif;
