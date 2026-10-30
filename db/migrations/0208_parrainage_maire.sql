-- +goose Up
-- Les maires qui ont parrainé un candidat à l'élection présidentielle, NOMMÉS.
--
-- La migration 0207 ne chargeait que des totaux (candidat × catégorie de mandat
-- × département × date) : ce dossier mesurait le soutien reçu par candidat, pas
-- la conduite de tel maire. Pour les maires, la question change. Le répertoire
-- national des élus ne donne à un maire aucune nuance politique, et la nuance
-- des listes municipales est « divers » pour 82,7 % des sièges nuancés ; le
-- parrainage est, pour un maire, l'un des rares actes publics qui le situent
-- (il a signé pour tel candidat). Le Conseil constitutionnel le publie lui-même,
-- nom, commune et candidat compris, parce que la loi organique du 6 novembre
-- 1962 le prévoit.
--
-- Cette table ne conserve donc que les maires (catégorie MAIRE : ni maires
-- délégués ni maires d'arrondissement, dont la commune du fichier n'est pas une
-- commune), et RIEN d'autre : pas de qualificatif politique, pas de famille
-- déduite. Le site affiche le fait brut — « a parrainé X en 2022 » — jamais une
-- couleur. Les autres élus (conseillers départementaux, parlementaires...)
-- restent agrégés dans core.parrainage_presidentielle.
--
-- Le rapprochement à la commune et au mandat en cours se fait à la lecture, par
-- les clés ci-dessous (nom sans accent ni ponctuation) : au chargement, ni le
-- COG ni le répertoire des élus ne sont forcément à jour, et un ordre
-- d'ingestion ne doit pas décider de ce qu'on retrouve.

-- +goose StatementBegin
CREATE FUNCTION core.f_cle_nom(text) RETURNS text
  LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $fn$
  SELECT regexp_replace(lower(core.f_unaccent(coalesce($1, ''))), '[^a-z0-9]+', '', 'g')
$fn$;
-- +goose StatementEnd

COMMENT ON FUNCTION core.f_cle_nom(text) IS
  'Clé de comparaison d''un nom propre : minuscules, sans accent, sans espace ni ponctuation.';

CREATE TABLE core.parrainage_maire (
  election         smallint NOT NULL,           -- année du scrutin : 2017, 2022, 2027
  nom              text     NOT NULL,           -- tel que publié : « CORDIVAL »
  prenom           text     NOT NULL,
  commune          text     NOT NULL,           -- la « circonscription » du fichier : le nom de la commune
  departement      text     NOT NULL,           -- tel que publié ; « Rhône », « Polynésie française »...
  candidat         text     NOT NULL,           -- tel que publié : « ARTHAUD Nathalie »
  date_publication date     NOT NULL,           -- le jour où le Conseil a publié ce parrainage
  source_id        bigint   NOT NULL REFERENCES raw.source(id),
  document_id      bigint   NOT NULL REFERENCES raw.document(id),
  nom_cle          text GENERATED ALWAYS AS (core.f_cle_nom(nom)) STORED,
  prenom_cle       text GENERATED ALWAYS AS (core.f_cle_nom(prenom)) STORED,
  commune_cle      text GENERATED ALWAYS AS (core.f_cle_nom(commune)) STORED,
  departement_cle  text GENERATED ALWAYS AS (core.f_cle_nom(departement)) STORED,
  PRIMARY KEY (election, nom, prenom, commune, departement, candidat)
);

CREATE INDEX parrainage_maire_cle_idx ON core.parrainage_maire (nom_cle, prenom_cle, commune_cle);

COMMENT ON TABLE core.parrainage_maire IS
  'Maires ayant parrainé un candidat à l''élection présidentielle, nommés, tels que le Conseil '
  'constitutionnel les publie (dernier jeu cumulatif de chaque scrutin). Un fait brut : la table '
  'ne porte aucune étiquette politique. Voir la note de la migration 0208.';

COMMENT ON TABLE core.parrainage_presidentielle IS
  'Parrainages publiés par le Conseil constitutionnel, agrégés par candidat, catégorie de mandat, '
  'département et date de publication. Dernier jeu cumulatif connu de chaque scrutin. Les élus ne '
  'sont pas nommés ici (migration 0207) ; les maires le sont dans core.parrainage_maire (migration 0208).';

-- +goose Down
DROP TABLE core.parrainage_maire;
DROP FUNCTION core.f_cle_nom(text);
