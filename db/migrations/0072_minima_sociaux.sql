-- +goose Up
-- Les minima sociaux, dispositif par dispositif, depuis 1990 — dont la
-- continuité RMI (1988-2009) → RSA (depuis juin 2009), le fil que
-- docs/chomage-donnees.md suit pour raconter trente-cinq ans d'un même
-- filet de sécurité sous deux noms.
--
-- France MÉTROPOLITAINE seulement (la Drees publie aussi un tableau France
-- entière, non chargé ici) : c'est le champ qui remonte le plus loin dans le
-- temps sans rupture liée à l'extension progressive du RSA aux DOM
-- (1er janvier 2011, puis Mayotte au 1er janvier 2012).
CREATE TABLE core.minima_sociaux_effectif (
  dispositif_code    text NOT NULL,
  dispositif_libelle text NOT NULL,
  annee              smallint NOT NULL,
  effectif           integer NOT NULL,
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dispositif_code, annee)
);

COMMENT ON TABLE core.minima_sociaux_effectif IS
  'Nombre d''allocataires au 31 décembre par dispositif de minimum social, '
  'France métropolitaine, 1990-2024 (Drees). dispositif_code=RSA porte le '
  'RSA socle depuis juin 2009 (RSA activité remplacé par la prime d''activité '
  'en 2016) ; RMI s''arrête la même année. Deux séries à ne jamais sommer '
  'pour une année de bascule sans lire dispositif_libelle.';

-- La dépense, disponible seulement depuis 2009 (l'année où la Drees a
-- harmonisé le suivi financier des dispositifs sur le RSA) — en euros
-- constants 2024, donc déjà comparable dans le temps sans déflater soi-même.
CREATE TABLE core.minima_sociaux_depense (
  dispositif_code       text NOT NULL,
  dispositif_libelle    text NOT NULL,
  annee                 smallint NOT NULL,
  depense_meur_reel2024 numeric NOT NULL,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dispositif_code, annee)
);

COMMENT ON TABLE core.minima_sociaux_depense IS
  'Dépense annuelle d''allocations par dispositif de minimum social, en '
  'millions d''euros constants 2024, France, 2009-2024 (Drees). Comparable '
  'directement d''une année à l''autre : la source a déjà déflaté.';

-- +goose Down
DROP TABLE core.minima_sociaux_depense;
DROP TABLE core.minima_sociaux_effectif;
