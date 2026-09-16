---
title: FPCTL-SYNC
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-sync - envoie un répertoire local vers l'object store

# SYNOPSIS

**fpctl sync archive** [**-bucket** *nom*] [**-dir** *répertoire*]

**fpctl sync site** [**-bucket** *nom*] [**-dir** *répertoire*]

# DESCRIPTION

N'est pas le chemin de service normal aujourd'hui : le site est servi
depuis le disque local (**site/**) et l'archive lue depuis **raw/**. Ces
commandes existent pour évaluer l'alternative — un object storage
compatible S3, local via **fpctl provision store** (MinIO) ou distant —
sans improviser un script à chaque fois.

Un objet déjà présent dans le bucket, à la même taille, n'est pas renvoyé.

**archive**
:   Envoie chaque document de l'archive scellée. Bucket par défaut :
    **fp-archive**. Répertoire par défaut : **raw**.

**site**
:   Envoie chaque page du site généré par **fpctl build site**. Bucket par
    défaut : **fp-site**. Répertoire par défaut : **site**.

# OPTIONS

**-bucket** *nom*
:   Bucket de destination.

**-dir** *répertoire*
:   Répertoire local à envoyer.

# VARIABLES D'ENVIRONNEMENT

**FP_S3_ENDPOINT**, **FP_S3_ACCESS_KEY**, **FP_S3_SECRET_KEY**,
**FP_S3_USE_SSL** — point d'accès et identifiants de l'object store. Sans
elles, fpctl parle au MinIO local par défaut (**fpctl provision store**).

# VOIR AUSSI

**fpctl**(1), **fpctl-provision**(1), **fpctl-build**(1)
