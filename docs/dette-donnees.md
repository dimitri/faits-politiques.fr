# La dette publique : ce que les sources permettent d'établir

> **Dossier** · version 2 · 15 septembre 2026
>
> Combien la France doit-elle, selon quelle définition, à qui emprunte-t-elle, que coûte
> l'emprunt, comment se compare-t-elle à ses voisins, et comment la Suisse encadre-t-elle sa
> propre dette ? Tous les chiffres sont lus dans la base après chargement, pas recopiés de
> publications.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| dette publique | 151 | 82 | 1er octobre 2024 | 10 juin 2026 |
| charge de la dette | 75 | 38 | 14 octobre 2024 | 7 juillet 2026 |

Une mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Le déficit rapporté au PIB le plus élevé de la zone euro, selon la Commission européenne** (10 décembre 2025). Le rapport cite un déficit public estimé par le Gouvernement à 5,4 points de PIB en 2025 (5,8 en 2024), qui serait selon la Commission européenne le plus élevé de la zone euro. — Sénat, commission des affaires sociales (rapport sur le PLFSS 2026) · [source](https://www.senat.fr/lessentiel/plfss2026.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **La France sous procédure de déficit excessif depuis juillet 2024** (26 juillet 2024). La France est à nouveau sous procédure européenne de déficit excessif depuis juillet 2024 et s'est engagée à ramener son déficit public sous 3 points de PIB en 2029. — Sénat, commission des affaires sociales (rapport sur le PLFSS 2026) · [source](https://www.senat.fr/lessentiel/plfss2026.pdf) · *officiel*
- **La loi de programmation des finances publiques 2023-2027** (18 décembre 2023). Trajectoire pluriannuelle de solde et de dette publics à laquelle le Haut Conseil des finances publiques compare chaque budget. — Parlement (loi n° 2023-1195) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000048581885) · *officiel*

<!-- faits:CADRE:fin -->

### 1. Les définitions : quatre montants de dette, trois grandeurs liées

#### 1.1 Une question, quatre montants

« La dette de la France » désigne au moins quatre grandeurs différentes, toutes
officielles. Au premier trimestre 2026 :

| Définition | Montant | Producteur | Ce qu'elle mesure |
|---|---|---|---|
| **Dette brute au sens du FMI** | **3 818,0 Md€** | INSEE | Tous les passifs sauf actions et dérivés, y compris les factures fournisseurs non payées (« autres comptes à payer », 241,9 Md€) |
| **Dette Maastricht** | **3 536,1 Md€** — 117,5 % du PIB | INSEE | Dette brute **consolidée** des administrations publiques, en valeur nominale : le chiffre des traités européens et du débat public |
| **Dette nette** | **3 301,1 Md€** | INSEE | Maastricht moins la trésorerie et certains actifs financiers liquides |
| **Dette négociable de l'État** | **2 823,8 Md€** (mars 2026) — **2 901,9 Md€** (juillet 2026) | AFT, republiée par l'INSEE | Les seuls titres émis par l'Agence France Trésor (OAT, BTF), pour le seul État |

Les quatre sont justes. **Une phrase qui compare deux chiffres de dette sans dire
lesquels compare le plus souvent deux définitions.** Le site doit donc toujours
nommer la définition, et le modèle de données la porte dans une colonne (`concept`),
pas dans un libellé.

La dette Maastricht se répartit ainsi (contributions consolidées, T1 2026) : État
2 889,0 Md€, organismes divers d'administration centrale 69,3 Md€, collectivités
locales 276,5 Md€, administrations de sécurité sociale 301,2 Md€. Par instrument :
titres 3 170,8 Md€, crédits 322,3 Md€, dépôts 42,9 Md€.

#### 1.2 Déficit, dette, intérêts : trois grandeurs liées, pas interchangeables

- Le **déficit** est un **flux** : ce qui manque dans l'année. 2025 : recettes
  publiques 1 561,6 Md€, dépenses 1 714,1 Md€, **besoin de financement 152,5 Md€
  (5,1 % du PIB)**.
- La **dette** est un **stock** : l'accumulation des déficits passés, qu'on
  refinance à mesure que les titres arrivent à échéance.
- Les **intérêts** sont une dépense **incluse** dans le déficit : **66,6 Md€ en
  2025** (2,2 % du PIB, 4,27 % des recettes publiques).

