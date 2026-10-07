-- +goose Up
-- La partition de l'Indochine en 1954, pour docs/guerres-decolonisation-donnees.md
-- (§ 1 et § 5).
--
-- Ce n'est PAS une extension de geo.territoire_colonial (migration 0128) :
-- cette table-là répond à « quand tel territoire a-t-il cessé d'être
-- français ? », avec un régime juridique français (colonie, protectorat,
-- mandat) et une date d'indépendance vis-à-vis de la France. Or CShapes 2.0
-- (même source, même CSV déjà téléchargé pour 0128) modélise aussi, à partir
-- du 1er mai 1954, deux entités nées de la partition actée par les accords
-- de Genève du 21 juillet 1954 : la République démocratique du Viêt Nam
-- (Nord, gwcode 816) et la République du Viêt Nam (Sud, gwcode 817). Aucune
-- des deux n'a « obtenu son indépendance de la France » à cette date-là :
-- l'indépendance de l'ensemble du Viêt Nam vis-à-vis de la France est déjà
-- actée par ces mêmes accords de Genève (voir la ligne « Vietnam
-- (Cochinchine, Annam, Tonkin) » de geo.territoire_colonial, indépendance au
-- 1954-07-21) ; ce que ces deux nouvelles entités montrent, c'est la
-- partition d'un même ancien territoire colonial entre deux États rivaux,
-- jusqu'à la chute de Saïgon le 30 avril 1975. D'où une table séparée,
-- avec ses propres colonnes (camp, pas régime juridique français).
CREATE TABLE geo.indochine_partition_1954 (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  territoire    text NOT NULL UNIQUE,
  camp          text NOT NULL CHECK (camp IN ('nord', 'sud')),
  date_debut    date NOT NULL,
  date_fin      date NOT NULL,
  note          text NOT NULL,
  geom          geometry(MultiPolygon, 4326),
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX indochine_partition_1954_geom_idx ON geo.indochine_partition_1954 USING gist (geom);

COMMENT ON TABLE geo.indochine_partition_1954 IS
  'La partition du Viêt Nam actée par les accords de Genève (21 juillet 1954) '
  'jusqu''à la chute de Saïgon (30 avril 1975) : deux entités (CShapes 2.0, '
  'gwcode 816 et 817), distinctes de geo.territoire_colonial car nées APRÈS '
  'le départ français, pas des territoires coloniaux français — voir le '
  'commentaire de la migration 0151.';

-- +goose Down
DROP TABLE geo.indochine_partition_1954;
