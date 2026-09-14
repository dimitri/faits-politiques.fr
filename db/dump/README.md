# Sauvegardes de la base

Ce répertoire est **ignoré par git** : un dump de cette base pèse plusieurs
gigaoctets, et il se refabrique. Ce fichier-ci est la seule chose qui y est
versionnée.

## Pourquoi un dump plutôt qu'une copie du volume

Le répertoire de données de PostgreSQL **n'est pas portable entre images**. Le
volume actuel a été initialisé par `postgis/postgis:17-3.5-alpine`, bâtie sur
musl ; la nouvelle image est bâtie sur Debian, donc sur glibc. Les collations,
et donc l'ordre des index, ne sont pas les mêmes. Copier le volume produirait
une base qui démarre et qui répond faux.

`pg_dump -Fc` produit un flux logique : des ordres SQL et des données, sans
dépendance à la bibliothèque C de l'hôte. C'est le seul chemin correct d'une
image à l'autre.

## Refaire le dump

```bash
make db-dump
```

## Restaurer dans l'image neuve

```bash
make db-restore
```

Mesuré sur la base complète (16 Go, dump de 1,26 Go) :

| | Durée |
|---|---|
| `pg_dump -Fc` | 321 s |
| `pg_restore -j 4`, vecteur en colonne générée | 2 078 s |
| `pg_restore -j 4`, vecteur en vue matérialisée | 1 269 s |

Et lors de la bascule réelle vers l'image Debian, le 14 septembre 2026, sur une
base passée entre-temps à 20 Go :

| | Mesure |
|---|---|
| `pg_dump -Fc` | 325 s, 1 388 775 797 octets |
| relecture intégrale de l'archive | 34 s |
| `make db-restore` | **1 045 s**, code 0 |
| décomptes exacts des 199 tables et vues | **identiques** |
| taille de la base restaurée | 13 Go, contre 20 Go avant — tables et index reconstruits sans leur gonflement |

`pg_restore -j` parallélise la restauration, et il a besoin d'un **fichier** —
il ne sait pas paralléliser depuis un tube. D'où le montage en lecture seule du
répertoire `db/dump` dans le conteneur, plutôt qu'une copie de 1,3 Go.

L'essentiel du temps part dans le `REFRESH` des vues de recherche du corpus du
Journal officiel, qui recalcule 3,8 millions de vecteurs : `pg_dump` n'emporte
que la définition d'une vue matérialisée, pas ses données.

**Après la restauration, le script d'extensions est rejoué.** Le dump ne porte
que les extensions de la base source : une base venue de l'ancienne image
arrive sans `vector` ni `rum`.
