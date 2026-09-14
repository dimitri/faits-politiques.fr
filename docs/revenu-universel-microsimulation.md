# Le seuil de pauvreté se lit par ménage : ce que cela change pour un revenu universel

> Note de méthode. Version 1 — 14 septembre 2026.
> **Cette note chiffre une hypothèse de politique publique, comme
> [docs/cotisations-et-droits.md](cotisations-et-droits.md) §7 dont elle prend la
> suite.** Elle n'est pas un constat et ne relève pas du périmètre factuel du
> projet (voir [docs/perimetre.md](perimetre.md) §2.1 et §2.2). Elle répond à une
> question précise et laisse le jugement au lecteur.
>
> §7.3 de la note précédente chiffrait le coût d'un socle universel avec un
> montant forfaitaire **par personne**. Ce chiffrage était cohérent avec les
> données alors disponibles, mais il ignore un fait qui change le résultat de
> façon substantielle : le seuil de pauvreté ne se calcule pas par personne, il
> se calcule **par unité de consommation d'un ménage**. Cette note en tire les
> conséquences, puis micro-simule le financement par une reprise fiscale ciblée
> sur le socle versé aux ménages aisés — pas par un impôt distinct.

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

§7.3 de la note précédente comptait 56,3 millions de résidents de 16 ans ou
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

| | forfait par personne (§7.3, ancien) | socle par UC (ici) |
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
  inférieur aux 382 Md€ « au mieux » du §7.3 de la note précédente : la
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

### 3.3 Conséquence pour le chiffrage du §2

Le tableau du §2.3 traite chaque type de ménage comme homogène, alors qu'il ne
l'est pas — un « couple sans enfant » recouvre aussi bien un couple de deux
smicards qu'un couple de deux cadres. **Le biais mesuré aux §3.1 et 3.2 va
systématiquement dans le même sens : le calcul sur moyenne SOUS-ESTIME la
reprise véritable**, et donc SURESTIME le coût net du socle. Le chiffre de
25-26 Md€/an du §2.3 est, par construction, un plafond — pas une estimation
centrale. Une micro-simulation menée sur des données individuelles (comme
Ines, le modèle de micro-simulation socio-fiscale de la Drees et de l'Insee)
donnerait un chiffre plus bas, dans une proportion que cette note ne peut pas
établir sans accès à ces données individuelles.

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
