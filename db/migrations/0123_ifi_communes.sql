-- +goose Up
-- IFICOM : la répartition, commune par commune, de l'impôt sur la fortune
-- immobilière (IFI) — publiée par la DGFiP pour les seules villes de plus de
-- 20 000 habitants comptant plus de 50 redevables (un seuil de publication
-- fixé par la DGFiP, pas par ce dépôt : la plupart des communes de France
-- n'apparaissent jamais dans cette table, faute d'atteindre ce double seuil).
--
-- Millésimes 2021 à 2025 seulement : le millésime 2020 et les précédents
-- publient le patrimoine moyen en MILLIONS d'euros et l'impôt moyen en
-- MILLIERS d'euros (une unité différente, vérifiée à l'inspection), pas
-- encore normalisée dans ce chargement — à reprendre séparément plutôt que
-- deviné.
--
-- Paris, seule ville découpée par arrondissement dans cette source certaines
-- années (jusqu'à 20 lignes « 751xx »), est chargé tel que publié : ces
-- codes n'existent pas dans ref.commune ni geo.contour_cog (qui ne
-- connaissent que « 75056 »), donc aucune clé étrangère vers ref.commune —
-- la carte les agrège au moment de l'affichage, pas au chargement.
CREATE TABLE core.ifi_commune (
  id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                 integer NOT NULL,
  code_insee            text NOT NULL,
  nom_commune           text NOT NULL,
  code_departement      text,
  region                text,
  nombre_redevables     integer NOT NULL,
  patrimoine_moyen_eur  numeric NOT NULL,
  impot_moyen_eur       numeric NOT NULL,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  UNIQUE (annee, code_insee)
);

CREATE INDEX ifi_commune_annee_idx ON core.ifi_commune (annee);
CREATE INDEX ifi_commune_dept_idx ON core.ifi_commune (code_departement);

COMMENT ON TABLE core.ifi_commune IS
  'IFICOM (DGFiP) : nombre de redevables, patrimoine moyen et impôt moyen à l''IFI, '
  'commune par commune, pour les seules communes de plus de 20 000 habitants comptant '
  'plus de 50 redevables (seuil de publication de la DGFiP). Millésimes 2021-2025 '
  '(le format 2020 et antérieur utilise des unités différentes, non normalisées ici).';

-- +goose Down
DROP TABLE core.ifi_commune;
