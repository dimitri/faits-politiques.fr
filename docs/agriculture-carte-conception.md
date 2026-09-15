# Cartographier l'agriculture : exploitations, surfaces, production

> **Méthode** · version 2 · 15 septembre 2026
>
> Note de conception.
> Question posée : peut-on représenter sur une carte la production agricole et le
> nombre de paysans, à partir du recensement agricole communal ?
>
> Réponse courte : **oui pour les exploitations et les surfaces, à la commune ;
> non pour la production, qui n'existe qu'au département ; et « paysans » n'est
> pas « exploitations ».** Trois pièges rendent une carte naïve fausse, et ils se
> mesurent sur les fichiers réels.

---

## 1. Ce que la base contient déjà

| objet | contenu | limite pour une carte |
|---|---|---|
| `core.agriculture_indicateur` | 315 valeurs, FAOSTAT | **national uniquement** |
| `geo.contour` | 101 départements, 18 régions, 7 collectivités | **aucun contour communal** |
| `core.epci`, `ref.commune` | intercommunalités (BANATIC), communes COG 2026 | pas de géométrie, pas de superficie |

Une carte départementale est donc réalisable aujourd'hui. Une carte communale
suppose d'abord de charger les contours des communes (§ 6).

## 2. Où se trouve réellement le recensement agricole communal

Le recensement agricole 2020 publie 56 indicateurs à la commune. Trouver un
fichier téléchargeable a demandé d'écarter trois fausses pistes.

**data.gouv.fr n'héberge aucun fichier.** Le jeu « Agreste — Données communales
du recensement agricole » (Licence Ouverte, maj. 3 octobre 2025) ne contient que
des **liens vers des applications web**. Sa seule ressource tabulaire — la liste
des indicateurs — renvoie **HTTP 400**.

**Le domaine de l'Agreste a une chaîne TLS incomplète.** `agreste.agriculture.gouv.fr`
et `stats.agriculture.gouv.fr` n'envoient que leur certificat feuille, émis par
*GEANT TLS RSA 1* (HARICA), sans le certificat intermédiaire. Un navigateur
compense en allant le chercher ; `curl` et le client HTTP de Go refusent la
connexion — `archive.Fetch` échouera. **Le correctif n'est pas de désactiver la
vérification** : c'est d'embarquer l'intermédiaire, publié à l'adresse indiquée
dans le certificat lui-même (`http://crt.harica.gr/HARICA-GEANT-TLS-R1.cer`), et
d'étendre le pool de confiance. Vérifié : avec la chaîne complétée, la connexion
passe.

**Geoclip n'expose pas d'export.** La cartographie interactive de l'Agreste est
une application RequireJS ; ses données transitent par une API non documentée.
Une ingestion qui s'appuierait dessus casserait à la première mise à jour.

**La porte qui fonctionne : l'Observatoire des territoires (ANCT).** Il republie
les indicateurs du recensement avec une API de téléchargement stable :

```
https://www.observatoire-des-territoires.gouv.fr/outils/cartographie-interactive/api/v1/functions/GC_API_download.php?type=stat&nivgeo=com2025&dataset=agri&indic=exp2020
```

`nivgeo` accepte `com2025` (communes), `epci2025` et `epci_ept2025`. Deux
indicateurs vérifiés le 14 septembre 2026 :

| `indic` | contenu | communes renseignées | total | contrôle |
|---|---|---|---|---|
| `exp2020` | nombre d'exploitations | 34 875 | **416 409** | cohérent avec le résultat national du recensement |
| `sau2020` | surface agricole utilisée | 33 591 | **26,88 M ha** | idem |

Réponse en XLSX, deux feuilles (données et métadonnées), géographie communale
2025. Le site de l'Observatoire affiche d'autres indicateurs agricoles —
évolution 2010-2020, part de la surface toujours en herbe, part en agriculture
biologique — dont les codes se relèvent page par page : **les deviner ne marche
pas**, un code inexistant renvoie HTTP 500.

## 3. Premier piège : une case vide n'est pas un zéro

Sur les 34 875 communes du fichier `exp2020` : **1 284 cellules vides, et pas un
seul zéro explicite.**

C'est impossible pour un recensement réel — des milliers de communes urbaines
n'ont aucune exploitation. Le vide absorbe donc deux situations sans les
distinguer :

- **aucune exploitation** dans la commune ;
- **secret statistique** : aucun résultat n'est publié qui porte sur moins de
  trois exploitations, ou dont une seule représente 85 % ou plus de la valeur.

Conséquence directe sur la carte : **une commune vide ne se colore jamais comme
« 0 »**. Elle porte une teinte neutre et une légende qui dit « aucune exploitation
ou donnée couverte par le secret statistique ». Colorer le vide en zéro ferait
apparaître des déserts agricoles là où il y a simplement moins de trois fermes.

## 4. Deuxième piège : la surface est au siège, pas au champ