La dette n'augmente pas **exactement** du déficit. L'écart s'appelle
**l'ajustement stock-flux** (vue `derived.dette_ajustement_stock_flux`) : 2020,
dette +276,5 Md€ pour un déficit de 207,1 Md€ (+69,4 Md€, trésorerie constituée
pendant la crise sanitaire) ; 2024, +202,8 Md€ pour 169,1 Md€ (+33,7 Md€) ; 2025,
+154,4 Md€ pour 152,5 Md€ (+1,9 Md€). Sur 1995-2025, la France n'a connu **aucune
année d'excédent** dans la série d'Eurostat.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Une dette à près de 118 points de PIB en 2026, une charge d'intérêts de 74 Md€** (9 octobre 2025). Selon le Haut Conseil, la dette publique passerait de plus de 113 points de PIB en 2024 à près de 118 en 2026, et la charge d'intérêts atteindrait 74 Md€, en hausse de plus de 13 Md€ en deux ans (prévision). — Haut Conseil des finances publiques (avis n° HCFP-2025-5) · [source](https://www.hcfp.fr/sites/default/files/2025-10/Avis%20HCFP%202025%20%E2%80%93%205%20PLF-PLFSS%202026_0.pdf) · *officiel*

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. Le prix de l'emprunt : taux de marché et taux apparent

#### 2.1 Taux de marché et taux apparent

- **Le taux à 10 ans** (Eurostat `irt_lt_mcby`, rendement des emprunts d'État sur
  le marché secondaire) est le prix **d'un emprunt neuf**, aujourd'hui. France :
  −0,34 % en août 2019, **4,00 % en août 2026**, son plus haut depuis 2010 — et,
  ce mois-là, au-dessus de l'Italie (3,99 %) et de la Grèce (3,87 %).
- **Le taux apparent** (vue `derived.dette_taux_apparent`) est ce que coûte **le
  stock** : intérêts versés dans l'année ÷ dette moyenne de l'année. Il mélange des
  titres émis à des dates et des taux très différents.

| Année | Taux apparent (France) | Taux à 10 ans (moyenne annuelle) |
|---|---|---|
| 2008 | 4,30 % | 4,23 % |
| 2015 | 2,13 % | 0,84 % |
| 2020 | 1,18 % | −0,15 % |
| 2022 | 1,76 % | 1,70 % |
| 2023 | 1,76 % | 2,99 % |
| 2025 | **1,97 %** | **3,35 %** |

**La lecture juste** : une hausse des taux ne renchérit pas la dette entière le
lendemain. Elle ne touche que ce qu'on emprunte **ensuite** — les déficits nouveaux
et les titres anciens qu'on refinance à leur échéance. Le taux apparent suit donc
le taux de marché **avec plusieurs années de retard**. De 2015 à 2021, il est resté
au-dessus du taux de marché (les vieux titres à 4 % s'éteignaient lentement) ;
depuis 2023, il est en dessous et remonte (on remplace des titres à 0 % par des
titres à 3 %). **Les deux phrases « la charge de la dette explose » et « la dette
coûte moins de 2 % » sont vraies en même temps.**

#### 2.2 La dette indexée

312,8 Md€ de la dette négociable (juillet 2026, 10,8 %) sont **indexés sur
l'inflation** (OATi sur l'inflation française, OAT€i sur celle de la zone euro) :
leur nominal est revalorisé chaque année. En 2022-2023, l'inflation a ainsi
alourdi la charge sans aucune hausse de taux. L'INSEE en publie l'encours ; la
revalorisation annuelle elle-même relève du programme 117 du budget (voir
`budget-donnees.md`), non chargé ici.

#### 2.3 La demande des investisseurs : les adjudications

L'AFT emprunte par **adjudication** : elle annonce un montant, les banques
« spécialistes en valeurs du Trésor » font des offres, les meilleures sont servies.
Le **taux de couverture** rapporte les offres au montant adjugé (RAP 2025,
programme 117) : **OAT 2,30 en 2023, 2,45 en 2024, 2,97 en 2025** ; BTF 2,67 →
3,19 → 3,40. **Aucune adjudication non couverte** n'est rapportée. La dette
française ne manque pas de preneurs ; la question est le prix qu'ils demandent.

### 3. À qui la France emprunte

#### 3.1 Les détenteurs des titres de l'État (Banque de France, T1 2026)

Vue `derived.dette_detention_etat`, sur 2 602 Md€ de titres **en valeur de marché** :

| Détenteur | Part | Encours |
|---|---|---|
| **Non-résidents** | **57,5 %** | 1 496 Md€ |
| **Banque de France** | **18,1 %** | 470 Md€ |
| Banques résidentes | 10,5 % | 274 Md€ |
| Assurances | 9,6 % | 249 Md€ |
| Fonds de placement | 1,8 % | 47 Md€ |
| Administrations publiques | 0,9 % | 24 Md€ |
| Fonds de pension | 0,9 % | 22 Md€ |
| Ménages (en direct) | 0,01 % | 0,2 Md€ |

Trois lectures, et trois contresens à éviter :

1. **« Non-résident » n'est pas « étranger ».** C'est le **domicile** du détenteur :
   la filiale luxembourgeoise d'un assureur français, un fonds irlandais qui gère
   l'épargne de ménages français, la Banque centrale européenne elle-même sont non
   résidents. La statistique ne dit pas quelle part appartient à des intérêts
   étrangers, et aucune source publique ne le dit.
2. **Les ménages ne détiennent presque rien en direct** — mais beaucoup en
   indirect, via l'assurance-vie (les 249 Md€ des assureurs) et les fonds.
3. **La part de la Banque de France est l'empreinte des rachats de l'Eurosystème**
   (PSPP, PEPP) : 0,8 % fin 2008, 12,7 % fin 2016, **28,1 % fin 2022**, puis décrue
   à 18,1 % avec l'arrêt des réinvestissements. Pendant ce temps, la part des
   non-résidents est passée de 67,9 % (fin 2009) à 47,5 % (fin 2021), puis remonte
   (56,1 % fin 2025) : quand la banque centrale cesse d'acheter, d'autres doivent
   le faire — et ce sont surtout des investisseurs non résidents.

Quand son résultat est positif, la Banque de France en reverse l'essentiel à l'État
(dividende et impôt) : une partie des intérêts qu'elle perçoit sur ses titres revient
alors au budget. Ce circuit dépend de son résultat annuel, qui varie fortement avec
les taux, et **n'est pas chiffré ici** (il demanderait ses comptes annuels).

#### 3.2 La même question en Europe (Eurostat, 2025, dette Maastricht)

Part de la dette détenue par des non-résidents : Grèce 70,8 %, Autriche 66,2 %,
Belgique 63,4 %, Finlande 61,2 %, Irlande 55,0 %, **France 54,3 %**, Allemagne
51,3 %, Pays-Bas 49,2 %, Espagne 48,9 %, **Italie 34,3 %**, Suède 23,0 %.
L'Italie est un contre-exemple à retenir : dette plus lourde (137,1 % du PIB),
mais davantage détenue en interne, dont 11,8 % par les ménages en direct.

**Ne pas rapprocher les 54,3 % d'Eurostat des 57,5 % de la Banque de France** : le
premier porte sur toute la dette Maastricht en valeur nominale (crédits et dette
locale compris), le second sur les seuls titres de l'État en valeur de marché.

### 4. Les échéances

Deux conventions incompatibles coexistent (colonne `base_echeance`) :

- **À l'émission** (INSEE, AFT, Banque de France) : un titre à 10 ans reste « long
  terme » jusqu'à la veille de son remboursement. Dette négociable à court terme
  (BTF, un an et moins) : 219,6 Md€ en juillet 2026, soit 7,6 %.
- **Durée restant à courir** (Eurostat `gov_10dd_ggd`) : ce qui arrive à échéance
  dans l'année, quelle que soit la durée initiale. France 2025 : 10,9 points de PIB
  sur 115,6, soit **9,4 % de la dette remboursable dans l'année**.

Pour la France, Eurostat ne publie que la coupure « un an et moins / plus d'un
an » ; les tranches 1-5, 5-10, 10-30 ans existent pour d'autres pays. L'échéancier
titre par titre, et la durée de vie moyenne de la dette, sont publiés par l'AFT
sur un site inaccessible aux robots (§ 7).

### 5. La comparaison européenne (Eurostat, 2025)

| Pays | Dette (% PIB) | Solde (% PIB) | Intérêts (% PIB) |
|---|---|---|---|
| Grèce | 146,1 | +1,7 | 3,2 |
| Italie | 137,1 | −3,1 | 3,9 |
| **France** | **115,6** | **−5,1** | **2,2** |
| Belgique | 107,9 | −5,2 | 2,2 |
| Espagne | 100,7 | −2,4 | 2,4 |
| Portugal | 89,7 | +0,7 | 1,9 |
| Zone euro (20) | 87,8 | −2,9 | 1,9 |
| Union (27) | 81,7 | −3,1 | 1,9 |
| Allemagne | 63,5 | −2,7 | 1,1 |
| Pays-Bas | 44,4 | −1,6 | 0,7 |
| Suède | 35,1 | −1,4 | 0,6 |

La singularité française de 2025 n'est pas le niveau de dette (l'Italie et la
Grèce sont plus haut) mais **le déficit** : le plus élevé du tableau avec la
Belgique, là où le Portugal et la Grèce sont en excédent.

Pour les pays hors de l'Union, le FMI (dernière année observée, dette brute au sens
du FMI, plus large que Maastricht) : Japon 214,5 % (2024), États-Unis 123,9 %,
Royaume-Uni 102,3 %, Norvège 52,8 % (2024), **Suisse 39,4 %** (2025).

