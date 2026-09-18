# La retraite en France : dépense, pensions, minimum vieillesse

> **Dossier** · version 4 · 15 septembre 2026
>
> Combien coûte le système de retraite et comment ce coût a-t-il évolué, comment les
> pensions se distribuent-elles réellement, et que dit le minimum vieillesse d'un système par
> répartition ? Le financement par cotisations est traité dans le dossier
> [cotisations-et-droits.md](cotisations-et-droits.md), sur lequel celui-ci s'appuie.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| retraites | 2789 | 379 | 19 juillet 2024 | 21 juillet 2026 |
| âge de départ | 176 | 91 | 1er octobre 2024 | 2 juillet 2026 |

Une mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Les dépenses de retraite rapportées au PIB, indicateur de soutenabilité** (12 juin 2025). Le COR retient la part des dépenses de retraite dans le PIB comme indicateur déterminant de la soutenabilité financière du système. — Conseil d'orientation des retraites (rapport annuel 2025) · [source](https://www.cor-retraites.fr/sites/default/files/2025-06/Synth%C3%A8se_Def_.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **La loi de financement rectificative de la sécurité sociale pour 2023** (14 avril 2023). Son article 10 porte l'âge d'ouverture des droits à la retraite à soixante-quatre ans, progressivement selon la génération. — Parlement (loi n° 2023-270) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000047445077) · *officiel*

<!-- faits:CADRE:fin -->

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **407 Md€ de dépenses de retraite en 2024, 13,9 % du PIB ; un déficit de 1,7 Md€** (12 juin 2025). Le COR évalue les dépenses de retraite à 407 Md€ en 2024 (13,9 % du PIB, 24,4 % des dépenses publiques) et le solde du système à −1,7 Md€, hors produits et charges financiers. — Conseil d'orientation des retraites (rapport annuel 2025) · [source](https://www.cor-retraites.fr/sites/default/files/2025-06/Synth%C3%A8se_Def_.pdf) · *officiel*

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 1. La dépense : la plus grosse fonction de la protection sociale

Cette série (Drees, comptes de la protection sociale, tous régimes, 2024) :

| fonction | Md€ |
|---|---:|
| VIEILLESSE (pensions de droit direct et dérivé) | 381,6 |
| **VIEILLESSE-SURVIE** (vieillesse + pensions de réversion) | **426,7** |

**La vieillesse est, à elle seule, la première fonction de la protection
sociale française** — devant la santé (338,9 Md€, docs/cotisations-et-droits.md
§ 4) — et de très loin la plus contributive (§ 4 de cette même note : 402,9 Md€
sur 426,7 relèvent d'un droit ouvert par la cotisation, pas par la condition
de ressources).

**La comparaison européenne harmonisée** (Eurostat ESSPROS) situe
la trajectoire dans le temps :

| année | Md€ |
|---|---:|
| 1990 | 89,9 |
| 2000 | 151,0 |
| 2010 | 246,3 |
| 2020 | 319,5 |
| 2023 | 357,5 |

