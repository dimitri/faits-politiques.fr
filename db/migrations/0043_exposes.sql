-- +goose Up
-- L'exposé des motifs : ce que l'auteur d'un texte dit qu'il veut faire.
--
-- L'open data parlementaire ne publie aucun résumé : ni l'Assemblée, ni le
-- Sénat, ni le Parlement européen. Un scrutin n'y porte qu'un libellé de
-- procédure — « l'amendement n° 2332 de M. Travert à l'article 25 bis » — qui
-- ne dit rien de l'objet.
--
-- L'exposé des motifs comble ce trou, et c'est le seul texte qui le comble sans
-- que nous ayons à écrire quoi que ce soit : il est rédigé par l'auteur du
-- texte, publié avec lui, et il dit l'intention déclarée.
--
-- CE N'EST PAS UN RÉSUMÉ NEUTRE. C'est un plaidoyer : l'auteur y défend sa
-- proposition. Le stocker tel quel et le citer comme tel est honnête ; le
-- présenter comme une description objective ne le serait pas. D'où `chapeau`,
-- qui est un EXTRAIT et non une synthèse — aucune ligne de ce projet ne doit
-- résumer un texte à la place de son auteur.
CREATE TABLE core.texte_expose (
  texte_id    bigint PRIMARY KEY REFERENCES core.texte(id) ON DELETE CASCADE,
  source_uid  text NOT NULL UNIQUE,
  url         text NOT NULL,
  -- L'exposé intégral, tel qu'il est publié.
  integral    text NOT NULL,
  -- Les premiers paragraphes, coupés à une fin de phrase. Extrait, jamais
  -- reformulation.
  chapeau     text NOT NULL,
  n_caracteres integer NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  provenance  core.provenance NOT NULL DEFAULT 'OFFICIAL',
  recupere_le timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.texte_expose IS
  'Exposé des motifs, rédigé par l''auteur du texte. Intention déclarée, pas '
  'description neutre : à citer avec sa source, jamais à présenter comme un '
  'résumé du projet.';

-- Ce que chaque scrutin peut afficher comme présentation. Un scrutin porte sur
-- un texte, qui appartient à un dossier ; l'exposé remonte par ce chemin.
--
-- Un dossier peut porter plusieurs textes — le texte initial, celui adopté en
-- commission, celui transmis. Le plus ancien est retenu : c'est celui qui
-- portait l'intention d'origine.
CREATE VIEW derived.scrutin_presentation AS
  SELECT s.id AS scrutin_id,
         s.slug,
         s.institution,
         s.date_seance,
         s.objet,
         e.chapeau,
         e.url AS expose_url,
         e.n_caracteres AS expose_longueur,
         d.titre AS dossier_titre
    FROM core.scrutin s
    JOIN core.dossier d ON d.id = s.dossier_id
    JOIN LATERAL (
      SELECT t.id FROM core.texte t
       WHERE t.dossier_id = d.id
       ORDER BY t.date_depot NULLS LAST, t.id
       LIMIT 1
    ) tx ON true
    JOIN core.texte_expose e ON e.texte_id = tx.id;

COMMENT ON VIEW derived.scrutin_presentation IS
  'Présentation d''un scrutin par l''exposé des motifs du texte en cause. '
  'Le chapeau est une CITATION de l''auteur, pas une synthèse du projet.';

-- +goose Down
DROP VIEW derived.scrutin_presentation;
DROP TABLE core.texte_expose;
