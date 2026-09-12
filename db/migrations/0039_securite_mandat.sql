-- +goose Up
-- La couleur EN VIGUEUR une année donnée, et non toutes les couleurs connues.
--
-- La première version de la vue joignait derived.commune_couleur sans condition
-- d'année. Tant qu'un seul scrutin était chargé, elle donnait une ligne par
-- observation ; depuis que 2020 et 2026 coexistent (D-036), elle en donnerait
-- deux — la même année de délinquance rattachée à deux municipalités
-- différentes, dont l'une n'était pas encore élue.
--
-- La règle est celle du sens commun, écrite explicitement : la couleur en
-- vigueur en année N est celle du dernier scrutin tenu au plus tard en N.
DROP VIEW IF EXISTS derived.commune_securite;

CREATE VIEW derived.commune_securite AS
  SELECT d.commune_code,
         c.nom AS commune,
         d.annee,
         d.indicateur_code,
         d.nombre,
         d.taux_pour_mille,
         d.diffuse,
         d.population,
         cc.nuance_code,
         cc.circulaire_millesime,
         cc.scrutin_annee AS mandature_depuis,
         -- Une série n'a de sens comme bilan que si la mandature couvre
         -- l'année observée. La colonne le dit au lieu de le laisser déduire.
         CASE WHEN cc.scrutin_annee IS NULL THEN 'HORS_MANDATURE_CONNUE'
              WHEN cc.nuance_code IS NULL   THEN 'MANDATURE_SANS_NUANCE'
              ELSE 'DANS_LA_MANDATURE' END AS rattachement
    FROM core.commune_delinquance d
    JOIN ref.commune c
      ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
    LEFT JOIN LATERAL (
      -- Le dernier scrutin tenu au plus tard l'année observée.
      SELECT x.nuance_code, x.circulaire_millesime, x.scrutin_annee
        FROM derived.commune_couleur x
       WHERE x.commune_code = d.commune_code
         AND x.scrutin_annee <= d.annee
       ORDER BY x.scrutin_annee DESC
       LIMIT 1
    ) cc ON true;

COMMENT ON VIEW derived.commune_securite IS
  'Délinquance enregistrée par commune, rattachée à la municipalité EN '
  'VIGUEUR cette année-là. Rappel qui doit accompagner tout affichage : la '
  'commune ne commande ni la police nationale ni la gendarmerie, et ces '
  'chiffres comptent des faits ENREGISTRÉS, non des faits commis.';

-- Le même rattachement pour les comptes communaux, qui posent exactement la
-- même question : les séries de l'OFGL courent de 2018 à 2025, c'est-à-dire
-- pendant la mandature 2020-2026 et non pendant celle qui commence en 2026.
CREATE VIEW derived.commune_budget_mandature AS
  SELECT i.commune_code,
         c.nom AS commune,
         i.period_year AS annee,
         i.indicator_code,
         i.value,
         cc.nuance_code,
         cc.scrutin_annee AS mandature_depuis,
         -- Une compétence transférée à l'intercommunalité retire à la commune
         -- la décision que le montant est censé mesurer (D-024). La colonne
         -- porte le nombre de compétences transférées, pour que le lecteur
         -- sache dans quelle proportion le budget communal est un résidu.
         (SELECT count(*) FROM core.commune_competence cp
           WHERE cp.commune_code = i.commune_code) AS competences_transferees
    FROM core.commune_indicator i
    JOIN ref.commune c
      ON c.code_insee = i.commune_code AND c.cog_millesime = i.cog_millesime
    LEFT JOIN LATERAL (
      SELECT x.nuance_code, x.scrutin_annee
        FROM derived.commune_couleur x
       WHERE x.commune_code = i.commune_code
         AND x.scrutin_annee <= i.period_year
       ORDER BY x.scrutin_annee DESC
       LIMIT 1
    ) cc ON true;

COMMENT ON VIEW derived.commune_budget_mandature IS
  'Comptes communaux rattachés à la municipalité en vigueur l''année de '
  'l''exercice, avec le nombre de compétences transférées à '
  'l''intercommunalité — sans lequel un montant communal n''est pas comparable.';

-- +goose Down
DROP VIEW derived.commune_budget_mandature;
DROP VIEW derived.commune_securite;
