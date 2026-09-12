-- +goose Up
-- La délinquance enregistrée, commune par commune et année par année.
--
-- Le SSMSI — service statistique ministériel de la sécurité intérieure —
-- publie sous Licence Ouverte le nombre de faits enregistrés par la police et
-- la gendarmerie, pour dix-huit indicateurs, de 2016 à 2025, dans chacune des
-- communes. 5,2 millions de lignes.
--
-- CE QUE CES CHIFFRES MESURENT, ET CE QU'ILS NE MESURENT PAS. Ils comptent les
-- faits ENREGISTRÉS, pas les faits commis : une hausse peut venir d'une hausse
-- de la délinquance, d'une hausse des plaintes, d'un changement de doctrine de
-- l'enregistrement, ou de l'ouverture d'un commissariat. Le SSMSI le dit
-- lui-même dans sa documentation, et toute page qui affiche ces séries doit le
-- répéter.
--
-- ET SURTOUT : LA COMMUNE NE COMMANDE PAS CES FORCES. La police nationale et la
-- gendarmerie relèvent de l'État. Un maire dispose au plus d'une police
-- municipale, dont les compétences ne couvrent presque aucun des indicateurs
-- ci-dessous — homicides, violences sexuelles, trafic de stupéfiants. Attribuer
-- ces chiffres à la couleur politique d'une mairie est une erreur de catégorie,
-- de même nature que comparer deux budgets communaux sans regarder ce qui a été
-- transféré à l'intercommunalité (D-024).
--
-- La table existe pour rendre les séries VÉRIFIABLES, pas pour fonder une
-- imputation.
CREATE TABLE ref.indicateur_delinquance (
  code            text PRIMARY KEY,
  libelle         text NOT NULL,
  unite_de_compte text NOT NULL,
  -- Ce que l'indicateur recouvre, dans les termes du SSMSI.
  definition      text
);

COMMENT ON TABLE ref.indicateur_delinquance IS
  'Nomenclature des indicateurs du SSMSI. L''unité de compte (victime, '
  'infraction, mis en cause) change d''un indicateur à l''autre : deux '
  'indicateurs ne s''additionnent pas.';

CREATE TABLE core.commune_delinquance (
  commune_code    text NOT NULL,
  cog_millesime   integer NOT NULL,
  annee           integer NOT NULL,
  indicateur_code text NOT NULL REFERENCES ref.indicateur_delinquance(code),
  nombre          integer,
  taux_pour_mille numeric,
  -- Le secret statistique. Le SSMSI ne diffuse pas les effectifs trop faibles
  -- pour rester anonymes : `diffuse` vaut alors faux et `nombre` est NULL.
  -- « Non diffusé » et « zéro fait » sont deux choses différentes, et les
  -- confondre ferait dire à la base ce que la source refuse de dire.
  diffuse         boolean NOT NULL,
  population      integer,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  provenance      core.provenance NOT NULL DEFAULT 'OFFICIAL',
  PRIMARY KEY (commune_code, annee, indicateur_code),
  FOREIGN KEY (commune_code, cog_millesime)
    REFERENCES ref.commune(code_insee, cog_millesime)
);

CREATE INDEX commune_delinquance_indic_idx
  ON core.commune_delinquance (indicateur_code, annee);
CREATE INDEX commune_delinquance_commune_idx
  ON core.commune_delinquance (commune_code, indicateur_code);

COMMENT ON COLUMN core.commune_delinquance.nombre IS
  'Faits enregistrés. NULL quand le secret statistique s''applique — ce n''est '
  'PAS zéro.';

-- L'évolution d'un indicateur dans une commune, avec la couleur de sa
-- municipalité — et la mise en garde intégrée à la vue elle-même.
--
-- `periode` dit si l'année observée précède ou suit l'élection qui a donné
-- cette couleur. Toutes les années de 2016 à 2025 PRÉCÈDENT le scrutin de mars
-- 2026 : la colonne vaut donc « AVANT_MANDAT » partout aujourd'hui. C'est le
-- fait central, et la vue le rend impossible à ignorer — ces séries décrivent
-- ce dont une municipalité HÉRITE, jamais ce qu'elle a produit.
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
         cc.scrutin_annee AS couleur_issue_du_scrutin,
         CASE WHEN cc.scrutin_annee IS NULL THEN 'SANS_COULEUR'
              WHEN d.annee < cc.scrutin_annee THEN 'AVANT_MANDAT'
              ELSE 'PENDANT_MANDAT' END AS periode
    FROM core.commune_delinquance d
    JOIN ref.commune c
      ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
    LEFT JOIN derived.commune_couleur cc
      ON cc.commune_code = d.commune_code;

COMMENT ON VIEW derived.commune_securite IS
  'Séries de délinquance enregistrée par commune, avec la nuance de la liste '
  'majoritaire. La colonne `periode` dit si l''année observée précède le '
  'scrutin qui a donné cette couleur : tant qu''elle vaut AVANT_MANDAT, la '
  'série décrit un héritage et non une action.';

-- +goose Down
DROP VIEW derived.commune_securite;
DROP TABLE core.commune_delinquance;
DROP TABLE ref.indicateur_delinquance;
