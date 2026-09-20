---
title: FPCTL
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl - chaîne de construction de faits-politiques.fr

# SYNOPSIS

**fpctl** *verbe* *nom* [options]

**fpctl help** [*verbe*]

# DESCRIPTION

**fpctl** assemble en un seul exécutable l'ingestion des données, leur
vérification et la construction du site statique. L'arbre de commandes suit
un seul principe, comme **git** : chaque verbe s'applique à un nom —
**fpctl build site**, jamais **fpctl build** seul ni **fpctl site**.

La plupart des verbes routent directement vers le paquet **internal/**
correspondant ; **build site** reste un binaire séparé, compilé à la volée.
Dans les deux cas, fpctl ne réimplémente rien.

# VERBES

**build site** [options] | **build** **scrutin**|**communes**|**reste**
:   Génère le site statique et le met en place, en entier ou par section
    (pour itérer localement). Voir **fpctl-build**(1).

**ingest data** [options]
:   Télécharge, archive et charge les jeux de données sources.
    Voir **fpctl-ingest**(1).

**verify data**
:   Contrôles de cohérence des données chargées, avant publication.
    Voir **fpctl-verify**(1).

**list sources** | **list connectors** | **list stats**
:   Catalogue des sources, des connecteurs, ou résumé du contenu de la
    base. Voir **fpctl-list**(1).

**generate dossiers** | **generate bulletin**
:   Régénère les sections chiffrées des dossiers documentaires
    (docs/*.md). Voir **fpctl-generate**(1).

**provision db** | **provision store**
:   Démarre Postgres ou l'object store local (docker compose).
    Voir **fpctl-provision**(1).

**sync archive** | **sync site**
:   Envoie l'archive ou le site généré vers l'object store.
    Voir **fpctl-sync**(1).

**help** [*verbe*]
:   Affiche cette page, ou la page de manuel d'un verbe précis.

# OPTIONS

Chaque nom transmet ses propres options telles quelles au paquet ou au
binaire qu'il route — **-h** ou **--help** en première position ouvre la
page de manuel dédiée plutôt que de les transmettre.

# CONVENTIONS

fpctl doit être lancé depuis n'importe où **à l'intérieur** du dépôt : il
retrouve la racine par lui-même (via `go env GOMOD`) et s'y place avant
toute chose, pour que les chemins par défaut des commandes routées
(**web/templates**, **docs**, **data**, **raw**) restent corrects quel que
soit le répertoire de travail au moment de l'appel.

# VOIR AUSSI

**fpctl-build**(1), **fpctl-ingest**(1), **fpctl-verify**(1),
**fpctl-list**(1), **fpctl-generate**(1), **fpctl-provision**(1),
**fpctl-sync**(1)
