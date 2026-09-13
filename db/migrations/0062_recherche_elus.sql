-- +goose Up
-- Retrouver les actes qui concernent un élu, et non ceux qui portent son nom.
--
-- LE PROBLÈME QUE CE SCHÉMA RÉSOUT. Chercher « NUNEZ » dans 1,24 million
-- d'actes rend tous les Nunez du Journal officiel depuis 1861 — le ministre,
-- l'agent nommé dans un corps préfectoral, le titulaire d'une décoration. Et
-- chercher « Laurent » puis « Nunez » séparément rend en plus tous les actes où
-- les deux mots apparaissent sans se toucher. 41,9 % de nos élus ont un
-- homonyme exact en nom et prénom.
--
-- La réponse est un THÉSAURUS : les graphies d'un nom se ramènent à un jeton
-- canonique, ici l'identifiant de la personne. « Laurent NUNEZ », « Laurent
-- Nunez » et « Laurent Nuñez » donnent tous `elu:20476`, et une recherche sur
-- ce jeton ne rend que des actes qui nomment CETTE personne.
--
-- POURQUOI PAS UN DICTIONNAIRE `thesaurus` DE POSTGRESQL. C'est l'outil prévu,
-- et il ne convient pas ici pour trois raisons, dans l'ordre de gravité :
--
--   1. il se configure par un FICHIER déposé dans $SHAREDIR/tsearch_data. Sur
--      une base gérée — Scaleway, où ce projet doit tourner — on n'écrit pas de
--      fichier sur le serveur. Le schéma ne serait pas déployable ;
--   2. le fichier vit dans l'image, pas dans le dépôt. Il disparaît au premier
--      changement d'image, et l'image vient d'en changer ;
--   3. la documentation de PostgreSQL avertit qu'un thésaurus est chargé en
--      mémoire à son premier usage DANS CHAQUE SESSION, et qu'il n'est pas
--      prévu pour un grand nombre d'entrées.
--
-- Le thésaurus est donc ici une TABLE, et la substitution se fait à l'indexation
-- plutôt qu'à l'analyse lexicale. Le résultat est le même — un jeton canonique
-- par personne — et il se réplique, se sauvegarde et se déploie.

CREATE TABLE ref.elu_recherche (
  person_id bigint PRIMARY KEY REFERENCES core.person(id) ON DELETE CASCADE,
  -- Le jeton canonique. Pas de séparateur : `elu:20476` serait refusé par
  -- to_tsquery — le deux-points y est l'opérateur de pondération — et
  -- `elu_20476` serait découpé en deux lexèmes par l'analyseur. `elu20476` est
  -- un lexème et un seul, et aucun mot français ne lui ressemble.
  jeton     text NOT NULL UNIQUE,
  prenom    text NOT NULL,
  nom       text NOT NULL,
  -- La requête de reconnaissance, précalculée. C'est une recherche de PHRASE :
  -- le prénom doit précéder immédiatement le nom. « Laurent NUNEZ » correspond,
  -- « Laurent Martin … Pierre Nunez » non. C'est ce qui fait la différence entre
  -- reconnaître une personne et croiser deux mots.
  requete   tsquery NOT NULL,
  -- Ce qui vaut à cette personne d'être dans le thésaurus, pour qu'on puisse
  -- expliquer une présence et refaire la sélection.
  mandats   text[] NOT NULL,
  CONSTRAINT jeton_forme CHECK (jeton ~ '^elu[0-9]+$')
);

COMMENT ON TABLE ref.elu_recherche IS
  'Thésaurus des élus : les graphies d''un nom se ramènent à un jeton canonique. '
  'Implémenté en table et non en dictionnaire `thesaurus` — un dictionnaire se '
  'configure par un fichier, ce qui n''est pas déployable sur base gérée.';
COMMENT ON COLUMN ref.elu_recherche.requete IS
  'Recherche de PHRASE (prénom suivi du nom), pas conjonction de deux mots : '
  'c''est la différence entre reconnaître une personne et croiser deux lexèmes.';

