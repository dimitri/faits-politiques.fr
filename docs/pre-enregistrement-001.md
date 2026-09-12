# Pré-enregistrement 001 — Action municipale par étiquette politique, mandat 2020-2026

> **Protocole scellé.** Ce document fixe la population, les indicateurs, la méthode et
> les règles de publication **avant tout calcul**. Il est horodaté et son empreinte est
> publiée. Toute modification ultérieure apparaît comme un amendement daté, jamais comme
> une réécriture.
>
> | | |
> |---|---|
> | Version | 1 |
> | Rédigé le | 11 septembre 2026 |
> | Statut | **BROUILLON — non scellé** |
> | Scellé le | — |
> | Empreinte SHA-256 | — |
> | Premier calcul autorisé | après scellement uniquement |

---

## 1. Pourquoi ce document existe

Sans pré-enregistrement, tout résultat sera attribué au choix des indicateurs, et
l'accusation sera irréfutable — y compris si elle est fausse. En figeant la liste avant
de connaître les résultats, un écart observé ne peut plus être expliqué par la sélection.

C'est la seule raison d'être de ce document. Il ne rend pas les résultats vrais ; il rend
la méthode opposable.

## 2. Ce qui est testé

**Question.** Les communes dirigées par un maire d'une étiquette donnée présentent-elles,
sur un mandat complet, des évolutions différentes de celles de communes comparables
dirigées par d'autres étiquettes, sur les domaines listés en §5 ?

**Formulation opérationnelle et symétrique.** Pour chaque nuance politique disposant d'un
effectif suffisant, et pour chaque indicateur retenu, on estime l'écart médian apparié
par rapport à l'ensemble des autres communes appariées.

Il n'y a **pas d'hypothèse privilégiée**. Aucune nuance n'occupe une position particulière
dans le protocole. Le même calcul, les mêmes indicateurs et la même page sont produits
pour toutes les nuances éligibles.

## 3. Ce que ce protocole ne teste pas et ne conclura pas

- **Aucune causalité.** Un écart observé n'établit pas qu'il résulte de la politique
  municipale. Les communes ne sont pas attribuées au hasard : elles choisissent leur
  maire, et ce choix est corrélé à tout le reste.
- **Aucune extrapolation nationale.** Ce que fait une municipalité ne dit rien de ce que
  ferait un gouvernement.
- **Aucun jugement de valeur** sur la direction d'un écart. Une baisse de dépenses est
  un fait ; la qualifier d'économie ou de coupe est une interprétation, qui n'a pas sa
  place dans le résultat publié.
- **Aucune prospective.**

## 4. Population

### 4.1 Critère d'inclusion principal

Communes pour lesquelles, de façon **continue** du 1er septembre 2020 au 1er mars 2026,
le maire en fonction porte la même nuance politique au Répertoire national des élus.

L'étiquette retenue est le **code nuance du RNE**, avec le millésime de circulaire, et
rien d'autre. Le libellé affiché est celui de la nomenclature officielle. Aucun
regroupement en « familles politiques » n'est effectué : les regroupements sont le
principal levier de manipulation involontaire d'un résultat de ce type.

### 4.2 Exclusions, à déclarer et à chiffrer

| Exclusion | Raison |
|---|---|
| Nuance absente, « divers » ou sans étiquette | Aucune attribution défendable |
| Commune sous le seuil d'attribution de nuance | La nuance n'est pas renseignée |
| Changement de maire ou de nuance en cours de mandat | Attribution ambiguë |
| Commune issue d'une fusion ou d'une scission sur la période | Série temporelle non continue |
| Effectif de la nuance < 5 communes après appariement | Aucun résultat ne sera publié pour cette nuance |

Le taux d'exclusion est publié avec le résultat, décomposé par motif.

### 4.3 Communes élues en mars 2026 : exclues

