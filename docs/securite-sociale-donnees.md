# La Sécurité sociale : le budget le plus lourd, en trois notes reliées

> Note de synthèse. Version 1 — 14 septembre 2026.
> Le budget social — retraites, chômage, maladie, famille — pèse plus lourd que
> le budget de l'État (803,5 Md€ de dépenses en 2025 contre 680,8 Md€ pour
> l'administration centrale) et se lit moins facilement : quatre périmètres
> différents portent des chiffres différents sous le même nom
> (`docs/decisions.md` D-048), et la Sécurité sociale, contrairement à l'État,
> ne publie pas de compte consolidé unique. Cette note ne refait pas le travail
> déjà fait ailleurs — elle pose le périmètre commun, puis renvoie vers les
> trois notes qui détaillent chaque volet.

---

## 1. Quatre périmètres, un seul mot

`docs/decisions.md` D-048 pose la distinction que toute lecture d'un chiffre
« sécu » doit garder en tête :

| Périmètre | Ce qu'il couvre |
|---|---|
| **Régime général** | Le socle : maladie, famille, une partie des retraites, une partie des accidents du travail. |
| **Régime général + FSV** | Ajoute le Fonds de solidarité vieillesse, qui finance des droits non contributifs (minimum vieillesse, validation de trimestres au chômage). |
| **Tous régimes obligatoires de base** | Ajoute les régimes spéciaux et indépendants — le périmètre que vote la LFSS chaque année. |
| **Protection sociale au sens DREES/Eurostat** | Le plus large : ajoute l'assurance chômage et les retraites complémentaires (AGIRC-ARRCO), que la LFSS **ne couvre pas**. C'est le périmètre le plus souvent cité dans le débat public (« la Sécu coûte X Md€ »), et le plus souvent confondu avec le périmètre LFSS, plus étroit.

`ref.budget_perimetre` porte cette distinction comme clé étrangère obligatoire
sur toute valeur chargée en base — aucun chiffre du site ne peut être publié
sans dire à quel périmètre il appartient.

## 2. Ce que la Sécurité sociale ne publie pas

Une recherche « comptes de la sécurité sociale » sur data.gouv.fr renvoie
**zéro jeu de données**. Le seul jeu rattaché à la loi de financement, les
REPSS, est **gelé depuis janvier 2022**. Le budget le plus lourd des deux
budgets publics est le moins documenté en données ouvertes — c'est la
contrainte qui façonne tout ce qui suit : les trois notes ci-dessous
s'appuient sur des séries DREES, Eurostat et Unédic publiées séparément,
faute d'un compte consolidé unique à interroger.

## 3. Trois volets, trois notes

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

## 4. Ce qui reste hors de portée, à ce périmètre

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
