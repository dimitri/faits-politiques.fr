-- +goose Up
-- Catalogues : indicateurs communaux et taxonomie thématique v1.
-- Ce sont des nomenclatures, pas des données : elles vivent dans les migrations pour
-- être versionnées avec le code et diffables en revue.

INSERT INTO ref.indicator (code, label, unit, producer, formula, caveat, competence_code, comparable) VALUES

-- Contexte et variables d'appariement -----------------------------------------
('insee.population_municipale','Population municipale','NOMBRE','INSEE',
 'population légale en vigueur au 1er janvier',
 'Change au fil des recensements ; une rupture de série peut venir d''une fusion de communes.', NULL, false),
('insee.revenu_median_uc','Revenu disponible médian par unité de consommation','EUR','INSEE Filosofi',
 'médiane du revenu disponible par UC',
 'Non diffusé pour les petites communes (secret statistique) : absence ≠ valeur basse.', NULL, false),

-- Solidarité -------------------------------------------------------------------
('dgfip.f5_depenses_par_hab','Dépenses fonction 5 (interventions sociales et santé) par habitant','EUR_PAR_HABITANT','OFGL-DGFiP',
 'depenses_fonction_5 / population_municipale',
 'La ventilation fonctionnelle n''est pas obligatoire en dessous de 3 500 habitants : '
 'une valeur manquante ne signifie pas une dépense nulle.', NULL, true),
('dgfip.subvention_ccas_par_hab','Subvention communale au CCAS par habitant','EUR_PAR_HABITANT','OFGL-DGFiP',
 'subvention_versee_ccas / population_municipale',
 'Le CCAS est un établissement public autonome doté de son propre budget : ce chiffre '
 'ne mesure PAS la dépense sociale totale, seulement la part portée par la commune.', NULL, true),
('dgfip.f251_depenses_par_hab','Dépenses restauration scolaire (sous-fonction 251) par habitant','EUR_PAR_HABITANT','OFGL-DGFiP',
 'depenses_sous_fonction_251 / population_municipale',
 'Disponibilité inégale selon la nomenclature comptable appliquée par la commune.', NULL, true),
('manuel.tarif_cantine_qf_min','Tarif cantine le plus bas de la grille','EUR','Collecte manuelle',
 'montant le plus bas de la grille tarifaire en vigueur',
 'Aucune source nationale n''existe : donnée relevée à la main dans la délibération '
 'tarifaire. Couverture limitée aux communes effectivement documentées.', NULL, true),
('manuel.tarif_cantine_qf_max','Tarif cantine le plus élevé de la grille','EUR','Collecte manuelle',
 'montant le plus élevé de la grille tarifaire en vigueur',
 'Idem. Comparer deux communes suppose des grilles de structure comparable, ce qui '
 'est rarement le cas : à lire commune par commune.', NULL, false),

-- Culture ----------------------------------------------------------------------
('dgfip.f3_depenses_par_hab','Dépenses fonction 3 (culture) par habitant','EUR_PAR_HABITANT','OFGL-DGFiP',
 'depenses_fonction_3 / population_municipale',
 'Le périmètre varie selon les transferts à l''intercommunalité : vérifier la compétence '
 'avant toute comparaison.', 'CULTURE', true),
('olp.biblio_acquisition_par_hab','Budget d''acquisition des bibliothèques par habitant','EUR_PAR_HABITANT','Observatoire de la lecture publique',
 'budget_acquisition / population_municipale',
 'Enquête annuelle déclarative : la non-réponse d''une commune une année donnée crée un '
 'trou, pas un zéro.', 'LECTURE_PUBLIQUE', true),
('olp.biblio_amplitude_hebdo','Amplitude d''ouverture hebdomadaire des bibliothèques','HEURES','Observatoire de la lecture publique',
 'heures_ouverture_hebdomadaires',
 'Déclaratif. Un réseau de plusieurs équipements peut être agrégé différemment selon '
 'les années.', 'LECTURE_PUBLIQUE', true),
('olp.biblio_etp_pour_10k_hab','Effectifs des bibliothèques pour 10 000 habitants','ETP','Observatoire de la lecture publique',
 'etp_total * 10000 / population_municipale',
 'Déclaratif. Les bénévoles ne sont pas comptés de façon homogène.', 'LECTURE_PUBLIQUE', true),
('olp.biblio_inscrits_pct','Part de la population inscrite en bibliothèque','TAUX_PCT','Observatoire de la lecture publique',
 'inscrits_actifs * 100 / population_municipale',
 'Un réseau intercommunal accueille des inscrits d''autres communes : le taux peut '
 'dépasser la réalité locale.', 'LECTURE_PUBLIQUE', true),

-- Sécurité ---------------------------------------------------------------------
('dgfip.f1_depenses_par_hab','Dépenses fonction 1 (sécurité et salubrité publiques) par habitant','EUR_PAR_HABITANT','OFGL-DGFiP',
 'depenses_fonction_1 / population_municipale',
 'La fonction 1 inclut la salubrité publique, pas seulement la sécurité : elle ne mesure '
 'pas le budget de la police municipale.', NULL, true),
('decp.videoprotection_par_hab','Montants notifiés en vidéoprotection par habitant','EUR_PAR_HABITANT','DECP consolidées',
 'somme des montants notifiés sur codes CPV de vidéoprotection / population_municipale',
 'Daté à la notification du marché : un investissement pluriannuel apparaît en une seule '
 'année. Les SIRET manquants empêchent parfois le rattachement.', NULL, true),
