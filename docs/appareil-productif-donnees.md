# L'appareil productif français : glissement sectoriel et délocalisations

> **Dossier** · version 1 · 17 septembre 2026
>
> En cinquante ans, la part de l'industrie dans l'emploi français a été
> divisée par deux et demi ; celle des services a gagné près de 25 points,
> de 56 % à 81 % de l'emploi total. Ce dossier
> décrit ce glissement avec les séries longues d'Eurostat, puis ce que l'on
> sait — et ce que l'on ne sait pas — des délocalisations d'emplois,
> notamment qualifiés, à partir de la seule étude française qui les a
> mesurées.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| délocalisation | 177 | 115 | 1er octobre 2024 | 20 juillet 2026 |
| désindustrialisation | 102 | 82 | 22 octobre 2024 | 6 juillet 2026 |

Une mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->



<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun texte n'est encore chargé pour ce dossier.

<!-- faits:CADRE:fin -->

### 1. Cinquante ans de glissement sectoriel

<!-- schema:glissement-sectoriel -->

Entre 1975 et 2024, l'emploi total en France est passé de 21,8 à 30,6
millions de personnes — mais sa répartition par secteur a changé de nature,
pas seulement de taille :

| Secteur | 1975 | 2024 | Évolution |
|---|---:|---:|---|
| Agriculture, sylviculture et pêche | 10,2 % | 2,3 % | divisée par 4,4 |
| Industrie (y compris énergie) | 24,7 % | 10,1 % | divisée par 2,4 |
| — dont industrie manufacturière | 23,5 % | 9,2 % | divisée par 2,6 |
| Construction | 8,8 % | 6,6 % | quasi stable en effectifs (1,91 → 2,02 million) |
| Services | 56,3 % | 81,0 % | +24,7 points |

