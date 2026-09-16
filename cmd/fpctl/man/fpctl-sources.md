---
title: FPCTL-SOURCES
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-sources - catalogue des sources de données ingérées

# SYNOPSIS

**fpctl sources** [**-out** *fichier*] [**-raw-root** *répertoire*]

# DESCRIPTION

Écrit un catalogue JSON de toutes les sources ingérées : éditeur, licence,
cadence de rafraîchissement attendue, et pour chacune la dernière
exécution d'ingestion connue (**raw.fetch_run** : réussie, en échec, date,
statistiques).

Chaque champ vient d'une table (**raw.source**, **raw.fetch_run**,
**raw.retrieval**, **raw.document**) ou d'une introspection des tables
**core**/**ref** qui portent un **source_id** — jamais d'une valeur recopiée
à la main, pour ne jamais dériver de l'état réel de la base.

# OPTIONS

**-out** *fichier*
:   Fichier JSON à écrire. Par défaut **docs/catalogue-sources.json**.

**-raw-root** *répertoire*
:   Racine locale de l'archive scellée. Par défaut **raw**.

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1)
