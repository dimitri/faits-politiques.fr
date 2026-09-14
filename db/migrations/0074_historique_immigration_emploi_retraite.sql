-- +goose Up
-- Quatre séries chargées pour la même raison : l'ÉVOLUTION HISTORIQUE que les
-- notes de synthèse (immigration, chômage, retraite) devaient déjà montrer et
-- ne montraient qu'en partie — un stock récent (2020-2024) ne dit rien de la
-- trajectoire sur cinquante ou cent ans.

-- La série la plus longue que ce projet charge sur l'immigration : un siècle
-- de recensements. « Français par acquisition » est la mesure du STOCK de
-- personnes naturalisées à un instant donné — à ne pas confondre avec le flux
-- annuel de naturalisations de core.flux_migratoire, une autre grandeur.
CREATE TABLE core.population_historique_nationalite (
  annee                      smallint NOT NULL PRIMARY KEY,
  population_totale_milliers numeric NOT NULL,
  immigres_milliers          numeric NOT NULL,
  immigres_pct               numeric NOT NULL,
  francais_naissance_milliers   numeric NOT NULL,
  francais_acquisition_milliers numeric NOT NULL,
  etrangers_milliers         numeric NOT NULL,
  etrangers_pct              numeric NOT NULL,
  champ                      text NOT NULL, -- 'METROPOLE' 1921-1982, 'FRANCE' depuis 1990
  source_id                  bigint NOT NULL REFERENCES raw.source(id),
  created_at                 timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.population_historique_nationalite IS
  'Population immigrée, étrangère et française (de naissance / par '
  'acquisition), recensements 1921-2025 (Insee). Rupture de série à partir '
  'de 2024 (protocole de collecte du recensement revu) et changement de champ '
  'en 1990 (métropole -> France) et 2014 (Mayotte incluse) : ne pas lisser '
  'une évolution qui traverse ces trois ruptures sans le dire.';

-- Les FLUX annuels — combien de personnes immigrent, combien acquièrent la
-- nationalité française chaque année — plutôt que le seul stock à un instant
-- donné. Comparaison harmonisée Eurostat, pas Insee : les deux se recoupent
-- mais ne coïncident pas au chiffre près, même logique qu'ailleurs dans
-- docs/immigration-donnees.md.
CREATE TABLE core.flux_migratoire (
  type_flux  text NOT NULL CHECK (type_flux IN ('IMMIGRATION', 'NATURALISATION')),
  pays       text NOT NULL DEFAULT 'FR',
  annee      smallint NOT NULL,
  effectif   numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (type_flux, pays, annee)
);

COMMENT ON TABLE core.flux_migratoire IS
  'Flux annuels (pas un stock) : immigration (Eurostat migr_imm1ctz, champ '
  'total citoyennetés, France, 2006-2024) et acquisitions de la nationalité '
  'française (Eurostat migr_acq, 1998-2024).';

-- L'âge auquel on part effectivement à la retraite, année par année — la
-- mesure la plus directement lisible de l'effet des réformes successives
-- (1993, 2003, 2010, 2014, 2023).
CREATE TABLE core.age_depart_retraite (
  annee      smallint NOT NULL PRIMARY KEY,
  age_femmes numeric NOT NULL,
  age_hommes numeric NOT NULL,
  age_ensemble numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.age_depart_retraite IS
  'Âge CONJONCTUREL moyen de départ à la retraite (Drees) : un indicateur '
  'calculé sur les départs d''une seule année, comme un indice conjoncturel '
  'de fécondité — pas l''âge moyen réel des retraités d''une génération, qui '
  'ne se connaît qu''après coup.';

-- Les demandeurs d'emploi INSCRITS à France Travail, par catégorie (A, B, C,
-- ABC…) — la mesure la plus citée dans le débat public, distincte et
-- complémentaire du taux de chômage au sens du BIT
-- (core.chomage_taux_trimestriel) : une inscription administrative, pas une
-- enquête sur l'activité déclarée.
CREATE TABLE core.demandeur_emploi_categorie (
  date_mois  date    NOT NULL,
  champ      text    NOT NULL CHECK (champ IN ('FRANCE', 'FRANCE_METRO')),
  categorie  text    NOT NULL,
  effectif   numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (date_mois, champ, categorie)
);

COMMENT ON TABLE core.demandeur_emploi_categorie IS
  'Demandeurs d''emploi inscrits à France Travail (ex-Pôle emploi), en fin de '
  'mois, données CVS-CJO, par catégorie (A : sans emploi ; B, C : activité '
  'réduite courte ou longue ; D, E : non tenus de rechercher un emploi). '
  'Source Dares. Catégorie ABC = A+B+C, la mesure la plus souvent citée. '
  'Champ agrégé (sexe, âge, ancienneté, tranche d''heures = Total) : la '
  'ventilation fine existe dans la source mais n''est pas chargée ici.';

-- +goose Down
DROP TABLE core.demandeur_emploi_categorie;
DROP TABLE core.age_depart_retraite;
DROP TABLE core.flux_migratoire;
DROP TABLE core.population_historique_nationalite;
