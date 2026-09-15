# La santé : FINESS comme clé pivot, et comment les médecins sont payés

> **Dossier** · version 5 · 15 septembre 2026
>
> Comment le système de santé est-il décrit par les données publiques, et comment les
> médecins sont-ils rémunérés ? Les sources sont les plus éclatées du projet — établissements,
> activité, séjours, remboursements, professionnels, qualité — sans identifiant commun garanti
> sauf le répertoire FINESS, par lequel le dossier commence.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| hôpital | 1669 | 354 | 1er octobre 2024 | 21 juillet 2026 |
| déserts médicaux | 333 | 141 | 18 juillet 2024 | 20 juillet 2026 |

Une mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **267,5 Md€ de dépenses prévues pour la branche maladie en 2026** (10 décembre 2025). L'objectif de dépenses de la branche maladie, maternité, invalidité et décès est fixé à 267,5 Md€ pour 2026, en hausse de 2 % sur l'exécution 2025 (objectif voté). — Sénat, commission des affaires sociales (rapport sur le PLFSS 2026) · [source](https://www.senat.fr/lessentiel/plfss2026.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **La loi relative à l'organisation et à la transformation du système de santé** (24 juillet 2019). Loi d'organisation du système de santé, dont l'article 41 crée la plateforme des données de santé. — Parlement (loi n° 2019-774) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000038821260) · *officiel*

<!-- faits:CADRE:fin -->

### 2. La rémunération des médecins : le secteur conventionnel

Source : Cnam (Assurance Maladie), `data.ameli.fr`, « Démographie secteurs
conventionnels » — 177 720 lignes, toutes professions de santé libérales
(pas seulement les médecins), 2010-2024.

**Ensemble des médecins, France entière, 2024** :

| Secteur | Effectif | Part |
|---|---:|---:|
| Secteur 1 (tarifs fixés par convention) | 76 910 | 68,6 % |
| Secteur 2 avec Optam/Optam-CO | 17 293 | 15,4 % |
| Secteur 2 sans Optam/Optam-CO | 17 029 | 15,2 % |
| Non conventionnés | 927 | 0,8 % |
| **Total** | **112 159** | |

**Ce que ces quatre catégories signifient concrètement** : en secteur 1, le
médecin applique le tarif fixé par la convention avec l'Assurance Maladie,
remboursé intégralement à ce tarif (hors participation forfaitaire). En
secteur 2, le médecin fixe librement ses honoraires ; l'Assurance Maladie ne
rembourse que sur la base du tarif conventionnel, jamais sur le dépassement.
L'Optam (option pratique tarifaire maîtrisée) est un engagement à modérer ce
dépassement, en échange d'un remboursement complémentaire plus favorable —
un médecin de secteur 2 adhérent à l'Optam n'est donc pas un troisième
secteur au sens strict, mais un sous-ensemble du secteur 2. Les non
conventionnés (0,8 %) fixent librement leurs honoraires et l'Assurance
Maladie rembourse sur une base forfaitaire très inférieure.

**Une rupture de série réelle, pas une lacune** : avant 2013, la donnée ne
distingue que trois catégories (secteur 1, secteur 2, non conventionné) —
l'Optam (sous son nom d'origine, le contrat d'accès aux soins) n'existe pas
avant cette date. `cmd/verify` l'accepte explicitement plutôt que de
signaler une anomalie à chaque millésime antérieur à 2013.

**Évolution 2010-2024** : l'effectif total de médecins recule légèrement
(118 133 → 112 159, soit −5,1 %) sur la période — un chiffre à mettre en
regard, sans le faire ici, de la démographie et des capacités de formation,
hors périmètre de cette note.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 1. FINESS : le référentiel, sans aucune activité ni finances

`ref.finess_etablissement`, 103 022 établissements sanitaires et sociaux
(48 760 entités juridiques distinctes — une entité gère souvent plusieurs
sites). Source : ANS (Agence du numérique en santé), via data.gouv.fr.

**Réserve à poser dès l'ouverture** : le jeu de données utilisé
(`etalab_cs1100502`) est signalé comme remplacé par de nouveaux flux
quotidiens de l'ANS depuis le 20 juillet 2026, mais reste publié et à jour au
moment du chargement (édition du 12 mai 2026). Utilisé faute d'avoir confirmé
une URL stable pour le nouveau flux — à revoir dès qu'elle sera identifiée.

**Ce que la table ne dit pas, et qu'il ne faut pas lui faire dire** : le
champ de statut (`code_sph`/`libelle_sph`, censé distinguer public / privé
d'intérêt collectif / privé) n'est renseigné que pour **12 % des
établissements** (11 970 sur 103 022) — il ne concerne pleinement que les
établissements de santé au sens strict (hôpitaux, cliniques), pas le
médico-social (EHPAD, établissements pour personnes handicapées) qui
compose l'essentiel du fichier. En déduire une répartition public/privé
exhaustive serait une extrapolation non fondée.

| Catégorie (agrégat) | Établissements |
|---|---:|
| Commerce de biens à usage médical (pharmacies, etc.) | 21 035 |
| Établissements et services multi-clientèles | 12 998 |
| Établissements d'hébergement pour personnes âgées (EHPAD, etc.) | 9 894 |
| Établissements et services d'hébergement pour adultes handicapés | 5 305 |
| Laboratoires de biologie médicale | 4 562 |

**Ce que cette table permet pour la suite** : `nofinesset` devient la clé sur
laquelle toute source secondaire (SAE, PMSI, DAMIR, RPPS) pourra se joindre
sans dupliquer l'identification d'un établissement — le même rôle que
`ref.commune` pour le volet territorial. `code_insee` (département +
commune FINESS concaténés par ce connecteur, à ne pas confondre avec un code
INSEE publié tel quel par la source) prépare une jointure géographique
future.

#### 1.1 Le personnel hospitalier par fonction (SAE)

`core.sae_personnel_fonction`, 3 808 établissements, 2024. Source : Drees,
SAE (Statistique annuelle des établissements de santé) — publiée sous forme
d'une archive `.7z` contenant une cinquantaine de bordereaux thématiques
(lits, activité par discipline, équipements, personnel), pas d'un jeu
tabulaire directement interrogeable. Chargeable depuis que 7-zip est
installé sur la machine de build ; `internal/sante/sae.go` est le seul
connecteur de ce dépôt qui invoque un binaire externe plutôt que du Go pur.

**Un bug de comptage trouvé et corrigé avant publication** : le fichier
source (bordereau Q24) publie, par établissement, une ligne par discipline
médicale **plus** une ligne portant le code 9999, déjà la somme des
précédentes (vérifié : établissement 010000024, disciplines 1000 + 2000 =
ligne 9999, à l'ETP près). Une première version de ce chargement additionnait
toutes les lignes et obtenait 3,01 millions d'ETP nationaux — trois fois le
chiffre plausible. Seule la ligne 9999 (le total déjà calculé par la Drees)
est retenue :

| | ETP |
|---|---:|
| Infirmiers (avec et sans spécialisation) | 328 700 |
| Aides-soignants | 235 496 |
| Administratifs et techniques | 284 577 |
| **Total personnel non médical, tous établissements SAE** | **1 088 252** |

**Ce total ne couvre que les 3 808 établissements répondant à la SAE**
(essentiellement les établissements de santé au sens strict), pas les
103 022 établissements de `ref.finess_etablissement`, dont la majorité sont
médico-sociaux et hors du champ de cette enquête.

#### 1.2 FINESS change de format : ce que la suite (ANS) publie déjà

Au moment de l'écriture, l'Agence du numérique en santé publie déjà les
deux jeux qui remplaceront `etalab_cs1100502` (§ 1) : **`finess-structures-1`**
et **`finess-activites-1`**, en JSON quotidien et mensuel
(`finess-structures-mensuel-202608.json.gz`, etc.), sur data.gouv.fr.
**Non repris dans cette version** : le nouveau format n'est pas une évolution
mineure du fichier plat actuel — c'est un modèle imbriqué (`pmej` : personne
morale/entité juridique, 98 193 entrées dans l'édition consultée ;
`ege` : établissement géographique, imbriqué par `pmej` ; `gco`/`gcc` :
groupements de coopération) qui distingue explicitement l'entité juridique du
site géographique, là où le fichier actuel les juxtapose sur une seule ligne.
Migrer vers ce format demande de modéliser cette hiérarchie proprement,
plutôt que de la forcer dans le schéma plat de `ref.finess_etablissement` —
un chantier à part, pas une mise à jour d'URL.

#### 1.3 La certification HAS : la seule mesure de qualité comparable

`core.certification_has_demarche` / `core.certification_has_chapitre`,
422 démarches de certification (6ᵉ cycle, 2025-), 421 rejointes à
`ref.finess_etablissement` par leur numéro FINESS. Source : Haute Autorité de
Santé, trois fichiers CSV normalisés et légers (quelques centaines de Ko
chacun) — sans commune mesure avec la complexité de SAE ou de FINESS lui-même.

| Décision de certification | Démarches |
|---|---:|
| Certifié | 239 |
| Certifié avec mention | 88 |
| Certifié sous conditions | 68 |
| Non certifié | 27 |

Chaque démarche porte un score sur 100 par chapitre du référentiel :

| Chapitre | Score moyen |
|---|---:|
| Le patient | 92,4 |
| Les équipes de soins | 94,2 |
| L'établissement | 91,6 |

**Seul le 6ᵉ cycle est chargé** — les cycles antérieurs suivent un référentiel
différent, publiés séparément, non comparables terme à terme sans un travail
de correspondance non fait ici.

#### 1.4 RPPS : le pont vers les professionnels, et une population plus large qu'il n'y paraît

`core.rpps_professionnel_activite` (Annuaire Santé, ANS, extraction en libre
accès) : **2 286 272 lignes d'activité pour 1 912 833 professionnels
distincts.** Une ligne par activité déclarée, pas par personne — un même
identifiant (`identifiant_pp`) revient sur plusieurs lignes s'il exerce sur
plusieurs sites ou avec plusieurs rôles ; ne jamais compter les lignes comme
un nombre de professionnels.

| Profession | Professionnels distincts |
|---|---:|
| Infirmier | 627 962 |
| Médecin | 391 578 |
| Psychologue | 119 104 |
| Masseur-kinésithérapeute | 117 782 |
| Pharmacien | 81 903 |
| Chirurgien-dentiste | 64 452 |

**391 578 « Médecin », un chiffre plus haut que les ~226 000 médecins en
activité habituellement cités (Ordre des médecins) — parce que le RPPS
enregistre une population plus large que « les médecins en exercice ».**
Décomposé par catégorie professionnelle : 346 695 relèvent du régime
« Civil », mais **43 105 sont des étudiants** (internes et externes en
formation, qui détiennent un numéro RPPS sans être médecins en exercice) et
2 331 sont des agents publics. Ce dossier cite le chiffre RPPS tel qu'il est
— une mesure d'enregistrement, pas d'activité — plutôt que de le corriger
par un filtre qui suppose une définition de « en exercice » que la table ne
porte pas explicitement.

**Le pont vers FINESS ne couvre qu'une minorité de lignes** : 942 448 lignes
sur 2 286 272 (41 %) portent un numéro FINESS de site. Le reste correspond
très probablement à l'exercice libéral hors structure (cabinet individuel),
qui n'a pas de numéro FINESS par construction — pas une donnée manquante à
corriger.

#### 1.5 PMSI-MCO : l'activité hospitalière, sur le bon portail

**ScanSanté (`scansante.fr/opendata`), longtemps cité comme l'unique porte
d'entrée du PMSI, est une application interactive sans export de fichier à
adresse stable.** Le vrai point d'entrée ouvert de l'ATIH pour ces mêmes
familles de chiffres est un second portail, moins connu :
**data-essentiel.atih.sante.fr**, un Opendatasoft comme celui de la Depp ou
de la Drees déjà utilisés ailleurs dans ce dépôt, sous licence ODbL.

`core.pmsi_mco_national`, `core.pmsi_mco_par_etablissement` et
`core.pmsi_mco_par_patient` (champ Médecine-Chirurgie-Obstétrique
seulement — pas SMR, HAD ni psychiatrie, publiés séparément et non chargés
ici) :

| Année | Séjours (Tous) | dont hospitalisation complète | dont ambulatoire | Durée moyenne (Tous) |
|---|---:|---:|---:|---:|
| 2023 | 19 703 163 | 9 587 483 | 10 115 680 | 3,68 j |
| 2024 | 20 431 758 | 9 699 182 | 10 732 576 | 3,61 j |
| 2025 | 21 207 964 | 9 762 710 | 11 445 254 | 3,51 j |

**« Tous » est la somme exacte des deux types d'hospitalisation, vérifiée
ligne à ligne à l'ingestion** — un contrôle systématique plutôt qu'une
confiance a priori dans la cohérence de la source. La durée moyenne de
séjour en ambulatoire est mécaniquement de 1 jour (un séjour ambulatoire est
par définition sans nuitée) : ce n'est pas une amélioration de la prise en
charge, c'est une définition.

**1 514 établissements** portent une activité MCO en 2025
(`core.pmsi_mco_par_etablissement`, 18 régions dont l'outre-mer). L'Île-de-
France et l'Auvergne-Rhône-Alpes concentrent les plus gros volumes de
séjours en établissement public — cohérent avec leur poids démographique,
que cette note ne rapporte pas ici faute d'avoir chargé une population de
référence par région dans ce même chargement.

## Ce que les données ne disent pas

### 3. Ce que ce dossier ne couvre pas encore

Elle ne dit pas combien un médecin gagne en euros — seulement dans quel
système de tarification il exerce. Le montant des dépassements et des
honoraires perçus existe dans un jeu de données distinct identifié
(`honoraires` sur `data.ameli.fr`), non chargé à ce stade : ses champs
« moyens » codent l'absence de donnée par la valeur littérale « NS » (non
significatif, secret statistique sur petit effectif) mêlée à des valeurs
numériques dans la même colonne — un traitement plus délicat que le
chargement fait ici, laissé à une prochaine itération plutôt que bâclé.

Deux autres limites, plus courtes :

- **DECP** pour les fournisseurs des établissements publics de santé : même
  limite que pour Éducation et Défense — la table `core.public_contract`
  existe (`docs/perimetre.md` § 4.4, priorité P2) mais aucun connecteur ne
  l'alimente encore.
- **PMSI, autres champs** (SMR, HAD, psychiatrie) : publiés séparément par
  l'ATIH sur le même portail que le MCO chargé au § 1.5, non explorés.

### 4. SNDS/Open Damir : localisé précisément, écarté pour sa taille

**Open Damir (l'extraction ouverte du Système national des données de
santé) est trouvé, accessible, sous Licence Ouverte — et délibérément non
chargé.** Chaque mois se télécharge en un fichier CSV compressé d'environ
**970 Mo** (vérifié sur janvier 2025), soit plus de 10 Go par année
complète ; le dépôt couvre 2009 à 2025. Le contenu, inspecté directement,
n'est pas un agrégat léger malgré la limitation à 9 puis 13 zones
géographiques annoncée pour préserver l'anonymat : chaque ligne croise déjà
mois, zone, âge, régime, nature de prestation et une **cinquantaine de
colonnes** de codes et de montants — une granularité comparable à celle des
DECP (2,5 Go), déjà écartées pour la même raison
(`docs/perimetre.md` § 4.4). Charger ne serait-ce qu'une année demanderait
un agrégat calculé en amont, pas un chargement direct — un chantier à part,
pas une extension de celui-ci.

**Ce que ce dossier a maintenant, contre ce qu'Open Damir aurait ajouté** :
le secteur conventionnel (§ 2, déjà chargé) donne la RÉPARTITION des
médecins par régime tarifaire ; Open Damir aurait donné les MONTANTS
remboursés par prestation. Les deux questions restent disjointes tant que
ce second chargement n'est pas fait.

## Sources

- ANS (Agence du numérique en santé), *FINESS — extraction du fichier des
  établissements*, data.gouv.fr ; *FINESS — Structures* et
  *FINESS — Activités* (nouveau format, § 1.2).
- Drees, *SAE — bases statistiques*, data.drees.solidarites-sante.gouv.fr.
- Cnam (Caisse nationale de l'Assurance Maladie), *Démographie des
  professionnels de santé libéraux par secteur conventionnel*,
  data.ameli.fr.
- Haute Autorité de Santé, *Certification des établissements de santé pour la
  qualité des soins (6ᵉ cycle)*, data.gouv.fr.
- ANS, *Annuaire Santé — extractions RPPS en libre accès*, data.gouv.fr
  (§ 1.4).
- ATIH, *MCO — Chiffres clés*, *Activité par type d'établissement*,
  *Caractéristique des patients hospitalisés*, data-essentiel.atih.sante.fr
  (§ 1.5).
- CNAM, *Open Damir : base complète sur les dépenses d'assurance maladie
  interrégimes*, data.gouv.fr (§ 4, non chargé).

## Annexe technique

### 5. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | ANS, référentiel FINESS des établissements | `ref.finess_etablissement` | 103 022 lignes |
| 2 | Drees, SAE, bordereau Q24 (personnel par fonction) | `core.sae_personnel_fonction` | 3 808 lignes, 2024 |
| 3 | Cnam, démographie par secteur conventionnel | `core.medecin_secteur_effectif` | 177 720 lignes, 2010-2024 |
| 4 | HAS, certification des établissements (6ᵉ cycle) | `core.certification_has_demarche`, `core.certification_has_chapitre` | 422 démarches, 981 résultats |
| 5 | ANS, Annuaire Santé (RPPS) | `core.rpps_professionnel_activite` | 2 286 272 lignes, 1 912 833 professionnels distincts |
| 6 | ATIH, PMSI-MCO (data-essentiel) | `core.pmsi_mco_national`, `core.pmsi_mco_par_etablissement`, `core.pmsi_mco_par_patient` | 15 + 245 + 1 400 lignes, 2021-2025 |

## Versions

- **Version 5** (15 septembre 2026) : RPPS chargé (§ 1.4, 1,9 million de
  professionnels) et PMSI-MCO chargé (§ 1.5) — le second sur un portail ATIH
  différent de celui longtemps cité (ScanSanté), trouvé en cherchant
  directement les jeux de données de l'ATIH plutôt qu'en creusant son
  interface interactive. Open Damir localisé précisément et écarté pour sa
  taille (§ 4), avec les chiffres qui le justifient plutôt qu'une simple
  mention.
- **Version 4** (15 septembre 2026) : plan commun des dossiers (D-066) ; cadre (loi de 2019) et enjeu budgétaire de la branche maladie (Sénat).
- **Version 3** (15 septembre 2026) : certification HAS des établissements (§ 1.3).
- **Version 2** : personnel hospitalier par fonction (SAE, § 1.1) ; suite de FINESS publiée par l'ANS (§ 1.2) ; accès PMSI et RPPS (§ 4). Les effectifs d'AESH relèvent du dossier éducation.
