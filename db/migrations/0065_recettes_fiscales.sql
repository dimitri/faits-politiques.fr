-- +goose Up
-- Les recettes fiscales par poste, et le piège qui va avec.
--
-- Eurostat publie 93 postes pour la France. Ils ne sont PAS une partition : ils
-- s'emboîtent. D2 contient D21 qui contient D211 ; D5 contient D51 qui contient
-- D51A et D51B ; D61 contient D611. Et certains codes sont des agrégats
-- composites — D2_D5_D91 additionne trois familles.
--
-- Les sommer donnerait un total plusieurs fois supérieur au produit réel de
-- l'impôt. C'est l'erreur que cette table doit rendre impossible à commettre
-- par inadvertance : d'où le drapeau `agregat`, et un commentaire qui le dit
-- avant qu'on écrive la première requête.
CREATE TABLE ref.poste_fiscal (
  code        text PRIMARY KEY,
  libelle     text NOT NULL,
  -- Vrai pour les codes composites, reconnaissables à leur underscore :
  -- D2_D5_D91 n'est pas un impôt, c'est une addition déjà faite.
  agregat     boolean NOT NULL,
  -- Profondeur dans l'emboîtement, déduite de la longueur du code élémentaire.
  -- Sert à choisir un niveau de lecture cohérent, pas à reconstituer un arbre :
  -- la nomenclature SEC a des exceptions que cette heuristique ignore.
  profondeur  smallint NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE ref.poste_fiscal IS
  'Nomenclature des recettes fiscales et sociales (SEC 2010, Eurostat). '
  'LES POSTES S''EMBOÎTENT : D2 ⊃ D21 ⊃ D211. Ne jamais sommer plusieurs postes '
  'sans avoir vérifié qu''ils sont disjoints. Pour un total, prendre un agrégat '
  'publié, pas une addition maison.';

CREATE TABLE core.recette_fiscale (
  poste      text NOT NULL REFERENCES ref.poste_fiscal(code),
  -- S13 = toutes administrations publiques. Le sous-secteur est conservé pour
  -- pouvoir distinguer un jour ce qui revient à l'État de ce qui revient à la
  -- Sécurité sociale — c'est exactement le canal n° 1 de budget-donnees.md.
  secteur    text NOT NULL,
  annee      integer NOT NULL,
  -- numeric, pas double precision : voir docs/monnaie-et-inflation.md § 5.
  montant_meur numeric NOT NULL,
  -- Euros courants, tels que publiés. Aucune déflation ici : c'est un calcul,
  -- il appartient à derived.
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  provenance core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (poste, secteur, annee)
);

CREATE INDEX recette_fiscale_annee_idx ON core.recette_fiscale (annee);

COMMENT ON COLUMN core.recette_fiscale.montant_meur IS
  'Millions d''euros COURANTS. Comparer deux années sans déflater compare des '
  'unités différentes — voir docs/monnaie-et-inflation.md.';

-- Les préfets : nommés, révocables, territoriaux — et donc hors core.mandate.
--
-- core.mandate ne contient que des mandats ÉLECTIFS, et tout le modèle repose
-- sur cette distinction. Un préfet est nommé par décret en conseil des
-- ministres sur le fondement de l'article 13 de la Constitution, et révocable à
-- tout moment : c'est un emploi à la discrétion du Gouvernement. Le ranger avec
-- les députés et les maires brouillerait la seule chose que core.mandate sait
-- dire avec certitude — que quelqu'un a été élu.
--
-- Aucune couleur politique n'est stockée, et ce n'est pas un oubli. Un préfet
-- n'a pas d'affiliation publiée, et la fonction est légalement une fonction de
-- neutralité. Ce qui est documentable sans rien inventer, c'est la DATE de sa
-- nomination — donc le gouvernement qui l'a signée. L'inférence appartient au
-- lecteur, pas à la table.
CREATE TABLE core.prefet (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  nom           text NOT NULL,
  prenom        text,
  -- Le poste tel que la source le nomme : le plus souvent un département, mais
  -- la série de 1800 à nos jours contient aussi des postes disparus.
  poste         text NOT NULL,
  -- Code INSEE du département quand il a pu être rattaché. NULL est une réponse
  -- de plein droit : un poste de 1812 n'a pas de code COG.
  code_departement text,
  -- Période d'exercice. daterange ouvert à droite pour un préfet en poste.
  -- Les dates anciennes sont souvent connues à l'année près : la borne porte
  -- alors le 1er janvier, et `precision_dates` le dit.
  validity      daterange NOT NULL,
  precision_dates text NOT NULL DEFAULT 'ANNEE'
    CHECK (precision_dates IN ('JOUR', 'ANNEE')),
  annee_naissance smallint,
  annee_deces     smallint,
  wikidata      text,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  provenance    core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX prefet_poste_idx ON core.prefet (poste);
CREATE INDEX prefet_validity_idx ON core.prefet USING gist (validity);

COMMENT ON TABLE core.prefet IS
  'Préfets de département depuis 1800. Source : Archives nationales / ministère '
  'de la Culture, alignée sur Wikidata. La série s''arrête environ six ans avant '
  'le présent : elle est alimentée par les versements aux Archives, faits après '
  'clôture du dossier de carrière. Pour les préfets en poste, la source est le '
  'décret de nomination au Journal officiel.';

-- +goose Down
DROP TABLE core.prefet;
DROP TABLE core.recette_fiscale;
DROP TABLE ref.poste_fiscal;
