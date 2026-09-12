-- +goose Up
-- Les grandes séries nationales, pour situer une politique dans son époque.
--
-- L'onglet GOUVERNEMENT demande de placer, sur une même frise, les présidences,
-- les gouvernements et ce qu'ont fait les grands agrégats : dette, dépenses par
-- fonction, recettes fiscales, chômage, pauvreté. Ces séries n'appartiennent à
-- aucune institution du projet : elles décrivent le pays, pas un acteur.
--
-- Comme partout ailleurs, la concomitance n'est pas une imputation. Une courbe
-- de dette qui monte sous une présidence ne dit pas que cette présidence l'a
-- fait monter : les décisions produisent leurs effets avec retard, et les chocs
-- extérieurs ne demandent l'avis de personne. La frise situe, elle n'explique
-- pas, et toute page qui l'affiche doit le dire.
CREATE TABLE ref.macro_serie (
  code        text PRIMARY KEY,
  label       text NOT NULL,
  unite       text NOT NULL,
  producteur  text NOT NULL,
  -- Ce que la série mesure EXACTEMENT. Sans cela, « le chômage » ou « la
  -- dette » veulent dire trois choses différentes selon la source.
  definition  text NOT NULL,
  -- Le regroupement d'affichage : DETTE, DEPENSE, RECETTE, EMPLOI, PAUVRETE.
  famille     text NOT NULL,
  -- Pour les dépenses par fonction : le code COFOG de la fonction.
  cofog       text,
  url         text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX macro_serie_famille_idx ON ref.macro_serie (famille);

CREATE TABLE core.macro_value (
  serie_code  text NOT NULL REFERENCES ref.macro_serie(code) ON DELETE CASCADE,
  annee       integer NOT NULL,
  valeur      numeric NOT NULL,
  -- Une valeur provisoire ou révisée reste distinguable d'une valeur définitive.
  statut      text,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  provenance  core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (serie_code, annee)
);

CREATE INDEX macro_value_annee_idx ON core.macro_value (annee);

COMMENT ON TABLE core.macro_value IS
  'Valeur annuelle d''une grande série nationale. Aucune valeur n''est '
  'interpolée : une année absente de la source est une année absente ici.';

-- Les gouvernements, pour poser la seconde strate de la frise sous les
-- présidences. Un gouvernement porte le nom de son Premier ministre ; les
-- ministres sont déjà dans core.mandate.
CREATE TABLE core.gouvernement (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom             text NOT NULL,
  premier_ministre_person_id bigint REFERENCES core.person(id) ON DELETE SET NULL,
  validity        daterange NOT NULL,
  source_url      text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  EXCLUDE USING gist (validity WITH &&)
);

COMMENT ON TABLE core.gouvernement IS
  'Gouvernements successifs. La contrainte d''exclusion interdit deux '
  'gouvernements simultanés : la passation a lieu un jour donné.';

-- +goose Down
DROP TABLE core.gouvernement;
DROP TABLE core.macro_value;
DROP TABLE ref.macro_serie;