-- L'association acte -> personne, telle que la reconnaissance l'établit.
--
-- Ce n'est PAS une affirmation sur la personne. Le Journal officiel ne porte
-- aucune date de naissance : un acte qui nomme « Laurent NUNEZ » nomme quelqu'un
-- de ce nom, et le thésaurus dit seulement que nous connaissons un élu ainsi
-- nommé. D'où la colonne `homonymes`, qui dit combien de personnes de notre base
-- portent ce nom — au-delà de 1, la citation est ambiguë et doit être présentée
-- comme telle.
CREATE TABLE jo.acte_elu (
  texte_id    text   NOT NULL REFERENCES jo.texte(id) ON DELETE CASCADE,
  person_id   bigint NOT NULL REFERENCES core.person(id) ON DELETE CASCADE,
  sections    text[] NOT NULL,
  homonymes   smallint NOT NULL,
  method_version text NOT NULL,
  PRIMARY KEY (texte_id, person_id)
);

COMMENT ON TABLE jo.acte_elu IS
  'Actes du Journal officiel qui nomment un élu de notre base. Reconnaissance '
  'par phrase prénom + nom : dit qu''un nom apparaît, pas que c''est cette '
  'personne. Voir la colonne homonymes.';

CREATE INDEX acte_elu_person_idx ON jo.acte_elu (person_id);

-- ---------------------------------------------------------------------------
-- Le second index plein texte : celui des élus.
--
-- Il est SÉPARÉ de celui du contenu, et c'est le point. L'index du contenu
-- répond à « quels actes parlent de retraites » ; celui-ci répond à « quels
-- actes concernent cette personne ». Les mêmes lexèmes ne servent pas les deux
-- questions, et un index unique répondrait mal aux deux.
--
-- La configuration est `simple` et non `fr` : un jeton canonique ne doit ni être
-- désuffixé ni passer par une liste de mots vides. `elu:20476` doit rester
-- `elu:20476`.
CREATE MATERIALIZED VIEW jo.recherche_elu AS
  SELECT a.texte_id,
         to_tsvector('simple', string_agg(e.jeton, ' ')) AS vecteur,
         count(*)::int AS nb_elus,
         max(a.homonymes) AS homonymes_max
    FROM jo.acte_elu a
    JOIN ref.elu_recherche e ON e.person_id = a.person_id
   GROUP BY a.texte_id;

CREATE UNIQUE INDEX recherche_elu_pk ON jo.recherche_elu (texte_id);
CREATE INDEX recherche_elu_gin ON jo.recherche_elu USING gin (vecteur);

COMMENT ON MATERIALIZED VIEW jo.recherche_elu IS
  'Index plein texte des élus cités, un jeton canonique par personne. '
  'Configuration `simple` : un identifiant ne se désuffixe pas.';

-- ---------------------------------------------------------------------------
-- La même donnée, indexée par TRIGRAMMES, pour comparaison.
--
-- pg_trgm découpe le texte en suites de trois caractères et indexe celles-ci.
-- Il ne connaît ni la langue, ni les mots, ni les accents : il compare des
-- formes. Cela lui donne ce que la recherche plein texte n'a pas — la tolérance
-- aux fautes de frappe et aux préfixes — et lui retire ce qu'elle a : le
-- désuffixage, les mots vides, la recherche de phrase.
--
-- La comparaison porte sur les TITRES et non sur le corps : un index trigramme
-- sur 2,9 Go de texte pèserait plus que la table. C'est déjà un résultat.
CREATE MATERIALIZED VIEW jo.recherche_titre AS
  SELECT t.id AS texte_id,
         coalesce(t.date_texte, t.date_publi) AS date_acte,
         t.nature,
         coalesce(t.titre_complet, t.titre, '') AS titre,
         to_tsvector('fr', coalesce(t.titre_complet, t.titre, '')) AS vecteur
    FROM jo.texte t
   WHERE coalesce(t.titre_complet, t.titre, '') <> '';

CREATE UNIQUE INDEX recherche_titre_pk ON jo.recherche_titre (texte_id);
CREATE INDEX recherche_titre_gin ON jo.recherche_titre USING gin (vecteur);
CREATE INDEX recherche_titre_trgm ON jo.recherche_titre USING gin (titre gin_trgm_ops);

COMMENT ON MATERIALIZED VIEW jo.recherche_titre IS
  'Les titres des actes, indexés DEUX FOIS : par lexèmes (GIN sur tsvector) et '
  'par trigrammes (GIN sur gin_trgm_ops). Sert à mesurer ce que chaque méthode '
  'sait faire et ce qu''elle coûte, sur exactement la même donnée.';

-- +goose Down
DROP MATERIALIZED VIEW jo.recherche_titre;
DROP MATERIALIZED VIEW jo.recherche_elu;
DROP TABLE jo.acte_elu;
DROP TABLE ref.elu_recherche;
