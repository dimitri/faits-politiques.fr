-- +goose Up
-- Couche DERIVED : tout est reconstructible, tout porte sa méthode.
-- Une ligne sans method_version ni empreinte d'entrées n'a pas sa place ici.

-- Clivage d'un scrutin : entropie normalisée de la distribution des positions
-- entre groupes, absents exclus du dénominateur.
-- Un scrutin voté 550 contre 0 ne porte aucune information et doit peser ~0 dans
-- tout calcul de proximité.
CREATE TABLE derived.scrutin_clivage (
  scrutin_id      bigint PRIMARY KEY REFERENCES core.scrutin(id) ON DELETE CASCADE,
  clivage         numeric NOT NULL CHECK (clivage BETWEEN 0 AND 1),
  n_exploitable   int     NOT NULL,   -- votants effectifs, hors ABSENT et NON_VOTING
  method_version  text    NOT NULL,
  computed_at     timestamptz NOT NULL DEFAULT now()
);

-- Position d'un groupe sur un scrutin, absents exclus du dénominateur.
-- Marquée non exploitable en dessous d'un seuil de participation : un groupe dont
-- 15 % des membres ont voté n'a pas de « position » mesurable.
CREATE TABLE derived.group_position (
  scrutin_id      bigint NOT NULL REFERENCES core.scrutin(id) ON DELETE CASCADE,
  organization_id bigint NOT NULL REFERENCES core.organization(id),
  score           numeric NOT NULL CHECK (score BETWEEN -1 AND 1),  -- (pour - contre) / exprimés
  n_exprimes      int     NOT NULL,
  n_effectif      int     NOT NULL,
  taux_participation numeric NOT NULL,
  exploitable     boolean NOT NULL,
  method_version  text    NOT NULL,
  computed_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (scrutin_id, organization_id)
);
CREATE INDEX group_position_org_idx ON derived.group_position (organization_id)
  WHERE exploitable;

-- La statistique reproductible.
-- « X a voté N % avec Y » dépend entièrement du jeu de scrutins retenu. Le périmètre
-- est donc encodé dans params et exposé dans l'URL : les deux camps contestent le
-- périmètre, pas le chiffre, et chacun peut publier sa propre version.
CREATE TABLE derived.stat_query (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug           text        NOT NULL UNIQUE,   -- permalien
  kind           text        NOT NULL CHECK (kind IN
                   ('GROUP_AGREEMENT','AUTHORSHIP_PROFILE','COMMUNE_COMPARISON','COVERAGE')),
  params         jsonb       NOT NULL,          -- législature, bornes, thèmes, seuils, traitement des absents
  method_version text        NOT NULL,
  inputs_hash    bytea       NOT NULL,          -- empreinte des identifiants d'entrée
  computed_at    timestamptz NOT NULL DEFAULT now(),
  n_used         int         NOT NULL,          -- scrutins/communes réellement exploités
  n_excluded     int         NOT NULL,
  exclusion_breakdown jsonb  NOT NULL DEFAULT '{}'::jsonb,  -- par ref.unverifiable_reason
  result         jsonb       NOT NULL,          -- valeurs + intervalles de confiance
  UNIQUE (kind, params, method_version, inputs_hash)
);

COMMENT ON TABLE derived.stat_query IS
  'Tout score publié porte son n exploitable, son intervalle de confiance et le détail '
  'de ce qui a été exclu. Un score sans intervalle de confiance est un score opaque.';

-- Appariement des communes. Comparer des communes par étiquette sans apparier sur
-- strate, région et revenu médian revient à mesurer la taille, pas le parti.
CREATE TABLE derived.comparison_run (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug            text    NOT NULL UNIQUE,
  indicator_code  text    NOT NULL REFERENCES ref.indicator(code),
  period_year     int     NOT NULL,
  matching_vars   text[]  NOT NULL,   -- {'strate','region','revenu_median'}
  method_version  text    NOT NULL,
  computed_at     timestamptz NOT NULL DEFAULT now(),
  n_communes      int     NOT NULL,
  n_sans_nuance   int     NOT NULL,   -- exclues faute d'étiquette exploitable
  taux_exclusion  numeric NOT NULL,
  result          jsonb   NOT NULL    -- médiane, distribution, IC par nuance
);

COMMENT ON COLUMN derived.comparison_run.n_sans_nuance IS
  'Une part importante des maires est « divers » ou sans étiquette. Ces communes sont '
  'exclues et le taux d''exclusion est affiché : le taire fausserait la comparaison.';

CREATE TABLE derived.comparison_member (
  run_id        bigint NOT NULL REFERENCES derived.comparison_run(id) ON DELETE CASCADE,
  commune_code  text   NOT NULL,
  nuance_code   text   NOT NULL,
  stratum_key   text   NOT NULL,      -- cellule d'appariement
  value         numeric NOT NULL,
  PRIMARY KEY (run_id, commune_code)
);
CREATE INDEX comparison_member_stratum_idx ON derived.comparison_member (run_id, stratum_key);

-- Transparence de la couverture. La plateforme doit pouvoir dire « données
-- insuffisantes » plutôt que produire une certitude artificielle.
CREATE TABLE derived.coverage (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  scope          text    NOT NULL,     -- 'an.17', 'communes.2026', 'debat.2026-09-10'
  metric         text    NOT NULL,     -- 'scrutins_avec_resume', 'affirmations_resolues'
  numerator      int     NOT NULL,
  denominator    int     NOT NULL CHECK (denominator > 0),
  method_version text    NOT NULL,
  computed_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (scope, metric, computed_at)
);

-- +goose Down
DROP TABLE derived.coverage;
DROP TABLE derived.comparison_member;
DROP TABLE derived.comparison_run;
DROP TABLE derived.stat_query;
DROP TABLE derived.group_position;
DROP TABLE derived.scrutin_clivage;
