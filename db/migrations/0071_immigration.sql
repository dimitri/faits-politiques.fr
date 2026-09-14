-- +goose Up
-- La population immigrée et la population étrangère, DEUX notions qui se
-- recoupent partiellement et que le débat public confond presque toujours.
--
-- Un IMMIGRÉ est une personne née étrangère à l'étranger et résidant en
-- France — qu'elle ait ou non acquis la nationalité française depuis son
-- arrivée. Un ÉTRANGER est une personne de nationalité étrangère résidant en
-- France, quel que soit son lieu de naissance — un enfant né en France de
-- parents étrangers est étranger, pas immigré, tant qu'il n'a pas la
-- nationalité française. En 2023, 34 % des immigrés ont la nationalité
-- française (Insee) : les deux catégories ne coïncident pas, et leurs
-- effectifs respectifs ne se comparent pas terme à terme.
--
-- Les deux classifications sont chargées côte à côte, jamais fusionnées : une
-- colonne `classification` (IMMIGRATION ou NATIONALITE) empêche qu'une requête
-- additionne un immigré français et un étranger né en France comme s'ils
-- appartenaient à la même catégorie.
CREATE TABLE core.population_statut_migratoire (
  classification text NOT NULL CHECK (classification IN ('IMMIGRATION', 'NATIONALITE')),
  -- Pour IMMIGRATION : 'IMMIGRE' ou 'NON_IMMIGRE'. Pour NATIONALITE :
  -- 'ETRANGER' ou 'FRANCAIS'. Jamais les deux vocabulaires dans la même ligne.
  categorie      text NOT NULL,
  annee          smallint NOT NULL,
  sexe           text NOT NULL CHECK (sexe IN ('HOMME', 'FEMME', 'TOTAL')),
  age_tranche    text NOT NULL,
  -- Statut d'emploi déclaré au recensement : actif occupé, chômeur, retraité,
  -- au foyer, étudiant, autre inactif, ou 'TOTAL'.
  statut_emploi  text NOT NULL,
  population     numeric NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (classification, categorie, annee, sexe, age_tranche, statut_emploi)
);

COMMENT ON TABLE core.population_statut_migratoire IS
  'Population par statut migratoire (immigré/non-immigré) OU par nationalité '
  '(étranger/français) — deux classifications distinctes, jamais à sommer '
  'ensemble — croisées avec l''âge, le sexe et le statut d''emploi. Insee, '
  'recensement de la population, diffusion API Melodi. France entière.';

-- La catégorie socioprofessionnelle, pour les 15 ans ou plus, même logique de
-- classification double.
CREATE TABLE core.population_statut_migratoire_csp (
  classification text NOT NULL CHECK (classification IN ('IMMIGRATION', 'NATIONALITE')),
  categorie      text NOT NULL,
  annee          smallint NOT NULL,
  sexe           text NOT NULL CHECK (sexe IN ('HOMME', 'FEMME', 'TOTAL')),
  -- Code de la nomenclature Insee des professions et catégories
  -- socioprofessionnelles (PCS), à un chiffre ou deux ; aucune table de
  -- référence commune n'existe encore dans ce schéma pour cette nomenclature.
  csp_code       text NOT NULL,
  csp_libelle    text NOT NULL,
  population     numeric NOT NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (classification, categorie, annee, sexe, csp_code)
);

COMMENT ON TABLE core.population_statut_migratoire_csp IS
  'Population de 15 ans ou plus par catégorie socioprofessionnelle, croisée '
  'avec le statut migratoire ou la nationalité. Mêmes réserves que '
  'core.population_statut_migratoire.';

-- Les origines géographiques : population IMMIGRÉE seulement (le pays de
-- naissance n'a pas de sens pour la classification par nationalité), par pays
-- ou zone de naissance.
CREATE TABLE core.population_immigree_origine (
  annee           smallint NOT NULL,
  sexe            text NOT NULL CHECK (sexe IN ('HOMME', 'FEMME', 'TOTAL')),
  age_tranche     text NOT NULL,
  pays_code       text NOT NULL,
  pays_libelle    text NOT NULL,
  population      numeric NOT NULL,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, sexe, age_tranche, pays_code)
);

COMMENT ON TABLE core.population_immigree_origine IS
  'Population immigrée par pays ou zone de naissance (regroupement Insee), '
  'âge et sexe. France entière. Insee, recensement de la population.';

-- La comparaison européenne harmonisée : Eurostat publie deux séries
-- distinctes, l'une par CITOYENNETÉ (l'équivalent européen de « étranger »),
-- l'autre par PAYS DE NAISSANCE (l'équivalent européen d'« immigré ») — la
-- même distinction que core.population_statut_migratoire, sous un autre nom,
-- ce qui permet de vérifier que la distinction n'est pas une particularité
-- française.
CREATE TABLE core.eurostat_population_migratoire (
  dimension  text NOT NULL CHECK (dimension IN ('CITOYENNETE', 'PAYS_NAISSANCE')),
  -- TOTAL, NATIONAL (né/citoyen du pays), UE27_AUTRE, HORS_UE27.
  categorie  text NOT NULL,
  pays       text NOT NULL,
  annee      smallint NOT NULL,
  population numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dimension, categorie, pays, annee)
);

COMMENT ON TABLE core.eurostat_population_migratoire IS
  'Population au 1er janvier par citoyenneté ou par pays de naissance '
  '(Eurostat, migr_pop1ctz et migr_pop3ctb), pour la France. Sert de '
  'comparaison harmonisée à core.population_statut_migratoire, qui vient '
  'd''une source et d''une méthode nationales différentes.';

-- Le stock de titres de séjour valides, la seule série DGEF au format
-- exploitable sans reconstruction manuelle (le détail par motif mélange
-- plusieurs tableaux et des notes de bas de page dans le même fichier CSV et
-- n'est pas chargé — voir docs/immigration-donnees.md).
CREATE TABLE core.titre_sejour_stock (
  annee      smallint NOT NULL,
  zone       text NOT NULL CHECK (zone IN ('METROPOLE', 'DOM', 'COM')),
  effectif   integer NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, zone)
);

COMMENT ON TABLE core.titre_sejour_stock IS
  'Stock de titres et documents de séjour valides au 31 décembre, par zone '
  '(DGEF/MIOM, ressortissants de pays tiers, hors Britanniques). Publication '
  'arrêtée après juin 2024 dans le catalogue data.gouv.fr consulté.';

-- +goose Down
DROP TABLE core.titre_sejour_stock;
DROP TABLE core.eurostat_population_migratoire;
DROP TABLE core.population_immigree_origine;
DROP TABLE core.population_statut_migratoire_csp;
DROP TABLE core.population_statut_migratoire;
