# La page Gouvernement : ce que les données ouvertes fournissent, et ce qu'elles ne fournissent pas

> **Méthode** · version 2 · 15 septembre 2026
>
> Comment construire une frise des présidences, gouvernements et ministères depuis
> Jacques Chirac, accompagnée des grands chiffres (dette, dépenses par fonction, recettes
> fiscales, chômage, RSA, pauvreté, dividendes et impôt sur les sociétés), et que peut-on
> dire des manifestations (chiffres de la police, interpellations, blessés) ? Étude du
> 12 septembre 2026 ; chiffres de la base relevés le 13 septembre 2026. Là où l'étude et la
> base divergent, c'est la base qui fait foi.

Tout n'est pas disponible. Ce document sépare ce qui est chargé, ce qui est
atteignable et ce qui n'existe pas.

## 1. Chargé et vérifié

### 1.1 Les grandes séries : Eurostat, 1995-2025

**18 séries, 529 valeurs.** Eurostat republie les comptes nationaux transmis
par l'INSEE, sous une forme stable et documentée. C'est la raison de le
préférer à trente fichiers annuels épars qu'il faudrait raccorder soi-même.

| Série | Couverture |
|---|---|
| Dette publique (M€ et % du PIB) | 1995-2025 |
| Solde public (% du PIB) | 1995-2025 |
| Recettes fiscales et cotisations | 1995-2024 |
| Chômeurs au sens du BIT (milliers) | 1995-2025 |
| Taux de chômage | 2003-2025 |
| Personnes sous le seuil de pauvreté (milliers) | 2004-2025 |
| Taux de pauvreté | 1995-2025 |
| Dépense publique par fonction COFOG (10 séries) | 1995-2024 |

**Les « pôles » de dépense sont les dix fonctions COFOG** : services généraux,
défense, ordre et sécurité, affaires économiques, environnement, logement,
santé, loisirs et culture, enseignement, protection sociale. C'est la seule
ventilation à la fois officielle, stable dans le temps et comparable. Les
« missions » du budget de l'État, elles, changent de périmètre à chaque réforme
et ne se raccordent pas d'une législature à l'autre.

Deux pièges que `ref.macro_serie.definition` documente pour chaque série :

- Le **chômage BIT** n'est pas le nombre d'inscrits à France Travail. Les deux
  chiffres diffèrent de plus d'un million et obéissent à des règles
  différentes ; ils ne sont pas interchangeables.
- Le **seuil de pauvreté est relatif** : 60 % de la médiane. Il bouge avec le
  niveau de vie médian, et une baisse du médian peut faire reculer le nombre de
  pauvres sans que personne se soit enrichi.

### 1.2 RSA : CNAF, 2016-2025 seulement

**10 années.** Le nombre de foyers allocataires du RSA au mois de décembre,
régime général, tous types additionnés — 1 811 239 foyers en décembre 2024.

Trois décisions que la source ne prend pas et qui sont écrites dans le
connecteur : le mois retenu est décembre (le RSA varie avec la saison), tous
les types sont additionnés, et l'unité est le **foyer**, pas la personne.

**Cette série ne remonte pas aux années Chirac.** Elle commence en juin 2016, et
le RSA lui-même n'existe que depuis 2009 — le RMI le précédait, sur des règles
différentes. Une frise depuis 1995 aurait ici un trou de vingt ans, et il vaut
mieux l'afficher comme tel que de raccorder deux dispositifs incomparables.

### 1.3 Présidences et gouvernements

Les présidences sont chargées (D-026) avec les intérims.

Les **gouvernements** viennent de `data/gouvernements.csv`, transcrit du jeu
officiel des services du Premier ministre — qui **s'arrête en 2014**. Dernière
mise à jour du jeu : 18 juin 2014, et aucun successeur au catalogue.