**Multiplication par quatre en trente-trois ans.** Deux moteurs, dans des
proportions que cette seule série ne permet pas de départager : l'augmentation
du nombre de retraités (vieillissement démographique, entrées des générations
nombreuses de l'après-guerre) et la revalorisation des pensions moyennes au
fil des carrières plus longues et mieux rémunérées.

### 2. La distribution des pensions : ce qu'une moyenne ne dit pas

Cette série (Drees, Échantillon interrégimes de retraités 2020, pension brute de droit
direct, 46 tranches de 100 €) — déjà chargée pour la micro-simulation du revenu
universel
([docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md) §
3.1), et directement réutilisable ici :

| tranche | part des retraités |
|---|---:|
| 800-900 € | 5,24 % *(la plus fréquente)* |
| 900-1 000 € | 4,65 % |
| 1 300-1 500 € | 8,92 % *(deux tranches cumulées)* |

**50,3 % des retraités de droit direct perçoivent une pension brute inférieure
au seuil de pauvreté** (1 288 €, voir
[docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md)
§ 3.1) — un fait que la seule pension moyenne (autour de 1 660-1 670 € bruts, tous régimes et compléments confondus) ne montre pas :
une distribution étalée sur plus de 4 500 € masque une moitié de la
population sous un seuil précis. La dispersion par sexe (le même fichier
source) est documentée dans
[docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md) § A.1 : la pension
moyenne des femmes (1 306 € bruts) est déjà sous le seuil de pauvreté à elle
seule.

### 3. L'âge de départ : ce que les réformes changent, visible année par année

Cette série (Drees, âge CONJONCTUREL moyen de départ — calculé sur les seuls départs
d'une année donnée, comme un indice conjoncturel de fécondité, pas l'âge réel d'une
génération) :

<!-- schema:age-depart-retraite -->

Quelques repères de cette série :

| année | femmes | hommes | ensemble |
|---|---:|---:|---:|
| 2004 | 61,12 | 60,06 | 60,59 |
| **2010** | **60,83** | **60,15** | **60,49** *(plus bas de la série)* |
| 2022 | 63,00 | 62,33 | 62,68 |

**L'âge de départ a d'abord légèrement BAISSÉ entre 2004 et 2010** (les
dispositifs de carrière longue introduits en 2003 ont permis des départs
anticipés), **avant de remonter fortement à partir de 2011** — l'effet direct
de la réforme de 2010, qui relève progressivement l'âge légal de 60 à 62 ans.
En douze ans (2010-2022), l'âge conjoncturel moyen a gagné plus de deux ans,
la hausse la plus rapide et la plus continue de la série chargée.

**Les femmes partent systématiquement plus tard que les hommes sur toute la
série** — un écart qui se réduit dans le temps (1,06 an en 2004, 0,67 an en
2022) mais ne s'annule pas : conséquence de carrières en moyenne plus courtes
ou plus hachées, qui obligent à attendre l'âge d'annulation de la décote pour
partir sans pension réduite.

### 4. Le minimum vieillesse (ASV puis ASPA) : une trajectoire en U

Cette série, dispositif `ASV_ASPA` (Allocation supplémentaire vieillesse jusqu'en
2006, Allocation de solidarité aux personnes âgées depuis le 13 janvier 2007 — la
Drees les suit comme une seule série continue) :

| année | allocataires |
|---|---:|
| 1990 | 1 182 900 |
| 2000 | 686 000 |
| **2010** | **511 200** *(plus bas de la série)* |
| 2024 | 693 200 |

**Le nombre de bénéficiaires du minimum vieillesse a baissé pendant vingt ans,
puis remonte depuis le milieu des années 2010.** La baisse initiale reflète
l'amélioration progressive des carrières et des droits propres, notamment des
femmes ; la remontée récente coïncide avec l'arrivée à la retraite de
générations aux carrières plus hachées (chômage, temps partiel subi) et avec
un changement de méthode de comptage introduit en 2021 par la Drees (les
effectifs sont désormais comptés en date d'entrée en jouissance du droit,
plutôt qu'en date de versement selon les caisses) — **une partie de la hausse
récente est donc une rupture de série documentée par la source, pas
seulement un effet démographique**, et les deux ne sont pas séparables avec
les seules données publiées.

**Dépense** (Md€ constants 2024) : **2 453 M€ en 2009 → 4 586 M€ en 2024**, soit **+87
%** — une hausse bien supérieure à celle des effectifs (+34 % sur la même période,
2009 : 517 000 → 2024 : 693 200), qui traduit une revalorisation réelle du montant
individuel de l'Aspa, pas seulement davantage de bénéficiaires.

### 5. Le taux de remplacement : ce que la pension remplace vraiment

Cette série (Drees, cohortes 2012-2020, quantiles à 10, 25, 50, 75 et 90 %) mesure la
part du revenu d'avant la retraite que la pension remplace — 100 signifie une pension
égale au revenu antérieur. Rapporté au **niveau de vie** (qui lisse les revenus au
sein du ménage, pas seulement le revenu personnel), cohorte 2020 :

<!-- schema:taux-remplacement-retraite -->

En chiffres :

| | q10 | q25 | **médiane** | q75 | q90 |
|---|---:|---:|---:|---:|---:|
| Ensemble | 66,3 | 80,9 | **97,4** | 118,5 | 150,9 |
| Femmes | 68,4 | 82,9 | **99,0** | 119,0 | 148,1 |
| Hommes | 64,1 | 78,9 | **95,4** | 117,5 | 155,3 |

**La moitié des nouveaux retraités voient leur niveau de vie proche de ce
qu'il était avant la retraite (médiane à 97,4 %), mais la dispersion est
large** : un dixième garde moins des deux tiers de son niveau de vie
antérieur (q10 = 66,3), un dixième voit son niveau de vie augmenter de plus de
moitié (q90 = 150,9 — un patrimoine ou des revenus d'activité qui cessent au
profit d'une pension plus favorable, ou un conjoint dont la situation change).

**Fait à contre-courant de l'intuition** : à niveau de vie égal avant la
retraite, une **carrière incomplète** donne un taux de remplacement MÉDIAN
plus élevé (102,4) qu'une **carrière complète** (95,1). Ce n'est pas un
avantage aux carrières courtes : c'est l'effet des mécanismes de solidarité
(minimum contributif, minimum garanti) qui portent la pension d'une carrière
courte à un niveau plancher, proportionnellement plus haut par rapport à un
revenu d'activité qui, pour ces mêmes carrières, était déjà plus faible.

**Femmes et hommes**, à l'inverse, montrent un écart resserré au sommet de la
distribution (q90 : 148,1 contre 155,3) mais plus marqué à la médiane (99,0
contre 95,4) — cohérent avec des pensions personnelles plus faibles pour les
femmes (§ 2) compensées, au niveau du MÉNAGE, par les revenus du conjoint
avant la retraite étant eux-mêmes souvent plus élevés que le revenu propre de
la femme, ce qui mécaniquement abaisse la base de comparaison et remonte le
taux de remplacement mesuré sur le niveau de vie.

### 6. Le ratio cotisants / retraités : la pression démographique, chiffrée

Cette série (Insee, 2004-2023, tous régimes) :

| année | cotisants (M) | retraités (M) | ratio |
|---|---:|---:|---:|
| 2004 | 26,2 | 13,0 | **2,02** |
| 2010 | 26,9 | 15,1 | 1,79 |
| **2016** | 27,7 | 16,1 | **1,72** *(plus bas de la série)* |
| 2020 | 28,6 | 16,7 | 1,72 |
| 2023 | 30,4 | 17,2 | 1,77 |

**Le ratio s'est dégradé sans interruption de 2004 à 2016** (2,02 → 1,72)
**puis s'est stabilisé, voire légèrement redressé, de 2016 à 2023**
(1,72 → 1,77) — grâce à une hausse des cotisants (+9,8 % sur la période) plus
rapide que celle des retraités (+6,6 %), elle-même en partie l'effet du recul
de l'âge de départ mesuré au § 3. **Le ratio ne s'est PAS dégradé continûment
jusqu'à aujourd'hui**, contrairement à une intuition répandue : la dernière
décennie chargée ici montre une stabilisation, pas un effondrement.

## Ce que les données ne disent pas

### 7. Ce qui est hors de portée de l'open data

- **La distribution des pensions par décile ou par CSP** au-delà des 46
  tranches de l'EIR 2020 (le prochain échantillon, EIR 2024, n'était pas
  encore publié au moment de l'écriture).
- **Les projections du Conseil d'orientation des retraites (COR)** : le jeu
  data.gouv.fr correspondant est orphelin depuis 2016, organisation
  supprimée ; les séries actuelles n'existent qu'en XLSX joints aux rapports
  annuels, sans URL stable d'une édition à l'autre (déjà documenté en
  [docs/budget-donnees.md](budget-donnees.md) § 4.3).
- **Les éditions annuelles Drees « Effectifs de retraités, montants des
  pensions et âges de départ »** (jeu de données n° 1393) : une nouvelle
  édition par an, sous un nom de fichier et parfois un format (`.xls` puis
  `.xlsx`) qui changent d'une année à l'autre, sans schéma commun exploitable
  par un connecteur unique — la même limite que le détail des titres de
  séjour par motif en
  [docs/immigration-donnees.md](immigration-donnees.md) § 7. Un extrait plus
  étroit mais stable de ce même sujet — le seul âge de départ, sans les
  effectifs ni les montants — existe séparément et EST chargé (§ 3).
- **AGIRC-ARRCO** (retraite complémentaire, hors LFSS mais dans les
  administrations de sécurité sociale au sens comptable) : **aucun portail
  d'open data identifié**, malgré une recherche dédiée. Les chiffres existent
  — 20,0 millions de cotisants fin 2023 — mais uniquement dans des
  publications PDF (« Chiffr'Agirc-Arrco », édition annuelle) et des pages web
  non structurées (« Cotisants et salaires », agirc-arrco.fr) : rien à
  télécharger sous une forme qu'un connecteur puisse relire.
- **La cotisation retraite isolée** dans les encaissements URSSAF : même
  limite que la cotisation chômage, voir
  [docs/chomage-donnees.md](chomage-donnees.md) § 4.2 — l'URSSAF ne publie pas
  ses comptes par branche.

## Sources

- Drees, *Les comptes de la protection sociale* (jeu de données ouvert).
- Eurostat, `spr_exp_fol` (dépense ESSPROS, fonction vieillesse).
- Drees, *Distribution des pensions mensuelles*, Échantillon interrégimes de
  retraités 2020 (jeu de données n° 4178).
- Drees, *Minima sociaux, RSA et prime d'activité* (jeu de données n° 336),
  dispositif ASV/ASPA.
- Drees, *Âge conjoncturel moyen de départ à la retraite selon le sexe*.
- Drees, *Répartition des taux de remplacement entre les revenus juste avant
  et juste après la retraite*.
- Insee, *Retraités et retraites* — fichier « Cotisants et retraités de droit
  direct » (`reve-protec-cotisant-retraite.xlsx`).
- [docs/cotisations-et-droits.md](cotisations-et-droits.md), pour le
  financement par répartition et la distinction contributif/non contributif.

## Annexe technique

### 8. Ce qui est chargé

| # | Source | Chargée pour |
| --- | --- | --- |
| 1 | Drees, comptes de la protection sociale | docs/budget-donnees.md |
| 2 | Eurostat ESSPROS, dépense fonction vieillesse | cette note |
| 3 | Drees, Échantillon interrégimes de retraités 2020 | docs/revenu-universel-microsimulation.md |
| 4 | Drees, minima sociaux — dispositif ASV/ASPA | docs/chomage-donnees.md |
| 5 | Insee, population par âge | docs/revenu-universel-microsimulation.md |
| 6 | Drees, âge conjoncturel moyen de départ à la retraite | cette note |
| 7 | Drees, répartition des taux de remplacement | cette note |
| 8 | Insee, cotisants et retraités de droit direct | cette note |

Les lignes 2, 6, 7 et 8 sont les séries chargées spécifiquement pour cette
note ; les quatre autres existaient déjà — la preuve que les sujets de ce
projet se recoupent plus qu'ils ne s'empilent.

## Versions

- **Version 4** (15 septembre 2026) : plan commun des dossiers ; cadre (loi de 2023) et évaluation du Conseil d'orientation des retraites.
- **Version 3** (14 septembre 2026) : taux de remplacement par quantile (§ 5), ratio cotisants / retraités (§ 6), absence de données ouvertes AGIRC-ARRCO (§ 7).
- **Version 2** : âge de départ à la retraite depuis 2004 (§ 3).
