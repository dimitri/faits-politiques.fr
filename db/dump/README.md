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

`pg_restore -j` parallélise la restauration ; l'essentiel du temps part dans la
reconstruction des index, en particulier les deux GIN plein texte du corpus du
Journal officiel.
