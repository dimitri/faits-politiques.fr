# La dimension mairies : ce que les sources permettent réellement

Étude mesurée sur les fichiers eux-mêmes le 2026-09-12, puis mise à jour après
chargement. Les connecteurs existent désormais (`internal/communes`,
`go run ./cmd/ingest -only=communes`) ; ce document dit ce que les données
permettent d'affirmer et ce qu'elles interdisent.

## 1. État de la base

**Mise à jour du 2026-09-12 : tout ce qui suit est chargé.** La section décrivait
un schéma vide ; elle décrit désormais son contenu.

| Table | Lignes | Ce que c'est |
|---|---|---|
| `ref.commune` | 34 875 | COG 2026 |
| `ref.commune_change` | 12 820 | fusions, scissions, changements de code |
| `ref.nuance_politique` | 25 | nomenclature, millésime 2026 |
| `core.mandate` (tous niveaux) | 613 256 | dont 508 788 conseillers municipaux et 34 743 maires |
| `core.municipal_list` | 53 893 | listes candidates, mars 2026 |
| `derived.commune_couleur` | 34 800 | couleur déduite, avec son statut |
| `core.commune_indicator` | 1 672 878 | OFGL, 6 indicateurs × 8 exercices |
| `core.epci` | 9 282 | groupements (BANATIC) |
| `core.epci_membre` | 129 022 | adhésions de communes |
| `core.epci_competence` | 51 006 | compétences exercées |
| `ref.competence` | 125 | nomenclature DGCL |

Le protocole `pre-enregistrement-001.md` n'est toujours pas scellé, et rien n'a
été calculé : la section 4 reste entière.

## 2. Les trois sources, vérifiées fichier en main

### 2.1 RNE — qui est maire (et rien d'autre)

`elus-maire-mai.csv`, publié le 11 août 2026, Licence Ouverte, **34 826 maires**.
Quatorze colonnes : département, commune, nom, prénom, sexe, date de naissance,
catégorie socio-professionnelle, date de début de mandat, date de début de
fonction.

**Aucune nuance politique.** Le périmètre affirmait le contraire ; c'était faux
(D-022). Le RNE donne l'identité du maire et le rattachement à sa commune —
c'est-à-dire le lien personne ↔ commune, qui est exactement ce qui manque au
fichier de résultats. Les deux sont complémentaires, aucun ne suffit.

### 2.2 Résultats du ministère de l'Intérieur — la nuance, attachée à une liste

Scrutin des 15 et 22 mars 2026, Licence Ouverte. Un fichier par tour, une ligne
par commune, jusqu'à 13 blocs de liste par ligne, chaque bloc portant
`Nuance liste`, `Voix`, `Sièges au CM` et `Sièges au CC`.

C'est là, et seulement là, que la couleur politique existe. Elle qualifie une
**liste**, pas une personne.

Couverture mesurée sur les 34 835 communes pourvues :

| | communes | part |
|---|---|---|
| au moins une liste nuancée | 3 282 | **9,4 %** |
| aucune nuance | 31 553 | **90,6 %** |

Seuil de population : 1 064 inscrits au minimum chez les nuancées, 4 343 au
maximum chez les autres. Et parmi les 3 282 communes nuancées, la majorité élue
est « divers » dans **85,9 %** des cas :

    LDVD  1 117   34,0 %        LUG    142    4,3 %
    LDVG    599   18,3 %        LLR     92    2,8 %
    LDIV    554   16,9 %        LRN     42    1,3 %
    LDVC    550   16,8 %        LSOC    41    1,2 %
                                … puis 15 nuances sous 30 communes

Les majorités d'extrême droite : **61 communes** (LRN 42, LUXD 11, LEXD 6,
LUDR 2), soit 0,18 % des communes françaises.

**Les fichiers 2020 sont d'une autre qualité.** Le jeu « Municipales 2020 —
Résultats 2nd tour » est publié en `.txt` et `.xlsx`, et sa licence est
**`notspecified`**. Comme pour CHES, une absence de licence n'est pas une
autorisation : classement `RESTRICTED`, affichable mais jamais reversable dans
un export ouvert. Ce point conditionne tout le §4.

### 2.3 OFGL — ce que la commune a fait de son budget