Deux branches de services concentrent une bonne part de cette croissance, et
ce sont des emplois qualifiés, pas seulement des emplois de proximité :
**information et communication** (J) passe de 373 000 à 1 150 000 emplois
(+208 %, sa part dans l'emploi total plus que double, de 1,7 % à 3,8 %), et **activités
spécialisées, scientifiques et techniques** (M_N — ingénierie, conseil,
recherche privée, activités administratives et de soutien) passe de 1 271 000
à 4 877 000 emplois (+284 %, sa part passe de 5,8 % à 15,9 %).

Cette série est mesurée par les comptes nationaux (emploi intérieur total,
un concept différent de l'enquête Emploi de l'Insee utilisée ailleurs dans
ce dépôt — voir « Ce que les données ne disent pas ») : elle ne dit rien, en
elle-même, de la cause du glissement (automatisation, externalisation
d'activités industrielles vers des services aux entreprises, concurrence
internationale, choix de consommation). C'est le sujet de la section
suivante, sur la seule partie de ce glissement qui a été mesurée
directement : les délocalisations.

### 2. Les délocalisations d'emplois : une mesure par modèle, jamais un comptage

Aucune statistique publique ne compte directement les emplois délocalisés :
une délocalisation n'est pas un événement déclaré comme tel à une
administration. La seule mesure française disponible est une **détection
statistique** : l'Insee croise les données d'entreprises (Ésane, l'enquête
CAM) avec les statistiques douanières d'importations (DGDDI) pour repérer
les entreprises dont l'évolution de l'emploi et des importations en
provenance d'un même pays ressemble au profil d'une délocalisation. Le
modèle (régression logistique, forêt aléatoire, XGBoost) a une aire sous la
courbe ROC de 0,54 à 0,80 selon la méthode retenue — un repérage
probabiliste, pas une certitude cas par cas. L'Insee publie systématiquement
trois scénarios, bas/central/haut : ce dossier fait de même, sans jamais
réduire l'écart à un seul chiffre.

<!-- schema:delocalisation-annuelle -->

Sur le champ couvert (secteurs principalement marchands hors agriculture et
finance, entreprises de 50 salariés ou plus), le nombre d'emplois ETP détectés
comme délocalisés chaque année a été divisé par 4 à 6 entre le pic de 2008
(entre 38 000 et 76 100 selon le scénario) et 2017 (entre 6 200 et 19 500) —
une baisse continue depuis la crise financière, pas une tendance récente.

<!-- schema:delocalisation-departement -->

Cumulés sur toute la période 1995-2017, les emplois délocalisés se
concentrent fortement en Île-de-France (118 200 dans les Hauts-de-Seine, 72
700 à Paris, 49 500 dans les Yvelines) — cohérent avec la nature des emplois
les plus touchés, décrite ci-dessous : des sièges sociaux et des fonctions
d'ingénierie et de gestion, pas seulement des usines en région.

**Ce sont des emplois qualifiés, pas seulement des ouvriers d'usine, qui sont
surreprésentés parmi les postes délocalisés** — c'est le point que le
discours courant sur les délocalisations, focalisé sur l'industrie
manufacturière, manque le plus souvent :

<!-- tableau:delocalisation-csp -->

Les ouvriers qualifiés de type industriel restent la catégorie la plus
concernée en écart absolu (18,7 % des postes délocalisés contre 12,5 % de
l'emploi général), mais les **ingénieurs et cadres techniques d'entreprise**
(12,5 % contre 9,9 %) et les **techniciens** (9,8 % contre 6,8 %) sont
surreprésentés dans une proportion comparable — la délocalisation ne touche
pas que la chaîne de production, elle touche aussi la conception et
l'encadrement technique qui l'accompagnent.

### 3. Des usines qui ont fermé, une production partie ailleurs

Aucune administration ne tient de registre des usines fermées et de la
destination de leur production : les cas ci-dessous viennent d'un
recoupement de communiqués d'entreprise, de presse professionnelle et
d'archives, pas d'une base de données unique — le niveau de preuve varie
d'un cas à l'autre, et c'est signalé pour chacun.

- **Renault Twingo** (Flins → Novo Mesto, Slovénie). La première génération
  sortait de l'usine de Flins (Yvelines) ; depuis 2007, la Twingo est
  fabriquée exclusivement chez Revoz, filiale à 100 % de Renault à Novo
  Mesto. — [Renault Group, page officielle de l'usine](https://www.renaultgroup.com/en/group/locations/novo-mesto-plant-revoz/) · *officiel*
- **Citroën C3** (Poissy → Trnava, Slovaquie). La troisième génération,
  dévoilée en 2016, n'a jamais été assemblée en France : elle sort de
  l'usine PSA/Stellantis de Trnava. — presse professionnelle (L'Usine
  Nouvelle, Largus.fr, 2016) · *déclaratif*
- **Sidérurgie, ArcelorMittal Florange** (Moselle). Arrêt définitif des
  hauts fourneaux annoncé le 17 décembre 2012, 629 postes supprimés sur la
  filière amont ; le site ne conserve que la galvanisation, alimentée par
  des brames produites sur d'autres sites du groupe. Citation d'ArcelorMittal :
  *« ArcelorMittal confirme ne pas vouloir relancer la production d'acier
  liquide sur le site »*. — presse (franceinfo.fr, Europe 1), questions
  parlementaires · *déclaratif, citation d'entreprise rapportée*
- **Pneumatiques, Continental Clairoix** (Oise → Timișoara, Roumanie).
  Fermeture annoncée le 11 mars 2009, effective le 31 mars 2010 (~1 120
  emplois, ~8 millions de pneus/an) ; la capacité du site roumain de
  Timișoara passe dans le même temps de 13 à 30 millions de pneus/an. —
  presse professionnelle et syndicale (L'Usine Nouvelle) · *déclaratif*
- **Électroménager, Whirlpool Amiens** (Somme → Łódź, Pologne). Annonce le
  24 janvier 2017 de cesser la production de sèche-linge à Amiens (286 à
  290 postes), fermeture effective en juin 2018 ; l'usine polonaise devient
  le site central de la nouvelle plateforme sèche-linge du groupe — un
  dossier devenu un sujet de la campagne présidentielle 2017. — presse,
  questions parlementaires (assemblee-nationale.fr) · *déclaratif*
- **Petit électroménager, Moulinex**. Dépôt de bilan le 7 septembre 2001,
  arrêt de l'activité le 11 septembre, cinq usines normandes fermées (environ
  4 500 licenciements). Actifs repris par SEB le 22 octobre 2001. — étude de
  cas Eurofound (« Moulinex: chronicle of a death foretold »), étude
  académique (Cairn.info) · *le mieux sourcé des cas listés ici*
- **Télévisions, Thomson Multimedia**. Restructurations en cascade des
  usines françaises dans les années 2000 (Angers, Brest, Gray, Bagneaux) ;
  TCL-Thomson Electronics annonce en novembre 2006 la fermeture des sites de
  production européens ; liquidation judiciaire de Technicolor (ex-Thomson)
  le 11 octobre 2012. — presse professionnelle, collectivités locales ·
  *déclaratif*
- **Lingerie, Lejaby** (Haute-Loire → Sfax, Tunisie). Délocalisation
  progressive depuis 1992 ; à la fermeture de la dernière usine française
  (Yssingeaux, 93 salariés), 83 % de la production était déjà en Tunisie,
  10 % en Chine. — presse régionale, question parlementaire · *déclaratif*
- **Houille, dernière mine française** (La Houve, Creutzwald, Moselle).
  Extraction arrêtée le 23 avril 2004, terme du « pacte charbonnier » de
  1994 — ce n'est pas une délocalisation au sens strict (aucune production
  française n'a été « déplacée », le gisement s'épuisait), mais la demande
  française de houille est depuis couverte par l'importation. À ne pas
  confondre avec les cas précédents. — archives INA, Encyclopædia
  Universalis · *déclaratif*

**Un correctif nécessaire** : contrairement à une idée reçue, aucune source
ne confirme qu'un modèle Dacia précis remplace une production
antérieurement française — Dacia (filiale roumaine de Renault, à Mioveni)
n'a jamais eu de production équivalente rapatriée depuis la France. Le cas
Renault solide et vérifié est celui de la Twingo (ci-dessus), vers la
Slovénie, pas la Roumanie.

### 4. D'où viennent aujourd'hui les biens autrefois fabriqués en France : trois secteurs

Une carte du monde des importations françaises, colorée par volume, mettrait
en avant l'Allemagne et l'Espagne — les deux grandes puissances automobiles
européennes historiques, pas des destinations de délocalisation. Ce serait
un contresens visuel. Les graphiques ci-dessous montrent plutôt, pour
chaque partenaire, la PART qu'il occupait en 2013 et celle qu'il occupe en
2024 — le déplacement, pas seulement le volume.

**Automobiles (HS 8703)**

<!-- schema:commerce-automobile -->

L'Allemagne et l'Espagne restent, en 2024, les deux premiers fournisseurs
de voitures de la France — un fait qui contredit tout récit d'une
disparition pure et simple de l'automobile « occidentale ». Mais parmi les
partenaires qui montent, le Maroc a multiplié par 5 sa part depuis 2013 et
la Roumanie par 3,4, tandis que la Slovaquie et la Tchéquie, sites
d'assemblage de plusieurs constructeurs français et allemands, comptent
déjà parmi les dix premiers fournisseurs.

**Textile-habillement (HS 61 + 62)**

<!-- schema:commerce-textile -->

Le secteur le plus anciennement délocalisé est aussi le plus concentré :
Chine et Bangladesh à eux seuls dépassent, en 2024, le quart des
importations françaises d'habillement. L'Italie reste le seul partenaire du
classement où l'essentiel de la production se fait encore en Europe de
l'Ouest.

**Télévisions et écrans (HS 8528)**

<!-- schema:commerce-electronique-tv -->

La Chine domine massivement ce secteur (environ un quart des importations
françaises en 2024). Les Pays-Bas apparaissent en bonne position sans être
un site d'assemblage connu : Rotterdam est un point d'entrée et de
réexpédition majeur pour l'Europe, pas nécessairement le pays de fabrication
réelle — un biais que Comtrade ne permet pas de lever (le pays déclaré est
celui de provenance directe, pas toujours celui de fabrication d'origine).

**Sur ces trois graphiques** : les valeurs viennent d'UN Comtrade, un miroir
onusien des déclarations douanières nationales — PAS les douanes françaises
elles-mêmes (leur portail, lekiosque.finances.gouv.fr, publie des données
plus détaillées mais sans API, un chantier d'intégration séparé). Les
montants sont en **dollars courants**, la convention Comtrade, jamais à
confondre avec les euros utilisés ailleurs dans ce dépôt.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Ce que les données ne disent pas

### 5. Ce qui reste hors de portée de cette version

- **Les huit cas du §3 ne sont pas un échantillon représentatif** : ce sont
  des cas notoires, documentés parce qu'ils ont fait l'actualité — aucune
  méthode ne permet de savoir combien de cas comparables, moins médiatisés,
  ont eu lieu sans laisser de trace publique équivalente.
- **Les douanes françaises (DGDDI, lekiosque.finances.gouv.fr) ne sont pas
  chargées** : les graphiques du §4 viennent d'UN Comtrade, un miroir
  onusien des mêmes déclarations douanières — une source secondaire par
  rapport à la source primaire française, qui n'a pas d'API et demanderait
  un connecteur de formulaire séparé.
- **Le pays déclaré par Comtrade est celui de provenance directe, pas
  toujours celui de fabrication d'origine** (voir la remarque sur les
  Pays-Bas au §4) : un bien peut transiter par un pays sans y être produit.
- **Les délocalisations depuis 2018 ne sont pas mesurées** : l'étude Insee
  s'arrête en 2017, faute d'une méthode équivalente republiée depuis
  (situation vérifiée en septembre 2026). Aucune tendance récente (Covid,
  relocalisations annoncées, tensions sur les chaînes d'approvisionnement)
  n'est donc documentée ici.
- **La destination des délocalisations n'est pas chargée** : l'étude Insee
  publie une répartition par zone géographique des importations associées
  (Figure 5, non chargée dans cette version) — un signal indirect, pas une
  destination certaine emploi par emploi.
- **Le glissement sectoriel (§1) et les délocalisations (§2) mesurent deux
  choses différentes, jamais additionnées ici** : le premier est un concept
  d'emploi intérieur total (comptes nationaux, Eurostat) ; le second est une
  détection sur un champ plus étroit (entreprises de 50 salariés ou plus,
  hors agriculture et finance). Une bonne part du glissement sectoriel tient
  à d'autres facteurs que la délocalisation (automatisation,
  externalisation vers des prestataires français, évolution de la demande) —
  ce dossier ne prétend pas isoler la part de chacun.
- **Les relocalisations** (le mouvement inverse, souvent annoncé depuis la
  crise sanitaire de 2020) ne sont pas mesurées par une source équivalente
  trouvée à ce jour — à rechercher séparément.
- **La qualité du chiffrage lui-même** : les scénarios bas/central/haut de
  l'Insee viennent d'un modèle de détection (AUC 0,54 à 0,80), pas d'un
  comptage ; ce dossier les présente tels quels, sans les recalculer ni les
  arbitrer.

## Sources

- Eurostat, `nama_10_a10_e` (emploi intérieur total par branche, NACE Rév.
  2, niveau A10), France, 1975-2025.
- Insee, *Les entreprises en France*, édition 2022, fiche « Plus de 10 000
  emplois délocalisés chaque année de 2011 à 2017, avant une chute continue
  du nombre de délocalisations » — Figures 2, 4, 6 et 7 (Ésane, enquête CAM,
  DGDDI/Douanes).
- UN Comtrade (reporterCode 251, France), importations par partenaire,
  codes HS 8703 (automobiles), 61 et 62 (textile-habillement), 8528
  (télévisions et écrans), 2013 et 2024.
- Renault Group, page officielle de l'usine Revoz (Novo Mesto, Slovénie).
- Sources de presse et archives pour les huit cas du §3, détaillées ligne
  par ligne dans le texte (L'Usine Nouvelle, franceinfo.fr, Europe 1,
  Eurofound, Cairn.info, archives INA, questions parlementaires).

## Versions

- **Version 2** (17 septembre 2026) : huit cas vérifiés d'usines fermées et
  de production partie à l'étranger (§3) ; trois graphiques de déplacement
  des importations françaises par partenaire, automobile/textile/télévisions
  (§4, UN Comtrade, 2013-2024) — après vérification qu'une carte du monde
  aurait mis en avant les mauvais pays (Allemagne, Espagne plutôt que les
  destinations de délocalisation).
- **Version 1** (17 septembre 2026) : glissement sectoriel de l'emploi
  1975-2025 (Eurostat) ; délocalisations d'unités légales et d'emplois,
  1995-2017, avec la carte départementale et la surreprésentation des
  emplois qualifiés parmi les postes délocalisés (Insee).
