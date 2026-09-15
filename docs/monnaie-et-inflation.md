# Comparer des montants dans le temps : francs, euros, et inflation

> **Méthode** · version 1 · 13 septembre 2026
>
> Elle s'applique à **toute valeur monétaire du projet**, pas seulement au budget :
> comptes des partis, comptes de campagne, budgets communaux, marchés publics,
> déclarations HATVP, séries macroéconomiques.
>
> Elle répond à trois questions : faut-il convertir les francs, faut-il corriger de
> l'inflation, et que doit porter le schéma pour que la réponse soit vérifiable.

---

## 1. Le problème, sur une série réellement chargée

`core.protection_sociale` couvre **1959 à 2024**. Prestations de protection sociale,
tous régimes :

| | montant |
|---|---|
| 1959 | **6 243 M€** |
| 2024 | **932 548 M€** |

Un facteur **149** en soixante-cinq ans. Tracée telle quelle, cette courbe ne montre pas
la protection sociale : elle montre l'inflation, et écrase les quarante premières années
contre l'axe. Le problème n'est pas cosmétique — un lecteur qui compare 1959 à 2024 sur
ce graphique conclut faux.

Deux précisions utiles avant d'aller plus loin :

- **Il n'y a pas un franc dans la base.** Toutes les séries monétaires viennent
  d'Eurostat ou de la DREES, qui publient en euros et ont déjà converti les millésimes
  antérieurs à 1999 à la parité fixe. La série la plus ancienne, `ENTREPRISES`, part de
  1971 — en euros.
- **La conversion a donc déjà eu lieu, en amont, sans qu'on la voie.** Le « 6 243 M€ »
  de 1959 est un montant en anciens francs, divisé par 100 puis par 6,55957 par la
  DREES. Ce n'est pas faux, mais ce n'est pas non plus ce que quiconque a lu en 1959.

## 2. Deux opérations à ne jamais confondre

### 2.1 La conversion est de l'arithmétique légale

Le règlement **CE 1103/97** fixe les règles, le règlement **CE 2866/98** fixe la parité :
**1 € = 6,55957 F**. Trois contraintes, qui ne sont pas des conventions internes mais du
droit :

- le taux comporte **six chiffres significatifs** et ne peut être ni arrondi ni tronqué ;
- **l'usage du taux inverse est interdit** — on divise par 6,55957, on ne multiplie
  jamais par 0,152449 ;
- l'arrondi se fait **au centime le plus proche**, après conversion.

S'y ajoute, pour toute série remontant avant 1960, le nouveau franc :
**1 F = 100 anciens francs**. `core.protection_sociale` commence en 1959 : elle traverse
les deux changements.

Une conversion n'introduit aucune incertitude. Elle est exacte, reproductible, et ne se
discute pas.

### 2.2 La déflation est un choix de méthode

Corriger de l'inflation suppose de choisir un indice, une population de référence et une
année de base. Ce sont trois décisions, et les institutions ne prennent pas les mêmes.

## 3. Ce que font les institutions

**Eurostat n'offre pas de prix constants sur ces séries.** Vérifié le 13 septembre 2026 :
`gov_10a_main` n'expose que `MIO_EUR`, `MIO_NAC` et **`PC_GDP`** ; `nasa_10_nf_tr` — la
série de 1971 déjà chargée — n'expose que `CP_MEUR`, prix courants. **Sa réponse à la
comparabilité dans le temps en finances publiques est le rapport au PIB**, pas la
déflation.

**Bercy utilise l'IPC hors tabac** pour mesurer la croissance en volume de la dépense
publique. Le motif mérite d'être retenu : cet indice est bien connu et **n'est pas révisé
après publication**, contrairement au déflateur du PIB, qui subit d'importantes
corrections deux ou trois ans après l'exercice concerné. Pour un site qui republie ses
chiffres en continu, une série révisée rétroactivement est un piège.

**L'INSEE convertit depuis 1901, avec trois indices successifs** :