`ofgl-base-communes`, API Opendatasoft, **21 857 255 enregistrements**,
**exercices 2018 à 2025**, Licence Ouverte. Granularité : commune × exercice ×
budget × agrégat, avec le montant, le montant par habitant et la population.

Une quarantaine d'agrégats, dont : frais de personnel, achats et charges
externes, dépenses d'équipement, encours de dette, annuité de la dette, épargne
brute / de gestion / nette, impôts locaux, DGF, FCTVA, capacité de financement.

Trouvaille qui change la conception : **OFGL publie lui-même les variables de
stratification** que le pré-enregistrement 001 exigeait de construire —
`tranche_population`, `rural`, `montagne`, `touristique`, `qpv`,
`tranche_revenu_imposable_par_habitant`, `epci_code`. Les strates ne seront donc
pas les nôtres mais celles de l'Observatoire, ce qui est très préférable : elles
sont définies avant nous et indépendamment de notre question.

## 3. La chaîne, et la décision qu'elle impose

    résultats Intérieur   liste, nuance, sièges au CM   -> commune
    RNE                   maire, dates                  -> commune
    OFGL                  agrégats 2018-2025            -> commune × exercice
    INSEE COG             fusions, scissions            -> commune × millésime

Le maillon manquant est au milieu : aucune source ne dit « cette commune est
de telle couleur ». Il faut le décider. La décision minimale et défendable est :

> La couleur d'une commune est la nuance de la liste ayant obtenu le plus de
> sièges au conseil municipal, au tour où le conseil a été pourvu.

Elle est simple, reproductible, et vérifiée : appliquée à Nice, elle donne LUXD
(liste Ciotti, 52 sièges contre 13 à la liste Estrosi) ; appliquée à Perpignan,
LRN (liste Aliot, 43 sièges). Elle reste une décision — elle doit vivre dans une
révision de cartographie, forkable, au même titre que le rattachement
parti→groupe (D-020), et non dans le corps d'une requête.

Le COG est indispensable et pas optionnel : entre 2020 et 2026 des communes ont
fusionné. Comparer une commune à elle-même sur sept exercices sans gérer les
changements de code produit des séries qui s'interrompent sans prévenir.

## 4. Ce qui est analysable, et sur quelle mandature

Le décalage temporel commande tout :

    mandature 2020-2026   |  OFGL 2018 -> 2025   couverture complète
    mandature 2026-       |  OFGL 2025 seul      une année de base, rien d'autre

**La seule mandature analysable est celle qui vient de finir.** Or ses nuances
sont dans les fichiers 2020, dont la licence n'est pas spécifiée. C'est la
contrainte structurante de tout le chantier, et elle est juridique, pas
technique.

Par solidité décroissante :

### 4.0 La chaîne est vérifiée de bout en bout

Contrôlé après chargement : partir d'une nuance, arriver aux comptes.

    derived.commune_couleur  ->  ref.commune  ->  core.commune_indicator

Pour les 61 communes à majorité d'extrême droite, la jointure renvoie bien un
encours de dette et des charges de personnel par habitant, exercice par
exercice, de 2018 à 2025. **Ces moyennes ne sont pas publiables en l'état** et
ne figurent pas ici : elles constateraient un écart sans strate ni compétence,
c'est-à-dire exactement ce que §4.3 interdit. Le contrôle porte sur la
jointure, pas sur le résultat.

Statut des 34 800 communes pourvues :

| Statut | Communes |
|---|---|
| `SANS_NUANCE` | 31 527 |
| `NUANCEE` | 3 269 |
| `EX_AEQUO` | 4 |

Les quatre `EX_AEQUO` sont des conseils où deux listes ont le même nombre de
sièges : la règle de déduction ne tranche pas, et le dit, plutôt que de laisser
croire à une donnée manquante.

### 4.1 La fiche de commune — sans comparaison

Pour une commune : son maire (RNE), la nuance de la liste majoritaire et le
détail du scrutin (Intérieur), ses agrégats financiers année par année (OFGL),
sa strate. Rien d'agrégé, rien de comparé, tout sourcé et daté.

C'est l'usage réel de l'outil : après un débat, on cherche *une* commune. Cela
ne demande aucun pré-enregistrement, et c'est publiable dès les connecteurs
écrits.

