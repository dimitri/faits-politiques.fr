---
title: FPCTL-DUMP
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-21
---

# NOM

fpctl-dump - exporte ou restaure le périmètre CI (schéma mv + TablesDirectes)

# SYNOPSIS

**fpctl dump ci** [**-out** *fichier*] [**-upload**] [**-bucket** *nom*] [**-key** *clé*]

**fpctl dump restore** [**-fichier** *fichier*] [**-download**] [**-bucket** *nom*] [**-key** *clé*]

# DESCRIPTION

Le périmètre CI (**internal/matview.Perimetre()**) est le schéma **mv** en
entier plus les tables suivies dans **internal/matview.TablesDirectes** —
tout ce dont **fpctl build site** a besoin, sans les ~10 Go de **core/ref/
geo/raw** ni la centaine de connecteurs d'ingestion qui les remplissent.
L'objectif : valider une reconstruction complète du site en CI à partir
d'un seul fichier restauré dans une base vide.

**fpctl dump ci** actualise d'abord chaque matvue
(**internal/matview.ActualiserToutes** — un REFRESH sauté si rien n'a
changé), vérifie ensuite que ce périmètre couvre bien tout ce que le
dernier **fpctl build site** a RÉELLEMENT lu (**core.sitegen_table_usage**,
mesuré à l'exécution par **internal/sitegen** — voir MÉTHODE plus bas), puis
recopie chaque matvue et chaque TableDirecte dans un schéma jetable
(**ci**) sous forme de tables ordinaires, colonnes d'un type énuméré
propre à **core** (**mandate_type**, **organization_kind**...) converties
en **text** au passage, puis exporte ce seul schéma avec **pg_dump -Fc**.
Deux raisons empêchent de dumper **mv** directement :

- **pg_restore restaure le contenu d'une matvue par REFRESH MATERIALIZED
  VIEW**, jamais par COPY — cela rejouerait la requête source à la
  restauration, donc exigerait les grandes tables **core/ref** que ce
  périmètre existe pour éviter.
- **pg_dump ne suit pas les types personnalisés d'une colonne** quand la
  sélection se fait par table plutôt que par schéma entier — sans
  conversion, **CREATE TABLE** échouerait à la restauration sur un type
  introuvable.

L'actualisation préalable des matvues n'est pas cosmétique : sans elle,
lancer **fpctl dump ci** hors de la séquence **fpctl ingest default** (qui
actualise déjà les matvues en dernière étape) capturerait silencieusement le
contenu d'un cycle d'ingestion antérieur, jamais signalé comme périmé.

**fpctl dump restore** restaure ce fichier dans la base ciblée par
**DATABASE_URL** (**pg_restore --clean --if-exists --no-owner**), puis
remet chaque table à sa place réelle (**core.mandate**, **mv.
person_actif**...) — la base cible n'a besoin d'aucun schéma préexistant,
seulement d'être vide et joignable. **fpctl build site** ne fait ensuite
aucune différence entre une vraie matvue et cette table ordinaire : ni
REFRESH ni écriture n'ont lieu pendant une construction.

**-upload**/**-download** parlent à un object storage compatible S3 par les
mêmes variables d'environnement que **fpctl sync** (voir
**internal/objectstore**) — par défaut le préfixe **ci/** du bucket
**fp-archive** déjà utilisé pour l'archive scellée, jamais un second bucket
à sécuriser séparément pour un fichier de quelques dizaines de méga-octets.

# MÉTHODE

**fpctl dump ci** refuse désormais l'export lui-même si une page a besoin
d'une table que ce périmètre ne couvre pas : il compare **core.
sitegen_table_usage** (ce qu'un **pgx.QueryTracer** a observé pendant le
dernier **fpctl build site** — jamais deviné par relecture du code, voir
**fpctl-list(1)**, **deps**) à **internal/matview.Perimetre()**, et liste
chaque table manquante avec les pages qui en ont besoin. Pour fermer une
lacune signalée ainsi :

1. Ajouter la table à **internal/matview** — une TableDirecte si elle
   reste petite, une matvue si son volume (comme **ref.unite_legale**,
   13 millions de lignes) rendrait le périmètre CI trop lourd pour une
   seule statistique.
2. **fpctl build site** une fois pour réécrire **core.sitegen_table_usage**
   avec la page corrigée, puis **fpctl dump ci** de nouveau.

Si **core.sitegen_table_usage** n'existe pas encore (migration pas encore
appliquée) ou si aucune construction n'a encore tourné depuis, ce contrôle
ne s'applique pas — repli sur la méthode manuelle d'avant ce contrôle :

1. **fpctl dump ci** puis **fpctl dump restore** dans une base de test.
2. **DATABASE_URL=... fpctl build site** contre cette base : une relation
   manquante y échoue explicitement, avec son nom.
3. Ajouter cette relation à **internal/matview**, recommencer depuis 1.

# OPTIONS

**-out** *fichier*
:   (dump ci) Fichier de sortie, format **pg_dump -Fc**. Défaut : **ci.dump**.

**-upload**
:   (dump ci) Envoie aussi le fichier vers l'object storage (**-bucket**/**-key**).

**-fichier** *fichier*
:   (dump restore) Fichier à restaurer. Défaut : **ci.dump**.

**-download**
:   (dump restore) Récupère d'abord le fichier depuis l'object storage
    (**-bucket**/**-key**) avant de le restaurer.

**-bucket** *nom*
:   Bucket source ou destination. Défaut : **fp-archive**.

**-key** *clé*
:   Clé source ou destination dans le bucket. Défaut : **ci/perimetre.dump**.

# VOIR AUSSI

**fpctl**(1), **fpctl-list**(1), **fpctl-ingest**(1), **fpctl-build**(1), **fpctl-sync**(1)
