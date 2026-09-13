# Violences policières : ce que les sources permettent de dire

> Note de méthode. Version 1 — 13 septembre 2026.
> Sujet où l'écart entre ce qui est débattu et ce qui est mesuré est le plus
> grand du projet. Cette note dit ce qui existe, ce qui n'existe pas, et le
> piège d'étiquette qui fausse la moitié des recherches sur le sujet.

---

## 1. Le piège d'étiquette, à lire avant toute requête

Deux catégories pénales portent des noms presque identiques et désignent des
faits **opposés** :

| libellé | ce que ça compte |
|---|---|
| violences **par** personne dépositaire de l'autorité publique | **un policier frappe** |
| violences **contre** personne dépositaire de l'autorité publique | **un policier est frappé** |

Ce sont deux NATINF distincts. Une recherche documentaire sur « violences
personne dépositaire de l'autorité publique » ramène massivement la seconde,
parce qu'elle est plus commentée et mieux recensée. **Toute requête, tout
chargement et tout graphique doit nommer laquelle des deux il porte**, et le
libellé abrégé « violences PDAP » est à proscrire : il est ambigu.

## 2. Ce que la France publie

**Rien de général.** Il n'existe pas de statistique publique d'ensemble des
violences policières, alors que les violences *contre* les forces de l'ordre
sont recensées — l'asymétrie est en elle-même un fait à signaler.

| source | ce qu'elle donne | limite |
|---|---|---|
| **IGPN** | depuis 2017, recensement des personnes blessées ou tuées à l'occasion de missions de police ; depuis 2020, un résumé du contexte de chaque décès | l'IGPN **ne traite qu'environ 10 %** des affaires pénales impliquant des policiers ; elle est saisie, elle ne recense pas |
| **Ministère de la Justice** (Cassiopée / SDSE) | nombre d'affaires ouvertes pour violences *par* personne dépositaire de l'autorité publique : **700 en 2016, 1 110 en 2024** | **ces chiffres circulent par voie de presse, pas comme série publiée**. L'open data justice porte sur les *décisions*, pas sur les affaires |
| Données départementales | — | **inexistantes en accès public** pour cette catégorie |

Conséquence pratique : le chiffre 700 → 1 110 ne peut pas être présenté comme
une donnée du projet tant qu'on n'a pas remonté la publication d'origine. Il
est cité ici comme une piste, pas comme un fait chargé.

## 3. Ce que l'Europe publie

**Rien d'harmonisé.** Le Conseil de l'Europe constate qu'il n'existe pas même
de définition commune de ce qu'est un décès en garde à vue, ni de méthodologie
d'enquête partagée. Eurostat ne publie pas la catégorie.

Le seul recensement comparatif est **journalistique** : le European Data
Journalism Network dénombre **488 décès en garde à vue ou en opération de
police dans treize pays de l'Union entre 2020 et 2022, dont 107 en France** —
le plus fort total absolu. Source de niveau 2 au sens du projet, et construite
précisément parce que la source officielle manque.

## 4. Ce que nous avons déjà, et qui vaut mieux

`core.cedh_arret` contient **1 181 arrêts de la Cour européenne des droits de
l'homme concernant la France, de 1986 à 2026**, avec les articles invoqués et
le sens de la décision.

| article | violations constatées | arrêts |
|---|---|---|
| **3** — torture, traitements inhumains ou dégradants | **92** | 120 |
| **2** — droit à la vie | **15** | 28 |

Trois à sept condamnations par an sur ces deux articles depuis 2019.

C'est de loin la meilleure source disponible sur le sujet, pour trois raisons :
elle est **juridictionnelle** — un fait établi contradictoirement, pas un
signalement ; elle est **longue** — quarante ans ; et elle est **déjà chargée**.

**La réserve est aussi importante que la donnée.** Les articles 2 et 3 couvrent
bien plus que les violences policières : conditions de détention, expulsions
vers un pays à risque, absence de soins en prison, traitement des personnes
vulnérables. **Ces chiffres sont un majorant, pas un compteur.** Les publier
comme « 92 condamnations de la France pour violences policières » serait faux.

Ce qu'il faudrait pour en faire un compteur : lire la conclusion de chacun des
120 arrêts et coder la nature des faits. Cent vingt lectures, une par arrêt,
avec un protocole de codage écrit d'avance — c'est faisable, ce n'est pas
automatisable, et cela relève d'une décision éditoriale datée.

## 5. Ce qui est publiable en l'état

1. **Les condamnations CEDH sur les articles 2 et 3**, avec leur libellé exact
   et la réserve du § 4 écrite sur la page, pas en note.
2. **Le constat d'absence lui-même** : il n'existe pas de statistique publique
   générale des violences policières en France, ni de définition commune en
   Europe. Le § 2.2 du périmètre fait du « non vérifiable » une réponse de plein
   droit — c'en est un cas d'école.
3. **L'asymétrie de recensement** : les violences contre les forces de l'ordre
   sont comptées, celles exercées par elles ne le sont pas de façon générale.

## 6. Ce qui n'est pas publiable en l'état

- Tout chiffre national d'« affaires de violences policières » tant que la
  publication d'origine n'est pas identifiée et scellée.
- Toute ventilation départementale : elle n'existe pas.
- Toute comparaison européenne présentée comme officielle : la seule qui existe
  est journalistique, et son auteur explique lui-même qu'elle pallie un manque.
- Tout dénombrement de « victimes » tiré des arrêts CEDH sans codage préalable
  des faits, arrêt par arrêt.
