# L'Éducation nationale : budget, effectifs, et trois idées reçues vérifiées

> Note de synthèse. Version 2 — 14 septembre 2026.
> Le budget de la mission Enseignement scolaire s'appuie sur
> `core.budget_programme`, la même table que
> [docs/securite-police-donnees.md](securite-police-donnees.md) et
> [docs/defense-donnees.md](defense-donnees.md) — un seul chargement PLF
> couvre déjà cette mission, rien à recharger.
>
> **Version 2** documente précisément, après une recherche dédiée, pourquoi
> les effectifs d'AESH restent hors de portée en jeu de données ouvert
> (§ 6) — sans rien changer aux données déjà chargées.

---

## 1. Le budget : 87 à 88 Md€, et neuf euros sur dix pour du personnel

Mission **Enseignement scolaire**, PLF, six programmes chargés intégralement :

| Programme | 2024 (Md€) | 2025 (Md€) |
|---|---:|---:|
| Enseignement scolaire public du second degré | 38,42 | 39,52 |
| Enseignement scolaire public du premier degré | 26,84 | 27,49 |
| Enseignement privé du premier et du second degrés | 9,04 | 8,94 |
| Vie de l'élève | 7,94 | 8,15 |
| Soutien de la politique de l'éducation nationale | 2,89 | 2,98 |
| Enseignement technique agricole | 1,70 | 1,73 |
| **Mission (total)** | **86,83** | **88,81** |

**93 % de ce budget est du personnel** (titre 2 : 80,67 Md€ sur 86,83 Md€ en
2024, 83,30 sur 88,81 en 2025) — une proportion encore plus extrême que celle
déjà relevée pour la police nationale (87 %,
[docs/securite-police-donnees.md](securite-police-donnees.md) § 2.1). Ce sont
des crédits de paiement (CP) votés au PROJET de loi de finances, pas la loi de
finances initiale adoptée ni l'exécution — même piège voté/exécuté que
[docs/budget-donnees.md](budget-donnees.md) § 2.

## 2. Les effectifs par établissement : Depp, granularité fine

Source : Depp, `data.education.gouv.fr`, deux jeux distincts et asymétriques —
le premier degré publie les rentrées 2024 et 2025, le second degré seulement
2024 au moment de l'écriture :

| | Premier degré (ETP enseignants) | Second degré (ETP enseignants) |
|---|---:|---:|
| Public, 2024 | 279 173 | 355 632 |
| Privé, 2024 | 37 714 | 85 821 |
| Public, 2025 | 277 148 | — |
| Privé, 2025 | 37 543 | — |

**Le premier degré recule légèrement d'une rentrée à l'autre** (279 173 →
277 148 ETP dans le public, −0,7 %) — cohérent avec la baisse démographique
scolaire déjà documentée par ailleurs, mais cette note ne l'établit pas ici,
faute d'avoir chargé les effectifs d'élèves en regard.

## 3. Le coût de structure : ce que le personnel hors enseignement révèle — et ce qu'il cache

