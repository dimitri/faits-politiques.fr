-- +goose Up
-- Le Journal officiel en entier, et sa recherche.
--
-- POURQUOI UN SCHÉMA À PART. Ce n'est ni `raw` ni `core`. Ce n'est pas `raw`,
-- qui garde les OCTETS scellés d'un document et son empreinte, pas leur contenu
-- déplié. Ce n'est pas `core`, qui ne contient que des faits normalisés et
-- vérifiés. C'est le corpus lui-même : 1,24 million d'actes de 1861 à 2025,
-- 3,8 millions de blocs de texte, 3,5 Go. On y cherche, on n'y affirme rien.
--
-- Les faits qu'on en tire — les nominations, la composition des gouvernements —
-- vivent dans `core` et portent leur statut de rapprochement.
CREATE SCHEMA IF NOT EXISTS jo;
COMMENT ON SCHEMA jo IS
  'Le Journal officiel déplié : sommaires, actes, blocs de texte. Corpus de '
  'recherche, pas source de faits — ce qui en est tiré passe par core avec son '
  'degré de certitude.';

-- Le sommaire d'un numéro : ce que le Journal officiel a publié ce jour-là.
CREATE TABLE jo.sommaire (
  id          text PRIMARY KEY,
  nature      text,
  titre       text,
  num         text,
  date_publi  date
);

-- L'acte. L'identifiant est celui de la DILA, qui est un permalien.
CREATE TABLE jo.texte (
  id            text PRIMARY KEY,
  nature        text,
  num           text,
  nor           text,
  date_publi    date,
  date_texte    date,
  titre         text,
  titre_complet text,
  ministere     text,
  origine_publi text
);

CREATE INDEX texte_date_idx ON jo.texte (coalesce(date_texte, date_publi));
CREATE INDEX texte_nature_idx ON jo.texte (nature) WHERE nature IS NOT NULL;

-- Le lien sommaire -> acte, tel que le sommaire le déclare.
CREATE TABLE jo.lien (
  sommaire_id text NOT NULL,
  texte_id    text NOT NULL,
  titre       text,
  ordre       integer NOT NULL,
  PRIMARY KEY (sommaire_id, texte_id, ordre)
);

