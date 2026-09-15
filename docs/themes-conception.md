# Les thèmes : ce qu'on peut en faire, ce qu'on ne peut pas

> **Méthode** · version 2 · 15 septembre 2026

Révision de la conception initiale. La conception initiale posait une taxonomie
maison `v1` (INSTITUTIONS, LIBERTES, DROITS_FEMMES, FISCALITE, SOCIAL, DEFENSE,
EUROPE, SECURITE) qu'il aurait fallu appliquer nous-mêmes, texte par texte. Elle
compte **0 assignation**. On garde les **30 thèmes du Sénat** (D-017) : ils sont
officiels, déjà posés, et ne nous demandent aucun jugement éditorial.

## 1. État mesuré des thèmes en base (2026-09-12)

| Taxonomie | Assignations | Porte sur | Couverture |
|---|---|---|---|
| Sénat (30 thèmes) | 17 660 | **dossiers** | 8 412 lois |
| EuroVoc (1 808 concepts) | 68 823 | **scrutins** | 6 540 scrutins PE, 2019→2026 |
| maison `v1` (8 thèmes) | 0 | — | — |

Deux taxonomies, deux objets différents. C'est la contrainte structurante.

### Ce que ça donne par institution

- **Parlement européen** — thème posé directement sur le scrutin. Analysable
  immédiatement. Mais EuroVoc est **plat chez nous** : 1 808 concepts, aucun
  parent chargé. Un concept EuroVoc n'est pas un thème de débat public, c'est un
  descripteur documentaire. Inutilisable tel quel pour une navigation citoyenne ;
  il faudrait charger la hiérarchie EuroVoc (21 domaines racines) pour agréger.
- **Assemblée nationale** — aucun thème sur les scrutins. Récupérable par la
  navette (D-019) : **2 577 scrutins AN** héritent du thème de la loi du Sénat,
  soit 98,8 % des scrutins AN rattachés à un dossier. Fenêtre : avril 2025 →
  juillet 2026.
- **Sénat** — 4 764 scrutins, 1 647 612 votes nominatifs, et **aucun lien vers
  une loi** dans le dump Dosleg (D-016). Les thèmes du Sénat ne s'appliquent pas
  aux votes du Sénat. C'est le paradoxe à assumer tel quel.

### Concentration : le thème « Société » avale tout

Répartition des 2 577 scrutins AN thémés :

    Société                            1 146   (44 %)
    Questions sociales et santé          631
    Agriculture et pêche                 473
    Police et sécurité                   455
    Affaires étrangères et coopération   339
    Économie et finances, fiscalité      318
    Défense                              293
    Éducation                            231
    … puis 20 thèmes sous 200, dont 6 sous 20

Un thème qui prend 44 % du corpus ne discrimine rien. Toute page « thèmes » qui
mettrait les 30 thèmes sur le même plan mentirait sur leur poids. Il faut afficher
l'effectif à côté du libellé, et se résoudre à ce qu'une dizaine de thèmes
seulement supportent une analyse.

## 2. Analyses réellement possibles sur les groupes et les partis

Par ordre de solidité décroissante.

### 2.1 Descriptif, sans comparaison — publiable tout de suite

Pour un groupe et un thème : nombre de scrutins, répartition pour/contre/
abstention/absent, et la liste des scrutins avec leur lien vers le dossier
officiel. C'est le cœur de l'outil : « sur les 455 scrutins touchant Police et
sécurité, voici les votes du groupe X, un par un, vérifiables ». Aucune
statistique comparative, donc aucun pré-enregistrement nécessaire.

### 2.2 Accord deux à deux par thème — la mesure solide

Pour chaque paire de groupes et chaque thème : part des scrutins où les deux
groupes ont pris la même position. Produit une matrice 13×13 par thème, et par
projection une carte des proximités.

C'est **la** mesure défendable, parce qu'elle est symétrique et qu'elle ne
dépend pas du statut majorité/opposition : deux groupes d'opposition qui votent
contre le même texte pour des raisons opposées sont comptés d'accord — ce qui est
factuellement ce qui s'est passé, et se lit dans le détail des scrutins.

Piège à documenter : l'absence. Un groupe absent n'est ni d'accord ni en
désaccord. Exclure les paires où l'un des deux est absent, et publier le taux
d'exclusion à côté du résultat.

### 2.3 Profil thématique d'un groupe — descriptif, honnête

Part de l'activité d'un groupe consacrée à chaque thème (par les dépôts, via
`core.dossier_author`, plus que par les votes : l'ordre du jour des votes n'est
pas choisi par les groupes). Répond à « de quoi ce parti parle-t-il quand il a
la main », question différente de « comment vote-t-il ».

