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

**fpctl list deps** [*nom*] [**--json**] [**--pages**]

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

**deps** [*nom*] [**--json**] [**--pages**]
:   Le graphe de dépendances du socle parlementaire (**download**,
    **partis**, **normalize**, **carto**, **senat**, **europe**,
    **themes** — voir **fpctl-ingest**(1), LE SOCLE PARLEMENTAIRE) vient du
    code (**internal/ingest**, **Source.Dependances**) : cette commande n'a
    besoin d'aucune base pour l'afficher. Si une base est joignable et
    migrée, elle enrichit chaque étape de sa dernière exécution réussie et
    de ce qu'elle pèse — jamais l'inverse, la base n'est qu'un REFLET
    républié par **internal/pipeline** ; sinon un avertissement le dit et
    le graphe s'affiche quand même, sans ces deux colonnes.

    Chaque nœud s'affiche par sa vraie commande (**fpctl ingest parlement
    download**, **fpctl build communes**), jamais un nom nu : deux
    commandes différentes peuvent partager un nom (la section **communes**
    de **fpctl build** et la source **collectivites communes** de **fpctl
    ingest**) — la carte interne ne les confond pas, l'affichage non plus.

    C'est un graphe orienté acyclique, pas un arbre (**normalize** a deux
    parents, **senat** et **europe**), à plusieurs racines (**carto** et
    **themes**, dans le socle complet). Rendu comme un arbre (**├──**,
    **└──**, **│**) une fois déroulé : une étape partagée réapparaît sous
    chacun de ses parents plutôt que d'être fusionnée en un seul nœud.

    Sans argument, deux arbres : le socle parlementaire, puis — sous
    l'en-tête « pages du site (fpctl build) » — chaque section de **fpctl
    build** (voir **fpctl-build**(1), SECTIONS) comme racine de ses
    préalables d'ingestion (voir **fpctl-build**(1), SECTIONS), avec le
    total à télécharger et à charger en base pour l'amener à jour depuis
    rien (dépendances comprises, chacune comptée une seule fois même si
    plusieurs chemins y mènent). Avec le nom d'une des sept étapes du
    socle, cette étape seule et sa chaîne de dépendances. Avec le nom d'une
    section de **fpctl build**, cette section seule, dans la même forme.
    Échoue avec la liste des noms valides sur tout autre nom.

    **--json**
    :   Écrit les nœuds concernés à plat (un objet par nœud : **nom**,
        **type** — **etape** ou **page** —, **commande**, **description**,
        **depend_de**, **archive_octets**/**base_octets** propres au nœud,
        et pour une page **archive_octets_transitif**/
        **base_octets_transitif**, le total dépendances comprises) plutôt
        que l'arbre déroulé.

    **--pages**
    :   N'affiche que le second arbre (les pages du site et leurs
        préalables), sans argument.

    **Tailles** : les octets archivés viennent de **raw.source.etape**
    (écrit par **internal/archive.Archive.Etape** à chaque ingestion via
    **fpctl ingest \<source|catégorie\>** — pas **fpctl ingest all**, la
    chaîne historique, qui n'étiquette pas ses sources) : un reflet
    générique qui couvre n'importe quelle étape du catalogue, pas
    seulement les sept du socle — c'est ce qui permet de chiffrer le coût
    d'ingestion d'une page entière. Les octets en base restent une liste de
    tables tenue à la main (**cmd/fpctl/list.go**, **composantesEtape**),
    à ce jour limitée aux sept étapes du socle.

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1)