La source le documente elle-même : les données sont **localisées à la commune du
siège de l'exploitation**, et une exploitation peut travailler des terres sur
plusieurs communes, voire plusieurs départements.

La « SAU d'une commune » est donc la surface des exploitations **qui ont leur
siège** dans cette commune, pas la surface agricole **située** dans la commune.
Une carte communale de `sau2020` montre la géographie des sièges d'exploitation.
Elle peut attribuer à un bourg les terres de tout un canton, et laisser vides des
communes entièrement cultivées.

Ce biais disparaît quand on agrège : il est fort à la commune, faible à l'EPCI,
presque nul au département. C'est un argument de plus pour choisir l'échelle
avant de choisir la couleur.

**Pour l'occupation réelle du sol, la source juste est le Registre parcellaire
graphique** (IGN) : les parcelles déclarées, géolocalisées au 1:5 000, mises à
jour chaque année, sous Licence Ouverte. Mais c'est un autre ordre de grandeur —
**9,8 millions d'enregistrements, 17 Go** — et il ne recense que les surfaces
déclarées pour les aides de la PAC.

*Le biais n'a pas pu être chiffré ici : il faudrait la superficie de chaque
commune, qui arrivera avec les contours du § 6. Une fois chargés, compter les
communes dont la SAU dépasse la superficie donnera sa mesure exacte.*

## 5. Troisième piège : exploitations, paysans, production

**Une exploitation n'est pas un paysan.** Le nombre d'exploitations ne dit pas
combien de personnes y travaillent : une exploitation individuelle compte une
personne, un GAEC plusieurs, une grande exploitation des salariés. Le
recensement mesure aussi les chefs d'exploitation et les unités de travail annuel,
mais **ces indicateurs n'ont pas été trouvés exposés à la commune** sur
l'Observatoire. « Nombre de paysans » doit donc s'afficher pour ce qu'il est —
« nombre d'exploitations » — tant que ces séries ne sont pas chargées.

**La production n'existe pas à la commune.** Le recensement décrit des
*structures* — exploitations, surfaces, cheptel —, pas ce qu'elles produisent.
Les quantités produites relèvent de la **Statistique agricole annuelle** de
l'Agreste, qui descend **au département et pas plus bas**. Une carte de la
production est donc départementale par construction, et `geo.contour` contient
déjà les départements.

## 6. Prérequis d'une carte communale

**Les contours.** IGN publie **Admin Express COG CARTO**, sous Licence Ouverte, y
compris dans des versions simplifiées pour la visualisation (GeoJSON, TopoJSON,
GeoParquet) et une variante qui rapproche les outre-mer de la métropole. C'est la
source à charger dans `geo.contour`, au niveau `COMMUNE` et `EPCI`.

**Le millésime.** Le fichier de l'Observatoire est en **géographie communale
2025**, `ref.commune` en **2026**. Les communes nouvelles de 2026 n'y ont pas de
correspondance directe : la jointure passe par `ref.commune_change`, et toute
commune non rapprochée doit apparaître comme telle, pas disparaître.

**Le poids.** 34 875 polygones dans un SVG statique : même simplifiés, plusieurs
mégaoctets par carte. Une carte nationale communale n'est pas une page, c'est un
fichier à charger à la demande — ou elle se remplace par une carte par
département, à 350 communes en moyenne.

## 7. Proposition

Trois niveaux, du plus immédiat au plus coûteux.

| niveau | ce qu'on montre | données | contours | effort |
|---|---|---|---|---|
| **Département** | production par filière (SAA) ; exploitations et SAU agrégées | SAA à charger ; RA agrégeable | **déjà en base** | faible |
| **EPCI** | exploitations, SAU, évolution 2010-2020 | Observatoire, `nivgeo=epci2025` | à charger (Admin Express) | moyen |
| **Commune** | exploitations | Observatoire, `nivgeo=com2025` | à charger (Admin Express) | moyen, et poids des SVG |

L'EPCI est probablement la bonne échelle de lecture : le biais du siège y est
faible, le secret statistique y mord beaucoup moins qu'à la commune, les
intercommunalités sont déjà dans la base avec leurs compétences — et c'est
l'échelon où se décident une partie des politiques foncières.

## 8. Ce que chaque carte doit porter

1. **L'indicateur exact** — « nombre d'exploitations », jamais « nombre de
   paysans ».
2. **La localisation au siège**, écrite sous toute carte de surface.
3. **Une teinte et une légende distinctes pour le vide**, qui nomment le secret
   statistique.
4. **Le millésime** du recensement (2020) et de la géographie (2025).
5. **La source et son intermédiaire** : Agreste, via l'Observatoire des territoires.

## Versions

- **Version 2** (15 septembre 2026) : en-tête commun des documents de méthode (perimetre.md § 2.8, D-066).
- **Version 1** (14 septembre 2026) : note de conception de la carte agricole.
