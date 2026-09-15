# La santé : FINESS comme clé pivot, la rémunération des médecins et les déserts médicaux

> **Dossier** · version 8 · 15 septembre 2026
>
> Comment le système de santé est-il décrit par les données publiques, comment les
> médecins sont-ils rémunérés, et que disent les données sur les déserts médicaux au-delà
> de la seule densité ? Les sources sont les plus éclatées du projet — établissements,
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

**Une lacune géographique réelle, trouvée en construisant la carte des
généralistes (§ 1.7)** : le champ « code département » de la structure
d'exercice est vide sur la quasi-totalité des lignes, et la commune
d'exercice elle-même n'est renseignée que pour 61,7 % des généralistes
(94 149 sur 152 598) — les remplaçants sans structure fixe, notamment, n'en
ont pas. Une fois jointe au millésime courant du code officiel géographique,
la couverture retombe encore à 91 654 généralistes localisables (56 %). Pas
un bug de jointure : une caractéristique du fichier RPPS lui-même, qui a
conduit à préférer la démographie Cnam par secteur conventionnel (§ 2) pour
la carte de densité — voir § 1.7.

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

#### 1.6 Open Damir : les remboursements de l'Assurance Maladie, agrégés en flux

Chargé depuis la version précédente de ce dossier (qui l'avait localisé et
écarté pour sa taille — 970 Mo compressés par mois, plus de 10 Go par année).
`core.remboursement_national` et `core.remboursement_region_prestation`
(Cnam, Open Damir, licence Ouverte, `data.gouv.fr`) : **147,1 Md€ remboursés
en 2025, 10,74 milliards d'actes** (12 mois, 101 980 lignes région×prestation).

**Jamais chargé ligne à ligne** : chaque fichier mensuel (36,6 millions de
lignes en janvier 2025) est agrégé en flux à l'ingestion
(`internal/damir/damir.go`) — seuls les totaux mensuels et les croisements
région×prestation sont écrits en base, jamais les lignes brutes.

| Mois 2025 | Md€ remboursés | Actes |
|---|---:|---:|
| Janvier | 12,1 | 1 021 523 048 |
| Juin | 12,6 | 881 056 449 |
| Décembre | 13,5 | 944 000 290 |
| **Total 12 mois** | **147,1** | **10 737 880 752** |

**Deux choix de méthode, documentés dans la table plutôt que découverts par
un chiffre qui ne colle pas** :

- **Filtré sur `PRS_REM_TYP = 0`** — le filtre que le descriptif des
  variables de la Cnam documente explicitement, sans quoi chaque
  remboursement serait compté deux fois.
- **Agrégé par mois de traitement (`FLX_ANN_MOI`), pas par mois de soins**
  (`SOI_ANN`/`SOI_MOI`) : un fichier mensuel contient des soins de centaines
  de mois différents (remboursements tardifs), qu'un agrégat par mois de
  soins laisserait incomplet tant que les fichiers postérieurs ne sont pas
  relus. Le total mesure donc un flux de paiement, pas la consommation de
  soins du mois calendaire correspondant.