Les communes dont la majorité a changé lors du renouvellement de mars 2026 sont
**exclues de toute analyse financière** jusqu'à la disponibilité des comptes 2027,
attendue au plus tôt en 2028.

Motif : le budget 2026 a été très majoritairement voté par l'équipe précédente. Leur
attribuer les chiffres 2026 serait une erreur factuelle. Ce point vaut pour toutes les
nuances sans exception.

### 4.4 Puissance statistique — limite majeure, déclarée d'avance

Plusieurs nuances ne compteront qu'une dizaine de communes éligibles. **À cet effectif,
seuls des écarts très importants sont détectables.** Un résultat non significatif ne
signifiera donc pas « pas de différence », mais « aucune différence détectable à cet
effectif ». Cette phrase figurera littéralement dans toute publication concernée.

Conséquence assumée sur la nature du livrable : le produit principal est un **ensemble
de monographies communales** documentées et sourcées. La comparaison inter-nuances est
un produit secondaire, dont l'issue la plus probable est l'absence de conclusion.

## 5. Indicateurs — liste figée

Sens de lecture : aucun. Les indicateurs sont descriptifs, pas orientés.

### 5.1 Contexte et variables d'appariement

| Code | Définition | Source |
|---|---|---|
| `insee.population_municipale` | Population municipale légale | INSEE COG |
| `insee.revenu_median_uc` | Revenu disponible médian par unité de consommation | INSEE Filosofi |
| `ofgl.strate` | Strate démographique | OFGL |
| `banatic.competences` | Compétences transférées à l'EPCI | BANATIC |

### 5.2 Solidarité

| Code | Formule | Source | Réserve |
|---|---|---|---|
| `dgfip.f5_depenses_par_hab` | dépenses fonction 5 / population | OFGL–DGFiP | Ventilation fonctionnelle non obligatoire sous 3 500 hab |
| `dgfip.subvention_ccas_par_hab` | subvention versée au CCAS / population | OFGL–DGFiP | **Le CCAS est une entité distincte** : sa dépense propre n'est pas dans ce chiffre |
| `dgfip.f251_depenses_par_hab` | dépenses sous-fonction 251 (restauration scolaire) / population | OFGL–DGFiP | Sous réserve de disponibilité |
| `manuel.tarif_cantine_qf_min` | Tarif le plus bas de la grille cantine | Délibération tarifaire | **Collecte manuelle.** Aucune source nationale |
| `manuel.tarif_cantine_qf_max` | Tarif le plus élevé de la grille | Délibération tarifaire | Idem |
| `manuel.quotient_familial_applique` | Existence d'une tarification au quotient | Délibération tarifaire | Idem |

### 5.3 Culture

| Code | Formule | Source | Réserve |
|---|---|---|---|
| `dgfip.f3_depenses_par_hab` | dépenses fonction 3 / population | OFGL–DGFiP | Périmètre variable selon transferts à l'EPCI |
| `olp.biblio_acquisition_par_hab` | budget d'acquisition / population | Observatoire de la lecture publique | Enquête déclarative, non-réponse possible |
| `olp.biblio_amplitude_hebdo` | heures d'ouverture hebdomadaires | OLP | Idem |
| `olp.biblio_etp_pour_10k_hab` | ETP × 10 000 / population | OLP | Idem |
| `olp.biblio_inscrits_pct` | inscrits actifs / population | OLP | Idem |

### 5.4 Sécurité

| Code | Formule | Source | Réserve |
|---|---|---|---|
| `dgfip.f1_depenses_par_hab` | dépenses fonction 1 / population | OFGL–DGFiP | Inclut salubrité, pas seulement sécurité |
| `decp.videoprotection_par_hab` | montants notifiés sur codes CPV de vidéoprotection / population | DECP consolidées | Marchés pluriannuels : lisser ou dater à la notification, choix figé ici sur la **date de notification** |
| `ssmsi.*_pour_1000_hab` | faits enregistrés × 1 000 / population | SSMSI | **Faits enregistrés, pas faits commis. Un renforcement de la police municipale augmente mécaniquement les constatations.** Publié en série, jamais imputé |

