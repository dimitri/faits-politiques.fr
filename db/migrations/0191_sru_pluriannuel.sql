-- +goose Up
-- L'inventaire SRU au 1er janvier 2025 était chargé comme un instantané
-- (0135_sru_communes.sql). La même source data.gouv.fr publie en réalité
-- QUATRE millésimes annuels distincts (2023 à 2026), avec pour chaque
-- commune un prélèvement net (le montant réel de la sanction, majoration de
-- carence comprise) que le connecteur d'origine ne lisait pas alors qu'il
-- figurait déjà dans le fichier 2025.
--
-- La table devient pluriannuelle : la clé d'unicité passe de (code_insee) à
-- (code_insee, annee). `sru_commune_dernier` donne le dernier millésime par
-- commune, pour que le code qui n'a besoin que d'un instantané (la carte de
-- cmd/build/logement.go) n'ait rien à changer côté requête au-delà du nom de
-- la relation interrogée.
ALTER TABLE core.sru_commune ADD COLUMN annee smallint;
UPDATE core.sru_commune SET annee = 2025;
ALTER TABLE core.sru_commune ALTER COLUMN annee SET NOT NULL;

ALTER TABLE core.sru_commune ADD COLUMN prelevement_net double precision;
-- NULL et non 0 : le millésime 2023 ne publie pas ce prélèvement (colonne
-- absente de son fichier source), à ne jamais confondre avec un prélèvement
-- nul chez une commune conforme.
COMMENT ON COLUMN core.sru_commune.prelevement_net IS
  'Prélèvement SRU net des déductions, majoration de carence comprise. NULL '
  'pour le millésime 2023, dont le fichier source ne publie pas cette colonne '
  '(à distinguer d''un prélèvement réellement nul).';

ALTER TABLE core.sru_commune DROP CONSTRAINT sru_commune_code_insee_key;
ALTER TABLE core.sru_commune ADD CONSTRAINT sru_commune_code_insee_annee_key UNIQUE (code_insee, annee);

-- Une seule photographie cohérente, pas « la dernière ligne connue par
-- commune » : une commune sortie du périmètre en 2025 ne doit pas continuer
-- à apparaître avec son statut 2023 dès lors qu'un millésime plus récent
-- existe pour les autres. Toutes les lignes de la vue portent donc le MÊME
-- millésime, celui-là.
CREATE VIEW core.sru_commune_dernier AS
  SELECT * FROM core.sru_commune
  WHERE annee = (SELECT max(annee) FROM core.sru_commune);

COMMENT ON VIEW core.sru_commune_dernier IS
  'Le millésime le plus récent de core.sru_commune (une seule année, la même '
  'pour toutes les communes) — vue utilisée partout où une seule photographie '
  'suffit (carte de cmd/build/logement.go), la table elle-même portant '
  'l''historique 2023-2026.';

COMMENT ON TABLE core.sru_commune IS
  'Inventaire SRU par commune (DGALN/DHUP), un millésime annuel par ligne '
  '(2023 à 2026, clé (code_insee, annee)) : taux de logements sociaux par '
  'commune soumise à l''article 55 de la loi SRU, statut de carence et '
  'prélèvement net (majoration de carence comprise) là où la source le '
  'publie. Ne couvre que les communes dans le périmètre de la loi '
  '(~2 150 à ~2 200 selon le millésime), pas l''ensemble des communes '
  'françaises. Voir core.sru_commune_dernier pour le seul dernier millésime.';

-- +goose Down
DROP VIEW core.sru_commune_dernier;
ALTER TABLE core.sru_commune DROP CONSTRAINT sru_commune_code_insee_annee_key;
DELETE FROM core.sru_commune WHERE annee <> 2025;
ALTER TABLE core.sru_commune ADD CONSTRAINT sru_commune_code_insee_key UNIQUE (code_insee);
ALTER TABLE core.sru_commune DROP COLUMN prelevement_net;
ALTER TABLE core.sru_commune DROP COLUMN annee;
