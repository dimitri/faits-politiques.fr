# La jeunesse : études supérieures, apprentissage, premiers emplois

> **Dossier** · version 1 · 15 septembre 2026
>
> En miroir du dossier vieillesse : combien coûtent les études supérieures, l'apprentissage
> mène-t-il à un emploi, et que finance l'État pour les premiers pas dans la vie active ? Trois
> circuits de financement distincts, qu'il ne faut pas additionner sans le dire.

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

### 1. Méthode : trois circuits de financement, jamais additionnés sans le dire

L'enseignement supérieur (budget de l'État), l'apprentissage (taxe
d'apprentissage et contributions des employeurs, collectées et
redistribuées par France Compétences, **hors budget de l'État**) et les
dispositifs d'insertion comme le Contrat d'engagement jeune (budget de
l'État à nouveau, mais un guichet distinct) répondent à des logiques de
financement complètement différentes. Un total « jeunesse » qui les
additionnerait masquerait que l'essentiel de l'effort public (les études
supérieures) et l'essentiel de l'apprentissage (financé par les
entreprises, pas par l'impôt) n'ont ni la même origine ni le même
redevable.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. Les études supérieures : 18,5 Md€ isolables dans le budget de l'État

Mission « Recherche et enseignement supérieur » (PLF 2025,
`core.budget_programme`) — deux programmes isolables de la recherche pure :

| Programme | Md€ (2025) |
|---|---:|
| Formations supérieures et recherche universitaire | 15,28 |
| Vie étudiante (bourses, CROUS, santé étudiante) | 3,25 |
| **Total isolable « enseignement supérieur »** | **18,53** |
| *(pour mémoire, hors ce total)* Recherches scientifiques et technologiques pluridisciplinaires | 8,26 |

**Une simplification à ne pas cacher** : le programme « Formations
supérieures et recherche universitaire » finance des universités qui
mènent les deux missions à la fois — enseigner et faire de la recherche —
sans que le budget les sépare en interne. Ce dossier retient le programme
entier côté enseignement plutôt que d'inventer une clé de répartition non
publiée par la source.

### 3. L'apprentissage : l'insertion mesurée, le financement cité mais non chargé

**Ce que ce dépôt mesure directement** — DEPP, enquête InserJeunes
(`core.insertion_apprentissage`, 32 953 lignes, six promotions 2018-2019 à
2023-2024, par CFA et niveau de diplôme). Taux d'emploi 6 mois après la
fin du contrat, promotion 2023-2024, **médiane des CFA** (pas une moyenne
nationale pondérée — voir la réserve ci-dessous) :

| Niveau de diplôme | CFA mesurés | Taux d'emploi à 6 mois (médiane) |
|---|---:|---:|
| CAP | 1 134 | 58 % |
| BAC PRO | 843 | 66 % |
| BTS | 1 266 | 67 % |

**Pourquoi une médiane, pas une moyenne nationale** : ce jeu ne publie
aucun effectif par CFA — seulement des taux. Calculer une « moyenne
nationale » à partir de taux non pondérés par la taille réelle des
établissements donnerait un nombre qui a l'air précis sans l'être : un
petit CFA de 20 apprentis pèserait autant qu'un grand de 2 000. La
médiane des CFA mesurés est le résumé le plus honnête que permette cette
source.

**Ce que ce dépôt ne mesure pas directement, mais peut citer avec sa
source précise** — France Compétences, *Rapport d'activité 2024* :
**896 000 nouveaux contrats d'apprentissage dans le secteur privé en
2024** (+3,7 % sur 2023 ; +25 % depuis 2021, où ils étaient 719 000),
**9,4 Md€ engagés pour ces contrats**, dont **8,176 Md€ pris en charge
par les OPCO sur les fonds de France Compétences** au titre des coûts
pédagogiques (soit environ **9 125 € par contrat** en moyenne). Le coût
moyen d'un contrat hors salaire de l'apprenti a reculé de 22 478 € en
2021 à 17 404 € en 2024, après la révision générale des niveaux de prise
en charge (NPEC) engagée par France Compétences.

**Pourquoi ce chiffre est cité, pas chargé comme les autres données de ce
dépôt** : la Dares, qui publie la série statistique longue des contrats
d'apprentissage, bloque toute requête automatisée par un pare-feu
applicatif (vérifié : la page renvoie un défi anti-robot, pas une donnée,
quel que soit le client utilisé). France Compétences ne publie pas cette
statistique agrégée en fichier ouvert — seulement des référentiels de
financement par branche professionnelle (des dizaines de mégaoctets de
barèmes, pas une série temps) ou des rapports en PDF. Le chiffre ci-dessus
est donc rapporté avec sa source exacte, comme une citation vérifiable,
et non comme une table requêtable — la différence que `docs/perimetre.md`
§ 2 demande de toujours signaler.

### 4. Les premiers emplois : le Contrat d'engagement jeune, un guichet précis

Mission « Travail, emploi et administration des ministères sociaux »,
action « Insertion des jeunes sur le marché du travail » (PLF,
`core.budget_programme`) :

| Exercice | Contrat d'engagement jeune (Md€) |
|---|---:|
| 2024 | 1,084 |
| 2025 | 0,980 |

**Le CEJ a succédé à la Garantie jeunes** en 2022 — un accompagnement
individualisé et une allocation pour les jeunes sans emploi ni formation,
financé entièrement par l'État. Le recul entre 2024 et 2025 (−9,6 %)
n'est pas expliqué par ce seul chiffre budgétaire ; le rapprocher du
nombre de jeunes accompagnés (non chargé ici) serait nécessaire avant
toute lecture du sens de cette baisse.

## Ce que les données ne disent pas

### 5. Ce qui reste hors de portée

- **Le nombre de contrats d'apprentissage et leur financement en série
  ouverte** — cité avec sa source exacte (§ 3), non chargeable en l'état
  (Dares bloquée par pare-feu, France Compétences sans export agrégé).
- **Le nombre de jeunes accompagnés par le CEJ** — le budget (§ 4) est
  chargé, l'effectif ne l'est pas.
- **La ventilation par niveau de vie ou origine sociale des étudiants
  boursiers** — `docs/recherche-enseignement-superieur-donnees.md` cite
  déjà 662 000 boursiers (l'effectif le plus faible depuis 2015), un fait
  à mettre en regard des 18,53 Md€ de la mission sans que ce dossier le
  refasse en double.
- **La comparaison internationale de l'insertion des jeunes** — non
  identifiée à ce stade, un chantier distinct de `docs/international-
  donnees.md`.

## Sources

- PLF, mission Recherche et enseignement supérieur (§ 2).
- DEPP, enquête InserJeunes (§ 3).
- France Compétences, *Rapport d'activité 2024* (citation, § 3).
- PLF, mission Travail, emploi et administration des ministères sociaux,
  action Contrat d'engagement jeune (§ 4).
- [docs/recherche-enseignement-superieur-donnees.md](recherche-enseignement-superieur-donnees.md),
  pour les effectifs et boursiers déjà chargés.
- [docs/vieillesse-donnees.md](vieillesse-donnees.md), pour la comparaison
  symétrique sur la vieillesse.

## Annexe technique

### 6. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | PLF, mission Recherche et enseignement supérieur | `core.budget_programme` | déjà chargé |
| 2 | DEPP, InserJeunes | `core.insertion_apprentissage` | 32 953 lignes, 6 promotions |
| 3 | PLF, action Contrat d'engagement jeune | `core.budget_programme` | déjà chargé |

## Versions

- **Version 1** (15 septembre 2026) : premier chargement — enseignement
  supérieur isolé dans le budget de l'État, insertion des apprentis
  chargée (InserJeunes), Contrat d'engagement jeune isolé. Chiffres de
  financement de l'apprentissage (France Compétences) cités avec leur
  source précise, non chargés en série requêtable — raison technique
  vérifiée plutôt que supposée.
