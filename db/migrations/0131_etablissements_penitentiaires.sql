-- +goose Up
-- La population détenue par établissement pénitentiaire (et par quartier
-- au sein d'un même établissement — un centre pénitentiaire peut avoir
-- plusieurs quartiers de régimes différents, chacun sa propre capacité et
-- densité). Source : ministère de la Justice (GENESIS/DGAP), statistique
-- mensuelle « Répartition des personnes détenues par établissement »,
-- publiée par direction interrégionale — dix classeurs consolidés dans un
-- même fichier mensuel, vérifié directement (structure et colonnes
-- inspectées à l'exécution, pas supposées).
--
-- Limite documentée : aucun identifiant officiel (SIRET ou équivalent) ni
-- coordonnées géographiques ne sont publiés dans ce fichier — seulement un
-- nom d'établissement. Une carte demanderait un géocodage externe par nom
-- et commune, non fait ici ; voir docs/justice-donnees.md pour le détail.
CREATE TABLE core.etablissement_penitentiaire (
  id                       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  etablissement            text NOT NULL,
  quartier                 text NOT NULL,
  direction_interregionale text NOT NULL,
  capacite_norme           int,
  capacite_operationnelle  int,
  ecroues_detenus          int NOT NULL,
  densite_pct              double precision,
  date_reference           date NOT NULL,
  source_id                bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (etablissement, quartier, direction_interregionale, date_reference)
);

CREATE INDEX etablissement_penitentiaire_densite_idx
  ON core.etablissement_penitentiaire (date_reference, densite_pct DESC);

COMMENT ON TABLE core.etablissement_penitentiaire IS
  'Population détenue par établissement et quartier, statistique mensuelle du '
  'ministère de la Justice (GENESIS/DGAP). densite_pct = écroués détenus / '
  'capacité opérationnelle, calculé par la source elle-même, pas recalculé ici. '
  'Un établissement peut apparaître plusieurs fois (un par quartier).';

-- +goose Down
DROP TABLE core.etablissement_penitentiaire;
