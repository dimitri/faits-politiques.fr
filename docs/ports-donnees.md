# Les grands ports maritimes français : trafic, comparaison européenne

> **Dossier** · version 1 · 19 septembre 2026
>
> Le premier port français (HAROPA, Le Havre-Rouen) pèse cinq fois moins
> que Rotterdam. Ce dossier montre l'évolution du trafic des grands ports
> français depuis 2000 et les situe face au rang nord-européen — avec les
> deux précautions de méthode que cette comparaison exige.

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

### 1. HAROPA : une fusion administrative de 2021, pas toujours la même chose dans les sources

**HAROPA** (Le Havre, Rouen, Paris) est né de la fusion administrative de
2021 des trois ports de l'axe Seine. **Une précaution réelle avant toute
comparaison dans le temps** : le SDES (statistiques françaises, § 2)
regroupe Le Havre et Rouen sous le nom « HAROPA » sur toute sa série
depuis 2000, quand Eurostat (comparaison européenne, § 3) ne bascule Le
Havre vers « HAROPA » qu'à partir de 2022, en cohérence avec la date de la
fusion elle-même. **Les deux séries ne mesurent donc pas le même
périmètre avant 2022** — ce dossier ne les mélange jamais dans un même
graphique continu.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. Le trafic des grands ports français depuis 2000

<!-- schema:ports-francais -->

Les quatre plus grands ports maritimes français (HAROPA, Marseille,
Dunkerque, Nantes-Saint-Nazaire) montrent une trajectoire commune : un
recul entre 2000 et 2020, puis une reprise partielle jusqu'en 2025 sans
retrouver, pour la plupart, leur niveau de 2000. Marseille recule le plus
fortement en proportion (94,1 Mt en 2000 à 74,0 Mt en 2025, −21 %), quand
Dunkerque est le seul des quatre à dépasser son niveau de 2000 (45,3 Mt à
48,0 Mt, +6 %).

### 3. Face au rang nord-européen : un écart d'échelle, pas de rattrapage

<!-- schema:ports-europe -->

**Rotterdam (390 Mt en 2025) traite à lui seul plus de cinq fois le
tonnage de HAROPA (78 Mt)** et environ deux fois celui d'Anvers (216 Mt en
2021, dernière année disponible dans la source utilisée ici). Cet écart
n'est pas nouveau ni en train de se combler : sur la période disponible
dans Eurostat pour Le Havre seul (1997-2021, avant la fusion HAROPA), le
tonnage du port français est resté globalement stable, quand celui de
Rotterdam a continué de croître — un écart d'échelle structurel entre le
premier port du range nord-européen et les ports français, pas un
phénomène récent.

**Deux précautions de méthode, explicites plutôt que silencieuses** :
l'année de référence diffère selon le port (Anvers 2021, les cinq autres
2025 ou proche) — chaque barre du graphique porte sa propre année ; et la
comparaison porte sur le tonnage total, sans distinguer vrac et
conteneurs, une distinction que ce dossier n'a pas encore chargée par port
européen.

## Ce que les données ne disent pas

### 4. Ce qui reste hors de portée de cette version

- **L'emploi portuaire direct et induit** : des études régionales de
  l'Insee existent (par exemple 31 000 emplois liés aux infrastructures
  portuaires des Hauts-de-France, 9 000 pour les ports de Normandie), mais
  aucune série nationale consolidée par grand port, avec une méthode
  homogène, n'a été identifiée — ce dossier ne les additionne pas pour
  éviter un total qui mélangerait des méthodes différentes.
- **Les conflits sociaux et leur effet sur le trafic** : aucune donnée
  publique structurée trouvée. Les chiffres qui circulent (jours de grève,
  conteneurs perdus) proviennent d'enquêtes déclaratives auprès de
  transporteurs ou de la presse spécialisée, pas d'une statistique
  publique vérifiable indépendamment.
- **Le déclin relatif sur longue période (avant 1997)** : la source
  européenne utilisée ici (§ 3) ne remonte pas au-delà de 1997 ; une
  comparaison sur plusieurs décennies demanderait une source
  supplémentaire non identifiée à ce jour.
- **La répartition conteneurs (EVP) par port en comparaison européenne** :
  un dataflow Eurostat existe pour cet indicateur mais son code exact n'a
  pas pu être confirmé par un accès direct dans le temps disponible — non
  chargé plutôt que deviné.
- **Les emprises géographiques des zones portuaires** : des shapefiles
  existent (GéoLittoral, ministère) mais leur contenu exact n'a pas été
  vérifié avant l'écriture de ce dossier — à reprendre si une carte des
  emprises devient nécessaire.

## Sources

- SDES (ministère de la Transition écologique), *Transport maritime de
  marchandises*, data.statistiques.developpement-durable.gouv.fr, 2000-2025
  (§ 1, § 2).
- Eurostat, `mar_go_aa` (trafic portuaire, poids brut manutentionné),
  1997-2025 (§ 1, § 3).
- Insee, études régionales sur l'emploi portuaire (Hauts-de-France,
  Normandie, Île-de-France), citations non chargées (§ 4).

## Annexe technique

### 5. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | SDES, trafic maritime de marchandises | `core.trafic_portuaire` | 1 540 lignes, 42 ports, 2000-2025 |
| 2 | Eurostat, `mar_go_aa` | `core.trafic_portuaire_europe` | 141 lignes, 6 ports, 1997-2025 |

**Non chargé, et pourquoi :**

- Emploi portuaire consolidé : études régionales hétérogènes, pas de série
  nationale homogène (§ 4).
- Conflits sociaux et pertes de trafic associées : aucune statistique
  publique structurée identifiée (§ 4).
- Conteneurs (EVP) en comparaison européenne : code Eurostat non confirmé
  par accès direct (§ 4).
- Emprises géographiques portuaires : shapefiles identifiés, contenu non
  vérifié (§ 4).

## Versions

- **Version 1** (19 septembre 2026) : premier chargement — trafic des
  quatre grands ports français depuis 2000 (SDES), comparaison avec
  Rotterdam, Anvers et Hambourg (Eurostat), rupture de périmètre HAROPA
  entre les deux sources explicitée.
