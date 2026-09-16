-- +goose Up
-- Ce qu'il y a « dans » le dernier décile de niveau de vie (D10), que
-- core.filosofi_decile_national ne peut pas dire : D10 n'a pas de plafond,
-- mais le 1 % le plus aisé et le 0,1 % le plus aisé ne vivent pas la même
-- chose. Trois tables, une question chacune : combien gagne-t-on à chaque
-- seuil (revenu.md), quelle part de la masse totale ces groupes captent-ils
-- dans le temps (revenu, en série), et combien pèse le patrimoine des plus
-- hauts patrimoines. Voir docs/repartition-richesse-donnees.md.

-- ---------------------------------------------------------------------------
-- 1. Les seuils au sommet de la distribution de revenu, 2021 (Insee, fiche
--    « Très hauts revenus », Figure 1) — ce que core.filosofi_decile_national
--    (D1 à D9 seulement) ne peut pas montrer.
-- ---------------------------------------------------------------------------
CREATE TABLE core.filosofi_haut_revenu (
  annee                        smallint NOT NULL,
  seuil                        text NOT NULL CHECK (seuil IN ('D5', 'D9', 'Q99', 'Q99_9', 'Q99_99')),
  revenu_avant_redistribution_eur integer NOT NULL,
  niveau_de_vie_eur            integer NOT NULL,
  source_id                    bigint NOT NULL REFERENCES raw.source(id),
  created_at                   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, seuil)
);

COMMENT ON TABLE core.filosofi_haut_revenu IS
  'Insee-DGFiP-Cnaf-Cnav-CCMSA (Filosofi), fiche "Très hauts revenus", '
  'Insee Références "Les revenus et le patrimoine des ménages" 2024. Seuils '
  'annuels par unité de consommation (UC), France métropolitaine, ménages '
  'fiscaux à revenu déclaré positif ou nul. Q99 = 1 % les plus aisés, '
  'Q99_9 = 0,1 % les plus aisés, Q99_99 = 0,01 % les plus aisés. Millésime '
  'unique (2021) au moment du chargement — ce n''est pas une série.';

-- ---------------------------------------------------------------------------
-- 2. La part de la masse des revenus déclarés captée par chaque groupe,
--    2004-2021 (même fiche, Tableau complémentaire) — une vraie série.
-- ---------------------------------------------------------------------------
CREATE TABLE core.revenu_part_groupe (
  annee      smallint NOT NULL,
  groupe     text NOT NULL CHECK (groupe IN (
    '90_MODESTES', '9_SUIVANTS', '0_9_SUIVANTS', '0_1_PLUS_AISES', '1_PLUS_AISES'
  )),
  part_pct   numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, groupe)
);

COMMENT ON TABLE core.revenu_part_groupe IS
  'Part de la masse des revenus déclarés par unité de consommation, par '
  'groupe de personnes classées selon ce revenu (Insee, Revenus fiscaux '
  'localisés 2004-2011 puis Filosofi depuis 2012 — rupture de série '
  'documentée par la source à 2012 et 2013). Les quatre groupes '
  '90_MODESTES + 9_SUIVANTS + 0_9_SUIVANTS + 0_1_PLUS_AISES couvrent 100 % '
  'sans se recouper ; 1_PLUS_AISES (= 0_9_SUIVANTS + 0_1_PLUS_AISES) est '
  'redondant avec les deux, gardé tel que publié plutôt que recalculé.';

-- ---------------------------------------------------------------------------
-- 3. Les hauts patrimoines, 2015 et 2021 (Insee, fiche "Les hauts
--    patrimoines", Figure 1) — le pendant patrimoine du revenu ci-dessus :
--    deux notions distinctes, jamais additionnées dans ce dépôt.
-- ---------------------------------------------------------------------------
CREATE TABLE core.patrimoine_haut (
  annee              smallint NOT NULL,
  tranche            text NOT NULL CHECK (tranche IN ('P90_P95', 'P95_P99', 'SUP_P99')),
  seuil_bas_eur      integer NOT NULL,
  patrimoine_moyen_eur integer NOT NULL,
  part_masse_pct     numeric,  -- NULL en 2015 : la source ne publie la part de masse que pour 2021
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, tranche)
);

COMMENT ON TABLE core.patrimoine_haut IS
  'Insee, enquêtes Patrimoine 2014-2015 et Histoire de vie et Patrimoine '
  '2020-2021, fiche "Les hauts patrimoines", Insee Références "Les revenus '
  'et le patrimoine des ménages" 2024. Patrimoine BRUT (avant déduction des '
  'emprunts), France hors Mayotte, ménages en logement ordinaire. '
  'seuil_bas_eur : montant du centile inférieur de la tranche. '
  'part_masse_pct : NULL en 2015, la source ne la publie que pour 2021.';

-- +goose Down
DROP TABLE core.patrimoine_haut;
DROP TABLE core.revenu_part_groupe;
DROP TABLE core.filosofi_haut_revenu;
