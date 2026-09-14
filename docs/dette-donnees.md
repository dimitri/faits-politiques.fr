# La dette publique : ce que les sources permettent d'établir

> Document de conception. Version 1 — 14 septembre 2026.
> Questions : **combien doit la France, et selon quelle définition** ; **à qui elle
> emprunte** ; **ce que coûte l'emprunt, et pourquoi le taux à 10 ans n'est pas ce
> taux** ; **comment se compare-t-elle à ses voisins** ; et **pourquoi la Suisse
> passe pour « ne pas avoir de dette »**.
>
> Tous les chiffres de ce document sont lus dans la base après chargement (migration
> 0073, `go run ./cmd/ingest -only=dette`), pas recopiés de publications. Les
> identités comptables qu'ils vérifient sont dans `cmd/verify/dette.go`.

---

## 1. Le cadrage

### 1.1 Une question, quatre montants

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

### 1.2 Déficit, dette, intérêts : trois grandeurs liées, pas interchangeables

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

---

## 2. Le prix de l'emprunt : taux de marché et taux apparent

### 2.1 Deux taux qu'on confond

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

### 2.2 La dette indexée

312,8 Md€ de la dette négociable (juillet 2026, 10,8 %) sont **indexés sur
l'inflation** (OATi sur l'inflation française, OAT€i sur celle de la zone euro) :
leur nominal est revalorisé chaque année. En 2022-2023, l'inflation a ainsi
alourdi la charge sans aucune hausse de taux. L'INSEE en publie l'encours ; la
revalorisation annuelle elle-même relève du programme 117 du budget (voir
`budget-donnees.md`), non chargé ici.

### 2.3 La demande des investisseurs : les adjudications

L'AFT emprunte par **adjudication** : elle annonce un montant, les banques
« spécialistes en valeurs du Trésor » font des offres, les meilleures sont servies.
Le **taux de couverture** rapporte les offres au montant adjugé (RAP 2025,
programme 117) : **OAT 2,30 en 2023, 2,45 en 2024, 2,97 en 2025** ; BTF 2,67 →
3,19 → 3,40. **Aucune adjudication non couverte** n'est rapportée. La dette
française ne manque pas de preneurs ; la question est le prix qu'ils demandent.

---

## 3. À qui la France emprunte

### 3.1 Les détenteurs des titres de l'État (Banque de France, T1 2026)

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

### 3.2 La même question en Europe (Eurostat, 2025, dette Maastricht)

Part de la dette détenue par des non-résidents : Grèce 70,8 %, Autriche 66,2 %,
Belgique 63,4 %, Finlande 61,2 %, Irlande 55,0 %, **France 54,3 %**, Allemagne
51,3 %, Pays-Bas 49,2 %, Espagne 48,9 %, **Italie 34,3 %**, Suède 23,0 %.
L'Italie est un contre-exemple à retenir : dette plus lourde (137,1 % du PIB),
mais davantage détenue en interne, dont 11,8 % par les ménages en direct.

**Ne pas rapprocher les 54,3 % d'Eurostat des 57,5 % de la Banque de France** : le
premier porte sur toute la dette Maastricht en valeur nominale (crédits et dette
locale compris), le second sur les seuls titres de l'État en valeur de marché.

---

## 4. Les échéances

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

---

## 5. La comparaison européenne (Eurostat, 2025)

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

---

## 6. La Suisse : la prémisse est fausse, le mécanisme est réel

### 6.1 La Suisse a une dette

**39,4 % du PIB en 2025** (FMI), 40,5 % en 2024. Elle culminait à 57,1 % en 2004.
La Confédération seule porte 91,3 Md CHF d'engagements financiers (11,9 Md à court
terme, 79,4 Md à long terme, AFF 2025), contre 111,0 Md CHF en 2003. La question
juste n'est pas « pourquoi n'a-t-elle pas de dette », mais **« pourquoi sa dette
baisse-t-elle en proportion quand celle de la France monte »**.

### 6.2 Le frein à l'endettement

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

### 6.3 Ce que montrent les données

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

### 6.4 Ce qu'il ne faut pas en conclure

Le frein explique la **trajectoire** de la dette fédérale, pas tout l'écart avec la
France. Y contribuent aussi un taux d'intérêt structurellement bas (le franc est une
monnaie refuge), un poids de l'État plus faible, un système de retraite largement
capitalisé hors du périmètre public, et le fédéralisme budgétaire. Aucune de ces
causes ne se lit seule dans une série : **le site montre la règle et les courbes,
il n'impute pas l'écart à la règle.**

---

## 7. Les sources

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

---

## 8. Le modèle de données

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

---

## 9. Les pièges

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

