-- +goose Up
-- L'aide alimentaire : le dispositif de suivi Insee-Drees interroge six
-- réseaux nationaux (ANDES, Croix-Rouge, Fédération française des banques
-- alimentaires, Restos du Cœur, Secours catholique, Secours populaire), et
-- chacun publie des indicateurs DIFFÉRENTS (l'un compte des colis, l'autre
-- des repas, un seul les dépenses en euros, les tranches d'âge ne sont pas
-- découpées pareil d'un réseau à l'autre). Un format long (association,
-- indicateur, valeur), sur le modèle déjà utilisé par
-- core.prestation_solidarite, plutôt que forcer six réseaux hétérogènes dans
-- des colonnes fixes qui seraient vides pour la moitié d'entre eux.
CREATE TABLE core.aide_alimentaire (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  association    text NOT NULL,
  periode_type   text NOT NULL CHECK (periode_type IN ('ANNEE','TRIMESTRE','CAMPAGNE')),
  annee          smallint NOT NULL,   -- pour CAMPAGNE : année de début de la campagne
  trimestre      smallint CHECK (trimestre BETWEEN 1 AND 4),  -- NULL sauf periode_type='TRIMESTRE'
  periode_libelle text,               -- libellé exact de la source, requis pour CAMPAGNE (ex. « Décembre 2020 à Février 2021 »)
  indicateur     text NOT NULL,       -- ex. 'volume_tonnes', 'personnes_inscrites', 'depenses_aide_directe_eur'
  valeur         numeric,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX aide_alimentaire_uniq ON core.aide_alimentaire
  (association, periode_type, annee, coalesce(trimestre,0), coalesce(periode_libelle,''), indicateur);
CREATE INDEX aide_alimentaire_indicateur_idx ON core.aide_alimentaire (indicateur, annee);

COMMENT ON TABLE core.aide_alimentaire IS
  'Dispositif de suivi Insee-Drees de l''aide alimentaire, six réseaux nationaux, 2019-2021 '
  '(la dernière édition publiée par la Drees date de juillet 2021 — la série ne semble pas '
  'avoir été reconduite depuis, un fait à signaler, pas à cacher). periode_type=ANNEE, '
  '=TRIMESTRE et =CAMPAGNE ne se somment JAMAIS entre eux pour un même indicateur/association : '
  'certains totaux annuels sont mesurés directement, les trimestres correspondants sont des '
  'estimations dérivées (valeurs non entières dans la source pour plusieurs trimestres 2020) ; '
  'les Restos du Cœur (periode_type=CAMPAGNE) suivent leurs deux campagnes de distribution '
  '(hiver décembre-février, été juin-août), pas le calendrier civil des autres réseaux.';

-- +goose Down
DROP TABLE core.aide_alimentaire;