-- Un bloc de texte, avec l'endroit d'où il vient.
--
-- La SECTION importe autant que le contenu. Le Journal officiel range son texte
-- en NOTICE (à qui le texte s'adresse), VISAS (les fondements juridiques),
-- DISPOSITIF (ce que l'acte édicte, et lui seul), ABRO (les abrogations), SM et
-- SIGNATAIRES. Chercher « retraite » dans les visas d'un arrêté ne dit pas que
-- l'arrêté porte sur les retraites : il cite un texte qui en parle.
CREATE TABLE jo.bloc (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  texte_id    text NOT NULL,
  ordre       integer NOT NULL,
  section     text NOT NULL,
  article_id  text,
  article_num text,
  contenu     text NOT NULL
);

-- La clé étrangère est déclarée ici, mais le connecteur la RETIRE le temps du
-- chargement et la remet ensuite. Ce n'est pas une négligence, c'est ce qui
-- rend le chargement possible en une seule traversée : les quatre tables sont
-- remplies EN PARALLÈLE par quatre flux COPY, et un bloc peut arriver avant
-- l'acte qu'il désigne. Vérifier au fil de l'eau imposerait un ordre entre les
-- flux, donc de les sérialiser. La vérifier à la fin coûte quatre secondes.
ALTER TABLE jo.bloc ADD CONSTRAINT bloc_texte_fk
  FOREIGN KEY (texte_id) REFERENCES jo.texte(id) ON DELETE CASCADE;
CREATE INDEX bloc_texte_idx ON jo.bloc (texte_id);

COMMENT ON CONSTRAINT bloc_texte_fk ON jo.bloc IS
  'Retirée pendant le chargement en masse, remise ensuite par le connecteur. '
  'La vérifier au fil de l''eau imposerait un ordre entre les flux COPY.';

-- ---------------------------------------------------------------------------
-- La recherche en français.
--
-- La configuration livrée avec PostgreSQL, `french`, sait désuffixer le
-- français mais ne sait pas que « Elysée » et « Elysee » sont le même mot. Or
-- le Journal officiel écrit les deux, et un siècle et demi de saisies — dont
-- des reprises de documents papier — garantit que l'accent est parfois absent.
-- Une recherche qui distingue « présidence » de « presidence » rate des actes
-- sans le dire, ce qui est la pire des façons de rater.
--
-- D'où une configuration qui fait passer chaque lexème par `unaccent` AVANT le
-- désuffixage. L'ordre compte : désuffixer puis déaccentuer laisserait des
-- radicaux différents pour le même mot.
CREATE TEXT SEARCH CONFIGURATION fr (COPY = french);

ALTER TEXT SEARCH CONFIGURATION fr
  ALTER MAPPING FOR hword, hword_part, word, asciiword, asciihword
  WITH unaccent, french_stem;

COMMENT ON TEXT SEARCH CONFIGURATION fr IS
  'Français sans accents : unaccent puis french_stem. « Elysée » et « Elysee » '
  'donnent le même lexème, ce que la configuration `french` livrée ne fait pas.';

-- OÙ METTRE LE VECTEUR : la mesure a tranché, et elle a tranché CONTRE
-- l'intuition qu'on a en regardant le chargement.
--
-- Une colonne `tsvector` générée ne peut pas diverger de son texte, et aucun
-- code applicatif ne peut l'oublier sur un INSERT. Un index fonctionnel évite
-- de la stocker. Les deux ont été mesurés sur ce corpus, dans cet ordre :
--
--   AU CHARGEMENT, l'index fonctionnel gagne de peu :
--     colonne générée    chargement 1 832 s + index   211 s = 2 043 s, +3 Go
--     index fonctionnel  chargement   187 s + index 1 687 s = 1 874 s
--   Le calcul de to_tsvector coûte le même prix des deux côtés, environ
--   1 650 s, et il est SÉRIEL dans les deux cas : COPY ne parallélise pas les
--   colonnes générées, et PostgreSQL 17 ne parallélise pas un index GIN.
--
--   À LA REQUÊTE, l'écart est de trois ordres de grandeur :
--     recherche de phrase, vecteur stocké        15 ms
--     recherche de phrase, index fonctionnel  50 057 ms
--     sur la même table, même requête :      1 807 ms contre 12,6 ms
--                                            pour une simple conjonction
--
-- LA RAISON. GIN ne stocke pas les positions des lexèmes. Une conjonction se
-- résout donc dans l'index seul ; une recherche de PHRASE ne le peut pas, et
-- doit vérifier l'adjacence sur chaque candidat. Avec une colonne stockée,
-- cette vérification LIT un vecteur ; avec un index fonctionnel, elle le
-- RECALCULE — sur des blocs qui font parfois plusieurs mégaoctets.
--
-- Un corpus se charge une fois et s'interroge indéfiniment. Les trois
-- gigaoctets et les vingt-huit minutes sont donc le bon prix, et la colonne
-- générée est restaurée après avoir été retirée à tort.
--
-- (C'est aussi le cas d'usage de l'extension RUM, qui range les positions DANS
-- les listes d'occurrences et rend phrase et classement sans accès au tas. Elle
-- n'est pas dans l'image et reste une piste.)
--
-- La configuration `fr` est nommée explicitement des deux côtés : une colonne
-- générée comme un index exigent une expression IMMUTABLE, et `to_tsvector(text)`
-- ne l'est pas — elle dépend de default_text_search_config.
ALTER TABLE jo.texte
  ADD COLUMN recherche tsvector
  GENERATED ALWAYS AS (to_tsvector('fr', coalesce(titre_complet, titre, ''))) STORED;

ALTER TABLE jo.bloc
  ADD COLUMN recherche tsvector
  GENERATED ALWAYS AS (to_tsvector('fr', contenu)) STORED;

CREATE INDEX texte_recherche_gin ON jo.texte USING gin (recherche);
CREATE INDEX bloc_recherche_gin  ON jo.bloc  USING gin (recherche);

COMMENT ON COLUMN jo.bloc.recherche IS
  'Colonne générée, cohérente avec `contenu` par construction. Stockée et non '
  'calculée par un index fonctionnel : une recherche de phrase sur expression '
  'recalcule le vecteur de chaque candidat, ce qui coûte 50 s là où la lecture '
  'du vecteur stocké en coûte 15 ms.';

-- +goose Down
DROP TABLE jo.bloc;
DROP TABLE jo.lien;
DROP TABLE jo.texte;
DROP TABLE jo.sommaire;
DROP TEXT SEARCH CONFIGURATION fr;
DROP SCHEMA jo;
