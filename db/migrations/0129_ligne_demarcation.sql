-- +goose Up
-- La ligne de démarcation (1940-1942), séparant zone occupée et zone libre
-- — Département de l'Ain, republié sur data.gouv.fr, vérifié directement
-- (un unique tracé, 2938 points, 1219 km, longitude -1,31 à 6,08, latitude
-- 43,07 à 47,34 : de la frontière espagnole près d'Hendaye à la frontière
-- suisse près du Jura, cohérent avec le tracé historique connu). Pour le
-- dossier docs/seconde-guerre-mondiale-donnees.md.
--
-- Limite documentée : ce tracé couvre la seule ligne de démarcation
-- 1940-1942, pas les autres découpages de l'Occupation (l'annexion de fait
-- de l'Alsace-Moselle, la zone d'occupation italienne au sud-est à partir
-- de novembre 1942) — aucune géométrie vérifiée trouvée pour celles-ci au
-- moment de l'écriture, citées en prose seulement.
CREATE TABLE geo.ligne_demarcation (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  longueur_m  double precision NOT NULL,
  geom        geometry(LineString, 4326) NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ligne_demarcation_geom_idx ON geo.ligne_demarcation USING gist (geom);

COMMENT ON TABLE geo.ligne_demarcation IS
  'Tracé de la ligne de démarcation entre zone occupée et zone libre, 1940-1942 '
  '(Département de l''Ain, republié sur data.gouv.fr, licence Ouverte 2.0).';

-- +goose Down
DROP TABLE geo.ligne_demarcation;
