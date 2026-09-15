# Collectivités : d'où vient l'argent, et ce qu'un euro par habitant ne dit pas

> **Dossier** · version 2 · 15 septembre 2026
>
> Communes, intercommunalités, départements, régions : d'où vient l'argent que les
> collectivités dépensent ? Le dossier sépare deux origines — la fiscalité qu'elles votent et
> les transferts de l'État, dont la dotation globale de fonctionnement (DGF) — et ce qu'un
> montant par habitant ne dit pas.

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

### 1. Deux origines : fiscalité propre et transferts de l'État

Une collectivité locale finance ses dépenses de deux façons principales :
**la fiscalité qu'elle vote elle-même** (taxe foncière, part locale de
cotisation économique territoriale, etc. — regroupée ici sous « impôts et
taxes ») et **les transferts de l'État**, dont le plus connu est la
**dotation globale de fonctionnement (DGF)**. Confondre les deux revient à
confondre une collectivité qui s'autofinance et une collectivité sous
perfusion de l'État — une différence politique et budgétaire réelle, que
seule la ventilation permet de voir.

`core.commune_indicator` et `core.collectivite_budget` chargent maintenant
trois agrégats OFGL par habitant pour les quatre niveaux — **recettes
totales**, **DGF**, **impôts et taxes** — aux côtés des cinq indicateurs de
dépense déjà documentés sur la page `/collectivites/`.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. La photographie 2025

`derived.poids_des_niveaux` (vue qui somme les indicateurs par habitant en
masse totale, niveau par niveau) :

| Niveau | Recettes totales | dont DGF | dont impôts et taxes |
|---|---:|---:|---:|
| Communes (34 772) | 121,1 Md€ | 10 % | 54 % |
| Intercommunalités (1 262) | 51,1 Md€ | 13 % | 46 % |
| Départements (97) | 96,8 Md€ | 8 % | 63 % |
| Régions (17) | 41,8 Md€ | 1 % | 62 % |

**Le reste des recettes totales — 30 à 40 % selon le niveau — n'est pas
détaillé par cette seule agrégation** : subventions diverses, produits des
services (cantines, droits d'entrée), dotations autres que la DGF, emprunts
le cas échéant. Ce dossier cite ce qu'il mesure, pas un total reconstitué.

**N'additionnez jamais ces montants entre niveaux** : une subvention
départementale à une commune, ou l'attribution de compensation qu'une
intercommunalité reverse à ses communes membres, est comptée en recette chez
celui qui la reçoit et en dépense chez celui qui la verse — le même
avertissement que pour les cinq indicateurs de dépense déjà publiés.

### 3. Les régions, un cas à part : la DGF a presque disparu, pas les transferts de l'État

**1 % de DGF pour les régions, contre 8 à 13 % pour les autres niveaux — un
fait réel, pas une lacune de chargement.** Depuis la loi de finances pour
2018, la DGF des régions a été remplacée par une fraction du produit de la
TVA, une recette de l'État reversée sans que la région en vote le taux.

**Où se loge cette fraction de TVA dans cette agrégation reste une question
ouverte, honnêtement non tranchée.** La part d'« impôts et taxes » des
régions (62 %) est comparable à celle des départements (63 %) — un indice
que l'OFGL compte vraisemblablement cette recette de substitution dans cette
catégorie plutôt que dans une case à part, mais ce dossier n'a pas ouvert la
méthodologie détaillée de l'OFGL pour le confirmer. **Une région à faible DGF
n'est donc pas une région abandonnée par l'État** : c'est un changement de
tuyau budgétaire de 2018, pas un désengagement.

## Ce que les données ne disent pas

### 4. Ce que ce chargement ne permet pas encore de faire

**Le pont vers la santé et l'éducation nationale, posé mais pas chiffré.**
Les départements financent une part de l'action sociale et des collèges, les
régions les lycées et une part de la formation professionnelle, et certains
établissements de santé publics sont des établissements territoriaux — mais
aucune ligne budgétaire chargée ici n'isole la part d'un budget départemental
ou régional qui va spécifiquement à un collège, un lycée ou un hôpital.
`core.epci_competence`/`ref.competence` documentent déjà quelles
intercommunalités déclarent une compétence donnée (voir
[docs/bassins-versants-donnees.md](bassins-versants-donnees.md) pour le
même exercice appliqué à l'eau) ; le chiffrage financier de ce pont reste un
chantier séparé, non commencé.

**La fiscalité locale taux/bases (DGFiP REI)** — qui dirait, commune par
commune, quel taux de taxe foncière est voté et sur quelle base — reste hors
de ce chargement : plus lourde à charger, elle documenterait un niveau de
détail que la question posée ici (transferts vs fiscalité propre, en masse)
ne demande pas.

## Sources

- OFGL (Observatoire des finances et de la gestion publique locales) /
  DGCL, données par habitant par niveau de collectivité, 2018-2025.
- [docs/mairies-conception.md](mairies-conception.md), pour ce qui est
  comparable et ce qui ne l'est pas entre communes.
- [docs/bassins-versants-donnees.md](bassins-versants-donnees.md), pour un
  exemple déjà traité de compétence territoriale partagée entre niveaux.

## Annexe technique

### 5. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | OFGL / DGCL, budgets communaux | `core.commune_indicator` (codes `ofgl.dgf_par_hab`, `ofgl.recettes_totales_par_hab`, `ofgl.impots_taxes_par_hab`) | 34 256 à 34 772 communes, 2018-2025 |
| 2 | OFGL / DGCL, budgets région/département/groupement | `core.collectivite_budget` | mêmes trois codes, 2018-2025 |
| 3 | Vue dérivée | `derived.poids_des_niveaux` | déjà en place (migration 0044), étendue automatiquement aux trois nouveaux codes |

**Non chargé, et pourquoi** :
- **Détail de la fraction de TVA versée aux régions** : non isolée dans la
  nomenclature OFGL utilisée ici — § 3.
- **Fiscalité locale taux/bases (DGFiP REI)** : hors périmètre de cette
  question, plus lourde à charger — § 4.
- **Chiffrage du financement territorial de la santé et de l'éducation** :
  compétences identifiées, montants non isolés — § 4.

## Versions

- **Version 2** (15 septembre 2026) : plan commun des dossiers (D-066).
- **Version 1** (15 septembre 2026) : dotation globale de fonctionnement et fiscalité propre.
