-- +goose Up

-- Le graphe de dépendances d'ingestion (internal/pipeline), publié dans la
-- base à chaque exécution plutôt que gardé seulement dans le code Go : une
-- question comme « qu'est-ce qui dépend du Sénat » se pose alors en SQL,
-- sans lire internal/ingest/ingest.go, et l'historique d'exécution devient
-- une donnée qu'on peut interroger plutôt qu'un journal à relire.
--
-- Le code Go reste la source de vérité — ces tables sont un REFLET, réécrit
-- au début de chaque ingestion (voir pipeline.Registre.Publier), jamais
-- modifiées à la main : une dépendance retirée du code doit disparaître
-- d'ici, pas y survivre.
CREATE TABLE core.pipeline_etape (
  nom                        text PRIMARY KEY,
  description                text,
  -- NULL : jamais exécutée avec succès depuis que cette ligne existe (une
  -- étape neuve, ou une base neuve). Pas un défaut à deviner.
  derniere_execution_reussie timestamptz
);

COMMENT ON TABLE core.pipeline_etape IS
  'Les étapes du pipeline d''ingestion (internal/pipeline), republiées à '
  'chaque exécution — jamais la source de vérité, un reflet du code Go.';

CREATE TABLE core.pipeline_dependance (
  etape     text NOT NULL REFERENCES core.pipeline_etape(nom) ON DELETE CASCADE,
  depend_de text NOT NULL REFERENCES core.pipeline_etape(nom) ON DELETE CASCADE,
  PRIMARY KEY (etape, depend_de),
  CHECK (etape <> depend_de)
);

COMMENT ON TABLE core.pipeline_dependance IS
  'etape ne peut être lancée qu''après depend_de. Un cycle est une erreur de '
  'programmation détectée par pipeline.Registre au démarrage, jamais une '
  'donnée valide ici.';

-- +goose Down
DROP TABLE core.pipeline_dependance;
DROP TABLE core.pipeline_etape;
