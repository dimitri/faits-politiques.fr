-- +goose Up
-- Combien un médecin gagne, pas seulement dans quel secteur il exerce
-- (docs/sante-donnees.md § 2 donnait la RÉPARTITION par secteur ; cette
-- table donne les MONTANTS). Source : Cnam, data.ameli.fr, jeu
-- « Professionnels de santé libéraux : montants des honoraires par
-- territoire ».
--
-- Deux pièges réels, pas des suppositions :
--
--  1. Le jeu source publie lui-même un champ compagnon *_integer qui
--     REMPLACE deux sentinelles textuelles par 0 : "NS" (non significatif,
--     secret statistique sur petit effectif, 15 % des lignes de montant) et
--     "NC" (non concerné, une profession sans effectif en secteur 2 — la
--     question du taux de dépassement ne se pose pas) sur les trois taux.
--     Les utiliser reviendrait à confondre une valeur non publiée ou sans
--     objet avec un montant ou un taux nul (docs/README.md, règle 3). Cette
--     migration ne reprend donc PAS les champs *_integer ; le connecteur
--     (internal/sante/honoraires.go) reparse la valeur littérale et écrit
--     NULL pour les deux sentinelles, en gardant la colonne nullable plutôt
--     que de coder la raison dans une seconde colonne — un montant NULL
--     suffit à déclencher la bonne question, la distinction NS/NC ne
--     conditionne aucun calcul en aval.
--  2. code_departement='999' porte DEUX niveaux d'agrégat différents selon
--     le code_region qui l'accompagne : un total RÉGIONAL (« Tout
--     département », une région par ligne) et, pour code_region='99', le
--     total NATIONAL (« FRANCE »). Sommer les départements nommés d'une
--     région redonne son propre total régional, mais sommer les dix-neuf
--     totaux régionaux ne redonnerait PAS le total France par une simple
--     addition, car code_region regroupe les DOM différemment de
--     core.remboursement_region_prestation (chaque DOM a son propre code
--     ici, quand Open Damir les regroupe tous sous un seul — encore une
--     nomenclature régionale distincte, propre à ce jeu).
CREATE TABLE core.medecin_honoraires (
  annee                          smallint NOT NULL,
  profession_sante               text NOT NULL,
  code_region                    text NOT NULL,
  libelle_region                 text NOT NULL,
  code_departement                text NOT NULL,  -- '999' = agrégat (régional ou national, voir libelle_region)
  libelle_departement            text NOT NULL,
  hono_sans_depassement_total    bigint,           -- euros, NULL = non significatif (NS) dans la source
  depassements_total             bigint,           -- euros, NULL = non significatif (NS) dans la source
  hono_sans_depassement_moyen    bigint,           -- euros, NULL = non significatif (NS) dans la source
  depassements_moyen             bigint,           -- euros, NULL = non significatif (NS) dans la source
  taux_depassement_s2            numeric,          -- part des honoraires en dépassement, secteur 2 tous régimes Optam confondus
  taux_depassement_s2_optam      numeric,
  taux_depassement_s2_non_optam  numeric,
  source_id                      bigint NOT NULL REFERENCES raw.source(id),
  created_at                     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, profession_sante, code_region, code_departement)
);

COMMENT ON TABLE core.medecin_honoraires IS
  'Cnam, montants des honoraires par territoire. code_departement=''999'' est '
  'un agrégat (régional si code_region<>''99'', national si code_region=''99'') '
  '— l''exclure avant toute somme par département. Les montants (total et moyen) valent NULL '
  'quand la source publie ''NS'' (secret statistique), jamais 0.';

-- +goose Down
DROP TABLE core.medecin_honoraires;
