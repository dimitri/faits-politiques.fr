-- +goose Up
-- Ajoute les deux bassins d'outre-mer que le catalogue BD Topage 2025 publie
-- au thème BassinHydrographique — Martinique et Mayotte, vérifié à
-- l'inspection (Guadeloupe, Guyane et Réunion n'y ont pas d'extrait à ce
-- thème, voir docs/bassins-versants-donnees.md § 9). La colonne territoire
-- distingue la métropole de l'outre-mer : la carte du § 3
-- (cmd/build/eau.go, chargerCarteBassins) calcule son cadrage par
-- st_extent sur l'ensemble de la table, et un bassin caribéen ou
-- mahorais dans le même calcul ferait exploser ce cadrage (et n'aurait de
-- toute façon aucun sens en Lambert-93, valide seulement pour la
-- métropole) — filtrée à 'metropole' pour cette carte, les deux bassins
-- d'outre-mer restent listés à part.
ALTER TABLE geo.contour_bassin ADD COLUMN territoire text NOT NULL DEFAULT 'metropole'
  CHECK (territoire IN ('metropole', 'outremer'));

COMMENT ON TABLE geo.contour_bassin IS
  'Bassins hydrographiques (BD Topage 2025, Sandre/IGN) : les 7 de France métropolitaine '
  '(les 6 comités de bassin classiques plus la Corse) et 2 d''outre-mer (Martinique, '
  'Mayotte — territoire = ''outremer''). Guadeloupe, Guyane et Réunion n''ont pas '
  'd''extrait à ce thème du catalogue Sandre au moment du chargement — absence vérifiée, '
  'pas silencieuse (voir docs/bassins-versants-donnees.md § 9). Chaque bassin d''outre-mer '
  'garde son SRID natif (srid_source) : Martinique en RGAF09/UTM20N (5490), Mayotte en '
  'RGM04/UTM38S (4471), ni l''un ni l''autre en Lambert-93 (2154) comme la métropole.';

-- +goose Down
ALTER TABLE geo.contour_bassin DROP COLUMN territoire;
