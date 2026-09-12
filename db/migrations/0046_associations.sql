-- +goose Up
-- Le tissu associatif, commune par commune.
--
-- Le Répertoire national des associations est tenu par le ministère de
-- l'Intérieur et publié chaque mois sous Licence Ouverte. Il recense toutes les
-- associations déclarées en préfecture depuis 1901 — celles qui existent encore
-- comme celles qui ont été dissoutes.
--
-- POURQUOI C'EST UN FAIT POLITIQUE. La densité associative d'une commune, ce
-- qu'elle couvre (sport, culture, action sociale, défense de l'environnement)
-- et son évolution disent quelque chose de la vie collective d'un territoire —
-- et se rapprochent des mandatures. Mais le RNA ne dit RIEN des subventions :
-- il enregistre des déclarations, pas des flux financiers.
--
-- LE RATTACHEMENT GÉOGRAPHIQUE EST IMPARFAIT ET IL FAUT LE DIRE. Le RNA publie
-- un code postal et un nom de commune, jamais le code INSEE. Le champ SIRET,
-- qui aurait permis de passer par SIRENE, est vide dans 99,99 % des lignes
-- (8 renseignées sur 145 704 mesurées). Le rattachement se fait donc par
-- égalité de nom DANS LE DÉPARTEMENT, ce qui est un rapprochement de noms —
-- borné, vérifié, mais un rapprochement quand même. Les lignes non résolues
-- sont comptées et conservées sans commune plutôt qu'attribuées au hasard.
CREATE TABLE core.association (
  rna_id        text PRIMARY KEY,
  titre         text NOT NULL,
  objet         text,
  -- Code de l'objet social, nomenclature du ministère de l'Intérieur.
  objet_social  text,
  nature        text,
  -- A = active, D = dissoute, S = supprimée : le RNA garde l'historique.
  position      text,
  date_creation date,
  date_publication date,
  code_departement text,
  code_postal   text,
  commune_libelle text,
  -- NULL quand le nom de commune n'a pas pu être résolu sans ambiguïté.
  commune_code  text,
  cog_millesime integer,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  provenance    core.provenance NOT NULL DEFAULT 'OFFICIAL',
  FOREIGN KEY (commune_code, cog_millesime)
    REFERENCES ref.commune(code_insee, cog_millesime)
);

CREATE INDEX association_commune_idx ON core.association (commune_code)
  WHERE commune_code IS NOT NULL;
CREATE INDEX association_departement_idx ON core.association (code_departement);
CREATE INDEX association_position_idx ON core.association (position);

COMMENT ON COLUMN core.association.commune_code IS
  'Résolu par égalité de nom dans le département. NULL quand le nom est '
  'ambigu ou introuvable : jamais attribué au hasard.';

-- La densité associative d'une commune, rapportée à la municipalité en vigueur.
-- Même construction que pour la sécurité et les budgets (D-036) : la couleur
-- retenue est celle du dernier scrutin tenu au plus tard l'année considérée.
CREATE VIEW derived.commune_vie_associative AS
  SELECT a.commune_code,
         c.nom AS commune,
         extract(year FROM a.date_creation)::integer AS annee_creation,
         count(*) AS associations_creees,
         cc.nuance_code,
         cc.scrutin_annee AS mandature_depuis
    FROM core.association a
    JOIN ref.commune c
      ON c.code_insee = a.commune_code AND c.cog_millesime = a.cog_millesime
    LEFT JOIN LATERAL (
      SELECT x.nuance_code, x.scrutin_annee
        FROM derived.commune_couleur x
       WHERE x.commune_code = a.commune_code
         AND x.scrutin_annee <= extract(year FROM a.date_creation)
       ORDER BY x.scrutin_annee DESC LIMIT 1
    ) cc ON true
   WHERE a.commune_code IS NOT NULL AND a.date_creation IS NOT NULL
   GROUP BY 1, 2, 3, 5, 6;

COMMENT ON VIEW derived.commune_vie_associative IS
  'Créations d''associations par commune et par année, avec la municipalité en '
  'vigueur. Une création n''est pas un financement : le RNA ne publie aucune '
  'subvention.';

-- +goose Down
DROP VIEW derived.commune_vie_associative;
DROP TABLE core.association;
