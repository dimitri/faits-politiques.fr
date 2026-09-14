-- +goose Up
-- FINESS — la clé pivot du volet Santé (docs/sante-donnees.md) : un
-- identifiant d'établissement sanitaire ou social sur lequel chaque source
-- secondaire (SAE, PMSI, SNDS/Damir, RPPS, HAS...) pourra se joindre plus
-- tard, exactement comme ref.commune sert de pivot au volet territorial.
-- Cette migration ne charge qu'un seul étage : le référentiel des
-- établissements lui-même, rien d'activité ni de finances.
CREATE TABLE ref.finess_etablissement (
  nofinesset            text PRIMARY KEY,
  nofinessej            text NOT NULL,      -- entité juridique (peut porter plusieurs établissements)
  raison_sociale        text NOT NULL,
  raison_sociale_longue text,
  code_commune          text,               -- partie commune FINESS (3 chiffres), PAS un code INSEE seul
  code_insee            text,               -- departement+commune concaténés par ce connecteur, voir notes
  code_departement      text,
  libelle_departement   text,
  ligne_acheminement    text,               -- code postal + libellé commune, tel que publié
  categorie_code        text NOT NULL,
  categorie_libelle     text,
  categorie_agregat_code   text NOT NULL,
  categorie_agregat_libelle text,
  siret                 text,
  code_mft              text,               -- mode de fixation des ressources
  libelle_mft           text,
  code_sph              text,               -- statut : public / PSPH / privé d'intérêt collectif...
  libelle_sph           text,
  date_ouverture        date,
  date_maj              date,
  source_id             bigint NOT NULL REFERENCES raw.source(id),
  created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX finess_etablissement_departement_idx ON ref.finess_etablissement (code_departement);
CREATE INDEX finess_etablissement_insee_idx ON ref.finess_etablissement (code_insee);
CREATE INDEX finess_etablissement_categorie_idx ON ref.finess_etablissement (categorie_agregat_code);

COMMENT ON TABLE ref.finess_etablissement IS
  'Référentiel FINESS des établissements sanitaires et sociaux, Agence du numérique en '
  'santé/data.gouv.fr. Table PIVOT : ne porte aucune donnée d''activité, de lits ou de '
  'personnel — voir docs/sante-donnees.md pour ce qui reste à joindre dessus. '
  'code_sph ne classe pas systématiquement public/privé lucratif : une large part des '
  'établissements (médico-sociaux notamment) portent la valeur « Non concerné », le champ '
  'ne concerne pleinement que les établissements de santé. Ne pas en déduire une '
  'répartition public/privé exhaustive sans vérifier categorie_agregat en complément. '
  'code_insee = code_departement + code_commune concaténés par ce connecteur (le fichier '
  'source ne publie le code commune que sur 3 chiffres, sans le département) — à valider '
  'avant toute jointure à ref.commune, la source ne garantit pas ce format pour les DOM.';

-- +goose Down
DROP TABLE ref.finess_etablissement;
