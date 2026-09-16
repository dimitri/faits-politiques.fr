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

**fpctl verify**

# DESCRIPTION

Exécute les contrôles portant sur les **données chargées** — un écart
signale une erreur d'ingestion, jamais une règle de schéma (celles-là sont
dans **db/tests/**). Chaque contrôle rapporte un nombre d'anomalies ; une
vérification passe quand ce nombre est zéro.

Les contrôles de volume (un minimum de lignes attendu) existent parce
qu'un contrôle de concordance passe trivialement sur une table vide : une
porte de publication qui s'ouvre sur une base vide est pire qu'inutile.

Ne prend aucune option ; le code de sortie est non nul si une anomalie est
trouvée, pour s'intégrer à un pipeline de publication.

# VOIR AUSSI

**fpctl**(1), **fpctl-ingest**(1), **fpctl-build**(1)