| période | indice retenu |
|---|---|
| 1901-1992 | ménages urbains dont le chef est ouvrier ou employé |
| 1993-1998 | ensemble des ménages, France métropolitaine |
| depuis 1999 | ensemble des ménages, métropole et DROM |

Et l'INSEE assortit son propre convertisseur de cette réserve :

> « La valeur fournie par ce convertisseur est indicative et ne peut servir de référence
> officielle dans un cadre juridique. »

**C'est la phrase la plus importante de cette note.** L'institut qui produit l'indice ne
présente pas une somme déflatée comme un fait officiel. Ce projet ne peut pas être plus
affirmatif que sa source.

## 4. Pourquoi la déflation ne peut pas être présentée comme un fait

L'indice des prix fait l'objet de critiques documentées — le logement occupé par son
propriétaire en est exclu, l'ajustement de la qualité repose sur des conventions. Ce
projet n'a pas à trancher ce débat, et cette note ne le tranche pas.

Mais il doit en tenir compte dans la présentation, **parce qu'un biais compose** :

| écart de série | +0,1 pt/an | +0,2 pt/an | +0,5 pt/an |
|---|---|---|---|
| 1990 → 2025 (35 ans) | 3,6 % | 7,2 % | 19,1 % |
| **1971 → 2025 (54 ans)** | 5,5 % | **11,4 %** | 30,9 % |
| **1959 → 2025 (66 ans)** | 6,8 % | **14,1 %** | 39,0 % |

Sur `core.protection_sociale`, un biais de deux dixièmes de point par an déplace la
valeur corrigée de 14 %. Le biais n'a pas besoin d'être grand, il a besoin d'être
constant : c'est la composition qui agit, pas l'ampleur.

Conséquence directe : **une valeur déflatée est une lecture, pas un fait.** Elle a sa
place sur un graphique, jamais dans une phrase du type « X a dépensé Y ».

## 5. Ce que le schéma porte aujourd'hui, et ce qui manque

État au 13 septembre 2026 : **vingt colonnes monétaires** réparties dans quinze tables
de `core`, sous **six conventions de nommage** — `montant`, `montant_eur`, `valeur`,
`valeur_meur`, `euros_par_hab`, `depenses_meur`.

**Aucune ne dit en euros de quelle année.** `montant_eur` nomme la devise mais pas le
millésime ; `valeur_meur` nomme l'unité mais pas le régime de prix. Une valeur de 1959
et une valeur de 2024 sont stockées de façon strictement indiscernable, alors qu'elles
ne sont pas comparables.

Deuxième anomalie, trouvée en inventoriant : **quatorze colonnes sont en `numeric`, six
en `double precision`** — `execution_etat.montant_eur`, `exoneration_cotisation.montant_eur`,
`protection_sociale.valeur_meur`, et les trois colonnes de `solde_vote`. Ce sont les plus
récemment ajoutées. De l'argent en virgule flottante ne s'additionne pas exactement ;
`numeric` est le type juste, et la correction est mécanique.

**Ce qu'il faut ajouter**, sur chaque table portant un montant :

- `devise` — `EUR` par défaut, mais explicite : le jour où une source publie des francs,
  la colonne existe déjà et rien ne se mélange en silence ;
- `regime_prix` — `COURANT` ou `CONSTANT` ;
- `annee_prix` — l'année de référence, **NULL si `COURANT`**, obligatoire sinon.

Une contrainte suffit à rendre la faute impossible :

```sql
CHECK ((regime_prix = 'COURANT' AND annee_prix IS NULL)
    OR (regime_prix = 'CONSTANT' AND annee_prix IS NOT NULL))
```

## 6. Les règles

1. **`core` ne contient que des euros courants**, tels que la source les publie. Aucune
   valeur déflatée n'entre dans `core` : ce serait transcrire un calcul comme un fait.
2. **La déflation vit dans `derived`**, avec sa `method_version`, au même titre que
   `derived.pdr_corps_electoral`. Elle se recalcule, se date et se conteste.