### 6. La Suisse : sa dette et son frein à l'endettement

#### 6.1 La Suisse a une dette

**39,4 % du PIB en 2025** (FMI), 40,5 % en 2024. Elle culminait à 57,1 % en 2004.
La Confédération seule porte 91,3 Md CHF d'engagements financiers (11,9 Md à court
terme, 79,4 Md à long terme, AFF 2025), contre 111,0 Md CHF en 2003. La question
juste n'est pas « pourquoi n'a-t-elle pas de dette », mais **« pourquoi sa dette
baisse-t-elle en proportion quand celle de la France monte »**.

#### 6.2 Le frein à l'endettement

- **Article 126 de la Constitution fédérale**, adopté par votation populaire le
  **2 décembre 2001** (84,7 % de oui), appliqué au budget fédéral depuis 2003.
- **La règle** : sur un cycle conjoncturel, les dépenses de la Confédération ne
  doivent pas dépasser ses recettes **corrigées de la conjoncture**. En récession,
  un déficit est permis ; en haute conjoncture, un excédent est exigé. Ce n'est
  **pas** une interdiction de tout déficit.
- **Le compte de compensation** : les écarts à la règle y sont inscrits et doivent
  être résorbés les années suivantes. Les dépenses extraordinaires (Covid) passent
  par un compte d'amortissement distinct, lui aussi à rembourser.
- La règle ne vaut **que pour la Confédération** : les cantons ont leurs propres
  freins, de rigueur inégale.

#### 6.3 Ce que montrent les données

- **Solde des administrations publiques** (Eurostat `gov_10a_main`) : **17 années
  d'excédent sur 30** (1995-2024) en Suisse, dont 2006-2012 et 2015-2019 sans
  interruption ; en France, **aucune**.
- **Charge d'intérêts** : 0,3 % du PIB en 2024, contre 1,7 % en 2000 ; France 2,2 %.
- **Taux** : la Confédération empruntait à 10 ans à 0,38 % en juillet 2025 (BNS).
  **La série de la BNS n'est plus mise à jour depuis** : ne pas la citer comme un
  taux actuel.
- **Ratios de l'AFF** : dette nette de l'ensemble des administrations publiques =
  47 % de leurs recettes fiscales (2024) ; assurances sociales **créancières
  nettes** (quotient négatif, −0,98 en 2025).

#### 6.4 Les limites de la comparaison

Le frein explique la **trajectoire** de la dette fédérale, pas tout l'écart avec la
France. Y contribuent aussi un taux d'intérêt structurellement bas (le franc est une
monnaie refuge), un poids de l'État plus faible, un système de retraite largement
capitalisé hors du périmètre public, et le fédéralisme budgétaire. Aucune de ces
causes ne se lit seule dans une série : **le site montre la règle et les courbes,
il n'impute pas l'écart à la règle.**

### 13. À quoi sert la dette, et ce qu'on met en regard

> Ajouté le 14 septembre 2026 (migration 0075). Deux questions posées ensemble :
> **à quoi sert la dette contractée** (fonctionnement, investissement, autre), et
> **peut-on la mettre en parallèle des exonérations et des dividendes**, pour
> examiner l'argument selon lequel les aides aux grandes entreprises seraient
> financées par les ménages et endetteraient le pays.

#### 13.1 Ce que la comptabilité permet de dire

**L'emprunt n'est affecté à aucune dépense.** L'argent public est fongible : aucune
source ne peut dire « ces 100 Md€ empruntés ont payé ceci ». La question admet en
revanche une réponse exacte **en comptabilité nationale**, par le compte de capital
(vue `derived.dette_compte_capital`, identité vérifiée à 1,5 M€ près) :

    besoin de financement = épargne brute négative + investissement + transferts en capital nets
    (−B9)                 = (−B8G)                 + (P5 + NP)      + (D9PAY − D9REC)

- **Épargne brute négative** : les recettes courantes ne couvrent pas les dépenses
  courantes (salaires, prestations, achats, intérêts). Ce qui manque est emprunté.
- **Investissement** : routes, bâtiments, équipements, logiciels.
- **Transferts en capital** : aides à l'investissement versées à d'autres.

C'est la lecture de la « règle d'or », et elle a une **convention** : elle finance
d'abord le courant par le courant. Elle ne prouve pas qu'un euro emprunté a payé un
investissement plutôt qu'un salaire.

| Administrations publiques | 2019 | 2020 | 2023 | 2024 | 2025 |
|---|---|---|---|---|---|
| Besoin de financement | 58,2 | 207,1 | 151,9 | 169,1 | 152,5 |
| … épargne brute négative (courant non couvert) | −59,0 | **83,2** | **17,3** | **22,3** | −0,8 |
| … investissement (P5 + NP) | 104,7 | 101,5 | 122,6 | 132,6 | 137,2 |
| … transferts en capital nets | 12,5 | 22,4 | 12,1 | 14,2 | 16,1 |
| Investissement **net** de l'usure (P51G − P51C) | 10,6 | 4,5 | 11,9 | 17,1 | 21,2* |

Md€, Eurostat `gov_10a_main`. Une épargne brute négative affichée « −59,0 » signifie
une épargne **positive** de 59 Md€. *2025 : la consommation de capital fixe est
reprise à l'identique de 2024 par Eurostat, estimation à ne pas citer seule.

**Lecture.** Les dépenses courantes n'ont été financées par l'emprunt que certaines
années : 2009-2010, 2020-2021, 2023-2024. Les autres années, l'épargne brute est
positive, et l'emprunt correspond, en convention brute, à de l'investissement. **En
convention nette** (épargne nette, B8N, après usure des équipements), le constat
s'inverse : −132,7 Md€ en 2024, −109,7 Md€ en 2025. Les deux lectures sont exactes ;
la page doit montrer les deux et nommer la convention.

**Par sous-secteur (2024).** L'État (S1311) emprunte 88,1 Md€ pour son courant ; les
collectivités locales dégagent 49,5 Md€ d'épargne brute (la loi leur interdit
d'emprunter pour fonctionner) ; les administrations de sécurité sociale 16,3 Md€.
**Piège** : les comptes par sous-secteur ne sont pas consolidés entre eux. Les
dotations de l'État aux collectivités sont une dépense courante **de l'État** qui
finance en partie l'investissement **des collectivités**. Le déficit courant de
l'État est donc surestimé, et l'épargne des collectivités gonflée, par cette
convention.

**Comparaison (2024, épargne brute en % des dépenses).** Pologne −2,3 ; Belgique
−1,7 ; **France −1,3** ; Espagne −0,8 ; zone euro +2,4 ; Italie +3,3 ; Allemagne
+3,7 ; Suisse +13,6.

**La dépense par nature** (2025, Md€, somme vérifiée égale à la dépense totale) :
prestations en espèces 579,5 ; rémunérations 370,0 ; prestations en nature 191,5 ;
consommations intermédiaires 163,0 ; formation de capital 134,2 ; autres transferts
courants 94,0 ; revenus de la propriété (dont intérêts) 66,7 ; subventions 56,5 ;
transferts en capital 41,6.

