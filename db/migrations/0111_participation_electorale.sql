-- +goose Up
-- Représentativité des dirigeants (docs/international-donnees.md § 7) :
-- International IDEA publie un export statique (xls) qui donne la
-- PARTICIPATION électorale par pays et par élection — pas la part du
-- vainqueur ni le mode de scrutin, qui resteraient à compiler à la main
-- pays par pays. Cette table charge donc une part du sujet, pas sa
-- totalité, et le dit explicitement plutôt que de suggérer le contraire.
--
-- Chine et Arabie saoudite sont absentes par construction : IDEA ne
-- recense que les élections législatives et présidentielles au suffrage
-- direct, qu'aucun des deux pays ne tient dans ce sens — une absence qui
-- est elle-même un fait sur le système politique, pas un trou de données à
-- corriger.
CREATE TABLE core.participation_electorale (
  pays_iso3            text NOT NULL,
  pays_label           text NOT NULL,
  type_election        text NOT NULL,  -- Parliamentary, Presidential, EU Parliament
  date_election        date NOT NULL,
  taux_participation_inscrits numeric,       -- % des inscrits, NULL si non publié
  taux_participation_vap      numeric,       -- % de la population en âge de voter (VAP)
  population_vap        bigint,
  population_totale     bigint,
  vote_obligatoire      boolean NOT NULL,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pays_iso3, type_election, date_election)
);

COMMENT ON TABLE core.participation_electorale IS
  'International IDEA, Voter Turnout Database (export statique du site, pas '
  'une API). Le taux sur les inscrits et le taux sur la population en âge de '
  'voter (VAP) répondent à deux questions différentes — un pays à '
  'enregistrement automatique et un pays à enregistrement volontaire ne se '
  'comparent pas sur le premier sans le second. vote_obligatoire distingue '
  'les pays où l''écart avec un pays à vote volontaire n''a pas le même sens.';

-- +goose Down
DROP TABLE core.participation_electorale;
