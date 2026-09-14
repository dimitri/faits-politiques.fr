# Écologie et climat : cinq notions que le débat public confond

> Note de synthèse. Version 1 — 14 septembre 2026.
> Budget vert, dépenses environnementales, investissements climat, fiscalité
> écologique, résultats physiques (CO2, énergie, biodiversité) : cinq mesures
> différentes, souvent citées l'une pour l'autre. Cette note pose la
> distinction avant les chiffres — chacun répond à une question différente,
> et aucun ne se déduit d'un autre.

---

## 1. Le budget de la mission Écologie : neuf programmes, deux logiques différentes

`core.budget_programme` (même table que
[docs/securite-police-donnees.md](securite-police-donnees.md),
[docs/defense-donnees.md](defense-donnees.md) et
[docs/education-donnees.md](education-donnees.md) — un seul chargement PLF
couvre déjà cette mission) : la mission **Écologie, développement et
mobilité durables** totalise **21,63 Md€ en 2024** et **20,50 Md€ en 2025**
en crédits de paiement, PLF (montants votés au projet, pas exécutés — voir
[docs/budget-donnees.md](budget-donnees.md) § 2).

**Ce total mélange des politiques sans rapport entre elles** — transports,
énergie, météorologie, biodiversité — sous un même intitulé budgétaire.
Le seul programme directement consacré à l'eau et à la biodiversité,
**« Paysages, eau et biodiversité »**, ne pèse que **0,51 Md€ en 2024**
(0,45 Md€ en 2025) — 2,3 % du total de la mission. Présenter le total de
21,63 Md€ comme « le budget de l'eau et de la biodiversité » serait faux par
un facteur supérieur à 40.

**Une chute qui n'est pas une anomalie de chargement** : le programme
« Énergie, climat et après-mines » passe de 4,89 Md€ (2024) à 2,11 Md€
(2025), presque entièrement porté par le titre 6 (intervention : 4,69 → 1,87
Md€) — cohérent avec le reflux des dispositifs exceptionnels de soutien au
prix de l'énergie mis en place pendant la crise énergétique de 2022-2023,
sans que cette note en établisse la cause précise faute d'avoir chargé le
détail des dispositifs eux-mêmes.

## 2. Le budget vert : une cotation, pas une dépense

Déjà chargé et documenté en détail dans
[docs/dette-donnees.md](dette-donnees.md) (`core.depense_fiscale`) : le
budget vert **cote** les dépenses fiscales existantes selon leur impact
environnemental, il ne mesure pas une dépense propre. Cette note ne répète
pas ce travail — voir la note citée pour ses pièges de lecture (révisions
fortes d'un PLF à l'autre, lignes à dédoublonner par mesure et non par
cotation).

## 3. La fiscalité écologique : un impôt étroit, à ne pas confondre avec « toute taxe verte »

`core.recette_fiscale`, poste **D29F « Impôts sur les émissions
polluantes »** (nomenclature SEC2010, comptabilité nationale, déjà chargé) :

| Année | Montant (M€) |
|---|---:|
| 2020 | 1 121 |
| 2022 | 1 985 |
| 2024 | 2 433 |

**Ce poste ne couvre pas la TICPE** (taxe intérieure de consommation sur les
produits énergétiques, plusieurs dizaines de milliards d'euros par an), qui
est classée ailleurs dans la nomenclature comme un droit d'accise
(poste D214A) — une décision de comptabilité nationale, pas un choix éditorial
de ce projet. **« La fiscalité écologique » citée dans le débat public
regroupe généralement plusieurs postes disjoints** (TICPE, TGAP, malus
automobile, redevances des agences de l'eau — § 5) que la nomenclature de
comptabilité nationale ne réunit pas sous un poste unique. Cette note ne
recompose pas cet agrégat, faute d'une définition officielle stable à
appliquer.

## 4. Ce qui manque encore : dépenses environnementales, investissements climat, résultats physiques

**Non chargé, identifié, avec la raison précise :**

- **Dépenses de protection de l'environnement au sens large** (nomenclature
  européenne CEPA/CReMA, l'agrégat le plus souvent cité autour de 100 Md€) :
  publiées par Eurostat, non encore chargées — un nouveau connecteur à
  ajouter à `internal/dette/eurostat.go`, qui interroge déjà Eurostat pour
  d'autres séries de ce projet.
- **Investissements climat** (au sens du financement de la transition,
  hors budget vert) : aucune source unique et stable identifiée à ce stade.
- **Résultats physiques** (émissions de CO2, consommation d'énergie,
  indicateurs de biodiversité) : sources probables SDES (Service des
  données et études statistiques) et Citepa (inventaire national des
  émissions), identifiées mais pas explorées pour leur format d'accès.

## 5. Ce que ce chargement prépare pour le dossier « pour aller plus loin »

[docs/bassins-versants-donnees.md](bassins-versants-donnees.md) traite un
sujet adjacent mais distinct : la gouvernance de l'eau et son découpage par
bassin hydrographique. Les agences de l'eau y sont décrites en détail — leurs
redevances ne figurent pas dans le poste D29F ci-dessus (ce sont des
redevances perçues par des établissements publics, pas un impôt d'État au
sens de la comptabilité nationale) : encore une distinction entre deux
prélèvements qui financent tous deux la politique de l'eau, sans se
recouper comptablement.

## 6. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Direction du budget, PLF, mission Écologie | `core.budget_programme` | 9 programmes, 2024-2025 (table partagée, § 1) |
| 2 | Eurostat/Insee, comptabilité nationale, poste D29F | `core.recette_fiscale` | déjà chargé, § 3 |
| 3 | PLF, budget vert (cotation environnementale) | `core.depense_fiscale` | déjà chargé, voir docs/dette-donnees.md |

## Sources

- Direction du budget, *PLF — dépenses par mission, programme et action*,
  data.economie.gouv.fr, éditions 2024 et 2025.
- [docs/dette-donnees.md](dette-donnees.md), pour le budget vert et ses
  pièges de lecture.
- [docs/budget-donnees.md](budget-donnees.md), pour le piège voté/exécuté.
