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

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Ce que les données ne disent pas

### 3. Ce qui reste hors de portée de cette version

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

## Versions

- **Version 1** (17 septembre 2026) : glissement sectoriel de l'emploi
  1975-2025 (Eurostat) ; délocalisations d'unités légales et d'emplois,
  1995-2017, avec la carte départementale et la surreprésentation des
  emplois qualifiés parmi les postes délocalisés (Insee).
