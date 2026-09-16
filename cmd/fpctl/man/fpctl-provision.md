---
title: FPCTL-PROVISION
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-provision - démarre un service local nécessaire au développement ou à la CI

# SYNOPSIS

**fpctl provision db**

**fpctl provision store**

# DESCRIPTION

Les deux services locaux dont ce dépôt a besoin, démarrés par **docker
compose** (voir **docker-compose.yml**) — fpctl n'y ajoute que l'attente du
healthcheck et l'idempotence déjà offertes par compose.

**db**
:   Démarre Postgres (service **db**). Au premier démarrage d'un volume
    neuf : initdb puis les extensions (PostGIS comprise), jusqu'à 180s.

**store**
:   Démarre un MinIO local, compatible S3 (service **minio**). API S3 sur
    le port **9090**, console web sur le port **9091**. Identifiants et
    point d'accès par défaut : voir **internal/objectstore**. Sert à
    **fpctl sync archive** et **fpctl sync site**.

# VOIR AUSSI

**fpctl**(1), **fpctl-sync**(1)
