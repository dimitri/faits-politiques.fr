-- +goose Up
-- D'où viennent, aujourd'hui, les biens que la France importe dans trois
-- secteurs emblématiques de délocalisation (automobile, textile-habillement,
-- télévisions/écrans) — et comment cette origine a bougé depuis 2013. Source :
-- UN Comtrade (miroir onusien des douanes nationales), pas les douanes
-- françaises elles-mêmes : les valeurs sont en DOLLARS COURANTS (convention
-- Comtrade), pas en euros comme le reste de ce dépôt — ne jamais les
-- comparer directement à un montant en euros sans conversion explicite.
--
-- Un code HS (Harmonized System) par ligne, pas un secteur agrégé au
-- chargement : "textile-habillement" additionne les codes 61 (bonneterie) et
-- 62 (habillement autre qu'en bonneterie), tous deux chargés séparément et
-- sommés à l'affichage, jamais fusionnés dans une seule ligne qui masquerait
-- la composition. Tous les partenaires disponibles sont chargés (pas une
-- sélection de pays choisis pour appuyer un récit) : le classement se fait à
-- l'affichage, sur la donnée complète.
CREATE TABLE core.commerce_partenaire_secteur (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  secteur           text NOT NULL,
  code_hs           text NOT NULL,
  annee             integer NOT NULL,
  code_partenaire   integer NOT NULL,
  nom_partenaire    text NOT NULL,
  valeur_usd        numeric NOT NULL,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  created_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (code_hs, annee, code_partenaire)
);

CREATE INDEX commerce_partenaire_secteur_secteur_idx ON core.commerce_partenaire_secteur (secteur, annee);

COMMENT ON TABLE core.commerce_partenaire_secteur IS
  'Importations françaises par partenaire commercial, par code HS, 2013 et dernière année '
  'disponible — UN Comtrade (reporterCode 251 = France), valeurs en dollars courants. '
  'code_partenaire = 0 désigne le total mondial. Secteurs : automobile (HS 8703), '
  'textile-habillement (HS 61 + 62, à sommer), télévisions et écrans (HS 8528).';

-- +goose Down
DROP TABLE core.commerce_partenaire_secteur;
