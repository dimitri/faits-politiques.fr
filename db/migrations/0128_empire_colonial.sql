-- +goose Up
-- L'empire colonial français : géographie et chronologie des territoires,
-- pour le dossier docs/empire-colonial-donnees.md.
--
-- Les géométries et les dates de fin (indépendance ou rétrocession) viennent
-- de CShapes 2.0 (ETH Zürich, icr.ethz.ch/data/cshapes), un jeu de contours
-- historiques à précision journalière — vérifié directement : chaque ligne
-- couvre une période où l'entité (identifiée par son nom de pays actuel, pas
-- par la puissance qui l'administre) a des frontières inchangées. Limite
-- réelle et documentée : le panel démarre au 1er janvier 1886 pour toutes
-- les entités — la conquête de l'Algérie (1830) n'y est donc pas visible
-- comme changement de frontière, seule la situation à partir de 1886 l'est.
--
-- Les dates de rattachement (avant 1886, ou pour les territoires que
-- CShapes ne couvre pas) et le régime juridique (colonie, protectorat,
-- mandat/tutelle) viennent d'une vérification faite territoire par
-- territoire sur Wikidata (déclarations sourcées P571/P576), la
-- classification Wikidata elle-même n'étant pas assez cohérente pour une
-- seule requête automatisée : une colonie (Q133156) et un protectorat
-- (Q164142) et un mandat de la SDN (Q426759) sont trois classes distinctes,
-- et certains territoires (Cameroun, Togo) ne portent même pas de
-- déclaration "pays = France" (P17) alors qu'ils étaient administrés par la
-- France — un choix de modélisation de Wikidata, pas une erreur à corriger
-- ici. D'où une table saisie à la main plutôt qu'une requête rejouable.
CREATE TABLE geo.territoire_colonial (
  id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  territoire            text NOT NULL UNIQUE,
  region                text NOT NULL CHECK (region IN (
                           'Afrique du Nord', 'Afrique de l''Ouest', 'Afrique équatoriale',
                           'Afrique de l''Est et océan Indien', 'Asie du Sud-Est', 'Autre'
                         )),
  regime                text NOT NULL CHECK (regime IN ('colonie', 'protectorat', 'mandat puis tutelle', 'colonie puis protectorat')),
  annee_rattachement    int NOT NULL,
  note_rattachement     text NOT NULL,
  date_independance     date NOT NULL,
  note_independance      text NOT NULL,
  geom                  geometry(MultiPolygon, 4326),
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX territoire_colonial_geom_idx ON geo.territoire_colonial USING gist (geom);

COMMENT ON TABLE geo.territoire_colonial IS
  'Empire colonial français : géographie (CShapes 2.0, ETH Zürich) et chronologie '
  '(dates vérifiées territoire par territoire sur Wikidata) de 22 territoires. '
  'annee_rattachement est une année communément retenue par l''historiographie, pas '
  'une date à la précision du jour comme date_independance (issue de CShapes).';
COMMENT ON COLUMN geo.territoire_colonial.geom IS
  'Contour à la dernière période précédant l''indépendance (CShapes). NULL pour les '
  'territoires hors couverture CShapes (petits comptoirs sans polygone séparé).';

-- +goose Down
DROP TABLE geo.territoire_colonial;
