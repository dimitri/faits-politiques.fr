-- Les extensions d'une base NEUVE.
--
-- Ce fichier n'est joué qu'une fois, sur un répertoire de données vide. Une
-- restauration par pg_restore recrée les extensions elle-même — le dump les
-- porte — mais une base fraîche doit pouvoir servir sans qu'on y pense.
--
-- Chaque ligne dit à quoi l'extension sert ici, pour qu'on sache laquelle
-- retirer le jour où elle ne sert plus.

-- Les contours administratifs, dessinés en SVG par la base (schéma geo).
CREATE EXTENSION IF NOT EXISTS postgis;

-- La recherche par voisinage dans le corpus du Journal officiel.
CREATE EXTENSION IF NOT EXISTS vector;

-- La recherche plein texte avec POSITIONS dans les listes d'occurrences : ce
-- que GIN ne fait pas, et qui rend la recherche de phrase et le classement par
-- pertinence sans accès au tas (D-051).
CREATE EXTENSION IF NOT EXISTS rum;

-- La recherche par trigrammes : la tolérance aux fautes de frappe, que le plein
-- texte ne sait pas offrir (docs/recherche-jo.md).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Le déaccentuage, employé par la configuration de recherche `fr`. Sans lui,
-- « Élysée » et « Elysee » sont deux mots différents.
CREATE EXTENSION IF NOT EXISTS unaccent;

-- Les contraintes d'exclusion temporelles sur les mandats : EXCLUDE USING gist
-- avec un person_id en égalité demande btree_gist.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Combiner un index GIN plein texte avec une égalité scalaire — chercher un mot
-- dans les actes d'une année donnée, par exemple.
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- Les empreintes, employées par l'archivage scellé (raw.document.sha256).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- L'observation de la charge. Demande shared_preload_libraries, positionné par
-- la commande du service dans docker-compose.yml.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
