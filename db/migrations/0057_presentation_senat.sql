-- +goose Up
-- Le Sénat écrit lui-même l'objet de ses textes. Nous ne l'avions pas lu.
--
-- `derived.scrutin_presentation` ne savait présenter qu'un scrutin de
-- l'Assemblée, à partir des exposés des motifs récupérés page par page sur
-- assemblee-nationale.fr. Les scrutins du Sénat n'avaient rien.
--
-- Or le dump Dosleg, scellé depuis le premier jour, porte trois champs que
-- personne n'avait ouverts :
--
--   loi.objet        1 454 présentations rédigées, jusqu'à 14 649 caractères
--   loi.motclef      5 069 jeux de mots-clefs
--   loi.en_clair_url   234 pages « La loi en clair » du service des études
--
-- C'est la deuxième fois que ce dump se révèle plus riche qu'annoncé — après
-- les 1,65 million de votes nominatifs et les 30 thèmes officiels. La leçon est
-- notée : avant de télécharger une source nouvelle, finir de lire l'ancienne.
CREATE TABLE core.dossier_presentation (
  dossier_id  bigint PRIMARY KEY REFERENCES core.dossier(id) ON DELETE CASCADE,
  objet       text,
  mots_clefs  text,
  en_clair_url text,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  method_version text NOT NULL,
  charge_le   timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT presentation_non_vide CHECK (
    objet IS NOT NULL OR mots_clefs IS NOT NULL OR en_clair_url IS NOT NULL)
);

COMMENT ON TABLE core.dossier_presentation IS
  'Présentation d''un dossier par son producteur. Ce sont les mots du Sénat, '
  'repris tels quels : ni résumé ni reformulation.';
COMMENT ON COLUMN core.dossier_presentation.en_clair_url IS
  'Page « La loi en clair » du Sénat. Chemin relatif à senat.fr : la page '
  'elle-même n''est pas récupérée, seul son adresse est conservée.';

-- La présentation d'un scrutin, quelle que soit son assemblée.
--
-- L'ordre des sources est celui de la proximité au texte voté : l'exposé des
-- motifs du texte déposé d'abord, l'objet du dossier ensuite. Le champ
-- `origine` dit lequel a servi — une présentation dont on ignore la provenance
-- ne vaut rien.
DROP VIEW IF EXISTS derived.scrutin_presentation;

CREATE VIEW derived.scrutin_presentation AS
  SELECT s.id AS scrutin_id,
         s.slug,
         s.institution,
         s.date_seance,
         s.objet,
         d.titre AS dossier_titre,
         coalesce(e.chapeau, left(p.objet, 600))            AS chapeau,
         coalesce(e.n_caracteres, length(p.objet))          AS presentation_longueur,
         e.url                                              AS expose_url,
         p.mots_clefs,
         p.en_clair_url,
         CASE WHEN e.texte_id IS NOT NULL THEN 'EXPOSE_DES_MOTIFS'
              WHEN p.objet    IS NOT NULL THEN 'OBJET_DU_DOSSIER'
         END AS origine
    FROM core.scrutin s
    JOIN core.dossier d ON d.id = s.dossier_id
    LEFT JOIN LATERAL (
           SELECT t.id FROM core.texte t
            WHERE t.dossier_id = d.id ORDER BY t.date_depot, t.id LIMIT 1) tx ON true
    LEFT JOIN core.texte_expose e ON e.texte_id = tx.id
    LEFT JOIN core.dossier_presentation p ON p.dossier_id = d.id
   WHERE e.texte_id IS NOT NULL OR p.objet IS NOT NULL;

-- +goose Down
DROP VIEW derived.scrutin_presentation;
DROP TABLE core.dossier_presentation;
CREATE VIEW derived.scrutin_presentation AS
  SELECT s.id AS scrutin_id, s.slug, s.institution, s.date_seance, s.objet,
         e.chapeau, e.url AS expose_url, e.n_caracteres AS expose_longueur,
         d.titre AS dossier_titre
    FROM core.scrutin s
    JOIN core.dossier d ON d.id = s.dossier_id
    JOIN LATERAL (SELECT t.id FROM core.texte t WHERE t.dossier_id = d.id
                   ORDER BY t.date_depot, t.id LIMIT 1) tx ON true
    JOIN core.texte_expose e ON e.texte_id = tx.id;
