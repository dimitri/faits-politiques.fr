-- +goose Up
-- Un fond de carte mondial (pays), réutilisable pour tout dossier ayant
-- besoin d'une carte du monde — pas propre à la Francophonie. Natural Earth,
-- échelle 1:50 000 000 : suffisant pour un repère mondial, pas pour un
-- cadastre. Certains très petits territoires (les départements d'outre-mer
-- français Guadeloupe, Martinique, Réunion, Guyane, Mayotte) n'existent pas
-- comme entités séparées à cette échelle — absorbés dans la métropole ou
-- absents du fond ; documenté dans les dossiers qui s'en servent, pas
-- corrigé ici par une géométrie inventée.
CREATE TABLE geo.contour_pays (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom_fr      text NOT NULL,
  nom_en      text NOT NULL,
  souverain   text NOT NULL,
  iso_a3      text,
  geom        geometry(MultiPolygon, 4326) NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contour_pays_geom_idx ON geo.contour_pays USING gist (geom);
CREATE INDEX contour_pays_nom_fr_idx ON geo.contour_pays (nom_fr);

COMMENT ON TABLE geo.contour_pays IS
  'Fond de carte mondial par pays/territoire — Natural Earth 1:50m (domaine public), '
  'admin-0 countries. souverain = le pays souverain déclarant (utile pour distinguer '
  'les territoires d''outre-mer de leur métropole).';

-- La répartition mondiale des francophones — Observatoire démographique et
-- statistique de l'espace francophone (ODSEF, Université Laval) et
-- Observatoire de la langue française de l'OIF, "Francoscope". Le fichier
-- source mélange pays souverains, territoires et entités infranationales
-- (provinces canadiennes, Louisiane, la Sarre, la Fédération
-- Wallonie-Bruxelles) et deux sous-totaux ("France Outre-mer", "France
-- (ensemble)") dans une même liste à plat — type_entite les distingue pour
-- qu'aucune carte ne les confonde ni ne les additionne par erreur.
CREATE TABLE core.francophonie_entite (
  id                        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  entite                    text NOT NULL UNIQUE,
  type_entite               text NOT NULL CHECK (type_entite IN ('pays', 'territoire', 'sous-national', 'agregat')),
  population_2025_milliers  numeric,
  francophone_pct           numeric,
  francophone_milliers      numeric,
  source_id                 bigint NOT NULL REFERENCES raw.source(id),
  created_at                timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.francophonie_entite IS
  'Population et nombre/part de francophones par entité, 2025 — ODSEF (Université Laval) '
  'et Observatoire de la langue française de l''OIF, outil "Francoscope".';

-- +goose Down
DROP TABLE core.francophonie_entite;
DROP TABLE geo.contour_pays;
