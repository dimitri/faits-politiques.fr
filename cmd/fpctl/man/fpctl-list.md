---
title: FPCTL-LIST
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-list - liste une collection (sources, connecteurs, statistiques, graphe de dépendances)

# SYNOPSIS

**fpctl list sources** [**-out** *fichier*] [**-raw-root** *répertoire*]

**fpctl list connectors**

**fpctl list stats**

**fpctl list deps** [*nom*]

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

**deps** [*nom*]
:   Le graphe de dépendances du socle parlementaire (**download**,
    **partis**, **normalize**, **carto**, **senat**, **europe**,
    **themes** — voir **fpctl-ingest**(1), LE SOCLE PARLEMENTAIRE), tel que
    publié en base par **internal/pipeline** : dernière exécution réussie
    de chaque étape, sa taille (archive scellée pour ce qu'elle télécharge,
    tables **core**/**derived** pour ce qu'elle écrit), et ce dont elle
    dépend.

    Sans argument, les sept étapes. Avec le nom de l'une d'elles, cette
    étape seule et la chaîne complète de ce dont elle dépend,
    transitivement — chacune une seule fois, même si plusieurs chemins y
    mènent. Avec le nom d'une section de **fpctl build** (**scrutin**,
    **communes**, **reste**), ce qu'elle ingère d'abord (voir
    **fpctl-build**(1), SECTIONS) : chaque préalable qui appartient au
    socle est développé à son tour, un préalable hors du socle est
    simplement nommé avec sa description.

    Échoue avec la liste des noms valides si *nom* n'est ni l'un ni
    l'autre. Nécessite que **core.pipeline_etape** existe déjà (une
    migration récente) — sinon, lance d'abord **fpctl ingest migrate**.

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1)
