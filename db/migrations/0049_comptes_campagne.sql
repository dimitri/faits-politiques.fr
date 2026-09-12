-- +goose Up
-- Les comptes de campagne, candidat par candidat.
--
-- La Commission nationale des comptes de campagne et des financements
-- politiques publie, pour chaque scrutin, ce que chaque candidat a déclaré
-- avoir dépensé et reçu — et ce qu'elle a RETENU après contrôle. L'écart entre
-- les deux est le fait le plus intéressant du jeu : il dit ce que la Commission
-- a rejeté.
--
-- LICENCE : la CNCCFP publie ces jeux sans licence déclarée (`notspecified`),
-- comme les résultats municipaux de 2020 (D-036). Le raisonnement est le même —
-- ce sont des documents administratifs et les comptes sont publiés au Journal
-- officiel par obligation légale — mais la règle du projet reste qu'une absence
-- de licence n'est pas une autorisation. La source est donc classée RESTRICTED
-- et attend une décision explicite avant tout export ouvert.
CREATE TABLE core.compte_campagne (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  type_election  text NOT NULL REFERENCES ref.type_election(code),
  annee          integer NOT NULL,
  -- Identifiant du candidat dans le fichier de la Commission. Il vaut pour ce
  -- scrutin seulement : ce n'est pas un identifiant de personne.
  candidat_ref   text,
  candidat_nom   text NOT NULL,
  circonscription text,
  departement    text,
  code_departement text,
  nuance         text,
  monnaie        text,
  depenses_declarees numeric,
  depenses_retenues  numeric,
  recettes_declarees numeric,
  recettes_retenues  numeric,
  -- NULL tant que le rattachement à une personne n'est pas décidé : le fichier
  -- ne porte ni date de naissance ni identifiant, et 41,9 % de nos cibles ont
  -- un homonyme exact en base.
  person_id      bigint REFERENCES core.person(id) ON DELETE SET NULL,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  provenance     core.provenance NOT NULL DEFAULT 'OFFICIAL',
  UNIQUE (type_election, annee, candidat_nom, circonscription)
);

CREATE INDEX compte_campagne_person_idx ON core.compte_campagne (person_id)
  WHERE person_id IS NOT NULL;
CREATE INDEX compte_campagne_scrutin_idx ON core.compte_campagne (type_election, annee);

COMMENT ON COLUMN core.compte_campagne.depenses_retenues IS
  'Après contrôle de la Commission. L''écart avec les dépenses déclarées dit ce '
  'qui a été rejeté — c''est le chiffre qui a une portée, pas le déclaré seul.';

-- Le détail poste par poste, avec le libellé de la Commission conservé tel quel.
-- Une table plutôt que quatre-vingts colonnes : la nomenclature change d'un
-- scrutin à l'autre, et figer les postes en colonnes obligerait à migrer le
-- schéma à chaque élection.
CREATE TABLE core.compte_campagne_poste (
  compte_id bigint NOT NULL REFERENCES core.compte_campagne(id) ON DELETE CASCADE,
  poste     text NOT NULL,
  -- DECLARE ou RETENU : le même poste existe dans les deux versions.
  etat      text NOT NULL CHECK (etat IN ('DECLARE','RETENU')),
  montant   numeric NOT NULL,
  PRIMARY KEY (compte_id, poste, etat)
);

COMMENT ON TABLE core.compte_campagne_poste IS
  'Postes de dépense et de recette, libellés de la Commission conservés sans '
  'regroupement : « Enquêtes et sondages », « Honoraires et conseils en '
  'communication »… Les regrouper serait une décision éditoriale.';

-- +goose Down
DROP TABLE core.compte_campagne_poste;
DROP TABLE core.compte_campagne;