### 4.2 Les 61 communes, une par une

L'effectif est le bon argument : 61 communes se listent, se nomment et se
décrivent intégralement. Sept exercices, une quarantaine d'agrégats, la strate
et l'EPCI de chacune. Un lecteur qui veut savoir ce qu'a fait une mairie
d'extrême droite de son budget a la réponse, commune par commune, sans qu'on lui
serve une moyenne.

La symétrie est gratuite ici : la même page existe pour les 92 communes LLR, les
41 LSOC, les 19 LCOM. Le dispositif ne privilégie aucune étiquette parce qu'il
n'agrège rien.

### 4.3 La comparaison appariée — possible, mais étroite

Comparer 61 communes à un groupe de contrôle apparié sur strate de population,
tranche de revenu, caractère rural/urbain et région. Techniquement faisable :
OFGL fournit les appariements tout faits.

Trois limites à écrire avant de calculer, pas après :

1. **n = 61**, dont la moitié sous 10 000 habitants. Un écart de 5 % sur les
   frais de personnel par habitant ne sera pas distinguable du bruit.
2. **Le maire ne décide pas seul — et c'est massif.** BANATIC est chargé
   (D-024) : une commune a transféré **38 compétences en moyenne**, et
   **98,9 % d'entre elles** ont transféré la collecte des déchets, 90,3 % l'eau,
   76,9 % l'assainissement, 62,5 % le plan local d'urbanisme. Les « dépenses de
   fonctionnement » d'une commune sont donc un résidu de composition variable.
   Tout indicateur comparé doit être filtré par `core.commune_competence`, et
   le taux d'exclusion affiché — c'est à cela que sert le code
   `EPCI_COMPETENCE` prévu au périmètre.
3. **La sélection n'est pas aléatoire.** Ces communes ont élu cette majorité
   parce qu'elles avaient déjà certaines caractéristiques. Tout écart mesuré
   mêle l'effet du mandat et ce qui l'a produit, et aucun appariement sur
   variables observables ne sépare les deux.

Cette comparaison n'a de sens que scellée d'avance : indicateurs, strates,
période et règle d'exclusion fixés avant le premier calcul. C'est l'objet de
`pre-enregistrement-001.md`, qui doit être révisé (il suppose des strates
maison, désormais inutiles) puis scellé.

### 4.4 Ce qu'on ne fera pas

- **Une carte « communes RN » sans le reste.** 0,18 % des communes : une carte
  de France quasi vide, que le lecteur lira comme une carte de l'influence. La
  liste nommée dit la même chose sans l'illusion visuelle.
- **Une médiane par étiquette sur toutes les communes.** 90,6 % d'exclues, et
  des exclues systématiquement plus petites : la comparaison porterait sur un
  sous-ensemble urbain sans le dire.
- **Rebaptiser une nuance en parti.** LDVD n'est pas « la droite », LDIV n'est
  rien du tout. `core.nuance_party_link` existe pour ces rapprochements, avec un
  `aggregatable` qui vaut faux quand la nuance recouvre plus large que le parti.
  Il est vide, et le remplir est une décision éditoriale à part entière.

## 5. Ordre de travail proposé

1. `internal/insee` — COG : `ref.commune` et `ref.commune_change`. Rien ne tient
   sans les millésimes.
2. `internal/rne` — maires et conseillers : `core.mandate` avec la commune.
3. `internal/elections` — résultats 2026 (Licence Ouverte, sûr) : listes,
   nuances, sièges ; `ref.nuance_politique` millésimée par circulaire.
4. Règle de couleur (§3) chargée dans la révision de cartographie, à côté des
   liens parti→groupe.
5. `internal/ofgl` — agrégats communaux 2018-2025 vers `core.commune_indicator`,
   en conservant les strates publiées par l'Observatoire.
6. Publier §4.1 et §4.2. Aucun pré-enregistrement requis.
7. **Trancher la question de licence des résultats 2020** (`notspecified`) avant
   toute analyse de la mandature 2020-2026 : demander l'autorisation, ou s'en
   passer. Tant qu'elle n'est pas tranchée, §4.3 reste hors d'atteinte.
8. Réviser puis sceller `pre-enregistrement-001.md`, une fois 7 résolu.
