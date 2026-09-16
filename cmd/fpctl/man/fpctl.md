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

**fpctl** *commande* [options]

**fpctl help** [*commande*]

# DESCRIPTION

**fpctl** assemble en un seul exécutable l'ingestion des données, leur
vérification et la construction du site statique. Chaque commande reste le
binaire qui existait déjà (**cmd/build**, **cmd/ingest**, ...), compilé à la
volée et lancé avec les mêmes options qu'avant : fpctl route, il ne
réimplémente rien.

# COMMANDES

**build**
:   Génère le site statique et le met en place. Voir **fpctl-build**(1).

**ingest**
:   Télécharge, archive et charge les jeux de données sources.
    Voir **fpctl-ingest**(1).

**verify**
:   Contrôles de cohérence des données chargées, avant publication.
    Voir **fpctl-verify**(1).

**sources**
:   Catalogue des sources de données ingérées. Voir **fpctl-sources**(1).

**docs**
:   Régénère les sections chiffrées des dossiers documentaires
    (docs/*.md). Voir **fpctl-docs**(1).

**help** [*commande*]
:   Affiche cette page, ou la page de manuel d'une commande précise.

# OPTIONS

Chaque commande transmet ses propres options telles quelles au binaire
qu'elle enveloppe — **-h** ou **--help** en première position ouvre la page
de manuel dédiée plutôt que de les transmettre.

# CONVENTIONS

fpctl doit être lancé depuis n'importe où **à l'intérieur** du dépôt : il
retrouve la racine par lui-même (via `go env GOMOD`) et y exécute les
commandes qu'il enveloppe, pour que leurs chemins par défaut
(**web/templates**, **docs**, **data**) restent corrects quel que soit le
répertoire de travail au moment de l'appel.

# VOIR AUSSI

**fpctl-build**(1), **fpctl-ingest**(1), **fpctl-verify**(1),
**fpctl-sources**(1), **fpctl-docs**(1)