### 2.4 Taux de vote « pour » par thème — à manier avec des pincettes

Techniquement immédiat, interprétativement fragile : le taux de « pour » d'un
groupe mesure d'abord s'il soutient le gouvernement, pas son idéologie. Un même
parti passe de 90 % à 10 % en changeant de camp sans changer d'idée. À ne
publier qu'accompagné du statut majorité/opposition sur la période.

## 3. Les thèmes corrèlent-ils avec le classement gauche/droite ?

### 3.1 D'abord : où est ce classement

Il existe, vous ne le retrouviez pas parce qu'il n'est affiché que sur les pages
`/organisation/<slug>/`, en demi-jauge. C'est **CHES 2024** :

| Dimension | Partis français | Étendue observée |
|---|---|---|
| `lrgen` (gauche-droite général) | 10 | 0,8 → 9,7 |
| `lrecon` (économique) | 10 | 0,9 → 8,3 |
| `galtan` (libertaire/autoritaire) | 10 | 1,7 → 9,1 |
| `immigrate_policy` | 10 | 1,5 → 9,7 |
| `eu_position` | 10 | 1,6 → 6,8 |

73 codages au total dans `core.party_classification`.

### 3.2 Le blocage : il n'y a pas de pont en base

`core.party_group_link` est **vide** (D-020). Le rattachement parti → groupe vit
dans `data/organisations.csv` et n'est lu qu'au build. **Aucune requête SQL ne
peut aujourd'hui joindre un score CHES à un vote.** La corrélation demandée n'est
donc pas « à calculer » : elle est pour l'instant incalculable, faute de ce pont.

Une fois le pont chargé, on dispose de **9 partis** appariés à un groupe AN
(Renaissance→EPR, LR→DR, PCF→GDR, LE→EcoS, PS→SOC, Horizons→HOR, LFI→LFI-NFP,
RN→RN, MoDem→Dem), couvrant 1 202 334 des 1 270 476 bulletins (94,6 %). Restent
dehors LIOT, UDR et les non-inscrits — à exclure explicitement, pas à rattacher
d'office.

### 3.3 Ce que vaudrait la corrélation

**n = 9.** C'est le chiffre qui commande tout le reste. Avec 9 points, un
coefficient de corrélation a un intervalle de confiance à 95 % qui couvre
typiquement ±0,6 : un r de 0,5 est indiscernable de 0. Aucun r calculé sur
9 partis ne peut être présenté comme un résultat. Le nombre de scrutins (2 577)
ne rachète rien : il réduit le bruit sur **chaque point**, pas le nombre de
points.

Et il y a 30 thèmes × 5 dimensions CHES = 150 corrélations possibles. En chercher
une qui « marche » et la publier, c'est du p-hacking caractérisé — exactement ce
que la discipline de pré-enregistrement du projet (`pre-enregistrement-001.md`,
`-002.md`) existe pour interdire.

**Ce qu'on peut faire à la place, et qui est honnête** : sur la matrice d'accord
deux à deux (§2.2), tester **une seule** hypothèse, fixée avant calcul —
« l'accord entre deux groupes décroît quand l'écart de leurs `lrgen` croît ».
L'unité n'est plus le parti mais la **paire** : 36 paires, pas 9 points. C'est
encore peu, mais c'est une hypothèse unique, directionnelle, préenregistrable, et
falsifiable. Le thème devient une variable de **modération** : sur quels thèmes
cette relation est-elle la plus forte, sur lesquels s'effondre-t-elle.

Un résultat nul est publiable et attendu : « sur le thème Agriculture et pêche,
l'écart gauche-droite ne prédit pas le désaccord » est un fait utile, et c'est le
genre de fait qui protège l'outil du soupçon d'orientation.

## 4. Ordre de travail proposé

1. Charger `core.party_group_link` depuis `organisations.csv` (9 liens) — lève
   D-020, débloque toute jointure score↔vote.
2. Matérialiser l'héritage de thème AN via la navette (D-019), en table dérivée
   avec `method_version`, jamais en dur dans `core.topic_assignment`.
3. Charger la hiérarchie EuroVoc (domaines racines) pour rendre les 6 540
   scrutins PE agrégeables.
4. Publier §2.1 et §2.2 — descriptif et accord deux à deux, sans
   pré-enregistrement.
5. Sceller un pré-enregistrement 003 pour la seule hypothèse du §3.3 avant tout
   calcul de corrélation.

## Versions

- **Version 2** (15 septembre 2026) : en-tête commun des documents de méthode (perimetre.md § 2.8, D-066).
- **Version 1** (12 septembre 2026) : révision de la taxonomie, adoption des thèmes du Sénat (D-017).
