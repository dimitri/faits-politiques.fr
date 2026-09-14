# Le budget de la Défense : quatre programmes, un piège de lecture répété

> Note de synthèse. Version 1 — 14 septembre 2026.
> Cette note s'appuie sur la même table que
> [docs/securite-police-donnees.md](securite-police-donnees.md)
> (`core.budget_programme`, PLF par mission/programme/action) : les deux
> notes chargent une seule fois un jeu de données qui couvre déjà toutes les
> missions du budget général, Défense et Sécurités n'en sont que deux
> lectures.

---

## 1. La mission Défense : quatre programmes budgétaires

Source : PLF, `data.economie.gouv.fr`. La mission **Défense** est composée de
quatre programmes, chargés intégralement, aucun agrégé à un autre :

| Programme | 2024 (Md€) | 2025 (Md€) |
|---|---:|---:|
| 212 — Soutien de la politique de la défense | 24,64 | 24,92 |
| 146 — Équipement des forces | 16,59 | 18,69 |
| 178 — Préparation et emploi des forces | 13,58 | 14,32 |
| 144 — Environnement et prospective de la politique de défense | 1,97 | 2,08 |
| **Mission Défense (total)** | **56,78** | **60,00** |

**Ce sont des crédits de paiement (CP) votés au PROJET de loi de finances**,
pas la loi de finances initiale adoptée ni l'exécution — voir
[docs/budget-donnees.md](budget-donnees.md) § 2 pour ce piège, déjà documenté
ailleurs dans ce projet et qui vaut ici à l'identique.

**Ce que chaque programme couvre, en un mot** : 212 centralise le soutien
transversal (administration, immobilier, une bonne part des pensions — voir
§ 2) ; 146 finance les équipements (la trajectoire suivie par la loi de
programmation militaire, LPM) ; 178 finance l'entraînement et l'engagement
opérationnel des forces ; 144 finance le renseignement et la prospective
stratégique.

## 2. Le piège : ce total ne correspond pas au chiffre le plus souvent cité

**60,0 Md€ pour 2025 est plus élevé que le montant le plus souvent cité dans
le débat public pour « le budget de la Défense » (proche de 47 Md€, la
trajectoire suivie par la LPM 2024-2030).** L'écart n'est pas une erreur de
chargement — il s'explique en regardant la composition du titre 2
(personnel) de la mission :

| Catégorie de dépense de personnel (2025) | Md€ |
|---|---:|
| 21 — Rémunérations d'activité | 11,77 |
| 22 — Cotisations et contributions sociales | 11,06 |
| 23 — Prestations sociales et allocations diverses | 0,40 |

**La catégorie 22 — pour l'essentiel les cotisations au CAS Pensions, le
compte d'affectation spéciale qui finance les retraites des militaires — pèse
presque autant que les rémunérations elles-mêmes (11,06 Md€ contre
11,77 Md€).** Le chiffre couramment cité pour « le budget de la Défense »
suit la trajectoire de la LPM, qui raisonne hors CAS Pensions : soustraire
cette seule catégorie du total ci-dessus (60,0 − 11,1 ≈ 48,9 Md€) rapproche
immédiatement le chiffre de ce repère — sans que ce rapprochement soit ici
présenté comme une reconstitution exacte de la trajectoire LPM, dont le
périmètre précis n'est pas entièrement recoupé dans cette note.

**La leçon à retenir** : « le budget de la Défense » n'est pas un nombre
unique. Le total du tableau du § 1 est le total budgétaire complet de la
mission ; la trajectoire LPM qui fait l'actualité en exclut une part
substantielle. Aucun des deux chiffres n'est « le bon » — ce sont deux
périmètres différents, et les confondre produit une comparaison fausse d'une
année sur l'autre ou d'une source à l'autre.

## 3. Investissement (titre 5) : la trajectoire la plus directement liée à la LPM

| | 2024 (Md€) | 2025 (Md€) |
|---|---:|---:|
| Titre 5 — Investissement, mission Défense | 16,21 | 18,03 |

**+11 % entre 2024 et 2025**, porté pour l'essentiel par le programme 146
(Équipement des forces). C'est la mesure la plus proche de « l'effort
d'équipement » suivi par la LPM, même si elle n'épuise pas le sujet (une
partie de l'équipement est aussi financée en titre 3, maintenance en
condition opérationnelle notamment).

## 4. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Direction du budget, PLF par mission/programme/action | `core.budget_programme` | 4 796 lignes, toutes missions, 2024-2025 |

**Non chargé, et pourquoi :**

- La trajectoire pluriannuelle de la LPM elle-même (2024-2030), telle
  qu'annexée à la loi de programmation : un document législatif, pas un jeu
  de données ouvert au même format que le PLF annuel.
- Effectifs (ETPT) par programme : même limite que pour la police nationale,
  voir [docs/securite-police-donnees.md](securite-police-donnees.md) § 4.
- Exécution réelle (par opposition au PLF voté) : pas d'équivalent ouvert
  détaillé par mission à `core.execution_etat`.
- Millésimes antérieurs à 2024 : les jeux PLF de data.economie.gouv.fr
  changent de nom et de schéma de champs d'une édition à l'autre (déjà vrai
  entre 2024 et 2025, voir `internal/budget/plf_destination.go`) ; étendre la
  série en amont demande de retrouver et vérifier un identifiant par
  millésime, non fait à ce stade.

## Sources

- Direction du budget, *PLF — dépenses par mission, programme et action*,
  data.economie.gouv.fr, éditions 2024 et 2025.
- [docs/budget-donnees.md](budget-donnees.md), pour le piège voté/exécuté.