---

## 10. Les contrôles (`cmd/verify/dette.go`)

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

---

## 11. Exécution

```bash
set -a; . ./.env; set +a      # WEBSTAT_API_KEY, fichier non versionné
go run ./cmd/ingest -only=migrate
go run ./cmd/ingest -only=dette
go run ./cmd/verify
```

Sans `WEBSTAT_API_KEY`, la détention est sautée avec un avertissement, et le
contrôle « chaque source a des séries » bloque la publication — volontairement.

## 12. Ce qui reste

- L'échéancier et la durée de vie moyenne de la dette de l'État (AFT, § 7.2).
- La charge budgétaire de la dette (programme 117 : crédits, provision pour
  l'indexation), à rapprocher des intérêts en comptabilité nationale — les deux
  diffèrent par construction (caisse contre droits constatés).
- Le circuit Banque de France → État (dividende et impôt sur les bénéfices).
- Les millésimes antérieurs des indicateurs du programme 117 (un jeu par RAP,
  colonnes figées).

---

## 13. À quoi sert la dette, et ce qu'on met en regard

> Ajouté le 14 septembre 2026 (migration 0075). Deux questions posées ensemble :
> **à quoi sert la dette contractée** (fonctionnement, investissement, autre), et
> **peut-on la mettre en parallèle des exonérations et des dividendes**, pour
> examiner l'argument selon lequel les aides aux grandes entreprises seraient
> financées par les ménages et endetteraient le pays.

### 13.1 Ce que la comptabilité permet de dire

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

### 13.2 Les dépenses fiscales (« niches »)

| Source | Millésime | Années chiffrées | Nature du bénéficiaire |
|---|---|---|---|
| Voies et moyens, tome II (classeur sur data.economie.gouv.fr) | PLF 2023 | 2021 exécuté ; 2022, 2023 prévus | **oui**, par mesure |
| Budget vert | PLF 2024, 2025, 2026 | exécuté N−2 ; prévu N−1, N | non |

Tables `core.depense_fiscale` (un chiffrage par millésime, mesure et année) et
`ref.depense_fiscale_beneficiaire` ; vue `derived.depense_fiscale_retenue`
(exécution la plus récente, à défaut prévision la plus récente).

| Année | Total | Entreprises | Ménages | Entreprises et ménages, autres |
|---|---|---|---|---|
| 2021 | 89,6 | 51,6 | 36,6 | 1,4 |
| 2022 | 85,6 | 46,4 | 37,6 | 1,6 |
| 2023 | 83,0 | 41,1 | 39,4 | 2,5 |
| 2024 | 89,6 | 38,1 | 42,8 | 8,7 |
| 2025 (prévu) | 91,9 | 39,4 | 45,0 | 7,5 |

Md€. **Les pièges** :

1. **Le budget vert découpe une même mesure en plusieurs lignes**, une par cotation
   environnementale, chacune avec une quote-part : additionner, ne pas dédoublonner
   (2024 : 483 lignes, 457 mesures).
2. **« ε », « nc », « - » ne sont pas des zéros.** Des dizaines de mesures ne sont pas
   chiffrées chaque année : les totaux sont des **minorants**.
3. **Les révisions sont fortes** : 2024 prévu à 78,7 Md€ (PLF 2024), exécuté à
   89,4 Md€ (PLF 2026). Ne jamais comparer une prévision d'un millésime à l'exécution
   d'un autre sans le dire.
4. **La nature du bénéficiaire est celle que déclare l'administration**, pas
   l'incidence économique : le taux de TVA à 10 % sur la restauration est rangé
   « entreprises », mais le client paie moins cher. Les mesures créées après le PLF
   2023 sont « non classées » (0,3 Md€ en 2024 ; contrôle : moins de 5 %).
5. **Aucune ventilation par taille d'entreprise.** « Grandes entreprises » n'est
   mesurable par aucune de ces sources.
6. Les millésimes antérieurs de l'annexe sont sur budget.gouv.fr, derrière une
   protection anti-robot (Incapsula) : non chargés. Le jeu PLF 2024 de
   data.economie.gouv.fr ne contient que les libellés, sans montants.

### 13.3 Pourquoi les aides ne s'additionnent pas : le CICE compté trois fois

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

### 13.4 Contrôles ajoutés

- compte de capital : besoin de financement = somme des trois composantes, à 1,5 M€ ;
- dépense par nature : somme des opérations = dépense totale, à 2 M€, chaque année et
  chaque sous-secteur ;
- dépenses fiscales : quatre millésimes chargés ; totaux exécutés entre 60 et
  130 Md€ ; moins de 5 % du montant sans nature de bénéficiaire.