**Ce que ce total ne dit pas, et qu'il ne faut pas lui faire dire** : 147,1
Md€ est loin des 267,5 Md€ votés pour l'ensemble de la branche maladie 2026
(Enjeux, ci-dessus) — les deux chiffres ne sont **pas comparables tels
quels**. Open Damir recense des remboursements itemisés acte par acte (soins
de ville, actes techniques, pharmacie, dispositifs) ; il ne capte pas les
dotations forfaitaires versées aux établissements de santé (budget global
hospitalier — DAF, MIGAC — une part importante de l'ONDAM), qui ne
transitent pas acte par acte dans ce système. Ce chiffre est une donnée
mesurée, pas une vérification de l'ONDAM : le rapprochement précis des deux
périmètres reste à faire, pas à deviner.

**La zone de résidence (`BEN_RES_REG`) est décodée** (`ref.damir_region`,
migration 0107) — depuis le lexique des variables que la Cnam publie
elle-même (feuille « MOD OPEN DAMIR »), pas une correspondance devinée à
partir des codes INSEE usuels. Deux écarts réels que deviner aurait
manqués : un seul code regroupe tous les DOM (« 5 », Guadeloupe à Mayotte
confondues — à l'inverse de la démographie par secteur conventionnel, § 2,
qui les distingue), et un code « 99 » explicitement documenté « Inconnu »,
pas une absence de ligne :

| Région | Md€ remboursés, 2025 |
|---|---:|
| Inconnu (99) | 26,9 |
| Île-de-France | 19,7 |
| Auvergne-Rhône-Alpes | 13,7 |
| Provence-Alpes-Côte d'Azur et Corse | 13,1 |
| Occitanie | 12,3 |
| *(8 autres régions, 4,1 à 11,1 Md€)* | |
| DOM (code unique) | 4,5 |

**« Inconnu » porte le plus gros montant des quatorze codes (18 % du
total)** : la Cnam documente le code, pas ce qu'il recouvre — ce dossier ne
devine pas à sa place. Une piste plausible et non vérifiée : des
remboursements liquidés par des organismes ou régimes qui ne rattachent pas
systématiquement une région de résidence (cures thermales liquidées au lieu
de l'établissement quel que soit le domicile du bénéficiaire, régimes
« infogérés » — voir le commentaire de `ORG_CLE_REG` dans le même lexique).

**La nature de prestation (`PRS_NAT`, 937 codes distincts observés) reste,
elle, non décodée** — une nomenclature d'actes largement plus fine que
celle des régions, qui déborde le lexique consulté ici. Une carte des
remboursements par région existe désormais comme donnée (`ref.damir_region`
permet de la construire), mais reste à construire : ce dossier documente le
chiffre nouvellement décodé, pas encore une page du site.

#### 1.7 Déserts médicaux : la densité mesurée, pas la disponibilité

Trois sources de ce dossier éclairent chacune une face différente du même
débat (333 mentions à l'Assemblée depuis juillet 2024, Contexte ci-dessus),
sans qu'aucune ne le résume à elle seule :

- **Où sont les généralistes** : la carte « Médecins généralistes pour
  100 000 habitants » (`/collectivites/`) s'appuie sur la démographie Cnam
  par secteur conventionnel (§ 2), pas sur le RPPS (§ 1.4) — trouvaille faite
  en construisant cette carte. Le RPPS ne situe géographiquement que 56 % des
  généralistes (§ 1.4) ; la source Cnam couvre les 101 départements sans
  exception, par construction administrative (un médecin conventionné est
  rattaché à une caisse départementale), au prix d'un champ plus étroit
  (55 546 généralistes libéraux conventionnés en 2024 — ni les salariés
  hospitaliers, ni les 0,8 % non conventionnés).
- **Où se fait l'activité hospitalière** : le PMSI-MCO (§ 1.5) mesure des
  séjours hospitaliers par région, une population de patients différente de
  celle qui consulte un généraliste en ville — à ne jamais additionner à la
  carte de densité pour prétendre mesurer « l'offre de soins » globale d'un
  territoire.
- **Où va l'argent remboursé** : Open Damir (§ 1.6) le dit maintenant à
  l'échelle région (`ref.damir_region`, § 1.6), mais sur une géographie
  volontairement grossière (14 zones, DOM confondus, un code Inconnu) —
  utilisable pour un ordre de grandeur régional, pas pour une carte
  départementale comparable à celle des généralistes.

**Ce que la carte de densité ne mesure pas** : un décompte de présence, pas
une disponibilité. Un département dense en généralistes recensés peut avoir
des cabinets qui n'ouvrent plus de nouveaux dossiers de patientèle ; un
département moins dense peut être bien desservi si sa patientèle est plus
jeune, en meilleure santé, ou mieux couverte par la télémédecine — aucune de
ces dimensions n'est mesurée par les sources chargées ici. La densité situe
un débat, elle ne le tranche pas.

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

Une limite plus courte :

- **PMSI, autres champs** (SMR, HAD, psychiatrie) : publiés séparément par
  l'ATIH sur le même portail que le MCO chargé au § 1.5, non explorés.

**DECP, désormais chargées (transversalement, pas seulement pour la santé)** :
`core.public_contract` — vide au moment de la version précédente de ce
dossier — est maintenant alimentée pour l'ensemble de la commande publique
française, établissements de santé compris ; voir
[docs/commande-publique-donnees.md](commande-publique-donnees.md). Filtrer
`acheteur_siret` sur les FINESS des établissements publics de santé
(`ref.finess_etablissement.siret`) donnerait leurs fournisseurs — une
jointure encore à écrire, pas encore un tableau de ce dossier.

### 4. SNDS/Open Damir : chargé depuis la version 7 — voir § 1.6

Les versions 5 et 6 de ce dossier localisaient Open Damir sans le charger,
pour sa taille (970 Mo compressés par mois). L'agrégation en flux décrite au
§ 1.6 lève cette limite : 147,1 Md€ remboursés et 10,74 milliards d'actes
pour 2025, avec les deux réserves qui restent ouvertes — le rapprochement
avec l'ONDAM total n'est pas fait (périmètres différents), et le détail
région×prestation n'a pas de nomenclature de décodage chargée.

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
  interrégimes*, `open-data-assurance-maladie.ameli.fr` (§ 1.6).

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
| 7 | Cnam, Open Damir (remboursements interrégimes, agrégés en flux) | `core.remboursement_national`, `core.remboursement_region_prestation` | 12 lignes + 101 980 lignes, 2025 |
| 8 | Cnam, lexique Open Damir (nomenclature BEN_RES_REG) | `ref.damir_region` | 14 lignes |

## Versions

- **Version 8** (15 septembre 2026) : nomenclature des régions Open Damir
  décodée (§ 1.6, `ref.damir_region`, migration 0107) depuis le lexique des
  variables publié par la Cnam plutôt que devinée — deux écarts réels
  qu'une supposition aurait manqués (un code unique pour tous les DOM, un
  code « 99 » explicitement « Inconnu », qui porte le plus gros montant
  des quatorze). `PRS_NAT` (nature de prestation, 937 codes) reste non
  décodé — nomenclature d'actes hors du lexique consulté.
- **Version 7** (15 septembre 2026) : Open Damir chargé (§ 1.6, 147,1 Md€
  et 10,74 milliards d'actes sur 2025, agrégé en flux — jamais ligne à
  ligne) ; carte des généralistes reconstruite sur la démographie Cnam par
  secteur conventionnel plutôt que le RPPS, qui ne situait que 56 % d'entre
  eux (§ 1.4, § 1.7) ; nouvelle section déserts médicaux (§ 1.7) croisant
  densité, activité hospitalière et remboursements sans les confondre.
- **Version 6** (15 septembre 2026) : DECP chargées (§ 3,
  [docs/commande-publique-donnees.md](commande-publique-donnees.md)) — la
  limite « aucun connecteur ne l'alimente » citée en version 5 ne tient
  plus ; quatre usages concrets d'Open Damir explicités plutôt qu'une
  mention de principe.
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
