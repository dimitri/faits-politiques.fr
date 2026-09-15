# Le seuil de pauvreté se lit par ménage : ce que cela change pour un revenu universel

> **Hypothèse chiffrée** · version 3 · 15 septembre 2026
>
> **Cette note chiffre une hypothèse de politique publique, comme
> l'annexe A de ce document, reprise de
> [cotisations-et-droits.md](cotisations-et-droits.md), dont elle prend la suite.** Elle n'est pas un constat et ne relève pas du périmètre factuel du
> projet (voir [docs/perimetre.md](perimetre.md) §2.1 et §2.2). Elle répond à une
> question précise et laisse le jugement au lecteur.
>
> Le § A.3 chiffrait le coût d'un socle universel avec un
> montant forfaitaire **par personne**. Ce chiffrage était cohérent avec les
> données alors disponibles, mais il ignore un fait qui change le résultat de
> façon substantielle : le seuil de pauvreté ne se calcule pas par personne, il
> se calcule **par unité de consommation d'un ménage**. Cette note en tire les
> conséquences, puis micro-simule le financement par une reprise fiscale ciblée
> sur le socle versé aux ménages aisés — pas par un impôt distinct.
>
> **Ce que change cette version 2, par rapport à la version publiée initialement :**
> le §6 corrige à la hausse le coût net de l'architecture du §2 (≈ 217 Md€/an, pas
> 25-26 Md€) en le recalculant sur les déciles nationaux de niveau de vie plutôt
> que sur des moyennes par type de ménage ; le §7 redessine le barème de reprise
> pour qu'il ne prenne jamais rien à un ménage sous le niveau de vie médian et ne
> retire jamais net d'argent à personne — au prix, chiffré, d'environ 400 à
> 480 Md€/an.

---

## 1. Le seuil se lit par ménage : ce que cela change pour le montant à distribuer

### 1.1 L'unité de consommation, pas la personne