Le second degré publie, par établissement, l'ETP total, l'ETP enseignant et
l'ETP « personnels de vie scolaire ». Le résidu (`etp_total − etp_enseignants
− etp_vie_scolaire`) approxime le personnel de direction, administratif et
technique — ni devant les élèves, ni dans les établissements pour l'encadrement
de la vie scolaire (assistants d'éducation notamment) :

| Secteur | ETP total | ETP enseignants | ETP vie scolaire | ETP « autre » | Part hors enseignement |
|---|---:|---:|---:|---:|---:|
| Public | 487 281 | 355 632 | 59 807 | 71 841 | 27,0 % |
| Privé sous contrat | 86 410 | 85 821 | ~0 | 589 | 0,7 % |

**Cet écart (27,0 % contre 0,7 %) ne mesure pas une différence d'efficacité
entre public et privé — il mesure une différence de PÉRIMÈTRE de ce que
l'État paie.** Dans le privé sous contrat, l'État ne rémunère que les
enseignants (contrat d'association) ; le personnel de direction, administratif
et technique est employé et payé directement par l'établissement ou l'organisme
de gestion (OGEC), et n'apparaît donc pas dans ce jeu de données Depp, qui ne
compte que les emplois payés par l'État. **Présenter ce tableau comme « le
privé a 27 points de coût de structure en moins » serait faux** : c'est
27 points de personnel non comptés dans cette source, pas 27 points de
personnel absents. Le seul fait solide que ce tableau établit est le
périmètre budgétaire de l'État : dans le public, 27 % des emplois d'État en
établissement du second degré ne sont ni des enseignants ni de la vie
scolaire.

**Un second signal, plus directement comparable** : la proportion
d'enseignants non titulaires est **plus élevée dans le privé (19,5 % en
moyenne par établissement) que dans le public (8,5 %)** — une mesure qui, elle,
porte sur la même catégorie de personnel (les enseignants payés par l'État)
dans les deux secteurs, donc réellement comparable.

## 4. Qui est le supérieur hiérarchique d'un enseignant ?

Le Code de l'éducation (articles D. 422-5 à D. 422-11) dit précisément ceci :
le chef d'établissement (principal, proviseur) **« représente l'État au sein
de l'établissement »** et **« a autorité sur l'ensemble des personnels
affectés ou mis à disposition »** (art. D. 422-5, D. 422-7) — une autorité
large, sur tous les personnels sans distinction de statut dans le texte
lui-même.

**Dans la pratique et selon l'interprétation qu'en font l'administration et
les organisations syndicales, cette autorité est qualifiée de
« fonctionnelle » plutôt que de « hiérarchique » pour les enseignants** : le
chef d'établissement organise leur service au sein de l'établissement, mais
les actes qui déterminent une carrière — recrutement, évaluation
disciplinaire, notation, mutation — restent du ressort du recteur d'académie,
via le corps d'inspection (IA-IPR). Cette distinction fonctionnel/hiérarchique
n'est pas un terme que le Code de l'éducation emploie explicitement dans les
articles cités ; elle est l'interprétation dominante qu'en tirent
l'administration et les représentants du personnel, pas une citation littérale
de la loi — cette note le signale précisément pour ne pas citer comme
disposition légale ce qui est une lecture établie mais non textuelle.

## 5. La rémunération pendant les vacances d'été : ce que dit la mensualisation

**Il n'existe pas de dispositif où un enseignant serait payé pour dix mois de
service puis « réparti » sur douze.** Comme tout fonctionnaire, un enseignant
titulaire perçoit un traitement annuel, versé en douze mensualités égales, en
application du principe du service fait — le même mécanisme de mensualisation
que pour n'importe quel agent public, enseignant ou non. Le salaire continue
d'être versé en juillet et en août, sans interruption ni rattrapage.

**D'où vient alors l'idée reçue ?** Probablement de la comparaison avec
d'autres agents de même grade qui perçoivent davantage de primes et
d'indemnités que les enseignants — un écart de rémunération réel, mais qui
n'a rien à voir avec un calendrier de versement sur dix mois. Cette note ne
peut pas chiffrer précisément cet écart de primes faute d'une source ouverte
dédiée au moment de l'écriture ; elle se limite à corriger le mécanisme de
versement, qui est vérifiable dans les règles générales de la fonction
publique.

## 6. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Direction du budget, PLF, mission Enseignement scolaire | `core.budget_programme` | 6 programmes, 2024-2025 (table partagée, voir § 1) |
| 2 | Depp, personnels des établissements du premier degré | `core.education_personnel_etablissement` | 94 584 lignes, 2024-2025 |
| 3 | Depp, personnels des établissements du second degré | `core.education_personnel_etablissement` | 10 697 lignes, 2024 |

**Non chargé, et pourquoi :**

- **Les AESH (accompagnants d'élèves en situation de handicap) : recherche
  refaite, toujours aucun jeu de données ouvert structuré identifié.** Ils
  sont mêlés, dans les jeux Depp chargés (§ 2), aux « personnels de vie
  scolaire » sans être isolés — confirmé explicitement par la documentation
  du jeu de données lui-même (« les ETP des personnels de vie scolaire sont
  renseignés en nc [non concerné] » pour le secteur privé sous contrat,
  puisque ces personnels n'y sont pas payés par l'État). Le chiffre le plus
  cité — **86 502 ETP en 2024, 90 502 ETP en 2025** — vient du rapport de la
  Cour des comptes de septembre 2024 et de reprises parlementaires, pas d'un
  jeu de données consultable : cette note le cite comme un ordre de grandeur
  sourcé, pas comme une série chargée en base, faute d'un fichier à
  télécharger et à vérifier ligne à ligne.
- La dépense par élève (souvent citée : environ 8 450 €/an en primaire,
  11 320 €/an dans le secondaire) : chiffre publié par la Depp dans ses
  publications (RERS, « L'état de l'École ») mais aucun jeu de données ouvert
  structuré retrouvé au moment de l'écriture — seulement des documents PDF.
- Effectifs d'élèves par école : jeu identifié
  (`fr-en-ecoles-effectifs-nb_classes`) mais pas encore chargé, ce qui
  empêcherait de confirmer si la baisse d'ETP enseignants du § 2 suit la
  démographie scolaire ou s'en écarte.
- Séries historiques longues (jusqu'aux années 1980, que la RERS publie) :
  les jeux Depp chargés ici ne remontent pas au-delà de 2024 ; les éditions
  RERS antérieures existent en PDF, pas en jeu de données structuré comparable.

## Sources

- Direction du budget, *PLF — dépenses par mission, programme et action*,
  data.economie.gouv.fr, éditions 2024 et 2025.
- Depp (Direction de l'évaluation, de la prospective et de la performance),
  *Les personnels dans les établissements du premier degré* et *du second
  degré*, data.education.gouv.fr.
- Légifrance, Code de l'éducation, articles D. 422-5 à D. 422-11.
- Cour des comptes, rapport sur les AESH, septembre 2024 (ordre de grandeur
  cité § 6, non chargé en base).
- [docs/budget-donnees.md](budget-donnees.md), pour le piège voté/exécuté.
