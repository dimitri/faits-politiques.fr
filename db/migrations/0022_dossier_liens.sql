-- +goose Up
-- Liens vers les pages officielles du dossier.
--
-- L'open data ne publie aucun résumé ni exposé des motifs : seuls les titres,
-- auteurs, dates et étapes y figurent. Les chemins ci-dessous permettent au
-- moins de renvoyer le lecteur vers le dossier officiel, qui contient le texte
-- intégral, les rapports et les comptes rendus.
ALTER TABLE core.dossier
  ADD COLUMN titre_chemin text,
  ADD COLUMN senat_chemin text;

COMMENT ON COLUMN core.dossier.titre_chemin IS
  'Segment d''URL du dossier sur assemblee-nationale.fr. Publié par la source, '
  'jamais reconstruit à partir du titre.';

-- +goose Down
ALTER TABLE core.dossier DROP COLUMN senat_chemin;
ALTER TABLE core.dossier DROP COLUMN titre_chemin;
