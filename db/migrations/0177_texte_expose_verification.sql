-- +goose Up

-- core.texte_expose ne porte QUE les exposés trouvés : un texte sans exposé
-- (visé par la source mais transmis par le Sénat, par exemple) ou dont la
-- page était inaccessible n'y laisse aucune trace. Avant cette table,
-- internal/an/exposes.go les redemandait donc à chaque passage, pour
-- toujours redécouvrir la même absence — mesuré : ~990 requêtes à 400 ms
-- chacune, RE-jouées à l'identique à chaque « fpctl ingest exposes », même
-- une fois core.texte.id rendu stable (migration précédente).
--
-- Une table séparée plutôt qu'une ligne core.texte_expose à contenu NULL :
-- ce que core.texte_expose garantit (un exposé RÉELLEMENT trouvé, cité
-- verbatim) resterait vrai pour toute ligne qui y existe, jamais à vérifier
-- au cas par cas par ce que la lit ailleurs (pages, exports).
CREATE TABLE core.texte_expose_verification (
  texte_id     bigint PRIMARY KEY REFERENCES core.texte(id) ON DELETE CASCADE,
  verifie_le   timestamptz NOT NULL DEFAULT now(),
  trouve       boolean NOT NULL,
  raison       text
);

COMMENT ON TABLE core.texte_expose_verification IS
  'Chaque texte déjà VÉRIFIÉ par internal/an/exposes.go, trouvé ou non — '
  'sans elle, un texte sans exposé (ou une page inaccessible) était '
  'redemandé à chaque passage, à 400 ms la requête, sans jamais converger. '
  'Ne dit rien sur le CONTENU d''un exposé : voir core.texte_expose pour ça.';

-- +goose Down
DROP TABLE core.texte_expose_verification;
