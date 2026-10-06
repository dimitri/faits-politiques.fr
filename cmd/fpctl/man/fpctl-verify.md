---
title: FPCTL-VERIFY
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-16
---

# NOM

fpctl-verify - contrôles de cohérence des données chargées

# SYNOPSIS

**fpctl verify data** [**all**]

# DESCRIPTION

Exécute les contrôles portant sur les **données chargées** — un écart
signale une erreur d'ingestion, jamais une règle de schéma (celles-là sont
dans **db/tests/**). Chaque contrôle rapporte un nombre d'anomalies ; une
vérification passe quand ce nombre est zéro.

Les contrôles de volume (un minimum de lignes attendu) existent parce
qu'un contrôle de concordance passe trivialement sur une table vide : une
porte de publication qui s'ouvre sur une base vide est pire qu'inutile.

Sans option, seuls les contrôles dont la source est couverte par
**fpctl ingest all** (le socle parlementaire et son supplément habituel)
tournent — le scope qui correspond à un ingest normal, celui que
**build-pr**/**build-full** rejouent. Une centaine d'autres sources du
catalogue restent délibérément hors de cette chaîne par défaut (coûteuses,
ponctuelles, ou exigeant une clé/un binaire particulier — voir
**fpctl-ingest**(1)) ; les contrôles qui en dépendent (empire colonial,
SIRENE, ports...) échoueraient systématiquement après un ingest ordinaire
s'ils tournaient quand même.

**fpctl verify data all** : tous les contrôles connus, sans filtrage — à
réserver au suivi d'un ingest plus large (**fpctl ingest** *catégorie*
**all**, catégorie par catégorie), jamais après un **fpctl ingest all**
ordinaire.

Le code de sortie est non nul si une anomalie est trouvée, pour s'intégrer
à un pipeline de publication.

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1), **fpctl-build**(1)
