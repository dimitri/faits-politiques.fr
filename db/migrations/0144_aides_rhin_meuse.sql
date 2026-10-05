-- +goose Up
-- Ajoute Rhin-Meuse aux agences de l'eau chargées (§ 5 du dossier eau) : un
-- fichier consolidé unique, publié par l'agence elle-même
-- (eau-rhin-meuse.fr/upload/Bilan_Aides_AERM.xlsx), couvrant 2000-2026 sans
-- fragmentation par programme. À la différence de Loire-Bretagne et
-- Artois-Picardie, ce fichier porte aussi un type de bénéficiaire
-- (collectivité, entreprise, particulier...) — d'où la nouvelle colonne
-- type_beneficiaire, laissée nulle pour les agences qui ne publient pas
-- cette information plutôt que déduite.
ALTER TABLE core.aide_agence_eau DROP CONSTRAINT aide_agence_eau_agence_check;
ALTER TABLE core.aide_agence_eau ADD CONSTRAINT aide_agence_eau_agence_check
  CHECK (agence IN ('LOIRE_BRETAGNE', 'ARTOIS_PICARDIE', 'RHIN_MEUSE'));

ALTER TABLE core.aide_agence_eau ADD COLUMN type_beneficiaire text;

COMMENT ON TABLE core.aide_agence_eau IS
  'Décisions d''aide accordées par les agences de l''eau, une ligne par aide. '
  'Périmètre partiel et assumé : Loire-Bretagne (programmes 11 et 12, '
  '2019-2030), Artois-Picardie (conventions au format décret n° 2017-779, '
  '2017-2026) et Rhin-Meuse (bilan consolidé de l''agence, 2000-2026). '
  'Adour-Garonne, Rhône-Méditerranée-Corse et Seine-Normandie ne sont pas '
  'chargées (portails de recherche par critères ou fichiers fragmentés/'
  'obsolètes selon le cas, voir docs/bassins-versants-donnees.md § 9), pas '
  'plus que le 10e programme Loire-Bretagne (2013-2018, format hétérogène '
  'par millésime). date_decision reste du texte : les formats de date '
  'diffèrent selon la source et le programme, jamais normalisés au prix '
  'd''une supposition. type_beneficiaire n''est renseigné que pour les '
  'sources qui le publient (Rhin-Meuse) : NULL ailleurs, jamais déduit.';

-- +goose Down
ALTER TABLE core.aide_agence_eau DROP COLUMN type_beneficiaire;
ALTER TABLE core.aide_agence_eau DROP CONSTRAINT aide_agence_eau_agence_check;
ALTER TABLE core.aide_agence_eau ADD CONSTRAINT aide_agence_eau_agence_check
  CHECK (agence IN ('LOIRE_BRETAGNE', 'ARTOIS_PICARDIE'));
