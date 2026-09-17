-- +goose Up
-- L'appareil productif français : glissement sectoriel de l'emploi sur longue
-- période (Eurostat, comptes nationaux) et délocalisations d'emplois,
-- notamment qualifiés (Insee, étude "Les entreprises en France", 2022).

-- Emploi intérieur total par branche d'activité (NACE Rév. 2, niveau A10),
-- France, 1975-2025, en milliers de personnes — Eurostat nama_10_a10_e,
-- concept domestique (EMP_DC). Série annuelle continue, pas de rupture
-- documentée par Eurostat sur cette période.
CREATE TABLE core.emploi_secteur_nace (
  id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee            integer NOT NULL,
  code_nace        text NOT NULL,
  libelle_nace     text NOT NULL,
  emploi_milliers  numeric NOT NULL,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (annee, code_nace)
);

CREATE INDEX emploi_secteur_nace_annee_idx ON core.emploi_secteur_nace (annee);

COMMENT ON TABLE core.emploi_secteur_nace IS
  'Emploi intérieur total par branche (NACE Rév. 2, niveau A10), France, 1975-2025, '
  'milliers de personnes — Eurostat nama_10_a10_e (EMP_DC, THS_PER).';

-- Délocalisations d'emplois détectées par le modèle Insee (régression
-- logistique / forêt aléatoire / XGBoost, AUC 0.54-0.80 selon la méthode) —
-- des ESTIMATIONS avec bornes, pas un comptage administratif : trois
-- scénarios (bas/central/haut) publiés tels quels par l'Insee, jamais
-- réduits à un seul chiffre. Unités légales (1995-2017, Figure 2) et emplois
-- en équivalent temps plein (2001-2017 seulement, Figure 4) n'ont pas la
-- même couverture temporelle — colonnes nullables en conséquence.
CREATE TABLE core.delocalisation_annuelle (
  id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                  integer NOT NULL UNIQUE,
  unites_legales_bas     integer,
  unites_legales_central integer,
  unites_legales_haut    integer,
  emplois_etp_bas        integer,
  emplois_etp_central    integer,
  emplois_etp_haut       integer,
  source_id              bigint NOT NULL REFERENCES raw.source(id),
  created_at             timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.delocalisation_annuelle IS
  'Unités légales (1995-2017) et emplois ETP (2001-2017) détectés comme délocalisés '
  'chaque année par le modèle de l''Insee, trois scénarios bas/central/haut — '
  'Insee, "Les entreprises en France", éd. 2022, Figures 2 et 4.';

-- Répartition départementale du total des emplois délocalisés sur toute la
-- période 1995-2017 (Figure 6) — un cumul, pas une série annuelle par
-- département.
CREATE TABLE core.delocalisation_departement (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_departement  text NOT NULL UNIQUE,
  nom_departement   text NOT NULL,
  emplois_delocalises_1995_2017 integer NOT NULL,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  created_at        timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.delocalisation_departement IS
  'Cumul 1995-2017 des emplois délocalisés détectés par département (scénario central) — '
  'Insee, "Les entreprises en France", éd. 2022, Figure 6.';

-- Catégorie socioprofessionnelle des postes délocalisés comparée à l'emploi
-- général (Figure 7) : c'est ici que se lit la surreprésentation des postes
-- qualifiés (ingénieurs et cadres techniques, techniciens) parmi les emplois
-- délocalisés — pas seulement l'ouvrier industriel du discours courant.
CREATE TABLE core.delocalisation_csp (
  id                          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  categorie_socioprofessionnelle text NOT NULL UNIQUE,
  part_champ_general_pct     numeric NOT NULL,
  part_postes_delocalises_pct numeric NOT NULL,
  source_id                   bigint NOT NULL REFERENCES raw.source(id),
  created_at                  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.delocalisation_csp IS
  'Part de chaque catégorie socioprofessionnelle dans l''emploi général et parmi les postes '
  'délocalisés 1995-2017 — Insee, "Les entreprises en France", éd. 2022, Figure 7.';

-- +goose Down
DROP TABLE core.delocalisation_csp;
DROP TABLE core.delocalisation_departement;
DROP TABLE core.delocalisation_annuelle;
DROP TABLE core.emploi_secteur_nace;
