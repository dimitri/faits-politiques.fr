-- +goose Up

-- Un SDIS n'est pas le département qui le finance en grande partie : c'est un
-- établissement public local distinct (categ='EPL' côté OFGL), avec son
-- propre Siren — exactement la raison d'être de core.epci (0027_epci.sql)
-- pour les groupements, reprise ici à l'identique. Sans cette table, aucune
-- carte par département n'est possible : core.collectivite_budget ne garde
-- que le Siren, jamais le département qu'il dessert.
CREATE TABLE core.sdis (
  siren             text PRIMARY KEY,
  nom               text NOT NULL,
  code_departement  text NOT NULL,
  outre_mer         boolean NOT NULL DEFAULT false,
  departement_rural boolean,
  population_totale integer,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  provenance        core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sdis_code_departement_idx ON core.sdis (code_departement);

COMMENT ON TABLE core.sdis IS
  'Un service départemental d''incendie et de secours par ligne — nom, '
  'département de rattachement, population desservie. Indépendant de '
  'tout exercice budgétaire, voir core.collectivite_budget (niveau ''SDIS'') '
  'pour les comptes. Paris et Marseille ne sont pas des SDIS (BSPP, '
  'BMPM : unités militaires, financement et commandement distincts — voir '
  'docs/securite-civile-donnees.md).';

-- Les comptes des SDIS (services départementaux d'incendie et de secours)
-- rejoignent core.collectivite_budget comme un quatrième niveau, 'SDIS' :
-- même source (OFGL, Licence Ouverte), même forme de ligne (niveau, code,
-- exercice, indicator_code, montant, euros_par_hab, population), même
-- interdiction d'additionner les niveaux entre eux (le SDIS est financé PAR
-- les départements/communes/groupements/régions dont les budgets portent
-- déjà la dépense : additionner compterait le même euro deux fois).
--
-- Code = Siren du SDIS (comme code = Siren pour un groupement), pas le code
-- département : un SDIS n'est pas le département qui le finance en grande
-- partie, c'est un établissement public local distinct (categ='EPL' côté
-- OFGL) — categorie_juridique et budget propres, conseil d'administration
-- propre. La carte par département (internal/sitegen) joint sur code_dep,
-- une colonne que core.collectivite_budget ne porte pas : au chargement, pas
-- ici.
ALTER TABLE core.collectivite_budget DROP CONSTRAINT collectivite_budget_niveau_check;
ALTER TABLE core.collectivite_budget ADD CONSTRAINT collectivite_budget_niveau_check
  CHECK (niveau IN ('REGION','DEPARTEMENT','GROUPEMENT','SDIS'));

INSERT INTO ref.indicator (code, label, unit, producer, caveat, comparable) VALUES
  ('sdis.participation_departements', 'Participation du département au SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement (hors budgets annexes : écoles '
   'départementales, restaurants, téléassistance…). Une participation '
   'votée par le conseil départemental, pas nécessairement versée au même '
   'rythme que l''exercice comptable du SDIS.', true),
  ('sdis.participation_communes', 'Participation des communes au SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement. Agrège toutes les communes du '
   'ressort du SDIS : ne distingue pas une commune précise de ses '
   'voisines.', true),
  ('sdis.participation_groupements', 'Participation des groupements à fiscalité propre (contingent incendie) au SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement. Le contingent incendie peut être '
   'porté par la commune ou transféré à son groupement : ce total ne '
   'suffit pas à savoir lequel, sans croiser avec BANATIC.', true),
  ('sdis.participation_regions', 'Participation de la région au SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement. Résiduelle pour l''essentiel des '
   'SDIS : la région n''a pas de compétence incendie et secours de '
   'principe.', true),
  ('sdis.concours_etat', 'Concours de l''État au SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement. Ne couvre pas les moyens '
   'nationaux de la sécurité civile (colonnes de renfort, flotte '
   'aérienne, formations militaires) : voir le programme budgétaire '
   '« Sécurité civile » (core.budget_programme) pour ce volet-là, financé '
   'et compté séparément.', true),
  ('sdis.frais_personnel', 'Frais de personnel du SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement. Une masse salariale globale : ne '
   'distingue pas sapeurs-pompiers professionnels, volontaires (dont '
   'l''indemnisation n''est pas un salaire) et personnels administratifs '
   'et techniques — cette répartition n''existe qu''en PDF (statistiques '
   'annuelles de la DGSCGC), pas dans ce jeu de données.', true),
  ('sdis.depenses_totales', 'Dépenses totales du SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement.', true),
  ('sdis.recettes_totales', 'Recettes totales du SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement.', true),
  ('sdis.depenses_equipement', 'Dépenses d''équipement du SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement. Casernements, véhicules, '
   'matériel : un pic une année donnée reflète souvent un investissement '
   'immobilier ponctuel, pas une tendance.', true),
  ('sdis.encours_dette', 'Encours de dette du SDIS',
   'EUR_PAR_HABITANT', 'OFGL',
   'Budget principal du SDIS uniquement.', true);

-- +goose Down
DELETE FROM ref.indicator WHERE code LIKE 'sdis.%';
ALTER TABLE core.collectivite_budget DROP CONSTRAINT collectivite_budget_niveau_check;
ALTER TABLE core.collectivite_budget ADD CONSTRAINT collectivite_budget_niveau_check
  CHECK (niveau IN ('REGION','DEPARTEMENT','GROUPEMENT'));
DROP TABLE core.sdis;
