# Combien coûtent la Présidence, l'Assemblée et le Sénat

> **Dossier** · version 2 · 15 septembre 2026
>
> Combien coûtent la Présidence de la République, l'Assemblée nationale, le Sénat et
> les autres institutions de la mission « Pouvoirs publics » ? Ce qui est mesurable en données
> ouvertes, ce sont leurs dotations budgétaires, pas les rémunérations individuelles : le
> projet ne publie jamais de donnée nominative de rémunération sans source qui la publie.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->



<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **L'autonomie financière des pouvoirs publics, corollaire de leur indépendance** (20 novembre 2025). Le rapporteur spécial présente un niveau de réserves suffisant comme une condition de l'autonomie financière des pouvoirs publics, corollaire de leur indépendance institutionnelle. — Sénat, commission des finances (rapport spécial sur la mission « Pouvoirs publics », PLF 2026) · [source](https://www.senat.fr/rap/l25-139-322/l25-139-322-syn.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Le fonctionnement des assemblées parlementaires** (17 novembre 1958). Ordonnance qui organise le fonctionnement des assemblées parlementaires, dont leurs règles budgétaires propres. — Gouvernement (ordonnance n° 58-1100) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000000705067) · *officiel*

<!-- faits:CADRE:fin -->

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Des dotations en hausse de 12 % en euros courants de 2011 à 2025** (20 novembre 2025). Le rapporteur spécial relève que la dotation cumulée de la mission a progressé de 12 % entre 2011 et 2025 en euros courants, et alerte sur les effets du gel prolongé des dotations sur les réserves des institutions. — [Grégory Blanc](/depute/gregory-blanc/), Sénat, commission des finances (rapport spécial sur la mission « Pouvoirs publics », PLF 2026) · [source](https://www.senat.fr/rap/l25-139-322/l25-139-322-syn.pdf) · *officiel*

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 1. La mission « Pouvoirs publics » : quatre institutions, une seule mission

`core.budget_programme` (même table que
[docs/securite-police-donnees.md](securite-police-donnees.md),
[docs/defense-donnees.md](defense-donnees.md),
[docs/education-donnees.md](education-donnees.md) et
[docs/ecologie-donnees.md](ecologie-donnees.md) — un seul chargement PLF
couvre déjà cette mission) : la mission **Pouvoirs publics** regroupe les
crédits votés pour les institutions constitutionnelles, chacune son propre
programme budgétaire :

| Institution | 2024 (M€) | 2025 (M€) |
|---|---:|---:|
| Assemblée nationale | 607,65 | 617,98 |
| Sénat | 353,47 | 359,48 |
| Présidence de la République | 122,56 | 125,66 |
| La Chaîne parlementaire (LCP/Public Sénat) | 35,25 | 35,55 |
| Conseil constitutionnel | 17,93 | 16,85 |
| Cour de justice de la République | 0,98 | 0,98 |
| **Mission (total)** | **1 137,84** | **1 156,51** |

**Ce que représente ce total : environ 0,20 % du budget général de l'État**
(1 137,84 M€ sur 581,09 Md€ en 2024 — même table, même méthode que les autres
notes de cette famille). Ce sont des crédits de paiement votés au PROJET de
loi de finances, pas la loi de finances initiale adoptée ni l'exécution —
même piège voté/exécuté que [docs/budget-donnees.md](budget-donnees.md) § 2.

**Ce que ces montants couvrent** : une **dotation globale** par institution,
votée chaque année, qui finance l'ensemble de son fonctionnement —
indemnités des parlementaires, salaires des collaborateurs et fonctionnaires
de l'Assemblée/du Sénat, immobilier, sécurité, frais de mandat. Ce n'est
**pas** une ligne « salaires » isolée : le détail par nature de dépense
(personnel, fonctionnement, investissement) à l'intérieur de chaque dotation
n'est pas publié dans le PLF au même niveau de détail que pour les missions
ministérielles — les assemblées disposent de l'autonomie financière et
budgètent en interne, sans le détail par titre (LOLF art. 7) qui existe pour
les autres missions.

### 3. Historique : deux exercices seulement, pour l'instant

Comme pour Défense, Sécurité et Éducation, la série ne remonte qu'à 2024 :
les jeux PLF de data.economie.gouv.fr changent de nom et de schéma de champs
d'une édition à l'autre (voir `internal/budget/plf_destination.go`), et
étendre la série en amont demande de retrouver et vérifier un identifiant par
millésime — non fait à ce stade, comme documenté pour les autres notes de
cette même famille.

## Ce que les données ne disent pas

### 2. Ce que la dotation ne dit pas

**Les rémunérations individuelles ne sont pas publiées en données ouvertes.**
Quatre précisions manquantes, chacune pour une raison distincte :

- **Anciens présidents de la République** : leur dotation matérielle et en
  personnel est en principe distincte de celle du président en exercice
  (régime fixé par un arrêté du Premier ministre), mais ce projet n'a
  identifié aucune ligne budgétaire ouverte qui l'isole du budget global de
  la Présidence — non chargé.
- **Membres du gouvernement en exercice** : leur traitement est fixé par
  décret (indexé sur un indice de la fonction publique), mais son montant et
  sa masse totale ne relèvent pas de la mission Pouvoirs publics — ils sont
  budgétés dans les crédits de personnel (titre 2) des services du Premier
  ministre ou de chaque ministère, mêlés à l'ensemble de leurs
  fonctionnaires. Aucune ligne ouverte isolant la seule rémunération
  ministérielle n'a été identifiée.
- **Anciens membres du gouvernement** : aucune dotation ou avantage
  spécifique équivalent à celui des anciens présidents n'a été identifié
  comme publié en open data.
- **Députés et sénateurs pris individuellement** : la dotation de
  l'Assemblée nationale et celle du Sénat sont des enveloppes globales — le
  détail de l'indemnité parlementaire de base, de l'indemnité de frais de
  mandat (IRFM devenue AFM) ou de l'enveloppe collaborateurs par élu est
  publié par chaque assemblée séparément (barèmes, pas une exécution
  nominative), mais n'a pas été rapproché ici de la dotation globale du
  tableau du § 1.

**Cohérent avec le reste du projet** (`docs/perimetre.md` § 2, D-003) : ce
qui n'est pas publié comme donnée ouverte structurée n'est pas deviné ni
estimé — cette note s'arrête à la dotation globale, la plus fine maille
réellement disponible en open data budgétaire pour ces institutions.

## Sources

- Direction du budget, *PLF — dépenses par mission, programme et action*,
  data.economie.gouv.fr, éditions 2024 et 2025.
- [docs/budget-donnees.md](budget-donnees.md), pour le piège voté/exécuté.

## Annexe technique

### 4. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Direction du budget, PLF, mission Pouvoirs publics | `core.budget_programme` | 6 programmes, 2024-2025 (table partagée, § 1) |

## Versions

- **Version 2** (15 septembre 2026) : plan commun des dossiers (D-066) ; cadre (ordonnance de 1958) et contrôle du Sénat sur les dotations.
- **Version 1** (14 septembre 2026) : dotations de la mission « Pouvoirs publics ».