#### 13.2 Les dépenses fiscales (« niches »)

| Source | Millésime | Années chiffrées | Nature du bénéficiaire |
|---|---|---|---|
| Voies et moyens, tome II (classeurs en pièces jointes sur data.economie.gouv.fr) | PLF 2020, 2021, 2022, 2023 | exécuté N−2 ; prévu N−1, N | **oui**, par mesure et par millésime |
| Budget vert | PLF 2024, 2025, 2026 | exécuté N−2 ; prévu N−1, N | non |

Tables `core.depense_fiscale` (un chiffrage par millésime, mesure et année) et
`ref.depense_fiscale_beneficiaire` (clé millésime + mesure) ; vue
`derived.depense_fiscale_retenue` : **pour chaque année, un seul millésime**, le plus
récent qui publie une exécution de cette année. La nature retenue est celle du
millésime le plus récent qui classe la mesure.

| Année (exécution) | Millésime | Total | Entreprises | … dont CICE | Ménages | Entreprises et ménages, autres |
|---|---|---|---|---|---|---|
| 2018 | PLF 2020 | 99,0 | 58,5 | 19,4 | 39,1 | 1,4 |
| 2019 | PLF 2021 | 99,9 | 59,9 | 19,2 | 38,8 | 1,2 |
| 2020 | PLF 2022 | 92,7 | 50,4 | 8,7 | 41,2 | 1,2 |
| 2021 | PLF 2023 | 89,6 | 51,6 | 6,9 | 36,6 | 1,4 |
| 2022 | PLF 2024 | 85,6 | 46,4 | 5,5 | 37,6 | 1,6 |
| 2023 | PLF 2025 | 82,9 | 41,0 | 1,0 | 39,4 | 2,5 |
| 2024 | PLF 2026 | 89,4 | 38,0 | 0,2 | 42,8 | 8,6 |

Md€. **Les pièges** :

1. **Un total annuel ne mêle jamais deux millésimes.** Une première version de la vue
   choisissait le meilleur chiffrage mesure par mesure : une mesure renumérotée d'une
   annexe à l'autre était comptée sous ses deux numéros (2019 : 103,0 Md€ au lieu des
   99,9 exécutés). Contrôlé par `cmd/verify`.
2. **Le budget vert découpe une même mesure en plusieurs lignes**, une par cotation
   environnementale, chacune avec une quote-part : additionner, ne pas dédoublonner
   (2024 : 483 lignes, 457 mesures).
3. **« ε », « nc », « - » ne sont pas des zéros.** Des dizaines de mesures ne sont pas
   chiffrées chaque année : les totaux sont des **minorants**.
4. **Les révisions sont fortes** : 2024 prévu à 78,7 Md€ (PLF 2024), exécuté à
   89,4 Md€ (PLF 2026). Ne jamais comparer une prévision d'un millésime à l'exécution
   d'un autre sans le dire.
5. **La nature du bénéficiaire est celle que déclare l'administration**, pas
   l'incidence économique : le taux de TVA à 10 % sur la restauration est rangé
   « entreprises », mais le client paie moins cher. Les mesures créées après le PLF
   2023 sont « non classées » (0,3 Md€ en 2024 ; contrôle : moins de 5 %).
6. **Aucune ventilation par taille d'entreprise** dans les dépenses fiscales. Pour les
   exonérations de cotisations, l'URSSAF la publie : voir § 14.
7. **Les classeurs des PLF 2024 à 2026** ne sont publiés que sur une page de
   budget.gouv.fr protégée par un défi anti-robot (Incapsula), non contourné. Le jeu
   « PLF 2024 VM tome 2 » de data.economie.gouv.fr ne contient que les libellés. Les
   fichiers de budget.gouv.fr (`/documentation/file-download/…`) sont en revanche
   servis sans défi à un client qui s'identifie : une adresse connue se récupère, on
   n'énumère pas les identifiants. Les PDF de l'annexe existent aussi sur le site de
   l'Assemblée nationale.
8. **Le classeur du PLF 2020 n'a pas de ligne d'années** (déduites : N−2, N−1, N),
   écrit « Menages » sans accent, et laisse des cases réduites à une espace.
9. **La nature change parfois d'un millésime à l'autre** : les exonérations de taxe
   foncière passent de « ménages » ou « entreprises » à « locaux » en 2022.

#### 13.3 Pourquoi les aides ne s'additionnent pas : le CICE compté trois fois

Vue `derived.dette_aides_dividendes` : une ligne par année, colonnes **juxtaposées**.

Le **crédit d'impôt pour la compétitivité et l'emploi** (2013-2018) figure :
- dans les **exonérations de cotisations** de l'URSSAF (mesure 141 : 12,5 Md€ en
  2013, 22,0 Md€ en 2017), alors que c'était un crédit d'impôt ;
- dans les **subventions** de la comptabilité nationale (D3 : 34,8 → 46,6 Md€ de
  2012 à 2013, puis 55,8 → 40,6 Md€ de 2018 à 2019) ;
- dans les **dépenses fiscales** (mesure 210324 : encore 6,9 Md€ en 2021, créances
  résiduelles).

En 2019, il est transformé en **réduction des cotisations maladie** (22,0 Md€). Plus
généralement, les crédits d'impôt « payables » (PTC, 19,4 Md€ en 2025) sont à la fois
des dépenses fiscales et des dépenses publiques en comptabilité nationale. **Additionner
exonérations, dépenses fiscales et subventions compte certaines aides deux ou trois
fois.** Les colonnes `cice_*` de la vue rendent le recouvrement visible.

Autres réserves, à écrire à côté de toute juxtaposition :
- les exonérations de cotisations sont **compensées** par l'État, surtout par de la
  TVA affectée (2,63 Md€ non compensés en 2026, `budget-donnees.md`) ; certaines
  bénéficient aux **salariés** (heures supplémentaires, cotisations salariales :
  2,1 Md€ en 2024) ;
- les **dividendes** ne sont pas un flux public : 302 Md€ versés par les sociétés non
  financières en 2024, dont 84 % encaissés par d'autres sociétés et 68 Md€ par les
  ménages (`entreprises-perimetre.md`) ;
- mettre ces séries côte à côte montre des **ordres de grandeur**, pas une causalité
  (D-026, D-037). « Sans ces aides, le déficit aurait été moindre » suppose des
  comportements inchangés — ce qu'aucune donnée n'établit.

#### 13.4 Contrôles ajoutés

- compte de capital : besoin de financement = somme des trois composantes, à 1,5 M€ ;
- dépense par nature : somme des opérations = dépense totale, à 2 M€, chaque année et
  chaque sous-secteur ;
- dépenses fiscales : quatre millésimes chargés ; totaux exécutés entre 60 et
  130 Md€ ; moins de 5 % du montant sans nature de bénéficiaire.

### 14. Qui reçoit les aides : la taille des entreprises

