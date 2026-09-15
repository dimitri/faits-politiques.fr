# La Sécurité sociale : le budget le plus lourd, en trois notes reliées

> **Dossier** · version 2 · 15 septembre 2026
>
> Le budget social — retraites, chômage, maladie, famille — pèse plus que celui de
> l'État et se lit moins facilement : quatre périmètres portent des chiffres différents sous le
> même nom, sans compte consolidé unique. Le dossier pose ce périmètre commun, puis renvoie aux
> dossiers qui détaillent chaque volet.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| sécurité sociale | 2413 | 314 | 1er octobre 2024 | 20 juillet 2026 |

Une mention ne dit pas la position de l'orateur.

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Des finances publiques « très dégradées »** (10 décembre 2025). La commission ouvre son rapport sur le budget de la Sécurité sociale 2026 par la situation des finances publiques, qu'elle qualifie de très dégradée. — Sénat, commission des affaires sociales (rapport sur le PLFSS 2026) · [source](https://www.senat.fr/lessentiel/plfss2026.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **La loi de financement de la sécurité sociale pour 2024** (26 décembre 2023). Exemple de loi annuelle qui fixe les objectifs de dépenses et les prévisions de recettes des branches de la Sécurité sociale. — Parlement (loi n° 2023-1250) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000048668665) · *officiel*

<!-- faits:CADRE:fin -->

### 1. Quatre périmètres, un seul mot

La distinction que toute lecture d'un chiffre « sécu » doit garder en tête :

| Périmètre | Ce qu'il couvre |
|---|---|
| **Régime général** | Le socle : maladie, famille, une partie des retraites, une partie des accidents du travail. |
| **Régime général + FSV** | Ajoute le Fonds de solidarité vieillesse, qui finance des droits non contributifs (minimum vieillesse, validation de trimestres au chômage). |
| **Tous régimes obligatoires de base** | Ajoute les régimes spéciaux et indépendants — le périmètre que vote la LFSS chaque année. |
| **Protection sociale au sens DREES/Eurostat** | Le plus large : ajoute l'assurance chômage et les retraites complémentaires (AGIRC-ARRCO), que la LFSS **ne couvre pas**. C'est le périmètre le plus souvent cité dans le débat public (« la Sécu coûte X Md€ »), et le plus souvent confondu avec le périmètre LFSS, plus étroit.

Cette série porte cette distinction comme clé étrangère obligatoire sur toute valeur
chargée en base — aucun chiffre du site ne peut être publié sans dire à quel périmètre
il appartient.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Budget de la Sécurité sociale 2026 : des mesures de réduction du déficit ramenées à 9 Md€** (10 décembre 2025). Selon la commission, les mesures de réduction du déficit, de 15 Md€ dans le texte initial, n'étaient plus que de 9 Md€ dans le texte adopté. — Sénat, commission des affaires sociales (rapport sur le PLFSS 2026) · [source](https://www.senat.fr/lessentiel/plfss2026.pdf) · *officiel*

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 3. Trois volets, trois notes

- **[docs/cotisations-et-droits.md](cotisations-et-droits.md)** — la question
  de fond : qu'est-ce qu'une cotisation garantit ? Deux distinctions
  indépendantes (financement en répartition ou capitalisation ; droit
  contributif ou non), branche par branche, puis ce que chaque nature de
  droit pèse dans la dépense totale.
- **[docs/chomage-donnees.md](chomage-donnees.md)** — le taux de chômage
  depuis 1975, RMI puis RSA (trente-cinq ans d'un même filet sous deux noms),
  la prime d'activité qui a remplacé le RSA activité en 2016, la dépense
  ESSPROS et son financement (qui paie, comment il est redistribué).
- **[docs/retraite-donnees.md](retraite-donnees.md)** — la dépense (la plus
  grosse fonction de la protection sociale), la distribution réelle des
  pensions, l'âge de départ réforme par réforme, le minimum vieillesse, le
  taux de remplacement par quantile, et le ratio cotisants/retraités.

Chacune se lit seule ; ensemble, elles couvrent la chaîne complète — combien
ça coûte, qui est protégé par quoi, d'où vient l'argent — sans qu'aucune ne
tente de tout dire à la fois.

## Ce que les données ne disent pas

### 2. Ce que la Sécurité sociale ne publie pas

Une recherche « comptes de la sécurité sociale » sur data.gouv.fr renvoie
**zéro jeu de données**. Le seul jeu rattaché à la loi de financement, les
REPSS, est **gelé depuis janvier 2022**. Le budget le plus lourd des deux
budgets publics est le moins documenté en données ouvertes — c'est la
contrainte qui façonne tout ce qui suit : les trois notes ci-dessous
s'appuient sur des séries DREES, Eurostat et Unédic publiées séparément,
faute d'un compte consolidé unique à interroger.

### 4. Ce qui reste hors de portée, à ce périmètre

- **Un compte consolidé de la Sécurité sociale** : n'existe pas en open data
  (§ 2) — chaque note ci-dessus recompose son propre périmètre à partir de
  séries partielles, jamais d'un bilan unique.
- **AGIRC-ARRCO** (retraite complémentaire) : aucun portail d'open data
  identifié, malgré une recherche dédiée — voir
  [docs/retraite-donnees.md](retraite-donnees.md) § 7.

## Sources

Cette note n'introduit aucune donnée nouvelle — elle relie des sources déjà
citées dans les trois notes ci-dessus. Voir leurs sections « Sources »
respectives.

## Versions

- **Version 2** (15 septembre 2026) : plan commun des dossiers ; cadre (loi de financement 2024) et contrôle du Sénat sur le budget 2026.
- **Version 1** (14 septembre 2026) : périmètres et renvois.
