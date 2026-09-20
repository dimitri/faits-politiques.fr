---
title: FPCTL-BUILD
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-20
---

# NOM

fpctl-build - génère le site statique, en entier ou par section

# SYNOPSIS

**fpctl build site** [**-out** *répertoire*] [**-templates** *répertoire*]
[**-data** *répertoire*] [**-root** *préfixe*]
[**-max-scrutins** *n*] [**-cpuprofile** *fichier*]

**fpctl build** **scrutin**|**communes**|**reste** [**-dry-run**] [mêmes options]

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

**fpctl build scrutin**, **fpctl build communes** et **fpctl build reste**
ne reconstruisent qu'une partie du site — bien plus rapide pour itérer sur
une seule section, sans attendre le reste. **reste** est tout ce que
scrutin et communes ne couvrent pas (accueil, dossiers, thèmes,
gouvernement, budget...).

Chacune ingère d'abord ce qu'elle déclare nécessiter (**scrutin** :
**normalize**, **exposes** ; **communes** : **normalize**, **communes**,
**associations** ; **reste** : **normalize** — voir **ingestPrealables**
dans **cmd/fpctl/build.go**), idempotent : relancer ne refait pas ce qui
est déjà à jour, et **normalize** résout lui-même ses propres préalables
(voir **fpctl-ingest**(1), LE SOCLE PARLEMENTAIRE). **-dry-run** affiche
ces préalables sans rien ingérer ni construire.

Le site produit par une section est **délibérément incomplet** : jamais mis
en place automatiquement, jamais ce que doit servir le domaine réel —
réservé à l'itération locale, à écrire dans un **-out** distinct de celui
servi en production.

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

**fpctl**(1), **fpctl-ingest**(1)