> Ajouté le 14 septembre 2026 (migration 0077, paquet `internal/aides`,
> `go run ./cmd/ingest -only=aides`). Objet : examiner le premier maillon de l'argument
> « les aides profitent aux grandes entreprises » avec des données, et préparer le
> croisement avec des aides publiées bénéficiaire par bénéficiaire.

#### 14.1 Trois notions de taille, qui ne se convertissent pas

| Notion | Unité | Source | Où |
|---|---|---|---|
| **Tranche d'effectif de l'entreprise** (0-9 … 2 000 et plus) | la société (SIREN), effectifs moyens de l'année, base Sequoia | URSSAF | `core.exoneration_tranche`, `core.emploi_prive_tranche` |
| **Catégorie d'entreprise** (PME, ETI, GE ; loi LME, décret 2008-1354) | l'entreprise profilée, **le groupe** | INSEE, SIRENE | `ref.unite_legale.categorie_entreprise` |
| **Nature du bénéficiaire** d'une niche (entreprises, ménages) | la mesure | Direction du budget | `ref.depense_fiscale_beneficiaire` |

Une filiale de 300 salariés d'un groupe du CAC 40 est « 250 à 499 » pour l'URSSAF et
« GE » pour l'INSEE. Aucune table de passage n'est construite (D-057).

#### 14.2 Les exonérations par taille (URSSAF)

Vue `derived.exoneration_par_taille` : part des exonérations, part de la masse
salariale, taux d'exonération (exonérations ÷ masse salariale). Le total par taille
égale le total par mesure chaque année (contrôle).

| 2024 | Entreprises | Exonérations | Part | Part de la masse salariale | Taux |
|---|---|---|---|---|---|
| 0 à 9 | 1 302 504 | 17,3 Md€ | 21,4 % | 14,3 % | 16,7 % |
| 10 à 19 | 133 567 | 8,7 | 10,8 % | 8,0 % | 15,1 % |
| 20 à 49 | 82 092 | 11,6 | 14,3 % | 11,9 % | 13,3 % |
| 50 à 99 | 25 373 | 7,4 | 9,2 % | 8,5 % | 12,0 % |
| 100 à 249 | 14 547 | 9,0 | 11,2 % | 11,7 % | 10,6 % |
| 250 à 499 | 4 519 | 5,7 | 7,1 % | 8,5 % | 9,3 % |
| 500 à 1 999 | 2 967 | 8,6 | 10,7 % | 15,0 % | 7,9 % |
| **2 000 et plus** | **640** | **12,3** | **15,2 %** | **22,0 %** | **7,7 %** |

**Lecture.** Les plus grandes sociétés reçoivent moins que leur part des salaires :
les allègements généraux (77,4 des 80,6 Md€) sont dégressifs avec le salaire, et les
bas salaires sont plus fréquents dans les petites entreprises. Le CICE, proportionnel
à la masse salariale jusqu'à 2,5 SMIC, a fait exception : 21,7 % du CICE en 2017 pour
les 2 000 salariés et plus, qui versaient 23,3 % des salaires ; leur part des
exonérations est montée de 12,7 % (2012) à 18,0 % (2017), puis redescendue à 15,2 %.

**Pièges.**
1. **Société, pas groupe** : la part des grands groupes est minorée (§ 14.1).
2. **ODbL** : partage à l'identique de toute base dérivée incorporant ces données.
3. **Champ** : secteur privé du régime général, hors agriculture et Mayotte ; la
   masse salariale 2025 n'est pas encore publiée (exonérations 2025 sans dénominateur).
4. **Révision du 24 juillet 2026** (alternants réintégrés dans les mesures 151 et
   161) : les montants antérieurs ne sont pas comparables à des extractions plus
   anciennes du même jeu.

#### 14.3 La catégorie d'entreprise (SIRENE)

`ref.unite_legale` : une ligne par **personne morale** du stock SIRENE (catégorie
juridique ≠ 1000), avec la catégorie d'entreprise et son année, la tranche
d'effectif de l'unité légale, l'activité (NAF), l'état administratif. Rechargée
entière à chaque exécution (`-only=sirene`), depuis le fichier stock mensuel de
data.gouv.fr (≈ 975 Mo), sans compte : c'est l'API Sirene qui en demande un.

**Pourquoi exclure les entrepreneurs individuels.** Leur SIREN désigne une personne
physique. Aucune source d'aides envisagée ne demande de les identifier, et la
minimisation des données personnelles l'emporte.

**Ce que contient le stock du 1er septembre 2026.** 30 020 346 unités légales lues,
13 068 047 personnes morales chargées (11 min, dont l'essentiel en vérification des
clés étrangères en fin de copie ; 1,9 Go en base). Catégorie millésimée 2023.

| Personnes morales actives | Unités légales | … avec salariés |
|---|---|---|
| Grande entreprise (GE) | 37 504 | 16 498 |
| Entreprise de taille intermédiaire (ETI) | 95 371 | 54 536 |
| PME | 3 597 920 | 1 346 853 |
| Non catégorisée | 5 224 142 | 72 063 |

**Pièges.**
1. **La catégorie est celle du groupe** : 24 729 unités légales actives classées GE ont
   moins de 10 salariés ou aucun (holdings, sociétés immobilières, filiales). LVMH Moët Hennessy
   Louis Vuitton, la société cotée, a une tranche d'effectif de 20 à 49 salariés
   (code 12) et la catégorie GE.
2. **« Non catégorisée » n'est pas « petite »** : 5,2 millions d'unités actives, presque
   toutes sans salarié (sociétés civiles, associations immatriculées…).
3. **`caractereEmployeurUniteLegale` est vide dans tout le stock** : la présence de
   salariés se lit dans la tranche d'effectif (`NN` = non employeuse ou inconnue).
4. **Catégorie millésimée** : 2023 dans ce stock ; croiser une aide de 2018 avec une
   catégorie 2023 suppose que l'entreprise n'a pas changé de périmètre.

#### 14.4 Les aides nominatives : ce qui existe (étude du 14 septembre 2026)

Les trois premières sources du tableau sont désormais chargées : voir § 15. [V] = vérifié par requête ou lecture d'un échantillon ; [D] =
déclaré par une page.

