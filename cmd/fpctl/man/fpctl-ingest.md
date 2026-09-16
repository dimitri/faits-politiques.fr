---
title: FPCTL-INGEST
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-ingest - télécharge, archive et charge les jeux de données sources

# SYNOPSIS

**fpctl ingest** [**-only** *source*]

# DESCRIPTION

Sans **-only**, recharge toutes les sources dans l'ordre attendu par leurs
dépendances (référentiels avant ce qui les utilise, par exemple), puis
recalcule les empreintes de section que **fpctl build** compare avant de
décider de recopier ou reconstruire (**core.section_checksum**).

Chaque téléchargement passe par l'archive scellée (**internal/archive**) :
un document est identifié par l'empreinte de ses octets, jamais écrasé,
toute récupération est datée — voir docs/perimetre.md §2.5.

# OPTIONS

**-only** *source*
:   Ne recharge que cette source. Valeurs notables :

    **migrate**
    :   Applique les migrations de schéma en attente.

    **checksums**
    :   Recalcule seulement les empreintes de section
        (**core.section_checksum**) sans rien recharger — rarement
        nécessaire seul, une ingestion complète le fait déjà en dernière
        étape.

    *un connecteur*
    :   Le nom d'une source précise (communes, senat, hatvp,
        presidentielle, budget, macro, vieillesse, jeunesse, sante,
        rpps...). La liste complète, tenue à jour dans le code plutôt que
        recopiée ici, s'obtient avec :

            fpctl ingest -h

# VOIR AUSSI

**fpctl**(1), **fpctl-build**(1), **fpctl-verify**(1)
