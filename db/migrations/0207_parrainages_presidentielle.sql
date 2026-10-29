-- +goose Up
-- Les parrainages (« présentations ») de l'élection présidentielle, tels que le
-- Conseil constitutionnel les publie : un jeu CUMULATIF par scrutin, remplacé à
-- chaque publication (2022 : tous les trois ou quatre jours à partir du 1er
-- février, 13 427 parrainages au 7 mars ; 2017 : à partir du 1er mars).
--
-- Seul registre OFFICIEL de qui est candidat avant la liste du Conseil, et la
-- seule trace d'un soutien d'élus avant le scrutin (docs/pre-enregistrement-002.md
-- § 3). Rien n'existe pour 2027 avant la publication du Conseil (vers février
-- 2027) : le connecteur sait la chercher, et charge en attendant 2017 et 2022,
-- ce qui valide le format et donne la référence de comparaison.
--
-- AGRÉGÉ, pas nominatif. Le fichier du Conseil nomme chaque élu qui parraine
-- (maire, conseiller départemental...) : c'est public, et publié par le Conseil
-- lui-même. Mais ce dossier mesure le soutien reçu par candidat, pas la conduite
-- de tel maire — nom, prénom et commune de l'élu ne sont donc pas chargés,
-- seulement la catégorie de mandat, le département et la date de publication
-- (même raison que l'exclusion des entrepreneurs individuels de SIRENE,
-- migration 0077 : minimiser ce dont on n'a pas besoin). Le fichier d'origine
-- reste scellé dans raw.document : quiconque veut le détail nominatif le lit là.
CREATE TABLE core.parrainage_presidentielle (
  election         smallint NOT NULL,           -- année du scrutin : 2017, 2022, 2027
  candidat         text     NOT NULL,           -- tel que publié : « ARTHAUD Nathalie »
  categorie        text     NOT NULL CHECK (categorie IN
                     ('MAIRE','MAIRE_DELEGUE','MAIRE_ARRONDISSEMENT','CONSEILLER_DEPARTEMENTAL',
                      'CONSEILLER_REGIONAL','DEPUTE','SENATEUR','EURODEPUTE','PRESIDENT_INTERCO','AUTRE')),
  departement      text     NOT NULL,           -- tel que publié ; vide pour les élus de l'étranger
  date_publication date     NOT NULL,           -- le jour où le Conseil a publié ce parrainage
  nombre           integer  NOT NULL CHECK (nombre > 0),
  source_id        bigint   NOT NULL REFERENCES raw.source(id),
  document_id      bigint   NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (election, candidat, categorie, departement, date_publication)
);

COMMENT ON TABLE core.parrainage_presidentielle IS
  'Parrainages publiés par le Conseil constitutionnel, agrégés par candidat, catégorie de mandat, '
  'département et date de publication. Dernier jeu cumulatif connu de chaque scrutin. Les élus ne '
  'sont pas nommés : voir la note de la migration 0207.';

-- +goose Down
DROP TABLE core.parrainage_presidentielle;
