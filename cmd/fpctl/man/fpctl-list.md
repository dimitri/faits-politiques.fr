---
title: FPCTL-LIST
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-list - liste une collection (sources, connecteurs, statistiques)

# SYNOPSIS

**fpctl list sources** [**-out** *fichier*] [**-raw-root** *répertoire*]

**fpctl list connectors**

**fpctl list stats**

# DESCRIPTION

**sources**
:   Écrit un catalogue JSON de toutes les sources ingérées : éditeur,
    licence, cadence de rafraîchissement attendue, et pour chacune la
    dernière exécution d'ingestion connue (**raw.fetch_run** : réussie, en
    échec, date, statistiques).

    Chaque champ vient d'une table (**raw.source**, **raw.fetch_run**,
    **raw.retrieval**, **raw.document**) ou d'une introspection des tables
    **core**/**ref** qui portent un **source_id** — jamais d'une valeur
    recopiée à la main, pour ne jamais dériver de l'état réel de la base.

    **-out** *fichier*
    :   Fichier JSON à écrire. Par défaut **docs/catalogue-sources.json**.

    **-raw-root** *répertoire*
    :   Racine locale de l'archive scellée. Par défaut **raw**.

**connectors**
:   Liste les fonctions **Ingest\*** trouvées sous **internal/**, groupées
    par paquet. Le type de connecteur n'est pas déclaré dans un registre à
    part : cherché directement dans le code, pour ne jamais dériver de ce
    qui existe réellement.

**stats**
:   Résumé du contenu de la base : nombre de tables, lignes estimées
    (**n_live_tup**, pas un **COUNT(\*)** exact) et taille sur disque, par
    schéma applicatif (**core**, **ref**, **geo**, **raw**, **derived**...),
    puis les dix tables les plus lourdes tous schémas confondus.

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1)
