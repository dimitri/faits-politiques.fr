# Enrichir une fiche de candidat : ce que les données ouvertes permettent

> **Méthode** · version 2 · 15 septembre 2026

Étude des sources, vérifiées fichier en main le 12 septembre 2026. L'objet visé est
double : une **frise de carrière** replacée sous les présidences successives, et
la **liste des mandats en cours**.

## 1. Ce qui manquait, et pourquoi

Avant ce chantier, `core.mandate` ne contenait que quatre types :

    DEPUTE            1 065    17e législature
    DEPUTE_EUROPEEN     200
    MINISTRE            145
    MAIRE            34 743    (chargé le jour même)

Aucun sénateur, aucun élu départemental ou régional, aucun conseiller
municipal — et surtout **aucune présidence**. Les présidents de la Ve
République vivaient dans `data/presidents.csv`, lu au moment du rendu : aucune
requête ne pouvait demander « quels mandats se sont déroulés sous telle
présidence ». Une frise a besoin des deux : la suite des mandats, et le repère
chronologique qui les situe.

## 2. Les sources retenues et chargées

### 2.1 RNE — tous les niveaux d'un coup

Le Répertoire national des élus publie **sept fichiers**, un par type de mandat,
tous de même structure (Licence Ouverte, millésime du 11 août 2026) :

| Fichier | Lignes | Type de mandat |
|---|---|---|
| conseiller municipal | 511 225 | `CONSEILLER_MUNICIPAL` |
| conseiller communautaire | 62 129 | `CONSEILLER_COMMUNAUTAIRE` |
| maire | 34 826 | `MAIRE` |
| conseiller départemental | 4 037 | `CONSEILLER_DEPARTEMENTAL` |
| conseiller régional | 1 745 | `CONSEILLER_REGIONAL` |
| député | 577 | `DEPUTE` |
| sénateur | 348 | `SENATEUR` |
| représentant au Parlement européen | 81 | `DEPUTE_EUROPEEN` |

C'est la seule source qui couvre tous les niveaux simultanément, et donc la
seule qui permette d'établir la liste des mandats actuels d'une personne sans
la recomposer source par source.

Deux limites à afficher partout :

- **Aucune nuance politique** (D-022). Le RNE dit qui, pas de quel bord.
- **Aucune profondeur historique.** Le RNE ne publie que la mandature en cours
  et **aucune date de fin**. La frise est donc, pour l'instant, une frise du
  présent : elle montre ce qui est en cours, pas ce qui a été.

### 2.2 Présidences de la République

Chargées depuis `data/presidents.csv` (source Élysée) comme des mandats de type
`PRESIDENT_REPUBLIQUE`, intérims du président du Sénat compris — les omettre
laisserait des trous, et un mandat tombant dans un trou n'aurait aucun contexte.

Deux vues en découlent (migration 0029) :

- `core.mandat_contexte` — chaque mandat, avec la ou les présidences qu'il
  traverse et la période d'intersection. Un mandat à cheval produit deux lignes :
  c'est le fait, pas un défaut.
- `core.mandat_actuel` — les mandats dont la période contient la date du jour.

**La concomitance n'est pas une imputation.** Un ministre n'est pas responsable
des actes du président, ni l'inverse. Toute page qui affiche la frise doit le
dire.

### 2.3 Réconciliation des identités

Une même personne arrivait en double : conseiller municipal au RNE, député chez
l'Assemblée. Sur les seules données déjà chargées, **189 triplets
(nom, prénom, date de naissance) étaient déjà en doublon**.

Le rapprochement se fait sur ce triplet **exact**, insensible aux accents et à
la casse, et sur rien d'autre. Sans date de naissance, aucun rapprochement n'est
tenté : deux homonymes restent deux personnes — c'est le sens de l'erreur le
moins grave, un mandat manquant valant mieux qu'un mandat attribué à quelqu'un
d'autre.

C'est ce rapprochement qui rend la frise possible : un candidat qui fut
conseiller municipal, puis député, puis ministre, est une personne et non trois.

## 3. Sources identifiées, non encore chargées

### 3.1 HATVP — la plus prometteuse

La Haute Autorité pour la transparence de la vie publique publie en open data :

- `liste.csv` — une ligne par déclarant : civilité, nom, type de mandat,
  **qualité** (« Adjoint au maire de Rouen », « Vice-Président de la Métropole
  du Grand Nancy »), département, lien vers la page nominative ;
- `declarations.xml` — les déclarations elles-mêmes, dont
  `activProfCinqDerniereDto` : **les activités professionnelles des cinq
  dernières années**, avec description, employeur et dates.

C'est la seule source publique qui donne le **parcours professionnel** d'un
responsable, et elle comble en partie l'absence de profondeur historique du RNE.

Une limite éthique à trancher avant de charger : ces déclarations contiennent
aussi le **patrimoine**. Le projet n'a aucune raison de le reprendre — il
documente ce que font les élus, pas ce qu'ils possèdent. La règle proposée est
de n'ingérer que les blocs « mandats » et « activités professionnelles », et de
ne jamais toucher aux déclarations de situation patrimoniale.

### 3.2 Mandats internes à l'Assemblée — déjà scellés, non exploités

`raw.record` contient déjà **29 702 mandats** de l'Assemblée, jamais normalisés
au-delà de `DEPUTE`. Leur ventilation par type d'organe :

    COMPER      commissions permanentes      ~17 000
    GP          groupes politiques             3 642
    ASSEMBLEE                                  3 773
    GA          groupes d'amitié              plusieurs milliers
    GE          groupes d'études
    DELEG       délégations
    ORGEXTPARL  organismes extraparlementaires
    PARPOL      partis politiques                435

Deux gisements immédiats, sans nouvelle source : les **commissions** (où un
député travaille vraiment) et surtout `PARPOL`, **le rattachement d'un député à
un parti, publié par l'Assemblée elle-même** — à confronter au rattachement
publié au JO, qui est aujourd'hui la seule affiliation partisane officielle
retenue par le périmètre.

**Chargé le 2026-09-12** : 28 441 appartenances. La profondeur est cependant
plus faible qu'espéré — 32 appartenances seulement commencent avant 2010, 2 431
avant 2017 (D-032). Le jeu couvre les mandats des députés ACTUELS, et peu
d'entre eux siégeaient dans les années 1990. L'apport se compte en années, pas
en décennies.

### 3.3 Sycomore — introuvable aux URL testées

La base historique des députés depuis 1789 donnerait la profondeur qui manque.
Le schéma la prévoit déjà (`AN_SYCOMORE` est admis comme identifiant de
personne), mais aucune des URL essayées sur `data.assemblee-nationale.fr` ne
répond. À chercher autrement avant de conclure qu'elle n'est pas diffusable.

Note : le jeu déjà chargé s'appelle `AMO30_tous_acteurs_tous_mandats_tous_organes_historique`,
mais son « historique » ne porte que sur la 17e législature.

## 4. Ce que la frise pourra montrer, et ce qu'elle ne pourra pas

**Pourra** : tous les mandats en cours d'une personne, tous niveaux confondus ;
les mandats nationaux connus depuis 1988 (Assemblée) et 2007 (ministres) ; la
présidence sous laquelle chacun s'est déroulé ; la durée d'intersection exacte.

**Ne pourra pas, en l'état** : remonter avant 1988 ; montrer les mandats locaux
passés, le RNE ne publiant ni l'historique ni les dates de fin. Une frise qui
présenterait le présent comme une carrière complète mentirait par omission —
elle doit dire d'où commence ce qu'elle sait.
