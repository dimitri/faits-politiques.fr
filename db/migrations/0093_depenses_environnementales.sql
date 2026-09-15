-- +goose Up
-- Les dépenses de protection de l'environnement au sens large (CEPA —
-- classification des activités et dépenses de protection de l'environnement)
-- — l'agrégat souvent cité autour de 100 Md€, à ne pas confondre avec le
-- budget vert (core.depense_fiscale, une cotation de dépenses existantes) ni
-- avec la fiscalité écologique stricte (core.recette_fiscale, poste D29F).
-- Voir docs/ecologie-donnees.md § 4.
CREATE TABLE core.depense_environnementale (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee          smallint NOT NULL,
  purpose_code   text NOT NULL,     -- code CEP (Eurostat), ex. 'TOT_CEP_EP', 'CEP01'...
  purpose_libelle text NOT NULL,
  secteur_code   text NOT NULL,     -- S1 (total économie), S11_S12 (entreprises), S13_S15 (administrations), S14 (ménages)
  secteur_libelle text NOT NULL,
  unite          text NOT NULL,     -- MIO_EUR ou PC_GDP
  valeur         numeric,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX depense_environnementale_uniq ON core.depense_environnementale
  (annee, purpose_code, secteur_code, unite);

COMMENT ON TABLE core.depense_environnementale IS
  'Eurostat env_epea_neep : dépense de protection de l''environnement par objet (CEP) et '
  'secteur institutionnel, France, 2012-2025. purpose_code=TOT_CEP_EP est le total tous '
  'objets confondus — ne pas le sommer avec les sous-codes CEP01/CEP03/... sous peine de '
  'compter deux fois (même piège que la hiérarchie SAE, voir docs/sante-donnees.md § 1.1). '
  'secteur_code=S1 (total économie) inclut déjà S11_S12+S13_S15+S14 : même remarque.';

-- +goose Down
DROP TABLE core.depense_environnementale;
