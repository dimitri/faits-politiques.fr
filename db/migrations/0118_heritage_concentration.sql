-- +goose Up
-- L'héritage comme facteur d'accès à la richesse, et la comparaison directe
-- patrimoine/revenu que le § 3 de docs/repartition-richesse-donnees.md
-- promettait sans la donner : la même fiche Insee (« France, portrait
-- social » 2025, Éclairage 3) porte les deux, dans le même millésime
-- (2020-2021) que core.patrimoine_haut (migration 0117).

-- ---------------------------------------------------------------------------
-- 1. La part des ménages ayant hérité ou reçu une donation, par catégorie de
--    ménage et tranche d'âge (Figures 7a et 7b).
-- ---------------------------------------------------------------------------
CREATE TABLE core.menage_heritage (
  categorie   text NOT NULL CHECK (categorie IN (
    'HAUT_PATRIMOINE_ET_NIVEAU_VIE', 'HAUT_PATRIMOINE_SEUL', 'HAUT_NIVEAU_VIE_SEUL', 'ENSEMBLE'
  )),
  tranche_age text NOT NULL CHECK (tranche_age IN ('40_59', '60_PLUS', 'TOUS_AGES')),
  part_herite_pct   numeric NOT NULL,
  part_donation_pct numeric NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (categorie, tranche_age)
);

COMMENT ON TABLE core.menage_heritage IS
  'Insee, enquête Histoire de vie et Patrimoine 2020-2021, "France, portrait '
  'social" 2025, Éclairage 3, Figures 7a/7b. Part des ménages ayant hérité '
  '(ou reçu une donation) au moins une fois au cours de leur vie, par '
  'catégorie de ménage (haut patrimoine et/ou haut niveau de vie = les 10 % '
  'de chaque distribution) et tranche d''âge de la personne de référence. '
  'Ne mesure PAS la part de la richesse qui provient de l''héritage, '
  'seulement la part des ménages concernés — les deux questions sont '
  'différentes (voir le dossier, § 4).';

-- ---------------------------------------------------------------------------
-- 2. Concentration comparée du patrimoine et du niveau de vie, 2021
--    (Encadré Figure) — patrimoine et revenu, pour la première fois dans ce
--    dépôt, mesurés avec le même indicateur sur la même population.
-- ---------------------------------------------------------------------------
CREATE TABLE core.concentration_patrimoine_niveau_vie (
  position_distribution text NOT NULL,
  masse_patrimoine_pct  numeric NOT NULL,
  masse_niveau_vie_pct  numeric NOT NULL,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (position_distribution)
);

COMMENT ON TABLE core.concentration_patrimoine_niveau_vie IS
  'Insee, enquête Histoire de Vie et Patrimoine 2020-2021 (patrimoine) et '
  'enquête Revenus fiscaux et sociaux 2021 (niveau de vie), "France, '
  'portrait social" 2025, Éclairage 3, Encadré Figure. Part de la masse '
  'totale détenue par les ménages classés au-dessus (ou en-dessous) d''un '
  'seuil de distribution donné — patrimoine et niveau de vie classés '
  'chacun séparément, les ménages ne sont pas nécessairement les mêmes '
  'des deux côtés (voir la note de la source).';

CREATE TABLE core.gini_patrimoine_niveau_vie (
  annee                smallint NOT NULL PRIMARY KEY,
  indice_patrimoine     numeric NOT NULL,
  indice_niveau_vie     numeric NOT NULL,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.gini_patrimoine_niveau_vie IS
  'Indice de Gini du patrimoine brut et du niveau de vie, même source et '
  'même millésime que core.concentration_patrimoine_niveau_vie. 0 = '
  'égalité parfaite, 1 = concentration totale entre les mains d''un seul '
  'ménage.';

-- +goose Down
DROP TABLE core.gini_patrimoine_niveau_vie;
DROP TABLE core.concentration_patrimoine_niveau_vie;
DROP TABLE core.menage_heritage;