### 5.5 Finances générales

`ofgl.dette_par_hab`, `ofgl.investissement_par_hab`, `ofgl.fonctionnement_par_hab`,
`ofgl.masse_salariale_par_hab`, `ofgl.epargne_brute_par_hab`, `dgfip.taux_tfb`,
`dgfip.taux_tfnb`.

### 5.6 Gouvernance

| Code | Définition | Source | Réserve |
|---|---|---|---|
| `crc.observations_par_theme` | Observations des rapports définitifs des chambres régionales des comptes, comptées par thème | CRC, open data | Périodicité irrégulière : une commune non contrôlée n'est pas une commune sans irrégularité. **L'absence de rapport est publiée comme absence de données, jamais comme résultat favorable** |

### 5.7 Verrouillage

Cette liste est **close au scellement**. Ajouter un indicateur après avoir vu un résultat
invalide le protocole. Un ajout ultérieur constitue un pré-enregistrement distinct, avec
sa propre population et ses propres résultats, publié séparément.

## 6. Méthode

1. **Appariement** sur strate démographique × région × quartile de revenu médian.
   Une commune sans contrepartie dans sa cellule est exclue et comptée comme telle.
2. **Grandeur estimée** : écart médian apparié entre la nuance et l'ensemble des autres
   communes de la même cellule, sur l'évolution 2020 → 2025 de l'indicateur.
3. **Incertitude** : intervalle de confiance par bootstrap sur les cellules
   d'appariement. Aucun résultat n'est publié sans son intervalle.
4. **Données manquantes** : jamais imputées. Une commune sans valeur est exclue du calcul
   de cet indicateur et comptée dans le dénominateur de couverture.
5. **Compétences EPCI** : un indicateur dont la compétence a été transférée n'est pas
   calculé pour cette commune ; la raison `EPCI_COMPETENCE` est affichée.
6. **Fusions et scissions** : toute commune concernée est exclue de la série.

## 7. Règles de publication

1. **Tous les résultats sont publiés**, y compris ceux contraires à l'attente de leurs
   auteurs, et dans le même format.
2. **Aucun indicateur n'est retiré après coup.** Un indicateur qui se révèle
   inexploitable est publié avec le motif de son inexploitabilité.
3. **Symétrie stricte** : les pages produites pour chaque nuance éligible ont la même
   structure, les mêmes indicateurs, les mêmes réserves et la même mise en avant. Une
   asymétrie est un défaut à corriger, et elle est mesurée par la vue de couverture.
4. **Aucune conclusion générale n'est écrite dans le résultat.** Le résultat est un
   tableau, ses sources, ses réserves et son intervalle de confiance.
5. **Les regroupements thématiques sont des filtres** (schéma `selection`), sans une
   ligne de commentaire, et chacun affiche la sélectivité que le système lui calcule.
   Ils ne peuvent jamais modifier une donnée.

## 8. Ce qui invaliderait ce protocole

- Un effectif final inférieur à 5 communes pour une nuance → aucun résultat publié pour
  cette nuance.
- Une couverture d'indicateur inférieure à 60 % de la population retenue → indicateur
  publié comme non exploitable.
- La découverte que la ventilation fonctionnelle n'est pas disponible de façon homogène
  → les indicateurs `dgfip.f*` sont retirés **en bloc**, pour toutes les nuances, et le
  retrait est publié.

## 9. Amendements

Tout amendement est ajouté ici, daté, motivé, avec l'empreinte de la version précédente.
Un amendement postérieur au premier calcul doit indiquer explicitement quels résultats
étaient déjà connus au moment de sa rédaction.

| Date | Amendement | Motif | Résultats déjà connus |
|---|---|---|---|
| — | — | — | — |