('ssmsi.faits_pour_1000_hab','Faits enregistrés pour 1 000 habitants','POUR_1000_HABITANTS','SSMSI',
 'faits_enregistres * 1000 / population_municipale',
 'Faits ENREGISTRÉS par la police et la gendarmerie, pas faits commis. Le renforcement '
 'd''une police municipale augmente mécaniquement les constatations : une hausse peut '
 'traduire une activité accrue et non une dégradation. Série descriptive, jamais imputable '
 'à une équipe municipale.', NULL, false),

-- Finances générales -----------------------------------------------------------
('ofgl.dette_par_hab','Encours de dette par habitant','EUR_PAR_HABITANT','OFGL',
 'encours_dette_31_12 / population_municipale',
 'Le niveau dépend fortement du cycle d''investissement et de la strate : hors '
 'appariement, ce chiffre mesure surtout la taille de la commune.', NULL, true),
('ofgl.investissement_par_hab','Dépenses d''investissement par habitant','EUR_PAR_HABITANT','OFGL',
 'depenses_investissement / population_municipale',
 'Fortement cyclique : une année isolée n''a pas de sens, lire sur un mandat complet.', NULL, true),
('ofgl.fonctionnement_par_hab','Dépenses de fonctionnement par habitant','EUR_PAR_HABITANT','OFGL',
 'depenses_fonctionnement / population_municipale',
 'Le périmètre dépend des compétences conservées face à l''intercommunalité.', NULL, true),
('ofgl.masse_salariale_par_hab','Charges de personnel par habitant','EUR_PAR_HABITANT','OFGL',
 'charges_personnel / population_municipale',
 'Une externalisation fait baisser ce poste sans baisser la dépense totale.', NULL, true),
('ofgl.epargne_brute_par_hab','Épargne brute par habitant','EUR_PAR_HABITANT','OFGL',
 'epargne_brute / population_municipale', 'Sensible aux opérations exceptionnelles.', NULL, true),
('dgfip.taux_tfb','Taux de taxe foncière sur les propriétés bâties','TAUX_PCT','DGFiP-REI',
 'taux voté par la commune',
 'Le taux ne dit pas le montant payé : bases, exonérations et part intercommunale '
 'entrent dans le calcul.', NULL, true),

-- Gouvernance ------------------------------------------------------------------
('crc.observations_count','Observations des chambres régionales des comptes','NOMBRE','Cour des comptes - CRC',
 'nombre d''observations du rapport définitif, par thème',
 'Le contrôle est périodique et irrégulier. Une commune non contrôlée n''est PAS une '
 'commune sans irrégularité : l''absence de rapport est une absence de données.', NULL, false);

-- Taxonomie thématique v1 ------------------------------------------------------
-- Chaque thème porte sa définition : c'est elle qui rend une affectation contestable
-- plutôt qu'arbitraire.
INSERT INTO ref.taxonomy (version, label, published_at, frozen) VALUES
  ('v1','Taxonomie thématique v1','2026-09-11', false);

INSERT INTO ref.topic (code, taxonomy_version, label, parent_code, definition) VALUES
 ('LIBERTES','v1','Libertés individuelles et publiques',NULL,
  'Textes portant sur les libertés d''expression, de réunion, de manifestation, de la presse, '
  'la vie privée, la surveillance, ou l''étendue des pouvoirs de police.'),
 ('DROITS_FEMMES','v1','Droits des femmes',NULL,
  'Textes portant sur l''égalité femmes-hommes, les violences sexistes et sexuelles, '
  'la contraception et l''interruption volontaire de grossesse.'),
 ('FISCALITE','v1','Fiscalité et prélèvements',NULL,
  'Textes créant, supprimant ou modifiant un impôt, une taxe, une cotisation ou un crédit d''impôt.'),
 ('SOCIAL','v1','Protection sociale et droit du travail',NULL,
  'Textes portant sur les salaires et le SMIC, les retraites, l''assurance chômage, '
  'les minima sociaux et le droit du travail.'),
 ('DEFENSE','v1','Défense et forces armées',NULL,
  'Textes portant sur la programmation militaire, les effectifs, l''équipement et les opérations extérieures.'),
 ('EUROPE','v1','Union européenne',NULL,
  'Textes de transposition, de ratification, et résolutions européennes.'),
 ('INSTITUTIONS','v1','Institutions et État de droit',NULL,
  'Textes portant sur la Constitution, l''indépendance de la justice, le contrôle '
  'constitutionnel, les autorités indépendantes et les conventions internationales '
  'relatives aux droits fondamentaux.'),
 ('SECURITE','v1','Sécurité intérieure et justice pénale',NULL,
  'Textes portant sur les forces de sécurité, la procédure pénale et l''échelle des peines.');

-- +goose Down
DELETE FROM ref.topic WHERE taxonomy_version = 'v1';
DELETE FROM ref.taxonomy WHERE version = 'v1';
DELETE FROM ref.indicator WHERE code IN (
  'insee.population_municipale','insee.revenu_median_uc',
  'dgfip.f5_depenses_par_hab','dgfip.subvention_ccas_par_hab','dgfip.f251_depenses_par_hab',
  'manuel.tarif_cantine_qf_min','manuel.tarif_cantine_qf_max',
  'dgfip.f3_depenses_par_hab','olp.biblio_acquisition_par_hab','olp.biblio_amplitude_hebdo',
  'olp.biblio_etp_pour_10k_hab','olp.biblio_inscrits_pct',
  'dgfip.f1_depenses_par_hab','decp.videoprotection_par_hab','ssmsi.faits_pour_1000_hab',
  'ofgl.dette_par_hab','ofgl.investissement_par_hab','ofgl.fonctionnement_par_hab',
  'ofgl.masse_salariale_par_hab','ofgl.epargne_brute_par_hab','dgfip.taux_tfb',
  'crc.observations_count');
