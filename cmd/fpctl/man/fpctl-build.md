---
title: FPCTL-BUILD
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-21
---

# NOM

fpctl-build - génère le site statique, en entier ou par section

# SYNOPSIS

**fpctl build site** [**-out** *répertoire*] [**-templates** *répertoire*]
[**-data** *répertoire*] [**-root** *préfixe*]
[**-max-scrutins** *n*] [**-cpuprofile** *fichier*]

**fpctl build** **scrutin**|**communes**|**identite**|**indicateurs**|**gouvernance**|**fiches**|**dossiers**
[**-j** *n*] [**-dry-run**] [mêmes options]

**fpctl build section** *nom* [**-j** *n*] [**-dry-run**] [mêmes options]

**fpctl build topic** *id* [**-j** *n*] [**-dry-run**] [mêmes options]

# DESCRIPTION

**fpctl build site** construit le site dans **\<out\>.construction/**, puis
le met en place d'un coup à la place de **\<out\>** : le domaine réel n'est
jamais servi à moitié reconstruit, et si la construction échoue, le site en
ligne n'est pas touché. C'est la SEULE construction complète — la seule
jamais mise en place automatiquement.

Avant de reconstruire une section coûteuse (communes/EPCI, scrutins), fpctl
build compare l'empreinte des données sources (calculée par **fpctl
ingest**, table **core.section_checksum**) et l'empreinte du code qui la
construit à celles de la construction précédente ; si les deux concordent,
la section est recopiée telle quelle plutôt que refaite. Si rien nulle part
n'a changé (données, code, gabarits, CSV éditoriaux, dossiers
documentaires), la commande le constate en quelques requêtes et ne
construit rien du tout.

# SECTIONS

Chaque section ne reconstruit qu'une partie du site — bien plus rapide pour
itérer sur une seule chose, sans attendre le reste. **internal/sitegen**
déclare chaque section (et chaque sujet de campagne) comme un nœud nommé
d'un graphe de dépendances (**internal/pipeline.Registre**, voir
**internal/sitegen/graphe.go**) : demander une section ne charge plus que ce
dont elle dépend réellement, la fermeture transitive de ce nœud — jamais la
totalité du site. Il n'existe plus de drapeau **-only** : chaque nom de
section est une vraie sous-commande, validée contre **sitegen.Sections()**/
**sitegen.Topics()** (voir aussi **fpctl list sections**), et les groupes
ci-dessous en couvrent les plus utiles à nommer ensemble :

**scrutin**
:   Les pages de scrutin.

**communes**
:   Les pages communes et EPCI.

**identite**
:   Candidats, partis, Assemblée, sources, Parlement européen, thèmes,
    Sénat.

**indicateurs**
:   Frise chronologique, dette, chômage, vieillesse, jeunesse, richesse,
    dividendes, sécurité, agriculture, protection sociale.

**gouvernance**
:   Qui décide, présidentielle 2027, gouvernement, collectivités,
    circonscriptions, argent public (les fonctions de la dépense).

**fiches**
:   Fiches personnes, candidats, organisations, référentiels, groupes
    parlementaires.

**dossiers**
:   Accueil, index des sujets de campagne, documents de méthode, index de
    recherche — pas chaque sujet pris individuellement, voir
    **fpctl build topic**.

**section** *nom*
:   N'importe quelle autre section qu'**internal/sitegen** connaît et
    qu'aucun groupe ci-dessus ne couvre déjà (voir **sitegen.Sections()**,
    ou **fpctl build section** sans argument pour la liste). Sans préalable
    déclaré pour ce nom précis, ingère le socle parlementaire complet par
    défaut.

**topic** *id*
:   Un sujet de campagne individuel, par son identifiant (**eau**,
    **fraude-fiscale**, **appareil-productif**... voir
    **sitegen.Topics()**, **internal/sitegen/sujets.go**, ou
    **fpctl build topic** sans argument pour la liste). Même défaut
    d'ingestion que **section** sans préalable déclaré.

Chaque groupe, section ou sujet ingère d'abord ce qu'il déclare nécessiter
(voir **ingestPrerequisites** dans **cmd/fpctl/build.go**) — TOUS ENSEMBLE
plutôt qu'un par un : la plupart des sources n'ont aucune dépendance
déclarée entre elles (**internal/ingest.RunSources**), donc tournent de
front jusqu'à **-j** ; nommer une seule source suffit pour toute sa chaîne
(**themes** entraîne **senat**, **europe**, **normalize**, **download** et
**partis**). Idempotent : relancer ne refait pas ce qui est déjà à jour.
**-dry-run** affiche le plan d'ingestion (vagues, concurrence) sans rien
ingérer ni construire ; **-j** *n* (par défaut 4) borne le nombre de
préalables indépendants exécutés de front.

Le site produit par une section, un groupe ou un sujet est **délibérément
incomplet** : jamais mis en place automatiquement, jamais ce que doit
servir le domaine réel — réservé à l'itération locale, à écrire dans un
**-out** distinct de celui servi en production.

# OPTIONS

**-out** *répertoire*
:   Répertoire de sortie. Par défaut **site**.

**-templates** *répertoire*
:   Gabarits HTML. Par défaut **web/templates**.

**-data** *répertoire*
:   Décisions éditoriales (CSV). Par défaut **data**.

**-root** *préfixe*
:   Préfixe d'URL, pour une prévisualisation locale sous un sous-chemin.

**-max-scrutins** *n*
:   Limite le nombre de pages scrutin construites (0 = toutes). Réservé à
    l'itération locale : une construction tronquée n'alimente jamais le
    cache de la section scrutins.

**-cpuprofile** *fichier*
:   Écrit un profil CPU pprof à ce chemin (diagnostic).

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1), **fpctl-list**(1)
