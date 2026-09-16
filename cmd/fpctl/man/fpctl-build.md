---
title: FPCTL-BUILD
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-build - génère le site statique et le met en place

# SYNOPSIS

**fpctl build** [**-out** *répertoire*] [**-templates** *répertoire*]
[**-data** *répertoire*] [**-root** *préfixe*]
[**-max-scrutins** *n*] [**-only** *liste*] [**-cpuprofile** *fichier*]

# DESCRIPTION

Construit le site dans **\<out\>.construction/**, puis le met en place d'un
coup à la place de **\<out\>** : le domaine réel n'est jamais servi à moitié
reconstruit, et si la construction échoue, le site en ligne n'est pas
touché.

Avant de reconstruire une section coûteuse (communes/EPCI, scrutins), fpctl
build compare l'empreinte des données sources (calculée par **fpctl
ingest**, table **core.section_checksum**) et l'empreinte du code qui la
construit à celles de la construction précédente ; si les deux concordent,
la section est recopiée telle quelle plutôt que refaite. Si rien nulle part
n'a changé (données, code, gabarits, CSV éditoriaux, dossiers
documentaires), la commande le constate en quelques requêtes et ne
construit rien du tout.

# OPTIONS

**-out** *répertoire*
:   Répertoire de sortie publié. Par défaut **site**.

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

**-only** *liste*
:   Limite les sections coûteuses reconstruites (**scrutin**,
    **communes**, ou les deux séparées par une virgule). Produit un site
    **délibérément incomplet** : jamais mis en place automatiquement,
    jamais ce que doit servir le domaine réel.

**-cpuprofile** *fichier*
:   Écrit un profil CPU pprof à ce chemin (diagnostic).

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1)
