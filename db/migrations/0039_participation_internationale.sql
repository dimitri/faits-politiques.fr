-- +goose Up
-- La participation électorale comparée, pour situer un scrutin français.
--
-- Cette table répond à une question précise : quel est le dénominateur ? Pour
-- chaque élection nationale depuis 1945, elle porte le nombre d'inscrits, le
-- nombre de votants et la population en âge de voter, ce qui permet de calculer
-- une participation sur trois bases différentes — et de constater qu'elles ne
-- racontent pas la même chose.
--
-- CE QU'ELLE NE CONTIENT PAS, et il faut le savoir avant de l'interroger : les
-- VOIX PAR CANDIDAT. La source ne publie aucun résultat nominatif. On ne peut
-- donc PAS calculer ici « le score du vainqueur rapporté aux inscrits ». Pour la
-- France, ce calcul est possible via core.pdr_voix ; pour les autres pays, il
-- suppose d'aller chercher chaque commission électorale nationale, une par une.
-- Aucun jeu de données public ne fait les deux à la fois.
--
-- Niveau de preuve : International IDEA est un COMPILATEUR, pas une autorité
-- électorale. Ses chiffres proviennent des commissions nationales mais sont
-- retranscrits par un tiers, et la base ne publie pas de licence explicite —
-- d'où tier SECONDARY_PRESS et reuse_class RESTRICTED. Affichable sur le site,
-- jamais reversable dans un export.
CREATE TABLE core.turnout_election (
  pays_iso3      text NOT NULL CHECK (length(pays_iso3) = 3),
  pays_nom       text NOT NULL,
  -- Presidential | Parliamentary | EU Parliament, tel que la source l'écrit.
  type_scrutin   text NOT NULL,
  -- Chaque TOUR est une ligne : la source date les deux tours séparément, ce qui
  -- est exactement ce qu'il faut pour comparer un second tour français à un
  -- second tour polonais.
  date_scrutin   date NOT NULL,
  votants        bigint CHECK (votants >= 0),
  inscrits       bigint CHECK (inscrits >= 0),
  -- Population en âge de voter estimée par la source. Comme partout, elle compte
  -- les résidents, pas les citoyens : voir core.population_age.
  vap            bigint CHECK (vap >= 0),
  votes_invalides_pct numeric,
  vote_obligatoire    boolean,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  -- La source est un tiers compilateur : la provenance ne peut pas être OFFICIAL.
  provenance     core.provenance NOT NULL DEFAULT 'AI_EXTRACTED',
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pays_iso3, type_scrutin, date_scrutin)
);

CREATE INDEX turnout_election_type_idx ON core.turnout_election (type_scrutin, date_scrutin);

COMMENT ON TABLE core.turnout_election IS
  'Participation aux élections nationales depuis 1945 (International IDEA). '
  'Inscrits et votants seulement : aucun résultat par candidat. Source de '
  'niveau 2, sans licence explicite — non redistribuable.';

COMMENT ON COLUMN core.turnout_election.inscrits IS
  'Électeurs inscrits tels que compilés par la source. Pour la France, à '
  'confronter systématiquement à core.pdr_resultat.inscrits, qui vient de la '
  'proclamation du Conseil constitutionnel et fait seul foi.';

-- +goose Down
DROP TABLE core.turnout_election;
