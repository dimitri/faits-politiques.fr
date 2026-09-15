-- +goose Up
-- La branche autonomie n'a, jusqu'ici, aucune donnée dans ce dépôt — la
-- dépendance restait un angle mort du dossier santé et du dossier
-- vieillesse. L'APA à domicile (Allocation personnalisée d'autonomie) est
-- la prestation la plus proche d'une mesure directe de « ce que coûte la
-- dépendance » au niveau départemental (la DREES ne publie pas, à ce jour,
-- de série nationale en euros aussi longue pour l'APA en établissement).
--
-- Trois réserves réelles, trouvées dans le classeur source (DREES, enquête
-- Aide sociale) avant d'écrire ce connecteur :
--
--  1. RUPTURE DE PÉRIMÈTRE EN 2017. Avant 2017, le total ne couvre que la
--     « rémunération d'intervenants à domicile » ; à partir de 2017, il
--     couvre l'ensemble des dépenses (intervenants, aides diverses,
--     accueil de jour, accueil familial). Comparer 2016 à 2017 sans le
--     dire ferait passer un élargissement de périmètre pour une hausse de
--     dépense.
--  2. DONNÉES DÉCLARATIVES, PARFOIS MANQUANTES. La source elle-même
--     prévient : « données brutes [...] telles que transmises par les
--     services des conseils départementaux [...] peuvent être manquantes
--     ou partielles » (marque « ND »). Chargé en NULL, jamais en 0.
--  3. LA CORSE REPORTE EN DOUBLE. Le classeur porte à la fois un code
--     unique « 20 » (Collectivité de Corse, série complète 2010-2024) et
--     des lignes « 2A »/« 2B » incomplètes qui se recoupent avec lui sur
--     les années où les deux existent — additionner les trois donnerait un
--     compte en double. Seul le code « 20 » est chargé pour la Corse.
CREATE TABLE core.apa_domicile (
  annee                smallint NOT NULL,
  code_departement     text NOT NULL,   -- Corse : code '20' seul, voir note ci-dessus
  libelle_departement  text NOT NULL,
  nb_beneficiaires     integer,         -- payés au titre de décembre de l'année ; NULL = non publié
  depenses_total_eur   bigint,          -- NULL = non publié ('ND' dans la source)
  perimetre_depenses   text NOT NULL,   -- 'INTERVENANTS_DOMICILE' (< 2017) ou 'ENSEMBLE_APA_DOMICILE' (>= 2017)
  source_id            bigint NOT NULL REFERENCES raw.source(id),
  created_at           timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, code_departement),
  CONSTRAINT apa_domicile_perimetre_check CHECK (perimetre_depenses IN ('INTERVENANTS_DOMICILE', 'ENSEMBLE_APA_DOMICILE'))
);

COMMENT ON TABLE core.apa_domicile IS
  'DREES, enquête Aide sociale — APA à domicile, bénéficiaires et dépenses '
  'par département, 2010-2024. Hors Mayotte (hors champ de l''enquête) et '
  'hors APA en établissement (non couvert par cette série). Ne jamais '
  'sommer perimetre_depenses ''INTERVENANTS_DOMICILE'' et '
  '''ENSEMBLE_APA_DOMICILE'' dans une même série temporelle sans le dire.';

-- +goose Down
DROP TABLE core.apa_domicile;
