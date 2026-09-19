# La vieillesse : combien, qui paie, et la dépendance au-delà des retraites

> **Dossier** · version 1 · 15 septembre 2026
>
> Combien la vieillesse pèse-t-elle dans la dépense publique, qui la finance — l'État ou la
> Sécurité sociale, deux budgets que ce dépôt n'a jamais confondus — et que disent les données
> sur la dépendance, l'angle resté aveugle du dossier retraite comme du dossier santé ?

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

### 1. Méthode : ne pas confondre les budgets, ni les périmètres

Trois règles gouvernent ce dossier, déjà appliquées ailleurs dans ce dépôt
(`docs/budget-donnees.md`, `docs/cotisations-et-droits.md`) :

1. **Le budget de l'État et celui de la Sécurité sociale sont deux budgets
   distincts.** Les pensions civiles et militaires de retraite (CAS
   Pensions) sont dans le premier ; l'essentiel des retraites — régime
   général, régimes complémentaires — dans le second. Un chiffre de
   « dépense retraite » qui ne dit pas lequel des deux il mesure ne peut
   pas être comparé à un autre.
2. **COFOG (comptabilité nationale, toutes administrations) et LFSS
   (branches de la Sécurité sociale) ne recoupent pas exactement.** Les
   écarts entre les deux mesures sont documentés ci-dessous plutôt que
   lissés.
3. **Densité et disponibilité ne sont pas la même chose** — déjà la
   conclusion de la carte des généralistes (`docs/sante-donnees.md` § 1.7),
   qui vaut de la même façon pour l'APA à domicile : compter des
   bénéficiaires n'est pas mesurer un besoin couvert.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. Combien : deux mesures qui se recoupent sans coïncider

**Au sens de la comptabilité nationale (COFOG, toutes administrations
confondues)** — `core.macro_value`, fonction « Vieillesse » (GF1002)
isolée du reste de la protection sociale pour la première fois dans ce
dépôt :

| Année | Vieillesse (Md€) | Survivants (Md€) | Ensemble | Part de la dépense publique totale |
|---|---:|---:|---:|---:|
| 2023 | 369,1 | 38,6 | 407,7 | — |
| 2024 | 392,0 | 40,6 | 432,6 | **25,9 %** |

**Au sens du système de retraite lui-même (Conseil d'orientation des
retraites, tous régimes)** : **407 Md€ de dépenses de retraite en 2024,
13,9 % du PIB, 24,4 % des dépenses publiques**, système en déficit de
1,7 Md€ hors produits et charges financiers (COR, *Synthèse*, juin 2025).

**Les deux mesures se recoupent (24-26 % de la dépense publique) sans
coïncider au chiffre près** — 407 Md€ (COR) contre 432,6 Md€ (COFOG
vieillesse + survivants 2024) : la comptabilité nationale classe en
« Survivants » des prestations que le COR peut compter ou exclure
différemment, et les deux séries ne partagent pas exactement la même
année de référence dans leurs dernières publications respectives. Ce
dossier cite les deux plutôt que d'en choisir une seule qui masquerait
l'écart.

**Le rapport cotisants/retraités continue de se dégrader** — `core.cotisants_retraites_ratio`,
déjà chargée : **1,77 cotisant pour un retraité en 2023**, contre 1,72 en
2019 et 2020. Un chiffre qui monte, pas qui baisse : la légère hausse
récente tient à l'emploi, pas à un retournement démographique durable.
**L'âge moyen de départ à la retraite** (`core.age_depart_retraite`) était
de **62,68 ans** en 2022 (63,0 ans pour les femmes, 62,3 ans pour les
hommes) — la réforme de 2023, dont les effets se lisent avec retard dans
cette série, n'est pas encore intégralement reflétée aux derniers
millésimes disponibles.

### 3. Qui paie : le budget de l'État ne porte qu'une fraction étroite

Dans le budget de l'État (LFI 2025, `core.budget_programme`), la vieillesse
n'apparaît que par deux canaux, tous deux beaucoup plus étroits que le
chiffre COFOG ou COR :

| Mission / dispositif | Md€ (2025) | Ce qu'il couvre |
|---|---:|---|
| Pensions (CAS) | 68,5 | Pensions civiles et militaires de retraite des seuls fonctionnaires |
| Régimes sociaux et de retraite | 6,0 | Régimes spéciaux subventionnés (SNCF, RATP, marins, mineurs...) |
| **Total budget de l'État** | **74,5** | **17 % seulement du total COFOG vieillesse+survivants (432,6 Md€)** |

**L'écart (358 Md€) est financé par la Sécurité sociale, pas par l'État**
— la CNAV, les régimes complémentaires (Agirc-Arrco, hors périmètre
LFSS) et les autres caisses de retraite du secteur privé, dont ce dépôt
n'a toujours pas les comptes détaillés (`docs/securite-sociale-donnees.md`,
D-048 : « la Sécurité sociale ne publie pas ses comptes »). **Un chiffre
de « budget vieillesse » qui ne distingue pas ces deux origines confond
systématiquement deux masses d'ampleur très différente.**

### 4. La dépendance : l'APA à domicile, premier chiffre chargé de la branche autonomie

Aucune donnée sur la dépendance n'existait, jusqu'ici, dans ce dépôt — ni
dans le dossier santé, ni dans le dossier retraite. `core.apa_domicile`
(DREES, enquête Aide sociale, 2010-2024, 1 520 lignes) comble une partie
de ce vide :

| Année | Bénéficiaires (déc.) | Dépenses couvertes (Md€) | Périmètre |
|---|---:|---:|---|
| 2016 | 755 507 | 2,24 | Intervenants à domicile seulement |
| 2017 | 768 611 | 2,63 | Ensemble des dépenses APA à domicile |
| 2024 | 850 141 | 3,33 | Ensemble des dépenses APA à domicile |