Le seuil de pauvreté — 1 288 € par mois en 2023 — est le revenu disponible **par
unité de consommation (UC)** en dessous duquel une personne est considérée comme
pauvre. L'échelle d'équivalence (dite « de l'OCDE modifiée », utilisée par
l'Insee) compte 1 UC pour le premier adulte, 0,5 par personne supplémentaire de
14 ans ou plus, 0,3 par enfant de moins de 14 ans. Elle traduit un fait simple :
un couple ne dépense pas deux fois ce que dépense une personne seule pour vivre
aussi bien — le logement, le chauffage, une partie de l'équipement sont
partagés.

**La conséquence directe pour un revenu universel est arithmétique.** Verser un
même montant forfaitaire à chaque personne, quel que soit le ménage où elle vit,
ignore ce partage :

- un forfait par PERSONNE au niveau du seuil surpaie tout ménage de plusieurs
  personnes — un couple toucherait deux fois le seuil individuel, soit 33 % de
  plus que SON PROPRE seuil (2 × 1 288 = 2 576 €, contre un seuil de couple à
  1,5 UC × 1 288 = 1 932 €) ;
- un forfait par MÉNAGE au niveau du seuil sous-paie les familles nombreuses,
  dont le seuil (proportionnel aux UC) croît avec la taille alors que le forfait
  n'en tient pas compte.

**Un socle proportionnel aux UC ne fait ni l'un ni l'autre** : verser
1 288 € par UC donne à chaque ménage un revenu socle exactement égal à SON
PROPRE seuil de pauvreté, quelle que soit sa composition — ni plus (le socle ne
sur-corrige jamais un ménage plus grand), ni moins (aucun type de ménage n'est
structurellement sous-servi). C'est la définition même d'un socle « crédible et
efficace » au sens où l'entendait la demande initiale : il tient la promesse
« personne sous son propre seuil de pauvreté » au moindre coût, sans transferts
arbitraires entre types de ménages.

### 1.2 Un chiffrage recalculé, pas seulement corrigé

Le § A.3 comptait 56,3 millions de résidents de 16 ans ou
plus à 1 288 €, pour 870 Md€ bruts par an — un chiffre qui, de plus, omettait
les moins de 16 ans (environ 12 millions de personnes), leur laissant un socle
nul alors que l'échelle d'équivalence leur accorde 0,3 UC chacun.

Le bon calcul multiplie le seuil par le nombre **total** d'UC du pays, enfants
compris. Ce nombre n'est pas publié tel quel, mais se déduit de deux moyennes
que la Drees publie pour les mêmes catégories de ménages (revenu disponible
moyen par ménage et par UC, enquête ERFS 2023) : leur rapport donne le nombre
moyen d'UC par ménage, et la table par type de ménage de la section suivante
permet de le totaliser sur toute la population.

**Résultat : environ 43,8 millions d'UC pour les 29,5 millions de ménages
couverts par la micro-simulation (§2), soit, extrapolé à l'ensemble des 30,5
millions de ménages du recensement, environ 45,1 millions d'UC.**

| | forfait par personne (§ A.3, ancien) | socle par UC (ici) |
|---|---:|---:|
| Population couverte | 56,3 M de résidents ≥ 16 ans | tous les résidents, UC pondérée |
| Coût brut annuel | 870 Md€ | **≈ 698 Md€** |
| Traitement des enfants | aucun socle propre | 0,3 UC chacun, cohérent avec le seuil |
| Traitement de la composition du ménage | ignoré | exact par construction |

Le socle par UC coûte **moins cher en brut** que le forfait par personne (parce
qu'il ne surpaie plus systématiquement les ménages de plusieurs adultes) tout en
couvrant, de façon cohérente, une population plus large (les enfants). C'est la
première conséquence utile de prendre le seuil au sérieux : la bonne unité de
calcul n'est pas neutre sur le montant à distribuer.

---

## 2. La micro-simulation : neuf types de ménage, une reprise fiscale sur le socle

### 2.1 Méthode

Verser 698 Md€ bruts par an et s'arrêter là ne répond pas à la question posée :
le financement. La section 7.2 de la note précédente proposait trois
architectures (différentielle, additive, dégressive) appliquées BRANCHE PAR
BRANCHE aux droits contributifs. Cette section retient l'architecture
**dégressive**, mais appliquée non pas branche par branche : sur le **revenu
initial total** du ménage — salaires, pensions, allocation chômage, patrimoine,
tout ce qui existe avant impôts et prestations —, ce qui unifie en une seule
règle ce qui demandait sinon une formule par risque.

**La règle retenue (`socle-uc-v1-reprise-lineaire`, [derived.socle_universel_simulation](../db/migrations/0067_socle_universel.sql)) :**

1. Le socle brut d'un ménage = 1 288 € × son nombre d'UC (§1.1).
2. Une reprise fiscale récupère ce socle À PROPORTION de l'écart entre le
   revenu initial du ménage PAR UC et le seuil : **0 % de reprise au seuil,
   100 % (recouvrement intégral) à deux fois le seuil**, en pente linéaire
   entre les deux. Au-delà de deux fois le seuil, le ménage ne touche plus rien
   du socle — il a payé, par cette reprise, l'équivalent de ce qu'il aurait
   reçu.
3. Le socle NET (après reprise) remplace les prestations sociales non
   contributives actuelles du ménage (prestations familiales, allocations
   logement, minima sociaux, prime d'activité — Drees, ERFS 2023). Les impôts
   directs actuels ne sont PAS modifiés : la reprise sur le socle est un
   mécanisme additionnel, pas une refonte du barème de l'impôt sur le revenu.

Cette reprise n'est PAS un impôt de plus prélevé sur le travail ou le
patrimoine : c'est une clause de retour sur le socle lui-même, ciblée sur les
ménages dont le revenu, avant toute aide, dépasse déjà largement le seuil de
pauvreté — au sens littéral de la demande : « le financement par l'impôt sur le
socle versé aux plus aisés ».

### 2.2 Les données et leur univers

| table | source | ce qu'elle porte |
|---|---|---|
| `core.pauvrete_seuil_annuel` | Insee, enquêtes Revenus fiscaux et sociaux 1996-2023 | le seuil lui-même, chargé pour lui-même (56 lignes, deux seuils) |
| `core.menage_type_drees` | Drees, ERFS 2023, tableaux 4a et 4b | revenu initial, prestations non contributives, impôts directs, revenu disponible — par ménage ET par UC, pour 10 types de ménage |
| `core.menage_type_effectif` | Insee, recensement de la population 2023 (API Melodi) | nombre de ménages par type, 9 des 10 catégories |
| `core.pension_tranche_eir` | Drees, Échantillon interrégimes de retraités 2020 | distribution — pas la moyenne — de la pension brute de droit direct |
| `core.chomage_tranche_unedic` | Unédic, Fichier national des allocataires | distribution des allocataires par tranche d'indemnisation, 45 trimestres depuis 2014 |

**Deux univers, pas un seul.** L'enquête ERFS (Drees) et le recensement (Insee)
ne comptent pas exactement les mêmes ménages : l'ERFS exclut les ménages dont le
revenu déclaré est négatif et ceux dont la personne de référence est étudiante.
La micro-simulation couvre **29,5 millions de ménages sur 30,5 millions au
recensement, soit 97 %** ; le résidu (ménages monoparentaux sans enfant au
foyer, ménages complexes avec enfants) n'est pas dénombré séparément par la
nomenclature Insee retenue et n'est donc pas simulé — un écart mesuré et
documenté, pas corrigé en silence.

**`uc_empirique` n'est pas l'échelle théorique (1 / 0,5 / 0,3).** Elle est
recalculée comme le rapport entre le revenu initial par ménage et le revenu
initial par UC que la Drees publie séparément pour les mêmes catégories : elle
capture donc la composition RÉELLE de chaque type (un « couple avec deux
enfants » n'a pas toujours exactement deux enfants de moins de 14 ans, et la
Drees compte comme enfant tout enfant célibataire du ménage, sans limite d'âge).
Une échelle théorique appliquée à une catégorie hétérogène aurait été un chiffre
inventé maquillé en donnée.

### 2.3 Résultat, par type de ménage

Montants mensuels moyens, 2023 :

| type de ménage | ménages | UC | revenu initial / UC ÷ seuil | reprise | socle net | ancienne prestation | delta mensuel | Md€/an |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Monoparentale, 2 enfants ou plus | 1 045 629 | 2,05 | 1,05 | 5 % | 2 505 € | 843 € | **+1 662 €** | **+20,85** |
| Couple, 4 enfants ou plus | 335 062 | 3,17 | 1,10 | 10 % | 3 663 € | 1 210 € | **+2 453 €** | **+9,86** |
| Monoparentale, 1 enfant | 1 360 129 | 1,44 | 1,51 | 51 % | 909 € | 359 € | +550 € | +8,97 |
| Personne seule | 11 947 695 | 1,00 | 1,91 | 91 % | 119 € | 104 € | +15 € | +2,15 |
| Ménage complexe sans enfant | 663 372 | 1,76 | 1,76 | 76 % | 545 € | 383 € | +162 € | +1,29 |
| Couple, 3 enfants | 957 241 | 2,65 | 1,80 | 80 % | 667 € | 623 € | +44 € | +0,51 |
| Couple sans enfant | 7 906 806 | 1,50 | 2,70 | 100 % | 0 € | 46 € | −46 € | −4,36 |
| Couple, 1 enfant | 2 575 521 | 1,91 | 2,50 | 100 % | 0 € | 163 € | −163 € | −5,04 |
| Couple, 2 enfants | 2 752 171 | 2,25 | 2,26 | 100 % | 0 € | 272 € | −272 € | −8,98 |
| **Total (9 types, 29,5 M ménages)** | | | | | | | | **+25,25** |

*(reproduit par `SELECT * FROM derived.socle_universel_simulation ORDER BY delta_annuel_md_euros DESC`.)*

**Ce que ce tableau dit :**

- **Le coût NET de la réforme (socle par UC + reprise fiscale − prestations
  supprimées) est d'environ 25 Md€ par an** pour la population couverte —
  extrapolé aux 3 % de ménages non modélisés, autour de **26 Md€**. C'est très
  inférieur aux 382 Md€ « au mieux » du § A.3 : la
  différence tient presque entièrement à la reprise fiscale, qui récupère
  586 Md€ des 676 Md€ de socle brut versé à cette même population, contre les
  bornes « au plus » (266 + 38 Md€) que le chiffrage précédent posait sans
  simulation.
- **Le gain se concentre là où le taux de pauvreté est aujourd'hui le plus
  élevé** : les familles monoparentales de deux enfants ou plus (+1 662 €/mois)
  et les couples de quatre enfants ou plus (+2 453 €/mois) — cohérent avec le
  taux de pauvreté de 34,3 % mesuré pour les familles monoparentales par
  l'Insee (*Niveau de vie et pauvreté en 2023*, Insee Première n° 2063).
- **Les couples avec enfants et un revenu initial déjà supérieur à deux fois le
  seuil par UC perdent leurs prestations familiales/logement actuelles sans
  contrepartie** (reprise à 100 %, socle net nul) : une petite perte mensuelle
  (46 à 272 €) pour trois catégories qui représentent, ensemble, 13,2 millions
  de ménages. C'est un effet de bord du choix du seuil de reprise (deux fois le
  seuil) : le déplacer changerait qui perd et de combien, pas la logique.
- **Ce montant NE COMPTE PAS les impôts directs actuels**, laissés inchangés à
  dessein (§2.1). Il ne compte pas non plus les coûts ou économies de gestion
  qu'entraînerait la suppression des prestations remplacées.

---

## 3. Ce qu'une moyenne masque : le biais mesuré sur deux distributions réelles

Le tableau du §2.3 applique la reprise fiscale à des **moyennes** par type de
ménage. C'est une limite assumée, pas un détail : une reprise plafonnée entre 0
et 1 est une fonction NON LINÉAIRE du revenu, et une fonction non linéaire
appliquée à une moyenne ne donne pas la moyenne de la fonction (inégalité de
Jensen). Deux distributions réelles, chargées pour cette seule vérification,
mesurent l'ampleur du biais.

### 3.1 Les retraités (Drees, Échantillon interrégimes de retraités 2020)

La pension brute de droit direct moyenne, pondérée sur les 46 tranches de
100 € publiées, est de **1 428 €/mois**.

| méthode | taux de reprise obtenu |
|---|---:|
| Appliquer la règle à la seule moyenne (1 428 €) | **10,9 %** |
| Appliquer la règle tranche par tranche, puis moyenner | **26,9 %** |

**La méthode sur moyenne sous-estime la reprise réelle de plus de moitié.**
L'explication tient dans la distribution elle-même : **50,3 % des retraités
de droit direct ont une pension brute inférieure au seuil de pauvreté**
(1 288 €) — ils ne doivent RIEN à la reprise, quelle que soit la méthode — mais
l'autre moitié est étalée jusqu'à plus de 4 500 €, où le taux de reprise
plafonne à 100 %. Une moyenne masque cette bimodalité de fait : la « personne
moyenne » n'existe pas dans cette population.

### 3.2 Les allocataires de l'Assurance chômage (Unédic, 30 juin 2025)

Même exercice sur la répartition des allocataires de l'ARE, l'AREF et le CSP
par tranche de montant d'indemnisation mensuel (Ensemble Assurance chômage) :

| méthode | taux de reprise obtenu |
|---|---:|
| Appliquer la règle à la seule moyenne (1 301 €) | **1,0 %** |
| Appliquer la règle tranche par tranche, puis moyenner | **16,6 %** |

**59,7 % des allocataires indemnisés touchent une allocation inférieure au
seuil de pauvreté.** Le biais de la méthode sur moyenne est ici encore plus
marqué (facteur 16, contre facteur 2,5 pour les pensions) : la distribution des
indemnités chômage est plus resserrée près du seuil, précisément la zone où la
non-linéarité de la reprise pèse le plus.

### 3.3 Conséquence pour le chiffrage du §2 — corrigée au §6

Le tableau du §2.3 traite chaque type de ménage comme homogène, alors qu'il ne
l'est pas — un « couple sans enfant » recouvre aussi bien un couple de deux
smicards qu'un couple de deux cadres. Le biais mesuré aux §3.1 et 3.2, pour les
retraités et les allocataires du chômage pris isolément, va dans un sens
précis : le calcul sur la seule moyenne sous-estime la reprise pour CES deux
populations.

> **Correction (ajoutée à la version 1 de cette note).** Cette section
> affirmait que le chiffre de 25-26 Md€/an du §2.3 était, de ce fait, « un
> plafond ». **C'est faux, et le §6 le montre avec une distribution nationale
> plus fine que les neuf types de ménage** : un calcul sur les déciles de
> niveau de vie donne un coût net proche de 217 Md€/an pour la même
> architecture — bien AU-DESSUS de 25-26 Md€, pas en dessous. La raison n'est
> pas celle avancée ici : le découpage par TYPE de ménage et le découpage par
> DÉCILE de revenu sont deux partitions différentes de la même population, et
> rien ne garantit qu'elles s'accordent. Ici, elles ne s'accordent pas, d'un
> ordre de grandeur. Voir le §6 pour le calcul et pourquoi le résultat par
> décile est le plus fiable des deux.

---

## 4. Ce que cette note ne fait pas

- **Elle ne simule pas de comportement.** Une reprise à 100 % entre une et deux
  fois le seuil crée, sur cette tranche, un taux marginal élevé qui pourrait
  décourager une heure de travail supplémentaire — un effet que seul un modèle
  comportemental, pas une micro-simulation statique, peut chiffrer.
- **Elle ne compte pas les coûts de transition ni de gestion** : réaffecter les
  agents aujourd'hui chargés de gérer le RSA, la prime d'activité et les
  allocations logement n'est ni gratuit ni instantané.
- **Elle laisse ouvert le choix du barème de reprise** (ici linéaire entre une
  et deux fois le seuil) : un barème plus étalé réduirait le taux marginal au
  prix d'un coût net plus élevé, un barème plus resserré ferait l'inverse. Le
  choix est entièrement politique ; cette note en chiffre UNE version, pas LA
  version.
- **Elle ne dit pas si cette réforme est souhaitable.** Comme le rappelle
  [docs/perimetre.md](perimetre.md) §2.1, ce projet ne rend pas de verdict.

---

## 5. Une variante « complément » : fusionner RSA, minimum vieillesse et ASS sans autre condition que le revenu

Le socle des sections 1 et 2 est UNIVERSEL : il est versé à tout le monde, puis
repris chez les plus aisés. Une question différente se pose : peut-on, sans
verser un euro à qui n'en a pas besoin, fusionner en une seule allocation le
RSA, le minimum vieillesse (ASPA) et l'allocation de solidarité spécifique
(ASS, le plancher de l'assurance chômage pour qui a épuisé ses droits), avec
pour seule condition d'ouverture le niveau de revenu actuel du foyer — sans
condition d'âge, sans obligation de recherche d'emploi, sans recours sur
succession ?

### 5.1 Le mécanisme, et pourquoi il n'est PAS le même calcul que le socle

Ici, l'allocation d'un ménage vaut `max(0, seuil × UC − revenu initial)` : rien
pour un ménage déjà au-dessus du seuil, un COMPLÉMENT exact pour amener
au seuil celui qui est en dessous. C'est l'architecture « différentielle » de
l'annexe A.2, mais appliquée
au revenu total du ménage plutôt qu'à chaque droit contributif séparément.

**Ce calcul est IMPOSSIBLE à faire sérieusement avec les moyennes par type de
ménage du §2** — et c'est instructif de voir pourquoi. Les neuf ratios
revenu-initial-par-UC-sur-seuil du tableau du §2.3 sont TOUS supérieurs à 1
(de 1,05 pour les familles monoparentales de deux enfants ou plus à 2,70 pour
les couples sans enfant) : au niveau de la MOYENNE de chaque type, aucun
ménage n'est sous le seuil, et le complément calculé ainsi vaudrait
rigoureusement zéro. C'est absurde — 15,4 % de la population est pourtant sous
ce seuil — et cela illustre en clair la limite déjà nommée au §3 : une
allocation qui cible spécifiquement la queue basse d'une distribution ne peut
pas se calculer sur la moyenne de cette distribution, la moyenne étant par
construction plus haute que ce qu'elle est censée mesurer.

### 5.2 Un ordre de grandeur, à partir des statistiques de pauvreté elles-mêmes

Faute de distribution fine du revenu par ménage dans la base, l'ordre de
grandeur se lit dans deux séries que l'Insee publie déjà pour caractériser
LA POPULATION PAUVRE ELLE-MÊME — chargées dans `core.pauvrete_seuil_annuel`
(§2.2) :

- le **taux de pauvreté** à 60 % de la médiane : 15,4 % en 2023, soit
  9,792 millions de personnes ;
- l'**intensité de la pauvreté** : l'écart relatif entre le niveau de vie
  MÉDIAN des personnes pauvres et le seuil, 19,2 % en 2023 — soit un écart de
  **247 € par UC et par mois** (1 288 × 19,2 %).

En rapportant le nombre de personnes pauvres au nombre d'UC total de la
population (§1.2 : environ 45,1 millions d'UC pour 63,6 millions de résidents
dans le champ de l'enquête, soit 0,71 UC par personne en moyenne), la
population pauvre représente environ **6,9 millions d'UC**. Combler leur écart
au seuil coûterait environ :

**6,9 millions d'UC × 247 €/mois × 12 ≈ 20,6 Md€ par an.**

Ce que cette allocation remplacerait, aux montants annuels les plus récents
publiés (Cnaf, Cnav, Unédic/DREES) :

| allocation remplacée | dépense actuelle |
|---|---:|
| RSA (Cnaf, 2024) | 11,9 Md€ |
| Minimum vieillesse — ASPA et 1er étage (Cnav, 2023) | 4,3 Md€ |
| ASS, plancher de l'assurance chômage (DREES, 2023) | 1,7 Md€ |
| **Total actuel** | **17,9 Md€** |

**Coût net supplémentaire, à ces ordres de grandeur : environ 3 Md€ par an** —
un montant sans commune mesure avec les 25 Md€ du socle universel du §2, et
sans aucune mesure avec les centaines de milliards que l'intuition pourrait
prêter à une promesse aussi large que « personne sous le seuil de pauvreté ».
C'est cohérent : cette variante ne fait rien pour les 84,6 % de la population
déjà au-dessus du seuil, alors que le socle universel touche tout le monde
avant de reprendre chez les plus aisés.

**Deux réserves sur ce chiffre, dans des sens opposés :**

- **Il minore probablement le coût réel**, parce que l'intensité de la
  pauvreté est calculée sur la MÉDIANE des revenus des personnes pauvres, pas
  sur leur MOYENNE — et combler un écart coûte la moyenne des écarts, pas
  l'écart au revenu médian. Si la distribution des revenus des pauvres a une
  frange à revenu très faible (personnes sans aucune ressource), la moyenne
  des écarts est plus grande que 247 €, et le coût réel dépasse 20,6 Md€.
- **Il ne majore PAS artificiellement le montant actuellement dépensé** : le
  taux de non-recours au RSA est documenté et significatif (de l'ordre du
  tiers des foyers éligibles, selon la Cnaf), et le recours-sur-succession de
  l'ASPA est identifié comme un frein à son recours par les personnes âgées.
  Les 17,9 Md€ actuels ne couvrent donc pas tous les foyers qui seraient
  éligibles à la variante « complément », alors que les 20,6 Md€ ci-dessus,
  fondés sur la pauvreté RÉELLEMENT mesurée (pas sur les demandes déposées),
  couvrent par construction tout le monde. Une partie de l'écart de 3 Md€ est
  donc simplement ce que le non-recours actuel ne fait pas apparaître dans la
  dépense publiée.

### 5.3 Le compromis que ce chiffrage ne doit pas faire oublier

Une allocation qui vaut exactement `seuil − revenu` en dessous du seuil et
zéro au-dessus crée, PILE au niveau du seuil, un taux marginal de **100 %** :
le premier euro gagné au-delà de ce que l'on a déjà fait perdre un euro
d'allocation. C'est la critique classique, déjà nommée en
l'annexe A.2, des dispositifs
d'assistance à seuil dur — et la raison d'être du barème étalé (une à deux fois
le seuil) retenu pour le socle universel du §2, qui dilue ce taux marginal sur
une plage plus large au prix d'un coût plus élevé.

**La variante « complément » est donc moins chère et plus simple à faire
adopter (une seule allocation, un seul critère), mais elle ne résout PAS le
problème du taux marginal au seuil — elle le concentre même davantage, en un
point unique et net, là où le socle universel l'étale.** Les deux variantes
répondent à deux priorités différentes, pas à la même question : combler la
pauvreté au moindre coût (le complément), ou garantir un revenu à tous en
lissant les effets de seuil (le socle universel). Le choix entre les deux — ou
un compromis entre les deux barèmes — est, ici encore, hors du constat que ce
projet peut établir seul.

---

## 6. Vers une estimation sur données individuelles : les déciles nationaux du niveau de vie

### 6.1 Pourquoi pas Ines, directement

Le modèle de micro-simulation socio-fiscale de la Drees et de l'Insee, Ines,
tourne sur l'enquête ERFS **individuelle**, pas sur ses tableaux publiés. Cette
enquête, comme le panel de l'Échantillon démographique permanent ou les
extractions fiscales du Fichier localisé social et fiscal, n'est accessible
qu'**au Centre d'accès sécurisé aux données (CASD)**, sous convention de
recherche — jamais en téléchargement libre. Aucun connecteur de ce projet ne
peut donc reproduire Ines : ce que cette section fait est le meilleur
**succédané** que l'open data permette, pas Ines lui-même, et le mot n'est pas
choisi par coquetterie.

### 6.2 Ce qui est chargé : neuf points d'une vraie distribution nationale

L'Insee publie, dans son fichier Filosofi (`core.filosofi_decile_national`),
les neuf déciles du niveau de vie national — D1 à D9, y compris la médiane
(D5). Convertis en euros mensuels par UC, 2023 :

| décile | D1 | D2 | D3 | D4 | D5 (médiane) | D6 | D7 | D8 | D9 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| €/mois/UC | 1 100 | 1 418 | 1 692 | 1 933 | **2 160** | 2 405 | 2 698 | 3 115 | 3 875 |

Neuf points ne sont pas des données individuelles. Mais ils découpent la
population en **dix tranches de 10 % chacune**, une résolution bien plus fine
que les neuf types de ménage du §2 (dont les effectifs vont de 335 000 à
12 millions de ménages) — et surtout, une résolution qui suit directement le
REVENU, l'axe sur lequel la reprise fiscale opère, plutôt que la composition
familiale, qui lui est orthogonale.

**Méthode d'estimation par tranche**, appliquée à chacune des dix tranches de
10 % de la population : un revenu représentatif, pris au MILIEU de la tranche
pour les huit tranches intérieures (D1-D2, D2-D3, … D8-D9), et une hypothèse
explicite pour les deux tranches ouvertes — 0,75 × D1 pour les 10 % les plus
modestes, 1,8 × D9 pour les 10 % les plus aisés. **Ces deux coefficients sont
des conventions, pas des mesures** : ils bornent une queue de distribution que
la source ne détaille pas plus finement en open data. La fonction de reprise
est ensuite appliquée à chacun des dix revenus représentatifs, et la moyenne
des dix résultats — chaque tranche pesant 10 % — donne le taux de reprise
moyen sur l'ensemble de la population.

### 6.3 Le résultat, et pourquoi il contredit le §3.3 de la version 1

Appliqué à l'architecture du §2 (reprise entre une et deux fois le SEUIL DE
PAUVRETÉ, 1 288 à 2 576 €/UC) :

| | taux de reprise moyen | coût brut | récupéré | net, avant retrait des prestations remplacées | **net** |
|---|---:|---:|---:|---:|---:|
| Neuf types de ménage (§2.3) | 0,313 | 676,5 Md€ | 586,4 Md€ | — | **25,2 Md€** |
| Dix tranches de revenu (ici) | 0,596 | 697,1 Md€ | 415,2 Md€ | 281,9 Md€ | **≈ 217 Md€** |

**Les deux méthodes ne s'accordent pas, et de loin.** Le calcul par tranche de
revenu récupère MOINS (415 Md€ contre 586 Md€) parce que la moitié de la
population reste, par construction, sous le seuil de reprise (les cinq
premières tranches ont un taux de reprise nul ou quasi nul) — alors que le
calcul par type de ménage, en faisant porter la reprise sur des MOYENNES de
groupes entiers (« couple sans enfant » pris comme un seul point à 2,70 fois
le seuil), place artificiellement plus de poids dans la zone de reprise
maximale. **Le découpage par décile suit directement l'axe sur lequel la
règle est écrite (le revenu) ; le découpage par type de ménage suit un axe
orthogonal (la composition familiale) qui ne capture la position dans la
distribution des revenus qu'indirectement, par la moyenne du groupe.** C'est
pour cela que le résultat par décile doit être tenu pour le plus fiable des
deux, et que **le chiffre à retenir pour le §2 est environ 217 Md€ net par
an, pas 25 Md€.**

Ce n'est pas un ajustement à la marge : c'est une division par presque neuf
de l'estimation initiale. **La leçon méthodologique dépasse ce chiffrage
précis** : deux partitions raisonnables d'une même population, appliquées à
la même règle, peuvent donner des résultats qui ne se recoupent pas d'un
ordre de grandeur. Aucune des deux n'est «&nbsp;fausse&nbsp;» en soi ; c'est
le choix de la partition qui doit suivre l'axe de la règle qu'on applique.

---

## 7. Version 2 du barème de reprise : ancré sur le niveau de vie médian, avec un plancher

La version 1 de la reprise (§2.1) démarrait à une fois le SEUIL DE PAUVRETÉ
(1 288 €/UC) — un niveau **inférieur au niveau de vie médian** (2 160 €/UC en
2023). Elle commençait donc à reprendre le socle à des ménages qui n'ont rien
d'aisé : la moitié la plus modeste du pays gagne, par définition, moins que la
médiane, et une partie d'entre elle se trouvait déjà dans la zone de reprise.
C'est ce défaut, et le fait que certains types de ménage y perdaient
concrètement de l'argent (§2.3, couples avec un ou deux enfants), qui motive
cette seconde version.

### 7.1 Ce qui change

1. **Les deux anses de la reprise sont déplacées au niveau de vie médian et à
   son double** — 2 160 € et 4 320 €/UC en 2023 — au lieu d'une et deux fois
   le seuil de pauvreté. Le double du niveau de vie médian est la définition
   la plus citée du seuil de richesse en France (Observatoire des
   inégalités) : ce n'est plus un multiple arbitraire, c'est une convention
   reconnue et déjà utilisée pour désigner « les plus aisés ». **Personne en
   dessous de la médiane — la moitié du pays — ne subit désormais la moindre
   reprise.**
2. **Un plancher interdit toute perte nette.** Le socle net d'un ménage ne
   peut jamais descendre sous ses prestations non contributives actuelles
   (`GREATEST(socle_net_calculé, prestations_actuelles)` dans
   `derived.socle_universel_simulation`, method_version
   `socle-uc-v2-reprise-mediane-plancher`). Un ménage entièrement repris ne
   perd donc plus jamais deux fois — le socle ET ses anciennes prestations —
   il échange au pire l'un contre l'autre, à montant égal.

### 7.2 Résultat, par type de ménage — plus personne ne perd

| type de ménage | ratio revenu initial / médiane | reprise | socle net | ancienne prestation | delta mensuel | Md€/an |
|---|---:|---:|---:|---:|---:|---:|
| Personne seule | 1,14 | 14 % | 1 111 € | 104 € | **+1 007 €** | +144,4 |
| Couple sans enfant | 1,61 | 61 % | 750 € | 46 € | **+704 €** | +66,8 |
| Couple, 2 enfants | 1,35 | 35 % | 1 896 € | 272 € | **+1 624 €** | +53,7 |
| Couple, 1 enfant | 1,49 | 49 % | 1 249 € | 163 € | **+1 086 €** | +33,6 |
| Couple, 3 enfants | 1,08 | 8 % | 3 150 € | 623 € | **+2 527 €** | +29,0 |
| Monoparentale, 1 enfant | 0,90 | 0 % (plancher) | 1 855 € | 359 € | **+1 496 €** | +24,4 |
| Monoparentale, 2 enfants ou plus | 0,63 | 0 % (plancher) | 2 644 € | 843 € | **+1 801 €** | +22,6 |
| Ménage complexe sans enfant | 1,05 | 5 % | 2 152 € | 383 € | **+1 769 €** | +14,1 |
| Couple, 4 enfants ou plus | 0,66 | 0 % (plancher) | 4 085 € | 1 210 € | **+2 875 €** | +11,6 |
| **Total (9 types, 29,5 M ménages)** | | | | | | **+400,1** |

**Plus aucun type de ménage ne perd** — le résultat que le §2.3 n'obtenait
pas. Même le couple sans enfant, qui touchait le taux de reprise le plus
élevé de la version 1 et perdait 46 €/mois, gagne désormais 704 €/mois : au
niveau médian, aucun ménage ordinaire n'est plus dans la zone de reprise
forte.

### 7.3 Le prix de cette garantie

**Coût net : environ 400 Md€/an sur les neuf types de ménage (§7.2),
confirmé à environ 479 Md€/an par le calcul en dix tranches de revenu du §6**
(153,5 Md€ récupérés sur 697,1 Md€ de socle brut, moins 64,8 Md€ de
prestations remplacées). Les deux méthodes, qui divergeaient d'un facteur
neuf pour la version 1 (§6.3), se rapprochent ici à moins de 20 % l'une de
l'autre — signe que l'essentiel de leur désaccord précédent venait de la
zone de reprise partielle entre une et deux fois le SEUIL DE PAUVRETÉ, une
zone que la version 2 déplace largement hors de portée des types de ménage
ordinaires.

**Ce prix doit être dit sans détour : protéger la moitié la plus modeste du
pays de toute reprise, et garantir qu'aucun ménage ne perde net, coûte de
l'ordre de 400 à 480 Md€ par an** — environ le double du chiffrage corrigé de
la version 1 (217 Md€, §6.3), et seize à dix-neuf fois le chiffre erroné
(25 Md€) que la première version de cette note avançait avant la correction
du §6. Ce n'est pas un artefact de méthode : c'est le
coût réel d'un principe — « ne jamais retirer d'argent à un foyer modeste » —
appliqué à une reprise qui, pour rester une reprise et non un nouvel impôt
général, ne peut s'exercer que sur les plus aisés — par construction une
minorité de la population, puisque la médiane, par définition, en laisse la
moitié en dessous.

**Ce que cela implique pour le financement.** Un socle universel qui ne
retire jamais rien à personne sous la médiane coûte, net, plus que le budget
actuel de l'ensemble des prestations familiales, de l'assurance chômage et
des minima sociaux réunis. Le comparer à la variante « complément » du §5
(coût net de l'ordre de 3 Md€/an, §5.2) mesure exactement le prix de
l'universalité : verser le socle À TOUT LE MONDE, y compris à qui n'en a pas
besoin avant reprise, coûte infiniment plus cher en trésorerie brute et en
mécanique de reprise que ne le fait cibler l'aide sur ceux qui en ont
besoin — même quand les deux visent, in fine, le même objectif de ne priver
personne sous le seuil de pauvreté.

## Sources

- Insee, *Niveau de vie et pauvreté en 2023*, Insee Première n° 2063, et
  fichiers de données associés (tableaux complémentaires 1 à 3).
- Drees, *Pauvreté avant et après redistribution, niveau de vie et
  décomposition du revenu* (jeu de données n° 4230), fiches 02 et 03 du
  Panorama *Minima sociaux et prestations de solidarité*.
- Drees, *Distribution des pensions mensuelles* (jeu de données n° 4178),
  Échantillon interrégimes de retraités 2020.
- Unédic, *Montant d'allocation chômage et salaires de référence des
  allocataires de l'Assurance chômage*, data.gouv.fr.
- Insee, recensement de la population 2023, jeux de données
  `DS_RP_TD_FAMILLE_NBENF_COMP` et `DS_RP_TD_MENAGES_TPH_COMP`, diffusion API
  Melodi.
- Cnaf, *RSA conjoncture*, dépense annuelle 2024 (non chargée en base — citée
  au §5.2 pour son seul ordre de grandeur).
- Drees, *Minima sociaux et prestations sociales*, fiches sur les dépenses du
  minimum vieillesse et de l'allocation de solidarité spécifique, édition 2023
  (non chargées en base — mêmes réserves).
- Insee, Filosofi (Fichier localisé social et fiscal), *Revenus et pauvreté des
  ménages — tous les niveaux géographiques*, jeu de données n° 8984752, champ
  France, 2023 — `core.filosofi_decile_national` (§6).
- Drees, présentation du Centre d'accès sécurisé aux données (CASD) et du
  modèle Ines — pour ce que ce projet ne peut pas reproduire en open data
  (§6.1).

## Annexe A. Point de départ : trois architectures pour un socle universel

> Cette annexe reprend le § 7 de la version 1 de
> [cotisations-et-droits.md](cotisations-et-droits.md), qui chiffrait déjà une hypothèse
> dans un dossier de constat. Elle est rangée ici, avec l'hypothèse qu'elle a ouverte
> (D-066). La numérotation « § 7.x » citée dans les versions antérieures correspond à
> « § A.x ».

> **Cette section chiffre une hypothèse de politique publique.** Elle n'est pas un
> constat et ne relève pas du périmètre factuel du projet. Elle est rédigée pour
> répondre à une question précise et laisse le jugement au lecteur.

### A.1 Le problème à résoudre

Un revenu universel **au niveau du seuil de pauvreté** — **1 288 € par mois** pour
une personne seule en 2023 selon l'INSEE — versé aux **56,3 millions** de résidents
de 16 ans ou plus coûterait **870 Md€ bruts**, soit **66 % de l'ensemble des impôts
et cotisations** prélevés en 2024 (1 319 Md€).

Le financer uniquement par les prestations **non contributives** ne suffit pas, et
de loin. La question devient : peut-on y faire contribuer les droits
**contributifs** sans rompre la promesse individuelle qu'ils portent ?

### A.2 Trois architectures

Soit **S** le socle universel et **P** le droit contributif calculé comme
aujourd'hui (pension, allocation chômage).

| architecture | ce que reçoit la personne | avantage | défaut |
|---|---|---|---|
| **Différentielle** | **max(S, P)** | personne ne perd ; le socle absorbe la première tranche de chaque droit, ce qui le finance en partie | **les premières cotisations deviennent inutiles** : celui dont le droit est inférieur à S reçoit exactement ce que reçoit celui qui n'a jamais cotisé |
| **Additive** | **S + P** | tout euro cotisé compte, toujours | la plus coûteuse : le socle ne se finance sur aucun droit existant |
| **Dégressive** | **S + α·P**, avec 0 < α < 1 | tout euro cotisé compte encore ; le socle est financé par la part (1 − α) de chaque droit | les droits élevés baissent ; le choix de α est entièrement politique |

**Le défaut de l'architecture différentielle est central** et mérite d'être nommé :
c'est un **effet de seuil sur la contributivité**. Il frappe précisément les
carrières courtes, hachées, à bas salaire — majoritairement féminines : la pension
moyenne des femmes est de **1 306 € bruts**, sous le socle envisagé. Une réforme
qui rendrait leurs cotisations sans effet sur leur pension serait perçue, à juste
titre, comme une spoliation.

**L'architecture dégressive est celle qui répond à la question posée** — un socle
au-dessus du seuil de pauvreté **et** une part individuelle liée aux cotisations.
Les régimes de retraite qui combinent une pension de base forfaitaire de résidence
et un étage professionnel proportionnel en sont des variantes.

### A.3 Ordre de grandeur, hypothèse la plus favorable

> **Ce chiffrage est dépassé par une version plus rigoureuse.** Il utilise un montant forfaitaire **par
> personne**, alors que le seuil de pauvreté se calcule **par unité de
> consommation d'un ménage** — un forfait par personne surpaie systématiquement
> les ménages de plusieurs adultes et laisse les moins de 16 ans sans socle
> propre. [docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md)
> refait ce calcul par unité de consommation (≈ 698 Md€ bruts, contre 870 ici)
> et le complète d'une micro-simulation par type de ménage, financée par une
> reprise fiscale sur le socle plutôt que par les bornes « au plus » ci-dessous
> (coût net obtenu : ≈ 25 Md€/an, pas 382). Le tableau qui suit reste ici comme
> trace du premier chiffrage, pas comme référence.

| poste | Md€ |
|---|---:|
| Socle à 1 288 €, 56,3 M de personnes | **870** |
| − prestations monétaires non contributives devenues redondantes | − 96 |
| − dépenses fiscales rendues redondantes (PLF 2026) | − 88 |
| − première tranche des pensions de droit direct, **au plus** | − 266 |
| − première tranche de l'allocation chômage, **au plus** | − 38 |
| **Reste à financer, au mieux** | **≈ 382** |

Soit **29 % des prélèvements obligatoires actuels**, dans l'hypothèse la plus
favorable. Lecture des trois lignes qui portent le calcul :

- **96 Md€ de prestations substituables** : logement, RSA, prime d'activité, autres
  prestations pauvreté, allocations familiales et assimilées, AAH, minimum
  vieillesse. **Ne sont pas comptés** les services en nature (soins, aide sociale à
  l'enfance, hébergement, accueil des jeunes enfants), qui ne disparaissent pas
  parce qu'un revenu est versé.
- **266 Md€ d'absorption des pensions** est un **plafond** : il suppose que chacun des
  17,2 millions de retraités de droit direct touche au moins 1 288 €. La pension
  moyenne est de 1 666 € bruts, mais une part importante des retraités — dont une
  majorité de femmes — est en dessous. **Le chiffre réel exige la distribution des
  pensions par décile**, publiée par la DREES, qui n'est pas chargée.
- **38 Md€ pour le chômage** est l'enveloppe entière de l'allocation, donc aussi un
  plafond.

### A.4 Ce que ce tableau ne dit pas, et qui compte plus que lui

**Le coût brut n'est pas le coût net.** Un socle versé à un salarié qui gagne
correctement sa vie est récupéré par l'impôt sur le revenu : c'est l'équivalence
connue entre un revenu universel assorti d'un impôt proportionnel et un crédit
d'impôt dégressif. Le « reste à financer » ci-dessus est donc **le montant à
récupérer**, pas nécessairement un montant de recettes nouvelles. Tout chiffrage
sérieux passe par une microsimulation qui applique à la fois le socle et le barème
fiscal réformé aux revenus réels des ménages — ce que ce document ne fait pas.

**Le seuil de pauvreté est calculé par unité de consommation.** Un ménage de deux
adultes n'a pas besoin de deux fois le revenu d'une personne seule pour atteindre
le même niveau de vie. Un socle strictement individuel au niveau du seuil place
donc les couples **au-dessus** de ce seuil : c'est un choix de conception, pas un
détail d'arrondi.

**Pour chiffrer précisément**, il manque trois séries : la distribution des
pensions par décile (DREES), celle des allocations chômage (Unédic), et un modèle
de microsimulation du revenu disponible des ménages.

## Versions

- **Version 3** (15 septembre 2026) : type « Hypothèse chiffrée » du plan commun (D-066) ; reprend en annexe A le chiffrage d'un socle universel qui figurait au § 7 de [cotisations-et-droits.md](cotisations-et-droits.md).
- **Version 2** (14 septembre 2026) : coût net recalculé sur les déciles nationaux de niveau de vie (§ 6) et barème de reprise version 2 (§ 7).
