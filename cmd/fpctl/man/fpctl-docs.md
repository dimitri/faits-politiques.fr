---
title: FPCTL-DOCS
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-docs - régénère les sections chiffrées des dossiers documentaires

# SYNOPSIS

**fpctl docs sections-dossiers**

**fpctl docs figure-bulletin**

# DESCRIPTION

Les documents sous **docs/*.md** sont écrits à la main, mais certaines de
leurs sections doivent rester exactement synchrones avec la base — une
citation, un chiffre, une figure. Ces générateurs réécrivent le texte entre
deux marqueurs dédiés à chaque exécution ; le reste du document n'est
jamais touché.

**sections-dossiers**
:   Réécrit, dans chaque **docs/\<dossier\>.md**, les sections contexte,
    enjeux, cadre, contrôles et évaluations, situation, tirées de
    **ref.fait_dossier** — et, pour les dossiers qui suivent des missions
    de l'État, le tableau des crédits par programme.

**figure-bulletin**
:   Réécrit, dans **docs/cotisations-et-droits.md**, la figure « D'une
    fiche de paie aux caisses », tirée des vues **derived.bulletin_\***.

# VOIR AUSSI

**fpctl**(1)