C'est toujours l'état de `core.gouvernement` au 13 septembre 2026 : 36 lignes,
de janvier 1959 à mars 2014. Le chargement du *Journal officiel* décrit en 1.4
n'a pas été reversé dans cette table ; il vit dans `core.acte_jo` et
`core.gouvernement_membre`. La table des gouvernements croit donc encore Manuel
Valls Premier ministre.

### 1.4 Les ministres : quatre sources essayées, une seule tient

| Source | Ce qu'elle donne | Retenue |
|---|---|---|
| [data.gouv.fr — composition des gouvernements](https://www.data.gouv.fr/datasets/composition-des-gouvernements-de-la-veme-republique-1959-2014) | Premiers ministres et ministres, 1959-2014 | **gelée en 2014**, aucun successeur |
| Légifrance (site et API) | le texte des décrets | **HTTP 403** derrière une protection anti-robot ; l'API exige un compte PISTE |
| [Annuaire de service-public.fr](https://api-lannuaire.service-public.fr) | les ministères d'aujourd'hui et leur décret d'attribution | **instantané sans historique** : il ne remonte à rien |
| Open data de l'Assemblée (AMO30) | mandats ministériels depuis 2007 | **partiel et contaminé** : seulement les ministres qui furent députés, et 398 lignes sur 1 103 sont des « parlementaires en mission », qui ne sont pas membres du Gouvernement |
| **DILA — Journal officiel, base complète** | **le décret de composition lui-même** | **retenue** → `core.gouvernement_membre` |

La source de droit était la seule réponse. Le décret relatif à la composition du
Gouvernement est publié au *Journal officiel*, la DILA le diffuse en open data
sous Licence Ouverte, et **sa prose est réglée parce que c'est du droit** — la
même phrase depuis 1959 :

```
Sont nommés ministres : M. Laurent NUNEZ, ministre de l'intérieur ; …
Mme Catherine PÉGARD est nommée ministre de la culture.
Il est mis fin aux fonctions de : Mme Rachida DATI, ministre de la culture ; …
```

Le repère qui rend la découpe possible tient en une convention typographique :
**le patronyme est en capitales, le prénom ne l'est pas.** C'est elle qui sépare
« Amélie de MONTCHALIN » et « Anne Le HÉNANFF » sans heuristique de position.

Le coût est la base complète du Journal officiel : 1,1 Go, 1 236 284 fichiers
XML. Le connecteur la traverse **en flux** et n'en retient que les décrets de
gouvernement — 231 actes de 1920 à 2026. Charger la totalité du Journal officiel
pour répondre à cette question serait disproportionné, et c'est une décision
séparée.

**Ce qui est chargé n'est pas attribué.** Une ligne de décret donne un nom, pas
une personne de notre base : 41,9 % de nos élus ont un homonyme exact en nom et
prénom, et un décret ne porte aucune date de naissance pour trancher. Le
rapprochement porte donc son statut — `CANDIDAT`, `AMBIGU` ou `ABSENT` — et rien
de `CANDIDAT` ne doit être publié comme un fait sans le dire.

#### Ce qui est en base — relevé du 13 septembre 2026

Les nombres ci-dessous sont comptés dans la base, pas repris de l'étude
initiale. Là où ils s'en écartent, c'est la base qui a raison.

| | |
|---|---|
| Actes du *Journal officiel* chargés | **4 821**, du 2 juillet 1901 au 13 septembre 2026, dont **1 025** nominatifs |
| Décrets de composition retenus | **160**, porteurs de texte, couvrant **18 juillet 1990 → 26 février 2026** |
| Citations de membres | **1 371** — 1 274 nominations, 97 cessations de fonctions |
| Par fonction | 636 ministres, 409 secrétaires d'État, 279 ministres délégués, 27 Premiers ministres, 16 ministres d'État, 4 hauts-commissaires |
| Rapprochement avec `core.person` | 972 `CANDIDAT`, 154 `AMBIGU`, 245 `ABSENT` |
| Mentions nominatives des autres actes | 3 266 — 259 `CANDIDAT`, 150 `AMBIGU`, 2 857 `ABSENT` |

**Aucun rapprochement n'est `CONFIRME`.** Le statut `CANDIDAT` signifie « un seul
homonyme possible », pas « vérifié par un humain ». Le site ne doit donc pas
présenter ces 972 lignes comme des mandats attestés d'une personne nommée sans
écrire à côté d'où vient l'appariement.

#### Ce qui n'est pas encore fait, et se voit sur le site

Trois écarts subsistent entre ce document et ce que les pages affichent. Les
noter ici vaut mieux que les découvrir en lisant une fiche.

1. **`core.gouvernement` n'a pas bougé.** Elle compte toujours **36 lignes,
   1959 → 2014**, avec 21 Premiers ministres rattachés à une personne. La
   succession reconstituée à partir du *Journal officiel* — celle du tableau
   ci-dessous — n'y a pas été versée. Une requête posée à cette table croit donc
   encore Manuel Valls Premier ministre.
2. **`derived.mandat_ministeriel` n'existe pas.** Les trois règles de déduction
   décrites plus bas sont écrites, pas exécutées : aucune table de `derived` ne
   les porte.
3. **Les mandats ministériels affichés viennent encore d'AMO30**, la source que
   ce document disqualifie deux pages plus haut. `core.mandate` compte 1 103
   lignes de type `MINISTRE`, pour 550 personnes, du 24 décembre 2002 à
   aujourd'hui — dont **398 « en mission »**, c'est-à-dire des parlementaires en
   mission temporaire, qui ne sont pas membres du Gouvernement. Tant que la
   déduction JORF n'est pas matérialisée, une fiche de député peut afficher
   « MINISTRE · en mission » pour quelqu'un qui n'a jamais été ministre.

Avant 1990, la DILA ne publie que les métadonnées : les 42 décrets antérieurs
sont en base avec leur titre et leur date, sans texte. Leur contenu n'existe pas
en données ouvertes, et `data/gouvernements.csv` — transcrit du document officiel
1959-2014 — reste la source pour cette période.

La succession des Premiers ministres, que la base ignorait entièrement après
mars 2014 :

```
Manuel Valls        2014-03-31 → 2014-08-25      Gabriel Attal     2024-01-09 → 2024-09-05
Manuel Valls (II)   2014-08-25 → 2016-12-06      Michel Barnier    2024-09-05 → 2024-12-13
Bernard Cazeneuve   2016-12-06 → 2017-05-15      François Bayrou   2024-12-13 → 2025-10-10
Édouard Philippe    2017-05-15 → 2017-06-19      Sébastien Lecornu 2025-10-10 → en cours
Édouard Philippe II 2017-06-19 → 2020-07-03
Jean Castex         2020-07-03 → 2022-05-16
Élisabeth Borne     2022-05-16 → 2024-01-09
```

#### Les périodes sont déduites, pas publiées

Un décret nomme ; il ne dit pas jusqu'à quand. `derived.mandat_ministeriel`
applique trois règles, et leur imperfection est la raison pour laquelle elles
sont dans `derived` avec une `method_version` :

1. un **Premier ministre** reste en fonction jusqu'à la nomination du suivant.
   Appliquer la règle des ministres lui donnait un mandat de quarante-huit
   heures — le décret qui nomme *ses* ministres ne le reprend pas ;
2. un **ministre** cesse à la première cessation nominative, ou au premier
   décret qui recompose un gouvernement entier sans le reprendre. « Entier » est
   ici *plus de dix ministres de plein exercice nommés* : c'est un seuil, donc
   une convention ;
3. une **nouvelle nomination** de la même personne à la même fonction clôt la
   précédente — sans quoi un ministre reconduit accumulait autant de périodes
   ouvertes que de reconductions.

## 2. Ce qui n'existe pas en open data

### 2.1 Dividendes et impôt sur les sociétés — par société nommée

**Aucune des deux données n'est publique sous forme exploitable au niveau d'une
société identifiée.**

Les dividendes versés figurent dans les rapports annuels de chaque société,
publiés en PDF, société par société — il n'existe aucun jeu agrégé, et les
compilations qui circulent dans la presse sont des travaux privés, non
reproductibles et souvent non sourcés dans le détail.

L'impôt sur les sociétés payé par une entreprise donnée est couvert par le
**secret fiscal**. La déclaration pays par pays existe depuis 2016 mais n'est
transmise qu'à l'administration ; la directive européenne de publicité ne
s'applique qu'à partir des exercices 2025 et avec un périmètre restreint.

Conséquence : ces deux chiffres ne peuvent pas figurer sur la frise **par
société** sans sortir des sources primaires vérifiables. Un onglet qui les
afficherait quand même reposerait sur des agrégations de presse — exactement le
type de chiffre que l'outil existe pour permettre de contester.

### 2.1 bis — La même question, posée au bon niveau : chargée

L'objectif énoncé était de **corréler les dividendes distribués par les grands
groupes avec les impôts acquittés, le budget de l'État et la dette**. Cette
corrélation ne demande pas les chiffres société par société : elle demande des
agrégats comparables, mesurés selon les mêmes conventions, sur la même période.

Les comptes nationaux les publient, et Eurostat les diffuse depuis **1971** —
plus de cinquante ans, contre trente pour les finances publiques :

| Série | 2024 | Profondeur |
|---|---|---|
| Dividendes versés par les sociétés non financières (D.42) | 302 Md€ | 1971 → |
| Dividendes versés par les sociétés financières | 58 Md€ | 1971 → |
| Dividendes reçus par les ménages | 68 Md€ | 1971 → |
| Impôts sur le revenu payés par les sociétés non financières (D.51) | 65 Md€ | 1971 → |
| Impôt sur les bénéfices encaissé par l'État (D.51B) | 84 Md€ | 1995 → |
| Rémunération des salariés versée par les sociétés non financières (D.1) | 992 Md€ | 1971 → |
| Excédent brut d'exploitation | 487 Md€ | 1971 → |
| Valeur ajoutée | 1 513 Md€ | 1971 → |

Toutes dans `core.macro_value`, aux côtés de la dette, du solde public et des
dépenses par fonction — donc jointes par l'année, sans retraitement.

Ce que la série permet de montrer, et qui n'était pas montrable autrement :

    part de l'EBE distribuée en dividendes, sociétés non financières
      1980  18,8 %      2010  67,8 %
      1990  22,0 %      2020  56,5 %
      2000  43,6 %      2024  62,1 %

**Trois mises en garde à afficher avec ces séries** :

1. **C'est un agrégat.** Il couvre toutes les sociétés résidentes, pas les
   quarante plus grandes. Il ne se rapporte à aucune société identifiable et ne
   remplace pas ce qu'un rapport annuel publie.
2. **Les dividendes versés ne vont pas tous à des actionnaires français.** L'écart
   entre le total versé (302 Md€) et la part reçue par les ménages résidents
   (68 Md€) mesure ce qui va aux autres sociétés et au reste du monde — une part
   étant du flux intra-groupe compté deux fois dans la chaîne de détention.
3. **Deux mesures de l'impôt coexistent**, et elles ne coïncident pas : le D.51
   payé par les sociétés non financières (65 Md€) et le D.51B encaissé par les
   administrations (84 Md€), qui inclut les sociétés financières et suit une
   autre convention de rattachement. Les deux sont justes. L'écart est un fait à
   montrer, pas à masquer.

**Et la corrélation ne sera pas une causalité.** Que la part distribuée du profit
ait triplé pendant que la dette publique quadruplait n'établit aucun lien entre
les deux. La frise met les séries côte à côte parce que le lecteur a le droit de
les voir ensemble ; elle ne doit pas suggérer qu'elles s'expliquent.

### 2.2 Manifestations, interpellations, blessés

Six recherches ciblées sur data.gouv.fr — comptage de manifestants, usage de la
force, interpellations, gardes à vue, blessés des forces de l'ordre, ordre
public — renvoient **zéro jeu de données**.

Ce qui existe, et qui ne suffit pas :

- Le ministère de l'Intérieur communique des chiffres de participation à la
  presse, manifestation par manifestation. Ils ne sont **pas publiés** sous
  forme de jeu de données, ni même de liste.
- L'IGPN et l'IGGN publient un rapport annuel d'activité en PDF, avec le nombre
  d'enquêtes ouvertes — jamais rattaché à une manifestation identifiée.
- Le SSMSI publie la délinquance enregistrée par commune, qui ne distingue pas
  ce qui relève d'une manifestation.

**Seconde recherche (2026-09-12).** Six requêtes supplémentaires — préfecture,
SSMSI, statistiques de sécurité intérieure, victimes de violences, outrage et
rébellion, armes intermédiaires — renvoient elles aussi zéro jeu exploitable.

La base du SSMSI a été téléchargée et examinée : elle est **annuelle et
départementale**, et ses dix-huit indicateurs sont thématiques (homicides,
violences intrafamiliales, vols, stupéfiants, dégradations). Aucun ne relève de
l'ordre public ; il n'y a ni interpellations, ni blessés, ni « violences contre
personnes dépositaires de l'autorité publique ».

**Conséquence pour l'idée de rapprochement par date : il n'y a rien à
rapprocher.** Un rapprochement suppose deux séries datées ; ici la seule série
existante est annuelle, et ne contient pas la grandeur cherchée. Même en
extrayant les dates de manifestations de la presse, aucune jointure ne
produirait le nombre d'interpellations ou de blessés d'un jour donné.

### Ce qui resterait possible, et à quel prix

Construire la liste depuis la presse est faisable, mais ce ne serait pas une
ingestion : ce serait un **fichier éditorial**, une ligne par manifestation,
chacune citant son article et attribuant chaque chiffre à qui l'annonce —
« 30 000 selon la préfecture, 120 000 selon la CGT ». C'est le modèle de
`data/presidents.csv` : sourcé ligne à ligne, contestable, et jamais présenté
comme une donnée publique.

Trois réserves à poser avant de s'y engager :

1. **La sélection est le biais principal.** Ce sont les manifestations couvertes
   qui entreraient, pas les manifestations. Un seuil « plus de N personnes »
   appliqué à un corpus de presse mesure la couverture médiatique autant que la
   mobilisation.
2. **Le chiffre policier cité dans un article reste une source secondaire.** Le
   périmètre le classerait en `SECONDARY_PRESS`, au même rang que le chiffre des
   organisateurs — ce qui est exact et doit rester visible.
3. **Interpellations et blessés ne sont presque jamais publiés par
   manifestation**, même dans la presse, et jamais de façon comparable d'un
   événement à l'autre.

La voie propre reste une **demande CADA** auprès du ministère de l'Intérieur pour
les chiffres de participation, et auprès de l'IGPN pour les enquêtes ouvertes.
C'est une démarche administrative, pas une ingestion — mais c'est la seule qui
produirait des données primaires.

## 3. Ce que la frise pourra montrer, et la limite à écrire dessus

**Pourra** : les présidences depuis 1959, les ministres depuis 2007, et dix-huit
séries nationales depuis 1995, avec pour chacune sa définition exacte et son
producteur.

**Ne pourra pas** : imputer. Une courbe de dette qui monte sous une présidence
ne dit pas que cette présidence l'a fait monter. Les décisions budgétaires
produisent leurs effets avec plusieurs années de retard, et les chocs
extérieurs — 2008, 2020 — ne demandent l'avis de personne. La frise **situe**,
elle n'explique pas, et la page doit le dire aussi clairement qu'elle affiche
les chiffres. C'est la même règle que pour les mandats replacés sous une
présidence (D-026).
