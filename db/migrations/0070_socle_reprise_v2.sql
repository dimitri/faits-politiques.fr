-- +goose Up
-- Deuxième version de la reprise fiscale du socle universel
-- (docs/revenu-universel-microsimulation.md) : le seuil de reprise de la
-- version 1 (une à deux fois le SEUIL DE PAUVRETÉ, soit 1 288 à 2 576 €/UC)
-- démarrait sous le niveau de vie MÉDIAN (2 160 €/UC en 2023) — la reprise
-- touchait donc déjà la moitié la plus modeste du pays, pas « les plus
-- aisés ». Corrigé ici sur deux points :
--
--   1. les anses de la reprise sont désormais le niveau de vie MÉDIAN (0 % de
--      reprise) et deux fois le niveau de vie médian (100 % de reprise) — la
--      définition du seuil de richesse la plus citée en France (Observatoire
--      des inégalités), pas un multiple arbitraire du seuil de pauvreté ;
--   2. un PLANCHER interdit qu'un type de ménage touche, après réforme, moins
--      que ce qu'il touchait avant : le socle net d'un ménage ne peut jamais
--      descendre sous ses prestations non contributives actuelles. Sans ce
--      plancher, un ménage dont le socle est intégralement repris (au-delà de
--      deux fois le médian) perdait aussi ses anciennes prestations,
--      c'est-à-dire deux pertes pour le même ménage — un ménage ne perd
--      jamais net à cette réforme, il ne fait qu'échanger une prestation
--      contre le socle, à montant égal ou supérieur.
--
-- Les données du niveau de vie médian viennent d'une nouvelle table,
-- core.filosofi_decile_national, chargée pour elle-même : c'est la première
-- fois que la base porte une distribution nationale du niveau de vie (Insee,
-- Filosofi), un jalon vers une micro-simulation sur données individuelles
-- plutôt que sur des moyennes par type de ménage — voir
-- docs/revenu-universel-microsimulation.md § 6.
CREATE TABLE core.filosofi_decile_national (
  annee               smallint NOT NULL,
  decile              smallint NOT NULL CHECK (decile BETWEEN 1 AND 9),
  niveau_vie_mensuel   numeric NOT NULL,
  source_id           bigint NOT NULL REFERENCES raw.source(id),
  created_at          timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, decile)
);

COMMENT ON TABLE core.filosofi_decile_national IS
  'Les neuf déciles du niveau de vie annuel par UC (Insee, Filosofi, champ '
  'France, tous niveaux géographiques confondus au niveau FRANCE), convertis '
  'en euros MENSUELS au chargement. decile=5 est la médiane. Neuf points ne '
  'sont pas des données individuelles, mais c''est la distribution nationale '
  'la plus fine que l''open data permette de charger : voir '
  'docs/revenu-universel-microsimulation.md § 6 pour ce que cela permet et ne '
  'permet pas.';

DROP VIEW derived.socle_universel_simulation;

CREATE VIEW derived.socle_universel_simulation AS
WITH seuil AS (
  SELECT seuil_euros FROM core.pauvrete_seuil_annuel
   WHERE seuil_relatif = 0.6 ORDER BY annee DESC LIMIT 1
),
mediane AS (
  SELECT niveau_vie_mensuel AS m FROM core.filosofi_decile_national
   WHERE decile = 5 ORDER BY annee DESC LIMIT 1
)
SELECT d.type_menage,
       d.annee,
       e.nb_menages,
       d.uc_empirique,
       s.seuil_euros AS seuil_mensuel_uc,
       me.m AS mediane_mensuelle_uc,
       round(d.revenu_initial_menage / d.uc_empirique / me.m, 3) AS ratio_revenu_initial_mediane,
       round(least(1, greatest(0, (d.revenu_initial_menage / d.uc_empirique - me.m) / me.m)), 3) AS taux_reprise,
       round(s.seuil_euros * d.uc_empirique) AS socle_brut_menage,
       -- Le plancher : jamais moins que les prestations non contributives
       -- actuelles, même pour un ménage entièrement repris.
       greatest(
         round(s.seuil_euros * d.uc_empirique
               * (1 - least(1, greatest(0, (d.revenu_initial_menage / d.uc_empirique - me.m) / me.m)))),
         d.prestations_non_contrib_menage
       ) AS socle_net_menage,
       d.prestations_non_contrib_menage,
       round(
         greatest(
           s.seuil_euros * d.uc_empirique
             * (1 - least(1, greatest(0, (d.revenu_initial_menage / d.uc_empirique - me.m) / me.m))),
           d.prestations_non_contrib_menage
         ) - d.prestations_non_contrib_menage
       ) AS delta_mensuel_menage,
       round(e.nb_menages
             * (greatest(
                 s.seuil_euros * d.uc_empirique
                   * (1 - least(1, greatest(0, (d.revenu_initial_menage / d.uc_empirique - me.m) / me.m))),
                 d.prestations_non_contrib_menage
               ) - d.prestations_non_contrib_menage)
             * 12 / 1e9, 2) AS delta_annuel_md_euros,
       'socle-uc-v2-reprise-mediane-plancher'::text AS method_version
  FROM core.menage_type_drees d
  JOIN core.menage_type_effectif e ON e.type_menage = d.type_menage AND e.annee = d.annee
 CROSS JOIN seuil s
 CROSS JOIN mediane me
 WHERE d.type_menage <> 'ensemble';

COMMENT ON VIEW derived.socle_universel_simulation IS
  'Micro-simulation du socle universel par unité de consommation, financé par '
  'une reprise fiscale entre le niveau de vie médian et son double, avec un '
  'plancher qui interdit toute perte nette par rapport aux prestations '
  'actuelles. Version 2 (method_version socle-uc-v2-reprise-mediane-plancher) '
  ': voir docs/revenu-universel-microsimulation.md pour la version 1 et ce '
  'qui a changé.';

-- +goose Down
DROP VIEW derived.socle_universel_simulation;

CREATE VIEW derived.socle_universel_simulation AS
WITH seuil AS (
  SELECT seuil_euros FROM core.pauvrete_seuil_annuel
   WHERE seuil_relatif = 0.6 ORDER BY annee DESC LIMIT 1
)
SELECT d.type_menage,
       d.annee,
       e.nb_menages,
       d.uc_empirique,
       s.seuil_euros AS seuil_mensuel_uc,
       round(d.revenu_initial_menage / d.uc_empirique / s.seuil_euros, 3) AS ratio_revenu_initial_seuil,
       round(least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)), 3) AS taux_reprise,
       round(s.seuil_euros * d.uc_empirique) AS socle_brut_menage,
       round(s.seuil_euros * d.uc_empirique
             * (1 - least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)))) AS socle_net_menage,
       d.prestations_non_contrib_menage,
       round(s.seuil_euros * d.uc_empirique
             * (1 - least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)))
             - d.prestations_non_contrib_menage) AS delta_mensuel_menage,
       round(e.nb_menages
             * (s.seuil_euros * d.uc_empirique
                * (1 - least(1, greatest(0, d.revenu_initial_menage / d.uc_empirique / s.seuil_euros - 1)))
                - d.prestations_non_contrib_menage)
             * 12 / 1e9, 2) AS delta_annuel_md_euros,
       'socle-uc-v1-reprise-lineaire'::text AS method_version
  FROM core.menage_type_drees d
  JOIN core.menage_type_effectif e ON e.type_menage = d.type_menage AND e.annee = d.annee
 CROSS JOIN seuil s
 WHERE d.type_menage <> 'ensemble';

DROP TABLE core.filosofi_decile_national;
