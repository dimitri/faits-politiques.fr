---
title: FPCTL-INGEST
section: 1
header: Manuel fpctl
footer: faits-politiques.fr
date: 2026-09-19
---

# NOM

fpctl-ingest - télécharge, archive et charge les jeux de données sources

# SYNOPSIS

**fpctl ingest all**

**fpctl ingest** *catégorie*

**fpctl ingest** *catégorie* **all**

**fpctl ingest** *catégorie* *source*

**fpctl ingest migrate**

**fpctl ingest deps**

# DESCRIPTION

Chaque source est rangée dans une catégorie plutôt qu'exposée comme une
valeur d'un flag plat — voir **CATÉGORIES** ci-dessous. Le catalogue
(**internal/ingest/catalogue.go**) reste la référence unique des noms et
des descriptions : cette page en résume la forme, pas le détail, qui
s'obtient à jour avec :

    fpctl ingest -h                    la liste des catégories
    fpctl ingest <catégorie> -h        les sources d'une catégorie

**fpctl ingest all** recharge le socle habituel — pas littéralement chaque
source du catalogue : plusieurs sont délibérément hors chaîne par défaut
(coûteuses, ponctuelles, ou exigeant une clé ou un binaire particulier).
**fpctl ingest** *catégorie* **all** est plus large : toutes les sources de
cette catégorie, y compris ce qu'elle a de plus coûteux — une décision
explicite du côté de qui la lance, pas un oubli du côté de fpctl.

Chaque téléchargement passe par l'archive scellée (**internal/archive**) :
un document est identifié par l'empreinte de ses octets, jamais écrasé,
toute récupération est datée — voir docs/perimetre.md §2.5.

# LE SOCLE PARLEMENTAIRE

Sept sources de la catégorie **parlement** (**download**, **partis**,
**normalize**, **carto**, **senat**, **europe**, **themes**) ont leurs
dépendances déclarées et vérifiées (**internal/pipeline**) — le reste du
catalogue, non encore audité, garde l'ordre implicite qu'il a toujours eu.
Demander l'une de ces sept résout et exécute d'abord ce qu'elle exige :
**fpctl ingest parlement normalize** sur une base neuve enchaîne
**download** puis **partis** sans qu'on ait à les nommer. **--dry-run**
affiche le plan par vagues sans rien exécuter ; **-j** *n* exécute jusqu'à
*n* étapes indépendantes d'une même vague de front (**carto**, **senat** et
**europe** ne dépendent que de **normalize**, jamais l'une de l'autre).
Les deux options échouent plutôt que de s'appliquer en silence en dehors de
ce socle. La topologie exécutée est republiée en base à chaque appel qui le
touche — **fpctl ingest deps** l'affiche, dépendances et dernière exécution
réussie de chaque étape.

# CATÉGORIES

**parlement**
:   Assemblée nationale, Sénat, Parlement européen, votes, textes,
    élections nationales.

**collectivites**
:   Communes, intercommunalités, élections locales, finances locales.

**budget**
:   Finances publiques, aides aux entreprises, paie.

**social**
:   Santé, vieillesse, jeunesse, pauvreté/richesse, éducation, logement,
    justice, culture.

**environnement**
:   Eau, écologie, agriculture, géographie hydrologique.

**economie**
:   Appareil productif, commerce extérieur, commande publique.

**transparence**
:   Intégrité publique, fiscalité comparée, souveraineté numérique, moteur
    « dossiers/faits ».

**international**
:   Comparaisons internationales et géopolitique.

**systeme**
:   Infrastructure partagée, pas propre à un dossier (migrations, contours
    de base, empreintes de section, médias).

# OPTIONS

**--raw** *répertoire*
:   Répertoire de l'archive scellée. Par défaut **raw**.

**--migrations** *répertoire*
:   Répertoire des migrations. Par défaut **db/migrations**.

**--dry-run**
:   Affiche l'ordre d'exécution par vagues sans rien exécuter — le socle
    parlementaire seulement (voir **LE SOCLE PARLEMENTAIRE**) ; refuse
    plutôt que d'exécuter en silence pour le reste du catalogue.

**-j** *n*
:   Jusqu'à *n* étapes indépendantes (même vague) exécutées de front — le
    socle parlementaire seulement. 1 par défaut (séquentiel).

# VOIR AUSSI

**fpctl**(1), **fpctl-build**(1), **fpctl-verify**(1)
