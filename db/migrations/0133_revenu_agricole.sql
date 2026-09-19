-- +goose Up
-- Le revenu agricole réel par actif (Eurostat, comptes économiques de
-- l'agriculture, aact_eaa06) : le revenu réel des facteurs de production en
-- agriculture par unité de travail annuel (UTA), en euros constants 2015 —
-- l'indicateur européen standard pour comparer le pouvoir d'achat agricole
-- dans le temps, pas un revenu personnel d'agriculteur individuel (un
-- agrégat national divisé par le nombre d'UTA, pas une enquête sur les
-- ménages). Vérifié directement : France 1973-2025, Union européenne
-- (agrégat 27) 2010-2025. Pour docs/agriculture-donnees.md.
CREATE TABLE core.revenu_agricole_reel (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  geo_code     text NOT NULL,
  geo_label    text NOT NULL,
  annee        int NOT NULL,
  euro_par_uta double precision NOT NULL,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (geo_code, annee)
);

COMMENT ON TABLE core.revenu_agricole_reel IS
  'Revenu réel des facteurs de production en agriculture par unité de travail '
  'annuel (UTA), euros constants 2015 (Eurostat aact_eaa06, indicateur '
  'RFI_AWU_CLV). Un agrégat macroéconomique par actif, pas un revenu personnel '
  'observé.';

-- +goose Down
DROP TABLE core.revenu_agricole_reel;
