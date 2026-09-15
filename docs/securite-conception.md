# Sécurité et mairies : ce qui est vérifiable, et ce qui ne l'est pas

> **Méthode** · version 2 · 15 septembre 2026

La question : une affirmation selon laquelle une couleur politique municipale
apporterait davantage de sécurité peut-elle être vérifiée ? Ce document dit ce que
les données permettent d'établir, dans quel sens, et à quelles conditions — pour
cette affirmation comme pour la même affirmation portée par n'importe quel bord.

## 1. La donnée

Le SSMSI — service statistique ministériel de la sécurité intérieure — publie
sous Licence Ouverte le nombre de faits enregistrés par la police et la
gendarmerie, **commune par commune, année par année, de 2016 à 2025**, pour
quinze indicateurs. 5 231 250 lignes, chargées dans
`core.commune_delinquance` et reliées à `ref.commune` par le code INSEE.

Deux propriétés de la source sont transcrites telles quelles :

- **Faits enregistrés, pas faits commis.** Une hausse peut venir d'une hausse
  de la délinquance, d'une hausse des plaintes, d'un changement de doctrine
  d'enregistrement, ou de l'ouverture d'un commissariat. Le SSMSI l'écrit dans
  sa documentation.
- **Secret statistique, et il est massif.** Les effectifs trop faibles pour
  rester anonymes ne sont pas diffusés : la colonne `diffuse` vaut alors faux et
  `nombre` est NULL. **2 425 782 lignes sur 5 231 250 — 46 % — sont dans ce cas**,
  et elles se concentrent dans les petites communes. « Non diffusé » n'est pas
  « zéro », et les confondre ferait mécaniquement baisser les communes rurales.

  Trois indicateurs publiés au niveau départemental ne le sont pas au niveau
  communal : homicides, tentatives d'homicide, et usage de stupéfiants hors
  amende forfaitaire. Une page communale ne peut donc rien dire des homicides.

## 2. L'obstacle principal : les dates ne se recouvrent pas

C'est le point qui commande tout le reste.

    délinquance enregistrée     2016 ──────────────────────── 2025
    couleurs municipales connues                                    mars 2026 ●
    mandature en cours                                              2026 ────>

**Les seules couleurs municipales dont nous disposons datent de mars 2026, et
toutes les années de délinquance leur sont antérieures.** Aucune de ces séries
ne décrit ce qu'une municipalité a fait : elles décrivent ce dont elle hérite.

La vue `derived.commune_securite` porte une colonne `periode` qui vaut
`AVANT_MANDAT` ou `PENDANT_MANDAT`. Aujourd'hui elle vaut `AVANT_MANDAT`
partout. Elle n'est pas là pour décorer : elle rend la confusion impossible à
faire par inadvertance, et elle basculera d'elle-même quand le SSMSI publiera
l'exercice 2026.

**Pourquoi la mandature 2020-2026 ne comble pas le trou** : ses couleurs sont
dans les fichiers de résultats de 2020, publiés sous licence `notspecified`.
Une absence de licence n'est pas une autorisation (même règle que pour CHES).
Tant que ce point n'est pas tranché — demande d'autorisation, ou renoncement —
le seul mandat comparable reste hors d'atteinte, pour des raisons juridiques et
non techniques.

## 3. L'obstacle de fond : la commune ne commande pas la police

Même avec les dates alignées, l'attribution resterait fautive.

La police nationale et la gendarmerie relèvent de **l'État**. Un maire dispose
au plus d'une police municipale, dont les compétences ne couvrent presque aucun
des quinze indicateurs : violences sexuelles, trafic de stupéfiants et
cambriolages relèvent d'enquêtes judiciaires que la commune ne conduit pas et ne
finance pas.

C'est exactement la logique du code `EPCI_COMPETENCE` appliquée à la sécurité
(D-024) : avant de comparer un chiffre entre deux communes, il faut savoir si
la commune décide de ce qu'on mesure. Ici, elle ne décide pas.

## 4. Ce qui reste, et qui n'est pas rien

### 4.1 La fiche de commune — publiable, sans réserve d'attribution

Pour une commune : dix ans de séries, quinze indicateurs, en nombre et en taux
pour mille, avec la mention du secret statistique là où il s'applique — ce qui,
dans une commune rurale, sera le cas de la plupart des lignes.
C'est l'usage réel de l'outil : après un débat, on cherche *une* commune, et on
veut voir la courbe plutôt qu'un chiffre choisi.

Aucune imputation n'est nécessaire pour que ce soit utile, et aucune n'est
faite.

### 4.2 L'état des lieux au moment de l'élection — descriptif et symétrique

Question posée honnêtement : **dans quel état de sécurité enregistrée se
trouvaient les communes qui ont élu telle nuance en mars 2026 ?**

C'est une description de l'héritage, pas de l'action. Elle est calculable
aujourd'hui, elle est symétrique par construction — la même page existe pour
LRN, LLR, LSOC, LDVG, LCOM — et elle éclaire réellement le débat : une nuance
qui gagne dans des communes déjà au-dessus de la moyenne ne dit pas la même
chose qu'une nuance qui gagne partout.

Trois précautions obligatoires :
1. Toujours au **taux pour mille**, jamais en nombre : les communes nuancées
   sont les plus peuplées (D-023), et un classement en volume brut ne
   classerait que la taille.
2. Toujours **à strate comparable** : les strates de l'OFGL (tranche de
   population, rural, QPV, tranche de revenu) sont là pour cela.
3. Toujours avec le **taux d'exclusion affiché** : 90,6 % des communes n'ont
   aucune nuance, et les exclues sont systématiquement les plus petites.

### 4.3 L'avant/après — à partir de 2027, et sous condition

Quand le SSMSI publiera 2026 puis 2027, la colonne `periode` basculera et la
comparaison avant/après deviendra calculable sur la mandature en cours. Elle
devra alors être **préenregistrée** : indicateurs, strates, période et règle
d'exclusion fixés avant le premier calcul, comme le prévoit
`pre-enregistrement-001.md`.

Et même alors, le §3 tiendra toujours : un écart mesuré entre communes de
nuances différentes ne s'attribuera pas à la politique municipale sans un
raisonnement qui reste à construire.

## 5. Ce qu'il ne faut pas faire

- **Classer les communes par délinquance et colorer par nuance.** La carte
  obtenue serait d'abord une carte de la densité urbaine, ensuite une carte de
  la propension à porter plainte, et accessoirement autre chose.
- **Additionner des indicateurs.** Leur unité de compte diffère — victime,
  infraction, mis en cause — et la somme n'a pas de sens.
- **Traiter « non diffusé » comme zéro.** Cela ferait baisser mécaniquement les
  petites communes, qui sont justement celles où le secret s'applique.
- **Présenter l'état des lieux de 2016-2025 comme un bilan de mandat.** C'est
  l'erreur que la colonne `periode` existe pour empêcher.

## Versions

- **Version 2** (15 septembre 2026) : en-tête commun des documents de méthode (perimetre.md § 2.8, D-066) ; la question est formulée sans reprendre la phrase qui l'a suscitée.
- **Version 1** (12 septembre 2026) : ce que les données de délinquance permettent d'établir pour les mairies.