**Trois réserves réelles, pas des détails techniques** (voir aussi le
commentaire de la migration 0112) :

1. **Rupture de périmètre en 2017** — avant cette date, le total ne
   couvre que la rémunération des intervenants à domicile ; depuis, il
   couvre aussi l'accueil de jour, l'accueil familial et les aides
   diverses. Comparer 2016 à 2017 sans le dire ferait passer un
   élargissement de périmètre pour une hausse de dépense.
2. **Un plancher, pas un total** : 22 % des lignes (335 sur 1 520) sont
   marquées « ND » (donnée non disponible) par les conseils départementaux
   eux-mêmes et valent NULL dans cette table, jamais 0. Le total 2024 de
   3,33 Md€ est donc un minorant du total réel.
3. **L'APA en établissement (EHPAD) n'est pas couverte** — aucune série
   nationale aussi longue trouvée à ce jour ; seule la part « à domicile »
   de l'allocation est mesurée ici.

**850 141 bénéficiaires pour 3,33 Md€, soit environ 3 900 €/an par
bénéficiaire** (326 €/mois) — un ordre de grandeur, pas un montant
individuel : la table agrège des situations très différentes (aide
ponctuelle, plan d'aide GIR 1 lourd) sous une même moyenne.

**La carte départementale rapporte les bénéficiaires à la population de 75
ans ou plus, pas à la population totale** — `core.population_age_departement`
(Insee, estimations de population par département et grande classe d'âge,
migration 0115) comble ce second vide : rapporter à la population totale
confondait un département dense en bénéficiaires avec un département
simplement plus âgé, deux choses différentes. **12,0 % des personnes de 75
ans ou plus bénéficient de l'APA à domicile en France (2024)**, avec un
écart réel entre départements (de 4,9 % à 32,0 % selon le classement de la
carte) — un écart qui, rapporté à la bonne population, mesure enfin un
taux de couverture plutôt qu'un simple effet de structure démographique.

## Ce que les données ne disent pas

### 5. Ce qui reste hors de portée

- **L'APA en établissement**, la seconde moitié de l'allocation, dont ce
  dossier ne dispose d'aucune série nationale comparable à celle de l'APA
  à domicile.
- **La comptabilité détaillée des caisses de retraite du secteur privé**
  (CNAV, Agirc-Arrco) — la Sécurité sociale ne publie pas ses comptes par
  branche à ce niveau de détail (D-048), le chiffre COR (§ 2) reste la
  seule mesure agrégée disponible.
- **L'âge de départ à la retraite comparé à l'étranger** — identifié dans
  `docs/international-donnees.md` § 10 (OCDE *Pensions at a Glance*),
  introuvable en série continue diffusée par API.
- **Le détail par GIR (groupe iso-ressources) de l'APA** — la DREES le
  publie par ailleurs (fichiers séparés, non chargés ici), ce qui
  permettrait de distinguer les plans d'aide légers des plans lourds
  plutôt qu'une moyenne unique.

## Sources

- Eurostat, `gov_10a_exp` (COFOG, sous-fonctions de la protection sociale,
  § 2).
- Conseil d'orientation des retraites (COR), *Synthèse*, juin 2025 (§ 2).
- DREES, enquête Aide sociale, *APA à domicile* (§ 4).
- INSEE, estimations de population par département, sexe et grande classe
  d'âge (§ 4, dénominateur de la carte).
- Assemblée nationale / PLF, mission Pensions et mission Régimes sociaux
  et de retraite (§ 3).
- [docs/retraite-donnees.md](retraite-donnees.md), pour le ratio
  cotisants/retraités et l'âge de départ, déjà chargés (§ 2).
- [docs/securite-sociale-donnees.md](securite-sociale-donnees.md), pour
  la limite structurelle sur les comptes de la Sécurité sociale (D-048).
- [docs/jeunesse-donnees.md](jeunesse-donnees.md), pour la comparaison
  symétrique sur l'accompagnement de la jeunesse.

## Annexe technique

### 6. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Eurostat, COFOG (sous-fonctions protection sociale) | `core.macro_value` (`cofog` `GF10__`) | 8 séries, 1995-2024 |
| 2 | PLF, missions Pensions et Régimes sociaux et de retraite | `core.budget_programme` | déjà chargé (chantiers 4-5) |
| 3 | DREES, APA à domicile | `core.apa_domicile` | 1 520 lignes, 2010-2024 |
| 4 | COR, *Synthèse* 2025 (citation) | `ref.fait_dossier` | 1 fait |
| 5 | Ratio cotisants/retraités, âge de départ | `core.cotisants_retraites_ratio`, `core.age_depart_retraite` | déjà chargés (`docs/retraite-donnees.md`) |
| 6 | Insee, population par département et grande classe d'âge | `core.population_age_departement` | 25 260 lignes, 1975-2025 (5 tranches × ~99 départements), tranche 75 ans ou plus utilisée pour la carte |

## Versions

- **Version 1** (15 septembre 2026) : premier chargement — COFOG vieillesse
  isolée, budget État (Pensions, régimes spéciaux) mis en regard du total
  COFOG et du chiffre COR, APA à domicile chargée pour la première fois
  (branche autonomie, angle mort jusqu'ici de ce dépôt).
- **Version 2** (15 septembre 2026) : la carte de l'APA à domicile rapporte
  désormais les bénéficiaires à la population de 75 ans ou plus
  (`core.population_age_departement`, Insee) plutôt qu'à la population
  totale — le taux de couverture réel remplace un indicateur qui mesurait
  pour partie la structure d'âge du département plutôt que l'APA elle-même.