| Source | Identifiant | Champ | Volume | Accès | Retenue |
|---|---|---|---|---|---|
| **Registre européen de transparence des aides d'État (TAM)**, Commission | SIREN / SIRET (≈ 98 % bien formés sur 2016-2020) | aides d'État de plus de 500 k€ (100 k€ depuis la révision de 2023 [D] ; encadrements Covid et Ukraine : 100 k€), toutes autorités | 6 777 aides France 2016-2020, 17 Md€ d'ESB (export republié par un paquet R, licence MIT) [V] | recherche par formulaire POST avec jeton CSRF, **pas d'API** ni de fichier en masse [V] ; extrait complet obtenable par demande d'accès à la DG COMP [D] | **la source décisive** pour les grosses aides ; accès à décider |
| **Aides financières de l'ADEME** (format SCDL) | SIRET [V] | tous dossiers engagés depuis 2021, sans seuil | 39 577 dossiers, 11,2 Md€ [V] | API data-fair ouverte, mise à jour quotidienne, Licence Ouverte [V] | **exploitable tout de suite** |
| **Registre public des aides de minimis** (DGE, décret 2025-1361) | SIREN à 99,9 % [V] | aides de minimis octroyées depuis le 1er janvier 2026, toutes autorités (Douanes, DGFiP, Bpifrance, Régions) | 16 618 aides, 161,6 M€ d'ESB au 8 septembre 2026 [V] | API data.economie.gouv.fr, quotidienne ; licence non renseignée [V] | exploitable ; plafond de 300 k€ sur 3 ans : mesure le **nombre** de bénéficiaires, pas la concentration des montants |
| CORDIS Horizon Europe | TVA → SIREN, indicateur PME [V] | fonds européens de recherche | fichier en masse de 36,7 Mo [V] | ouvert ; licence non vérifiée | complément, hors aides françaises |
| Liste nationale des opérations FEDER / FSE+ / FTJ | **nom seul** [V] | 16 625 opérations 2021-2027, 7,9 Md€ UE [V] | xlsx | ouvert | non croisable (rapprochement par nom interdit, D-025) |
| Kohesio (Commission) | URI, nom [V] | fonds de cohésion | 19 585 bénéficiaires France [V] | API non documentée | non croisable |
| Aides PAC (transparence) | nom, commune [D] | aides agricoles | — | application MicroStrategy en JavaScript, conservation 2 ans [D] | non croisable, surtout des personnes physiques |
| Plan de relance, projets industriels | SIREN, type d'entreprise [V] | 3 080 projets | **sans montant** [V] | ouvert, figé en 2022 | inutile pour les montants |
| France Num | identifiant pseudonymisé [V] | 284 124 lignes | — | ouvert | non croisable |
| Crédit d'impôt recherche | — | — | — | secret fiscal : aucune donnée par entreprise [D] | agrégats seulement |
| Marchés publics (DECP) | SIRET [V] | 702 092 marchés | — | ouvert | **pas des aides** : ne pas mélanger |

**Pièges déjà constatés.**
1. **Le type « PME » déclaré au TAM est inutilisable tel quel** : sur les quatre plus
   grosses aides marquées « SME » en 2016-2020, trois vont à des ETI ou GE selon
   l'INSEE (dont Storengy France, GE) [V]. C'est précisément ce que
   `ref.unite_legale` permet de corriger.
