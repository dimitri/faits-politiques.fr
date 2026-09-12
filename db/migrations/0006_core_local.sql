-- +goose Up
-- Volet communal.
--
-- Il n'existe aucune donnée nationale sur ce qu'une commune A DÉCIDÉ : pas
-- d'agrégateur de délibérations, pas de format obligatoire. Ce schéma documente
-- donc ce que les producteurs publics MESURENT chaque année sous un mandat.
-- Le glissement de l'un à l'autre est la faute à ne jamais commettre :
-- ref.indicator.caveat est affiché sur chaque fiche pour l'empêcher.

CREATE TABLE core.commune_indicator (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  commune_code   text    NOT NULL,
  cog_millesime  int     NOT NULL,     -- millésime COG utilisé par le producteur
  indicator_code text    NOT NULL REFERENCES ref.indicator(code),
  period_year    int     NOT NULL,
  value          numeric NOT NULL,
  source_id      bigint  NOT NULL REFERENCES raw.source(id),
  provenance     core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at     timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (commune_code, cog_millesime) REFERENCES ref.commune(code_insee, cog_millesime),
  UNIQUE (commune_code, indicator_code, period_year)
);
CREATE INDEX commune_indicator_lookup_idx
  ON core.commune_indicator (commune_code, period_year);
CREATE INDEX commune_indicator_series_idx
  ON core.commune_indicator (indicator_code, period_year);

COMMENT ON TABLE core.commune_indicator IS
  'Format long : une ligne par commune x indicateur x année. Décrit une évolution '
  'pendant un mandat, jamais l''effet d''une politique municipale.';

-- Commande publique. Les DECP sont déjà consolidées et dédoublonnées en amont :
-- on consomme la version consolidée, on ne refait pas le nettoyage.
CREATE TABLE core.public_contract (
  id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_uid       text    NOT NULL UNIQUE,
  acheteur_siret   text,
  commune_code     text,                  -- résolu quand l'acheteur est une commune
  titulaire_siret  text,
  titulaire_nom    text,
  objet            text,
  montant          numeric,
  date_notification date,
  duree_mois       int,
  cpv              text,
  source_id        bigint  NOT NULL REFERENCES raw.source(id),
  provenance       core.provenance NOT NULL DEFAULT 'OFFICIAL'
);
CREATE INDEX public_contract_commune_idx  ON core.public_contract (commune_code, date_notification DESC);
CREATE INDEX public_contract_titulaire_idx ON core.public_contract (titulaire_siret);

COMMENT ON COLUMN core.public_contract.titulaire_siret IS
  'Fréquemment absent ou invalide dans la source. Une absence de SIRET n''autorise '
  'aucun rapprochement d''entreprise : laisser NULL plutôt que deviner.';

-- +goose Down
DROP TABLE core.public_contract;
DROP TABLE core.commune_indicator;
