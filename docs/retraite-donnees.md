# La retraite en France : dépense, pensions, minimum vieillesse

> Note de synthèse. Version 1 — 14 septembre 2026.
> Trois questions, sur le modèle des notes voisines : combien coûte le système
> de retraite et comment ce coût a évolué, comment les pensions se distribuent
> réellement (pas seulement en moyenne), et ce que le minimum vieillesse
> raconte d'un système par répartition sous tension démographique. Le
> financement par cotisations et son évolution récente sont traités en détail
> dans [docs/cotisations-et-droits.md](cotisations-et-droits.md) — cette note
> ne les répète pas, elle s'appuie dessus.

---

## 1. La dépense : la plus grosse fonction de la protection sociale

`core.protection_sociale` (Drees, comptes de la protection sociale, tous
régimes, 2024) :

| fonction | Md€ |
|---|---:|
| VIEILLESSE (pensions de droit direct et dérivé) | 381,6 |
| **VIEILLESSE-SURVIE** (vieillesse + pensions de réversion) | **426,7** |

**La vieillesse est, à elle seule, la première fonction de la protection
sociale française** — devant la santé (338,9 Md€, docs/cotisations-et-droits.md
§ 4) — et de très loin la plus contributive (§ 4 de cette même note : 402,9 Md€
sur 426,7 relèvent d'un droit ouvert par la cotisation, pas par la condition
de ressources).

**La comparaison européenne harmonisée** (`core.macro_value`, série
`protection.depense.vieillesse`, Eurostat ESSPROS) situe la trajectoire dans
le temps :

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

## 2. La distribution des pensions : ce qu'une moyenne ne dit pas

`core.pension_tranche_eir` (Drees, Échantillon interrégimes de retraités
2020, pension brute de droit direct, 46 tranches de 100 €) — déjà chargée pour
la micro-simulation du revenu universel
([docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md)
§ 3.1), et directement réutilisable ici :

| tranche | part des retraités |
|---|---:|
| 800-900 € | 5,24 % *(la plus fréquente)* |
| 900-1 000 € | 4,65 % |
| 1 300-1 500 € | 8,92 % *(deux tranches cumulées)* |

**50,3 % des retraités de droit direct perçoivent une pension brute inférieure
au seuil de pauvreté** (1 288 €, voir
[docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md)
§ 3.1) — un fait que la seule pension moyenne (souvent citée autour de
1 660-1 670 € bruts, tous régimes et compléments confondus) ne montre pas :
une distribution étalée sur plus de 4 500 € masque une moitié de la
population sous un seuil précis. La dispersion par sexe (le même fichier
source) est documentée dans
[docs/cotisations-et-droits.md](cotisations-et-droits.md) § 7.1 : la pension
moyenne des femmes (1 306 € bruts) est déjà sous le seuil de pauvreté à elle
seule.

## 3. Le minimum vieillesse (ASV puis ASPA) : une trajectoire en U

`core.minima_sociaux_effectif`, dispositif `ASV_ASPA` (Allocation
supplémentaire vieillesse jusqu'en 2006, Allocation de solidarité aux
personnes âgées depuis le 13 janvier 2007 — la Drees les suit comme une seule
série continue) :

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

**Dépense** (`core.minima_sociaux_depense`, Md€ constants 2024) : **2 453 M€
en 2009 → 4 586 M€ en 2024**, soit **+87 %** — une hausse bien supérieure à
celle des effectifs (+34 % sur la même période, 2009 : 517 000 → 2024 :
693 200), qui traduit une revalorisation réelle du montant individuel de
l'Aspa, pas seulement davantage de bénéficiaires.

## 4. Ce qui est hors de portée de l'open data

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
  [docs/immigration-donnees.md](immigration-donnees.md) § 7.
- **La cotisation retraite isolée** dans les encaissements URSSAF : même
  limite que la cotisation chômage, voir
  [docs/chomage-donnees.md](chomage-donnees.md) § 4.2 — l'URSSAF ne publie pas
  ses comptes par branche.

## 5. Ce qui est chargé

Rien de nouveau n'a été ajouté spécifiquement pour cette note : elle s'appuie
entièrement sur des tables déjà chargées pour d'autres besoins, preuve que les
sujets de ce projet se recoupent plus qu'ils ne s'empilent.

| # | Source | Table | Chargée pour |
|---|---|---|---|
| 1 | Drees, comptes de la protection sociale | `core.protection_sociale` | docs/budget-donnees.md |
| 2 | Eurostat ESSPROS, dépense fonction vieillesse | `core.macro_value` (`protection.depense.vieillesse`) | cette note |
| 3 | Drees, Échantillon interrégimes de retraités 2020 | `core.pension_tranche_eir` | docs/revenu-universel-microsimulation.md |
| 4 | Drees, minima sociaux — dispositif ASV/ASPA | `core.minima_sociaux_effectif`, `core.minima_sociaux_depense` | docs/chomage-donnees.md |
| 5 | Insee, population par âge | `core.population_age` | docs/revenu-universel-microsimulation.md |

La ligne 2 (`protection.depense.vieillesse`) est la seule série chargée
spécifiquement pour cette note ; les quatre autres existaient déjà.

## Sources

- Drees, *Les comptes de la protection sociale* (jeu de données ouvert).
- Eurostat, `spr_exp_fol` (dépense ESSPROS, fonction vieillesse).
- Drees, *Distribution des pensions mensuelles*, Échantillon interrégimes de
  retraités 2020 (jeu de données n° 4178).
- Drees, *Minima sociaux, RSA et prime d'activité* (jeu de données n° 336),
  dispositif ASV/ASPA.
- [docs/cotisations-et-droits.md](cotisations-et-droits.md), pour le
  financement par répartition et la distinction contributif/non contributif.