2. **ADEME** : montants engagés, pas versés ; des intermédiaires (l'ASP reçoit
   730,6 M€ en deux dossiers, reversés à d'autres) et des organismes publics
   (2,4 Md€) à écarter avant toute répartition [V].
3. **TAM** : montant nominal vide dans 5 380 lignes sur 6 777, seul l'ESB est
   complet ; quelques fourchettes au lieu de montants [V].

**Ordre de chargement proposé** : ADEME et registre de minimis (API ouvertes, sans
décision préalable), puis le TAM selon la voie retenue (soumission automatisée du
formulaire public, à autoriser explicitement, ou demande d'extrait à la DG COMP).

### 15. Les aides nominatives croisées avec la catégorie d'entreprise

> Ajouté le 14 septembre 2026 (migration 0078, `internal/aides`,
> `go run ./cmd/ingest -only=aides-nominatives`, SIRENE requis). D-058.

#### 15.1 Ce qui est chargé

| Source | Aides | dont personnes morales de SIRENE | Montant retenu | Période |
|---|---|---|---|---|
| Registre européen de transparence des aides d'État (TAM), France | 100 316 | 91 965 | élément d'aide (ESB) ; 5 279 avantages fiscaux par tranches seulement | octrois 2016-2026 |
| ADEME, aides financières | 39 577 | 37 686 | montant engagé | conventions 2021-2026 |
| Registre public des aides de minimis (DGE) | 16 618 | 11 548 | ESB | octrois 2026 |

Table `core.aide_nominative` ; vues `derived.aide_par_categorie` (par source,
année, catégorie), `derived.aide_tam_type_declare` (type déclaré contre catégorie
INSEE), `derived.aide_montant_suspect` (montants aberrants écartés des sommes).

**Accès au TAM.** Formulaire public (pays, puis dates d'octroi), export CSV lié à la
session, avec l'accord explicite du responsable du projet. Au-delà d'environ 1 000
lignes (951 servies, 1 021 refusées), l'export n'est plus servi : le site propose de
l'envoyer par courriel contre prénom, nom et adresse, formulaire que le connecteur ne
remplit pas. Les périodes sont donc découpées d'emblée en tranches d'environ 800 aides
(203 exports). Chaque requête est espacée de deux secondes et reprise jusqu'à quatre
fois en cas de coupure. Les exports scellés depuis moins de deux jours sont réutilisés,
ce qui rend le parcours reprenable. Durée : plusieurs dizaines de minutes à froid, 7 minutes
en reprise complète.

#### 15.2 Ce que montrent les données (entreprises seulement)

Organismes publics (catégories juridiques 4 et 7), associations (92) et bénéficiaires
qui ne sont pas des personnes morales du répertoire sont exclus.

| | Registre européen (ESB, 2016-2026) | ADEME (engagé, 2021-2026) |
|---|---|---|
| Total entreprises | 65,0 Md€ | 7,4 Md€ |
| **Grandes entreprises** | **27,0 % des montants, 6,1 % des bénéficiaires** (3 529) | **35,6 % des montants, 8,5 % des bénéficiaires** (1 487) |
| ETI | 32,3 % / 14,2 % | 24,1 % / 16,3 % |
| PME | 35,4 % / 37,9 % | 32,4 % / 65,4 % |
| Non catégorisées | 5,2 % / 41,8 % | 7,9 % / 9,8 % |
| Aide moyenne par bénéficiaire, GE / PME | 4,97 M€ / 1,05 M€ | 1,78 M€ / 0,21 M€ |
| Part du 1 % des bénéficiaires les mieux dotés | 43,5 % | 47,9 % |

Registre de minimis (2026) : 65,2 % des bénéficiaires entreprises sont des PME, 0,4 %
des grandes entreprises — ce que le plafond de 300 k€ laisse attendre.

**Lecture.** Le constat est inverse de celui des exonérations de cotisations (§ 14.2),
et ce n'est pas contradictoire : les exonérations sont un dispositif de masse calé sur
les bas salaires ; les aides nominatives financent des projets (batteries,
semi-conducteurs, hydrogène, décarbonation) dont la taille suit celle des entreprises.
Plus grosses aides : ProLogium (1,37 Md€, gigafactory de batteries), STMicroelectronics
Crolles (1,06 Md€ cumulés), Automotive Cells Company (0,73 Md€), Symbio (0,68 Md€).

#### 15.3 Pièges

1. **Le type « PME » déclaré au TAM est faux pour 43,7 % des montants** : sur 50,6 Md€
   déclarés « PME », 16,1 Md€ vont à des ETI et 6,0 Md€ à des grandes entreprises selon
   l'INSEE. Il n'est jamais utilisé comme catégorie (`derived.aide_tam_type_declare`).
2. **Montant aberrant** : 1 061 M€ à un GAEC dans le régime SA.107520 (investissements
   agricoles, aide médiane 21 460 €). Écarté des sommes par la règle « plus de 10 M€ et
   plus de 10 000 fois la médiane d'un régime d'au moins 20 aides », qui ne retient que
   lui ; une règle à 1 000 fois retenait aussi Nuward (300 M€) et l'ONF (40 M€), aides
   réelles. Contrôle : moins de dix montants écartés.
3. **La catégorie INSEE ne voit que le périmètre français d'un groupe** : ProLogium
   Technology Europe, filiale d'un groupe taïwanais, est « PME ». La part des PME est
   donc majorée par les filiales de groupes étrangers.
4. **Les sources se recouvrent** : une aide de l'ADEME notifiée figure aussi au TAM.
   Jamais de somme entre sources.
5. **Montants de nature différente** : engagé (ADEME), ESB (TAM, minimis). Les
   avantages fiscaux du TAM sont publiés par tranches (dont des tranches ouvertes,
   « > 30,000,000 ») et restent hors des sommes.
6. **Seuils du TAM** : 500 k€ jusqu'à la révision de 2023, 100 k€ ensuite et pendant
   les encadrements temporaires ; la hausse du nombre d'aides en 2021 (22 422) est
   celle des aides Covid, pas un changement de politique mesurable en tant que tel.
7. **Catégorie « Small Mid-Caps »** : apparue en 2025 dans le TAM (19 aides), chargée
   comme `PETITE_ETI`.
8. **Données personnelles** : nom et identifiant conservés pour les seules personnes
   morales ; 8 351 aides TAM, 1 891 aides ADEME et 5 070 aides de minimis restent sans
   bénéficiaire identifié.

## Ce que les données ne disent pas

Les données ne disent pas à quoi sert un euro emprunté (§ 13.1 : il n'a pas
d'étiquette), ni qui détient individuellement les titres de l'État au-delà des catégories
de la Banque de France. Le travail restant est listé dans l'annexe technique (§ 12).

## Pièges de lecture

### 9. Les pièges

1. **UNIT_MULT.** L'INSEE publie la dette négociable en millions (6) et la dette
   trimestrielle en milliards (9), la Banque de France en milliers (3), Eurostat en
   millions. Oublier le multiplicateur produit un écart de mille, qui passe inaperçu
   dans un ratio. Appliqué au chargement, vérifié par des sommes croisées.
2. **La fréquence « T ».** L'INSEE code le trimestre `T`, pas `Q`.
3. **Valeur de marché contre nominal.** DET2 (Banque de France) est en valeur de
   marché : 2 602 Md€ au T1 2026 contre 2 824 Md€ de dette négociable nominale.
   En 2019, avec des taux proches de zéro, c'était l'inverse (2 153 contre 1 823).
   **Ne comparer que des parts.**
4. **« OAT » contient les OAT indexées** dans DET2 : OAT = OAT fixes + OATi + OAT€i.
   Additionner les trois compterait les indexées deux fois.
5. **Les BTAN** (2 à 5 ans), émis jusqu'en 2013 : ni « long » ni « court » terme
   chez la Banque de France, et non ventilés par secteur résident. D'où la catégorie
   calculée `RESIDENTS_BTAN_NON_VENTILES`, nulle depuis 2017.
6. **`S1` veut dire « tous secteurs de la zone »** dans les clés DET2 ; ramené à
   `'_T'` au chargement.
7. **Échéance initiale contre résiduelle** (§ 4).
8. **Non-résident ≠ étranger** (§ 3.1).
9. **Octobre 2017** : la ventilation « taux fixe + indexée » de la dette négociable
   vaut exactement le total de **septembre** (1 703 850 M€), 23,7 Md€ de plus que le
   total d'octobre. Défaut de la source, laissé tel quel en base et exclu nommément
   du contrôle.
10. **Avant 1998**, la dette trimestrielle de l'INSEE (base 2020) et la série annuelle
    d'Eurostat divergent (−13,1 Md€ en 1995, +6,4 Md€ en 1997). Concordance à
    0,1 Md€ près ensuite.
11. **Projections du FMI.** Le WEO mêle observations et projections jusqu'en 2031.
    Seules les années jusqu'à `LATEST_ACTUAL_ANNUAL_DATA`, publiée pays par pays,
    sont chargées (France : 2024 ; Suisse : 2025).
12. **Licence du FMI.** Le jeu SDMX affiche « All Rights Reserved » et renvoie aux
    conditions générales, qui autorisent la réutilisation des données avec mention
    de la source. **Usage commercial à confirmer** avant tout export ouvert.
13. **Les « - » des RAP** ne sont pas chargés : zéro ou « sans objet », le tableau
    ne permet pas de trancher. D'où l'absence de série « adjudications non
    couvertes » alors que le RAP n'en rapporte aucune.
14. **Le modèle SF de l'AFF n'est pas le SEC 2010** : les montants suisses ne se
    comparent pas à la dette Maastricht. Pour comparer, le FMI.
15. **Un null JSON-stat décodé dans un float donne 0.** Le décodeur l'écarte
    explicitement.
16. **La clé Webstat** passe dans un en-tête HTTP (`archive.FetchEntetes`), jamais
    dans l'URL : `raw.retrieval` conserve les URL, et une clé qui y figurerait serait
    publiée avec la provenance. Vérifié : aucune URL archivée ne la contient.

## Sources

### 7.1 Chargées

| Source | Accès | Licence (classe) | Cadence | Destination |
|---|---|---|---|---|
| **INSEE** — `DETTE-NEGOCIABLE-ETAT` (17 séries, AFT) | API SDMX BDM, ouverte | Licence Ouverte v2.0 (OPEN) | mensuelle | `concept = DETTE_NEGOCIABLE_ETAT` |
| **INSEE** — `DETTE-TRIM-APU-2020` (21 séries) | API SDMX BDM | idem | trimestrielle | `DETTE_MAASTRICHT`, `DETTE_NETTE_APU`, `DETTE_BRUTE_FMI`, `ACTIFS_COTES_APU` |
| **Banque de France** — `DET2` (305 séries) | Webstat (Opendatasoft), **clé d'API** | réutilisation avec mention (ATTRIBUTION) | trimestrielle | `DETENTION_TITRES_ETAT` |
| **Eurostat** — `gov_10dd_edpt1`, `gov_10a_main`, `gov_10dd_ggd`, `irt_lt_mcby_a`/`_m` | API JSON-stat | CC BY 4.0 (ATTRIBUTION) | annuelle / mensuelle | dette, intérêts, solde, recettes, dépenses, détenteurs, taux |
| **FMI** — WEO, 4 indicateurs × 14 pays | API SDMX `api.imf.org` | conditions du FMI (ATTRIBUTION, § 9) | semestrielle | `DETTE_BRUTE_FMI`, `DETTE_NETTE_FMI`, `SOLDE_PUBLIC`, `SOLDE_PRIMAIRE` |
| **AFF** (Suisse) — bilans et indicateurs financiers | fichiers XLSX | OGD Suisse (ATTRIBUTION) | annuelle | `BILAN_APU_CH`, `INDICATEUR_AFF` |
| **BNS** — `rendoblim` (10 ans) | CSV | mention de la source (ATTRIBUTION) | mensuelle, **figée depuis 07/2025** | `TAUX_LONG_TERME` (CH) |
| **Programme 117** — RAP 2025, adjudications | API data.economie.gouv.fr | Licence Ouverte v2.0 (OPEN) | annuelle | `ADJUDICATIONS_AFT` |

Volumes au 14 septembre 2026 : 1 186 séries, 53 171 observations.

### 7.2 Intégrer les données de l'Agence France Trésor

Le site `aft.gouv.fr` est **entièrement** derrière une protection anti-robot
(Cloudflare), y compris ses fichiers de données. **On ne la contourne pas.** Trois
voies légitimes couvrent l'essentiel :

1. **L'encours mensuel de la dette négociable** : republié par l'INSEE, source
   déclarée AFT, sous licence ouverte (`insee.go`). C'est la même donnée.
2. **La détention par secteur** : produite par la Banque de France, dont l'AFT
   reprend elle-même les chiffres dans son bulletin mensuel (`banque_de_france.go`).
3. **La performance des émissions** : indicateurs du programme 117, que l'AFT
   rapporte au Parlement (`aft.go`).

**Restent inaccessibles** : l'échéancier titre par titre, la durée de vie moyenne,
le programme d'émission annuel et le détail de chaque adjudication. Voie à
explorer, non automatisable sans l'AFT : demander un accès aux fichiers ou leur
dépôt sur data.gouv.fr.

### 7.3 Écartées ou bloquées

| Source | Motif |
|---|---|
| aft.gouv.fr | Protection anti-robot sur tout le domaine ; non contournée |
| FMI DataMapper (`imf.org/external/datamapper`) | Répond 403 à un User-Agent qui s'identifie honnêtement (règle Akamai) ; on ne se fait pas passer pour un navigateur. L'API SDMX du FMI fournit les mêmes séries **et** la dernière année observée |
| Séries Webstat « par jeu » (`det2-q-n-fr-…`) | Visibles au catalogue mais vides ; les valeurs sont dans le jeu restreint `observations` |
| OCDE (SDMX) | Redondant avec Eurostat et le FMI pour ce besoin |

## Annexe technique

### 8. Le modèle de données

Migration `0073_dette.sql`.

- **`ref.dette_serie`** — une série = une combinaison de dimensions fixée : `pays`,
  `frequence` (A/Q/M), `unite` (EUR, CHF, PCT, PCT_PIB, RATIO, NOMBRE), `concept`,
  `mesure` (ENCOURS, VARIATION_CUMULEE, FLUX, TAUX, PART, RATIO, NOMBRE),
  `secteur_emetteur`, `zone_detenteur` (W0 monde, W1 non-résidents, W2 résidents),
  `secteur_detenteur`, `echeance` + `base_echeance`, `instrument`,
  `monnaie_emission`. Toute dimension non ventilée vaut `'_T'` : NULL y
  signifierait « inconnu ». Code stable `'<producteur>:<code producteur>'`.
- **`core.dette_observation`** — `(serie, periode)` unique ; `periode` au format
  `AAAA`, `AAAA-Qn` ou `AAAA-MM`, `debut` pour trier et joindre ; `valeur` **à
  l'unité** (multiplicateur du producteur appliqué) ; `statut` du producteur ;
  `document_id` vers le document scellé. Pas de valeur, pas de ligne.
- **Vues** (`derived`, avec `method_version`) : `dette_taux_apparent`
  (`dette-taux-apparent-v1`), `dette_detention_etat` (`dette-detention-etat-v1`),
  `dette_ajustement_stock_flux` (`dette-asf-v1`).

Chaque source se recharge **entièrement** dans une transaction : les producteurs
révisent (l'INSEE à chaque compte trimestriel, Eurostat à chaque notification), et
compléter mêlerait deux millésimes.

**Pourquoi un modèle long plutôt qu'une table par source.** Les sources ne
partagent pas leurs dimensions (instrument pour l'AFT, secteur détenteur pour la
Banque de France, échéance résiduelle pour Eurostat, compte de bilan pour l'AFF).
Une table par source aurait été plus simple à écrire, mais aurait rendu impossibles
les contrôles croisés qui font la valeur de l'ensemble.

**Pourquoi pas `core.macro_value`.** Elle porte déjà la dette et le solde de la
France (Eurostat, annuel) pour situer une présidence dans son époque ; elle n'a ni
détenteur, ni échéance, ni pays. Les deux coexistent ; la concordance est vérifiable
(mêmes requêtes Eurostat).

### 10. Les vérifications de cohérence (`cmd/verify/dette.go`)

- chaque source a des séries ; au moins 40 000 observations ;
- dette négociable : court terme + long terme + devises = total ; taux fixe + indexée
  = total (sauf octobre 2017, § 9) ;
- dette Maastricht trimestrielle : dépôts + titres + crédits = total ; les quatre
  sous-secteurs = total ;
- Maastricht France : INSEE (T4) = Eurostat (annuel) à 1 Md€ près, depuis 1998 ;
- Eurostat : détention résidente + non résidente = total ;
- Banque de France : secteurs résidents feuilles = total résident ; parts des
  catégories = 100 % ; part non résidente recalculée = part publiée ;
- taux apparent entre 0 et 15 % ;
- fraîcheur : dette négociable (5 mois), détention et dette trimestrielle (10 mois).

### 11. Exécution

```bash
set -a; . ./.env; set +a      # WEBSTAT_API_KEY, fichier non versionné
go run ./cmd/ingest -only=migrate
go run ./cmd/ingest -only=dette
go run ./cmd/verify
```

Sans `WEBSTAT_API_KEY`, la détention est sautée avec un avertissement, et le
contrôle « chaque source a des séries » bloque la publication — volontairement.

### 12. Ce qui reste

- L'échéancier et la durée de vie moyenne de la dette de l'État (AFT, § 7.2).
- La charge budgétaire de la dette (programme 117 : crédits, provision pour
  l'indexation), à rapprocher des intérêts en comptabilité nationale — les deux
  diffèrent par construction (caisse contre droits constatés).
- Le circuit Banque de France → État (dividende et impôt sur les bénéfices).
- Les millésimes antérieurs des indicateurs du programme 117 (un jeu par RAP,
  colonnes figées).

## Versions

- **Version 2** (15 septembre 2026) : plan commun des dossiers (D-066) ; cadre (loi de programmation des finances publiques, procédure européenne) et contrôle du Haut Conseil des finances publiques ; le § 6 ne part plus d'une prémisse sur la Suisse.
- **Version 1** (14 septembre 2026) : définitions, prix de l'emprunt, détenteurs, échéances, comparaison européenne, Suisse.
