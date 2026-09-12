-- +goose Up
-- Extensions et types partagés.
--
-- Convention de clés : bigint identity pour les clés internes (compacité, localité
-- d'index), plus un slug stable et public partout où un permalien doit exister.
-- Les identifiants publics ne doivent jamais être des clés techniques : ils sont
-- immuables et cités à l'extérieur.

CREATE EXTENSION IF NOT EXISTS btree_gist;   -- EXCLUDE mêlant égalité scalaire et chevauchement de range
CREATE EXTENSION IF NOT EXISTS pg_trgm;      -- recherche floue sur la table d'alias
CREATE EXTENSION IF NOT EXISTS unaccent;     -- recherche plein texte française
CREATE EXTENSION IF NOT EXISTS pgcrypto;     -- digest() pour les empreintes calculées en base

CREATE SCHEMA raw;      -- copie exacte de ce qui a été récupéré, jamais écrasée
CREATE SCHEMA ref;      -- nomenclatures externes millésimées
CREATE SCHEMA core;     -- données normalisées, historisées, sourcées
CREATE SCHEMA derived;  -- indicateurs et statistiques, reconstructibles

COMMENT ON SCHEMA raw IS
  'Immuable. Aucun UPDATE, aucun DELETE. core doit être intégralement reconstructible depuis ici.';
COMMENT ON SCHEMA derived IS
  'Reconstructible. Toute ligne porte une method_version et une empreinte de ses entrées.';

-- unaccent() est STABLE et non IMMUTABLE (son dictionnaire est résolu à l'exécution),
-- donc inutilisable dans une expression d'index. Ce wrapper fige le dictionnaire et
-- devient indexable. Toute recherche insensible aux accents passe par lui.
-- +goose StatementBegin
CREATE FUNCTION core.f_unaccent(text) RETURNS text
  LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
AS $fn$
  SELECT public.unaccent('public.unaccent'::regdictionary, $1)
$fn$;
-- +goose StatementEnd

-- Institutions parlementaires couvertes.
CREATE TYPE core.institution AS ENUM (
  'ASSEMBLEE_NATIONALE',
  'SENAT',
  'PARLEMENT_EUROPEEN'
);

-- Positions de vote. ABSENT et NON_VOTING ne sont PAS des positions politiques :
-- ce sont des données manquantes, et aucun calcul de proximité ne doit les traiter
-- autrement (cf. docs/perimetre.md §5.2).
CREATE TYPE core.vote_position AS ENUM (
  'FOR',
  'AGAINST',
  'ABSTAIN',
  'ABSENT',
  'NON_VOTING'
);

-- Granularité réellement publiée par la source.
-- ASSEMBLEE_NATIONALE publie des positions nominatives  -> INDIVIDUAL
-- SENAT publie des positions par groupe avec exceptions -> GROUP
-- Ne jamais projeter une position de groupe sur un individu (perimetre.md §2.4).
CREATE TYPE core.scrutin_granularite AS ENUM (
  'INDIVIDUAL',
  'GROUP'
);

CREATE TYPE core.organization_kind AS ENUM (
  'PARTY',                -- parti politique
  'PARLIAMENTARY_GROUP',  -- groupe à l'AN ou au Sénat
  'EP_GROUP',             -- groupe au Parlement européen
  'COALITION',            -- coalition électorale
  'LOCAL_LABEL',          -- étiquette / nuance locale
  'GOVERNMENT',           -- gouvernement (origine d'un projet de loi)
  'COMMITTEE'             -- commission (origine d'un amendement)
);

CREATE TYPE core.mandate_type AS ENUM (
  'DEPUTE',
  'SENATEUR',
  'DEPUTE_EUROPEEN',
  'MINISTRE',
  'MAIRE',
  'ADJOINT_AU_MAIRE',
  'CONSEILLER_MUNICIPAL',
  'CONSEILLER_COMMUNAUTAIRE',
  'CONSEILLER_DEPARTEMENTAL',
  'CONSEILLER_REGIONAL'
);

-- Hiérarchie des sources. Une information de niveau 2 ou 3 ne peut jamais être
-- présentée comme un fait officiel.
CREATE TYPE core.source_tier AS ENUM (
  'PRIMARY_OFFICIAL',   -- niveau 1 : AN, Sénat, JO, DGFiP, INSEE, SSMSI...
  'SECONDARY_PRESS',    -- niveau 2 : presse
  'DECLARATIVE'         -- niveau 3 : partis, communiqués, comptes officiels
);

-- Provenance d'une donnée normalisée. Une sortie de modèle n'est jamais
-- la source finale d'un fait publié.
CREATE TYPE core.provenance AS ENUM (
  'OFFICIAL',        -- transcrit mécaniquement d'une source primaire
  'HUMAN_VERIFIED',  -- vérifié par une personne identifiée
  'AI_EXTRACTED',    -- extrait par modèle, non vérifié
  'AI_CLASSIFIED'    -- classé par modèle, non vérifié
);

CREATE TYPE core.verification_status AS ENUM (
  'UNVERIFIED',
  'AUTO_VERIFIED',
  'HUMAN_VERIFIED',
  'DISPUTED'
);

CREATE TYPE core.amendement_sort AS ENUM (
  'ADOPTE',
  'REJETE',
  'RETIRE',
  'NON_SOUTENU',
  'TOMBE',
  'IRRECEVABLE',
  'NON_EXAMINE'
);

-- Qualité de l'attribution d'une proposition à un auteur politique.
-- Seul UNAMBIGUOUS autorise à écrire « le groupe X a proposé ceci ».
CREATE TYPE core.author_attribution AS ENUM (
  'UNAMBIGUOUS',
  'PRIMARY_SIGNATORY',
  'COALITION',
  'GOVERNMENT',
  'UNRESOLVED'
);

-- +goose Down
DROP SCHEMA derived CASCADE;
DROP SCHEMA core CASCADE;
DROP SCHEMA ref CASCADE;
DROP SCHEMA raw CASCADE;
