# La commande publique : qui achète, à qui, pour combien

> Note de synthèse. Version 1 — 15 septembre 2026.
> Les Données essentielles de la commande publique (DECP) existaient comme
> table vide (`core.public_contract`) depuis un chargement plus ancien —
> plusieurs dossiers (Éducation, Défense, Santé) la citaient déjà comme
> réponse à « qui sont les fournisseurs », sans qu'aucun connecteur ne
> l'alimente. C'est fait : 2,1 millions de marchés, 2018-2026.

---

## 1. Ce que ces données sont, et ne sont pas

Les DECP consolidées (projet indépendant *decp-processing*, qui agrège et
nettoie les publications éparses de milliers d'acheteurs publics) recensent
les marchés publics **au moment de leur attribution** : qui a acheté, à qui,
pour quel objet, pour quel montant. Ce ne sont pas des dépenses exécutées —
un marché attribué peut être partiellement exécuté, résilié, ou voir son
montant réel s'écarter du montant notifié par avenant.

**Format Parquet plutôt que CSV** : le fichier consolidé pèse 235 Mo en
Parquet contre 2,5 Go en CSV pour le même contenu — le gain vient de la
lecture par colonne (`internal/decp/decp.go` ne lit que 15 des 60 colonnes
disponibles) et de la compression columnar. C'est la même donnée que celle
que `internal/numerique` lit par ailleurs en CSV pour ses propres besoins
(marchés informatiques, `core.marche_numerique`) — deux tables différentes,
un seul fichier source, deux découpages pour deux questions différentes.

## 2. Une ligne par (marché, titulaire) sur l'état actuel

**2 104 142 lignes**, une par combinaison marché × titulaire retenue à son
état le plus récent (`donneesActuelles=true` dans la source — les versions
antérieures à un avenant ne sont pas conservées). Un même marché peut avoir
plusieurs titulaires : jusqu'à **87 co-titulaires observés** sur un seul
marché dans les données. `source_uid` combine le marché, le titulaire et le
numéro de modification pour rester unique dans ce cas.

**Un doublon exact existe dans la source elle-même**, à hauteur d'environ
0,3 % des lignes retenues (7 407 sur 2 111 549) — le producteur des DECP le
documente lui-même dans un fichier séparé de statistiques de doublons.
Ce dossier les dédoublonne à l'ingestion (`DISTINCT ON`) plutôt que de les
compter comme des marchés distincts.

## 3. La montée en charge de l'obligation de publication, pas une hausse d'activité

| Année | Marchés | Montant (Md€) |
|---|---:|---:|
| 2018 | 21 539 | 22,5 |
| 2019 | 141 838 | 91,5 |
| 2020 | 191 123 | 167,9 |
| 2021 | 261 103 | 361,5 |
| 2022 | 287 392 | 454,4 |
| 2023 | 296 159 | 387,0 |
| 2024 | 321 419 | 438,7 |
| 2025 | 339 574 | 656,3 |

**La croissance 2018→2021 ne dit rien de l'activité économique** : elle
reflète la montée en charge de l'obligation légale de publier ces données
(généralisée par étapes après 2018), pas une hausse réelle de la commande
publique. Comparer 2018 à une année récente sans cette réserve donnerait une
fausse impression de multiplication par quinze du volume des marchés
publics. 2026 (242 313 marchés, 410,5 Md€ à la date du chargement) est une
année en cours, pas un exercice complet.

## 4. Le montant : corrigé des valeurs aberrantes par la source elle-même

`core.public_contract.montant` porte `montant_rationalise`, pas le montant
brut déclaré : la source elle-même identifie certains montants comme
aberrants (`montant_anomalie` non vide — **54 437 lignes, 2,6 %**) et les
recalcule. Ce dossier utilise cette valeur déjà corrigée plutôt que le
montant brut, en suivant le jugement du producteur plutôt qu'en le refaisant
sans disposer de sa méthode complète.

**Une petite fraction de dates de notification est manifestement fausse** :
253 lignes (0,01 %) portent une date antérieure à l'an 2000, jusqu'à des
dates de l'an 1 — des erreurs de saisie par les acheteurs eux-mêmes,
propagées telles quelles par la source. `cmd/verify` s'assure que cette
fraction reste marginale (moins de 0,1 %) plutôt que de la corriger en
devinant la bonne date.

## 5. Ce que la répartition par nature dit, et ne dit pas

| Nature | Marchés | Montant (Md€) |
|---|---:|---:|
| Services | 792 464 | 1 213,7 |
| Travaux | 818 418 | 821,6 |
| Fournitures | 488 681 | 954,0 |
| Non catégorisé | 4 579 | 2,5 |

Les travaux sont les plus nombreux, les services pèsent le plus lourd en
montant — deux classements différents selon qu'on compte les marchés ou
l'argent, à ne jamais confondre l'un pour l'autre.

## 6. Le pont vers les dossiers déjà chargés

`acheteur_siret` permet de joindre ce jeu à toute entité publique déjà
identifiée par son SIRET dans ce dépôt — notamment
`ref.finess_etablissement.siret` pour les fournisseurs des établissements de
santé (`docs/sante-donnees.md` § 3). Cette jointure n'est pas encore
écrite : ce dossier documente la clé, pas encore le résultat.

## 7. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | DECP consolidées (decp-processing), Parquet | `core.public_contract` | 2 104 142 lignes, 2018-2026 |

**Non chargé, et pourquoi** :
- **Le rapprochement avec les établissements de santé, l'Éducation nationale
  ou la Défense** (§ 6) : la clé existe, la jointure n'est pas écrite.
- **Les colonnes non retenues** (considérations sociales/environnementales,
  procédure, techniques d'achat, origine UE/France des fournitures…) :
  disponibles dans le fichier source, non chargées faute d'une question
  précise à laquelle elles répondraient aujourd'hui.

## Sources

- DECP consolidées, projet *decp-processing* (Colin Maudry), data.gouv.fr.
- [docs/perimetre.md](perimetre.md) § 4.4, pour le contexte de cette table
  avant son chargement.
- [docs/sante-donnees.md](sante-donnees.md) § 3, pour un premier usage
  envisagé (fournisseurs des établissements de santé).
