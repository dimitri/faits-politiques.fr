-- +goose Up

-- Quelle étape du catalogue (internal/ingest.Source.Nom — « download »,
-- « eau », « fraude-fiscale »...) a enregistré cette source RAW — pour
-- pouvoir répondre « combien d'octets fpctl ingest parlement download
-- télécharge-t-il ? » pour N'IMPORTE quelle étape du catalogue, pas
-- seulement les sept du socle parlementaire tenues à la main
-- (cmd/fpctl/list.go, composantesEtape) : une étape peut enregistrer
-- plusieurs sources (download en enregistre cinq, AN-AMO et consorts), donc
-- ce lien vit sur raw.source plutôt que raw.fetch_run/raw.retrieval, une
-- fois par source plutôt qu'à chaque récupération.
--
-- NULL pour une source jamais réingérée depuis cette migration (elle
-- recevra son étape à la prochaine exécution — internal/archive.EnsureSource
-- l'écrit à chaque UPSERT) ou pour une source enregistrée en dehors du
-- catalogue (aucune aujourd'hui). Un reflet, comme core.pipeline_etape :
-- réécrit par le code Go à chaque ingestion, jamais tenu à la main.
ALTER TABLE raw.source ADD COLUMN etape text;

COMMENT ON COLUMN raw.source.etape IS
  'internal/ingest.Source.Nom qui a enregistré cette source — un reflet '
  'écrit par internal/archive.EnsureSource à chaque ingestion, pour '
  'calculer la taille à télécharger par étape sans liste tenue à la main '
  '(voir fpctl list deps).';

CREATE INDEX source_etape_idx ON raw.source (etape) WHERE etape IS NOT NULL;

-- +goose Down
DROP INDEX raw.source_etape_idx;
ALTER TABLE raw.source DROP COLUMN etape;