3. **Le déflateur est une source de plein droit**, chargée comme les autres, avec ses
   ruptures documentées. L'API BDM de l'INSEE répond :
   `https://bdm.insee.fr/series/sdmx/data/SERIES_BDM/001763852` — 432 observations
   mensuelles de 1990-01 à 2025-12, base 2015. **Piège : c'est une « série arrêtée ».**
   L'INSEE rebase périodiquement ; une série longue est un **chaînage** de plusieurs
   bases, pas une lecture directe. Le chaînage est lui-même un calcul, donc `derived`.
4. **La valeur nominale reste toujours atteignable** depuis toute page qui affiche une
   valeur corrigée. Sans cela, la correction n'est pas vérifiable.
5. **Le déflateur retenu s'écrit sur le graphique**, pas dans une note de bas de page :
   « euros de 2025, déflatés par l'IPC hors tabac (INSEE, base 2015 chaînée) ». Le
   lecteur qui conteste l'indice sait alors exactement quoi contester — ce qui est la
   posture du § 2 du périmètre, pas une concession.

## 7. Quel affichage par défaut

Trois représentations sont légitimes, et le défaut dépend de la nature de la série.

| Représentation | Défaut pour | Motif |
|---|---|---|
| **% du PIB** | finances publiques : dépenses, recettes, solde, dette | convention d'Eurostat et de la Cour des comptes ; **esquive à la fois la conversion et la déflation** ; comparable entre pays |
| **Euros courants**, millésime affiché | montants individuels : un compte de campagne, un marché public, une déclaration | c'est le montant qui a été déclaré ; le déflater le rendrait introuvable dans la source |
| **Euros constants** d'une année nommée | séries longues sans dénominateur naturel : prestations sociales par risque, dividendes versés | seule façon de rendre lisible un facteur 149 |

**Le défaut recommandé pour les finances publiques est le rapport au PIB.** Ce n'est pas
un choix de confort : c'est la seule des trois qui ne repose sur aucune hypothèse
contestable. Les euros constants restent proposés, en option explicite, avec leur
déflateur nommé.

## 8. Si une source publie un jour des francs

Le cas n'existe pas encore mais viendra si le projet remonte avant 1999 sur une source
nationale plutôt qu'européenne. La règle est alors :

- stocker la valeur **telle que publiée**, avec `devise = 'FRF'` ;
- ne jamais convertir à l'ingestion — la conversion est une lecture, elle appartient à
  `derived` comme la déflation ;
- appliquer les trois contraintes du § 2.1, division par 6,55957, jamais de taux inverse ;
- pour un millésime antérieur à 1960, diviser d'abord par 100.

## 9. Ce qui reste à faire

1. Ajouter `devise`, `regime_prix` et `annee_prix` aux vingt colonnes monétaires, avec la
   contrainte du § 5. Migration mécanique, valeurs par défaut `EUR` / `COURANT` / `NULL`.
2. Corriger les six colonnes en `double precision` vers `numeric`.
3. Charger l'IPC comme série, et écrire le chaînage des bases dans `derived`.
4. **Trancher éditorialement** ce que le § 7 recommande : si le rapport au PIB devient le
   défaut des pages de finances publiques, cela doit figurer au journal des décisions.

Texte proposé pour ce journal, à déposer sous le prochain numéro libre :

> **D-0xx — Une valeur déflatée est une lecture, pas un fait**
> `core` ne stocke que des euros courants, tels que publiés. La conversion de devise et
> la correction d'inflation sont des calculs : ils vivent dans `derived`, portent une
> `method_version`, et la valeur nominale reste toujours atteignable depuis la page qui
> affiche la valeur corrigée. Motif : l'INSEE lui-même qualifie son convertisseur
> d'indicatif et sans valeur de référence officielle ; un biais de 0,2 point par an sur
> le déflateur déplace de 14 % une valeur de 1959. Le défaut d'affichage des finances
> publiques est le rapport au PIB, seule représentation qui ne repose sur aucune
> hypothèse contestable.
