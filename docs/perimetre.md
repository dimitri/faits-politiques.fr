# Périmètre du projet

> Document de cadrage. Version 1 — 11 septembre 2026.
> Il définit ce que l'outil fait, ce qu'il ne fait pas, et pourquoi.
> Toute fonctionnalité absente de ce document est hors périmètre jusqu'à révision explicite.

---

## 1. Objectif

Permettre de **vérifier ce que les partis politiques font quand ils votent et quand ils
dirigent une commune**, à partir de sources primaires, dans le contexte d'usage suivant :

- un citoyen sort d'un débat télé ou radio et veut contrôler une affirmation ;
- un journaliste doit sourcer ou contredire une affirmation sous contrainte de temps ;
- un élu ou un attaché parlementaire doit contester une erreur ou une insinuation
  formulée par un adversaire ou par un commentateur.

Ce troisième usage est structurant : **l'outil sera utilisé de façon contradictoire, par des
parties qui s'opposent.** Il doit donc être conçu pour résister à l'attaque, pas seulement
pour informer.

### Formulation opérationnelle

> Partir d'une affirmation entendue en public, et remonter en moins d'une minute
> au scrutin, à l'amendement ou à l'indicateur officiel qui permet de la contrôler —
> ou à la raison explicite pour laquelle elle n'est pas contrôlable.

---

## 2. Principes de conception

### 2.1 Aucun verdict

L'outil ne produit **jamais** de « vrai / faux ». Il produit un objet factuel, sa source
primaire, et ses limites explicites. La conclusion appartient au lecteur.

Raison : un verdict est attaquable, et une seule erreur de verdict détruit la crédibilité de
l'ensemble. Un document sourcé n'est pas attaquable de la même façon.

### 2.2 Le « non vérifiable » est une réponse de plein droit

Chaque réponse négative est **typée** et affichée comme telle :

| Code | Signification |
|---|---|
| `NO_ROLL_CALL` | Aucun scrutin public sur cet objet — le vote a eu lieu à main levée, sans trace nominative |
| `GROUP_LEVEL_ONLY` | Position connue au niveau du groupe seulement, pas de l'individu |
| `NOT_PRODUCED` | Aucun producteur public ne mesure cette donnée à ce niveau |
| `EPCI_COMPETENCE` | La compétence relève de l'intercommunalité, la commune ne décide pas |
| `OUT_OF_CORPUS` | Déclaration hors corpus (plateau TV, meeting, réseaux sociaux) |
| `OUT_OF_PERIOD` | Hors de la profondeur historique couverte |
| `AMBIGUOUS_SUBJECT` | L'affirmation ne se rattache pas à un objet identifiable |

Afficher l'absence protège mieux que la taire.

### 2.3 Jamais de causalité

Un indicateur communal décrit **une évolution pendant un mandat**, pas l'effet d'une
politique. Toute page portant un indicateur local affiche cette réserve de façon
non dissimulable.

### 2.4 Jamais de groupe projeté sur l'individu

Quand la source ne donne que la position d'un groupe (cas du Sénat), l'outil ne dit
jamais comment un sénateur a voté. La granularité est une propriété stockée de chaque
scrutin (`core.scrutin.granularite`), pas une convention implicite.

### 2.5 Pré-enregistrement et symétrie

Toute comparaison entre étiquettes est régie par un protocole scellé **avant le premier
calcul** : population, indicateurs, méthode, règles de publication
(voir [pre-enregistrement-001.md](pre-enregistrement-001.md)). Sans cela, tout écart
observé est attribuable au choix des indicateurs, et l'objection est irréfutable même
lorsqu'elle est fausse.

Corollaire : la même page, avec les mêmes indicateurs et les mêmes réserves, est produite
pour **toutes** les étiquettes éligibles. Une page qui n'existerait que pour une seule
organisation est un défaut, pas un choix éditorial — et la vue
`selection.template_coverage` la rend visible.

### 2.6 Un dossier est un filtre, pas un texte

Il n'y a aucune prose dans ce produit. Un dossier est une **liste de faits réunie pour
être relue facilement** dans un contexte donné, sans avoir à parcourir tout le corpus.
Il vit dans un schéma séparé (`selection`) qui ne référence que `core` et `derived`,
jamais l'inverse.

Conséquence : tout ce que produit un utilisateur est de la donnée structurée, donc
**aucun compte n'est nécessaire nulle part** — ni pour consulter, ni pour créer, ni pour
publier. Voir [contributions-utilisateurs.md](contributions-utilisateurs.md).

Mais le biais ne disparaît pas, il se déplace : sans prose, **le filtre est l'argument**.
D'où la garantie qui remplace l'obligation de citation — **toute sélection déclare sa
sélectivité**, calculée par le système et non par l'auteur. Un filtre ne peut être ni
gelé ni listé tant qu'il n'affiche pas combien de faits il retient sur combien
d'éligibles.

### 2.7 Toute page est scellée et citable

Permalien immuable, empreinte SHA-256 du document primaire, date de consultation,
version datée de la page. Un screenshot doit être authentifiable.

---

## 3. Ce que l'outil peut réellement vérifier

Taxonomie des affirmations entendues en débat, confrontée aux données existantes.
**C'est le seul critère de cadrage qui compte.**

| Affirmation type | Vérifiable | Source | Réserve |
|---|---|---|---|
| « Le groupe X a voté contre la loi Y » | ✅ si scrutin public | AN – Scrutins | La majorité des votes sont à main levée → `NO_ROLL_CALL` fréquent |
| « Le député X a voté contre » | ✅ AN et Sénat | Relevés nominatifs | Voir [D-016](decisions.md) : contrairement à ce que ce document affirmait, le Sénat publie bien les votes individuels |
| « X a déposé / proposé Y » | ✅ | AN – Amendements, Dossiers | Dimension inexploitée par les outils existants |
| « X a dit Y à l'Assemblée » | ✅ | AN – Comptes rendus | Hémicycle + une partie des commissions |
| « X a dit Y à la télé » | ❌ | — | `OUT_OF_CORPUS`. Aucun corpus, aucune intention d'en créer un |
| « Le parti X a voté N % avec le parti Y » | ⚠️ | AN – Scrutins | Le chiffre dépend du jeu de scrutins. Ne jamais publier sans périmètre explicite et reproductible (§5.2) |
| « X a augmenté les impôts » | ✅ | DGFiP – REI, taux votés | Taux ≠ montant payé (bases, exonérations, part EPCI) |
| « X a explosé la dette / n'investit pas » | ✅ | OFGL | Inutilisable sans appariement strate/région/revenu |
| « X n'a pas construit de logement social » | ✅ | RPLS, inventaire SRU | Décalage important entre décision et livraison |
| « La délinquance a explosé sous X » | ✅ série / ❌ imputation | SSMSI, base communale | Faits **enregistrés** ≠ faits commis. Afficher, jamais imputer |
| « X a versé N € à l'entreprise / l'association Y » | ⚠️ partiel | DECP consolidées | Couverture inégale, SIRET parfois manquants |
| « X a supprimé la cantine gratuite » | ❌ | Délibérations | Hors périmètre définitif (§6.1) |
| « X était absent N fois » | ❌ par choix | AN – Présences | Techniquement possible, structurellement trompeur. **Non produit** |
| « X avait promis Y » | ❌ | Programmes | Corpus manuel non maintenable |

### Taux de couverture attendu

**25 à 35 % des affirmations d'un débat télé.** Ce chiffre est une hypothèse de
conception, pas un objectif à maximiser. Il doit être mesuré sur un corpus réel de débats
dès la V1 et publié.

Un outil qui résout un tiers des affirmations de façon inattaquable est une référence.
Un outil qui prétend en résoudre 90 % est disqualifié à la première contestation.

---

## 4. Matrice des sources

Statuts : **P1** = socle V1, **P2** = socle V2, **P3** = ultérieur.

### 4.1 Parlement

| Source | Contenu | Granularité | Profondeur | Fréquence | Format | Licence | Priorité |
|---|---|---|---|---|---|---|---|
| [data.assemblee-nationale.fr](https://data.assemblee-nationale.fr/) — Acteurs & Organes | Députés, mandats, groupes, commissions | Individu | 14e lég. → | Continue (non contractuelle) | XML, JSON | Licence Ouverte | **P1** |
| AN — Scrutins | Scrutins publics, **positions nominatives** | Individu | 14e lég. → | Par séance | XML, JSON | Licence Ouverte | **P1** |
| AN — Amendements | Amendements, auteurs, cosignataires, sort | Individu | 14e lég. → | Continue | XML, JSON | Licence Ouverte | **P1** |
| AN — Dossiers législatifs | Dossiers, textes, lectures | Dossier | 14e lég. → | Continue | XML, JSON | Licence Ouverte | **P1** |
| AN — Comptes rendus | Débats en séance, texte intégral | Intervention | 14e lég. → | Par séance | XML | Licence Ouverte | P2 |
| [data.senat.fr](https://data.senat.fr/) — Sénateurs, Dosleg, Ameli | Sénateurs, dossiers (depuis 1977), amendements | Individu / dossier | 1977 → | Périodique | **Dump PostgreSQL**, CSV, XML Akoma Ntoso | Licence Ouverte | P2 |
| Sénat — base Dosleg | Sénateurs, scrutins, **votes nominatifs** (1,65 M depuis 2006), dossiers, **30 thèmes officiels** | Individu | 2006 → | Périodique | Dump PostgreSQL | Licence Ouverte | **Ingéré** |
| [HowTheyVote.eu](https://howtheyvote.eu/) | Scrutins nominatifs Parlement européen | Individu | 2019 → | Hebdomadaire | CSV, API | ODbL (code GPLv3) | P3 |

**Piège majeur — les dumps NosDéputés/NosSénateurs sont en CC BY-NC-SA.** Ils
interdiraient toute réutilisation commerciale et contamineraient les données dérivées.
**Ne pas les ingérer.** Aller aux sources primaires AN et Sénat, en Licence Ouverte, qui
autorisent explicitement l'usage commercial.

### 4.2 Élus locaux et territoires

| Source | Contenu | Granularité | Profondeur | Fréquence | Format | Licence | Priorité |
|---|---|---|---|---|---|---|---|
| [RNE](https://www.data.gouv.fr/datasets/repertoire-national-des-elus-1) | Maires et conseillers municipaux : identité, commune, CSP, dates de mandat. **Aucune nuance politique** | Individu | Mandature courante | Trimestrielle | CSV | Licence Ouverte | **P2** |
| [Résultats des élections municipales](https://www.data.gouv.fr/organizations/ministere-de-l-interieur/) (Intérieur) | **Nuance de chaque liste**, voix, sièges au conseil municipal | Commune × liste | Par scrutin, 2001 → 2026 | Par scrutin | CSV | Licence Ouverte | **P2** |
| Dictionnaire des nuances | Nomenclature des nuances politiques | — | Par circulaire | Par circulaire | CSV | Licence Ouverte | **P2** |
| INSEE — COG | Communes, fusions, scissions, millésimes | Commune | Historique | Annuelle | CSV | Licence Ouverte | **P2** |
| INSEE — Population, revenus (Filosofi) | Population municipale, revenu médian | Commune | Annuelle | Annuelle | CSV | Licence Ouverte | **P2** |
| [BANATIC](https://www.banatic.interieur.gouv.fr/) | Périmètres EPCI, **125 compétences transférées**, président de chaque groupement | EPCI × commune | Annuelle | Continue | API JSON + export XLSX | Licence Ouverte | **chargé** |
| [Résultats municipaux 2020](https://www.data.gouv.fr/datasets/municipales-2020-resultats-2nd-tour) (Intérieur) | Nuance de chaque liste, voix, sièges — **mandature 2020-2026** | Commune × liste | 2020 | Par scrutin | TXT (ISO-8859-1) | **Non déclarée** — usage autorisé par D-036, classée RESTRICTED | **chargé** |

**La mandature municipale courante court depuis mars 2026** (renouvellement général des
15 et 22 mars 2026). Le RNE a été actualisé en août 2026. La profondeur historique du RNE
est faible : reconstituer les mandatures antérieures demande les fichiers de résultats
électoraux du ministère de l'Intérieur, à traiter séparément.

**La nuance ne vient pas du RNE.** Le fichier public des maires ne porte que
l'identité, la commune, la catégorie socio-professionnelle et les dates de mandat :
quatorze colonnes, aucune politique. La nuance est attribuée par les préfectures
aux **listes candidates**, et n'est publiée que dans les fichiers de résultats du
ministère de l'Intérieur. Relier un maire à une nuance suppose donc de passer par
la liste qui a remporté la majorité des sièges au conseil municipal — une étape de
plus, et une décision de plus (voir D-022).

**Piège nuances politiques**, mesuré sur le scrutin de mars 2026 :

| | communes | part |
|---|---|---|
| avec au moins une liste nuancée | 3 282 | **9,4 %** |
| sans aucune nuance | 31 553 | **90,6 %** |

Le seuil est de population : parmi les communes nuancées, le minimum observé est
1 064 inscrits ; parmi les non nuancées, le maximum est 4 343. **Neuf communes sur
dix n'ont aucune étiquette politique, et ce n'est pas un défaut de collecte : c'est
la règle.**

Et parmi les 9,4 % nuancées, les quatre nuances « divers » (LDVD, LDVG, LDVC, LDIV)
représentent **85,9 %** des majorités élues. La nomenclature change à chaque
circulaire, d'où le millésime obligatoire. **Toute comparaison par étiquette doit
exclure ces communes explicitement et afficher le taux d'exclusion.**

### 4.3 Indicateurs communaux

| Source | Contenu | Granularité | Profondeur | Fréquence | Format | Licence | Priorité |
|---|---|---|---|---|---|---|---|
| [OFGL](https://data.ofgl.fr/) | Comptes des communes : dette, investissement, fonctionnement, charges de personnel, épargne, **strates** (population, rural, QPV, revenu) | Commune × année | 2018 → | Annuelle | API Opendatasoft, CSV | Licence Ouverte | **chargé** |
| DGFiP — REI / taux votés | Taux de fiscalité directe locale | Commune × année | Longue | Annuelle | CSV, XLSX | Licence Ouverte | **P2** |
| [SSMSI](https://www.data.gouv.fr/datasets/bases-statistiques-communale-departementale-et-regionale-de-la-delinquance-enregistree-par-la-police-et-la-gendarmerie-nationales) | Délinquance enregistrée, 15 indicateurs (46 % des lignes sous secret statistique) | Commune × année | 2016 → | Annuelle | CSV.gz (5,2 M lignes) | Licence Ouverte | **chargé** |
| RPLS | Parc locatif social | Commune × année | Longue | Annuelle | CSV | Licence Ouverte | P2 |
| Inventaire SRU | Taux de logement social, communes carencées | Commune × période | Longue | Annuelle | CSV | Licence Ouverte | P2 |
| [DECP consolidées](https://www.data.gouv.fr/datasets/donnees-essentielles-de-la-commande-publique-consolidees-format-tabulaire) | Attributions de marchés, acheteur, titulaire, montant | Marché | 2018 → | Quotidienne | Parquet, CSV | Licence Ouverte | P2 |
| [Observatoire de la lecture publique](https://www.culture.gouv.fr/thematiques/livre-et-lecture/pour-les-professionnels-des-bibliotheques/donnees-sur-les-bibliotheques/observatoire-de-la-lecture-publique) | Bibliothèques : budget d'acquisition, effectifs, horaires, inscrits, prêts | Commune × année | Longue | Annuelle | CSV | Licence Ouverte | **P2** |
| [Rapports CRC](https://www.data.gouv.fr/datasets/rapports-dobservations-definitives-des-chambres-regionales-et-territoriales-des-comptes) | Observations définitives des chambres régionales des comptes + réponses | Collectivité | 2016 → | Au fil des contrôles | Texte intégral | Licence Ouverte | P2 |

**Les DECP sont déjà consolidées, dédoublonnées et enrichies** en amont (DECP-RAMA,
DECP-augmented). Ne pas refaire ce nettoyage : consommer la version consolidée.

**Piège général sur les indicateurs communaux** : il n'existe aucune donnée nationale sur
ce qu'une commune **a décidé**. Ces sources décrivent ce que les producteurs publics
**mesurent** chaque année. Le glissement de l'un à l'autre est la principale erreur à ne
jamais commettre (§2.3).

### 4.6 Comptes nationaux, entreprises et transparence

Ajoutées le 2026-09-12. Toutes chargées.

| Source | Contenu | Granularité | Profondeur | Format | Licence | État |
|---|---|---|---|---|---|---|
| [Eurostat — comptes nationaux](https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data) | Dette, solde, recettes fiscales, dépense par fonction COFOG, chômage, pauvreté | France × année | 1995 → | API JSON-stat | Réutilisation autorisée avec mention (2011/833/UE) | **chargé** |
| Eurostat — compte des sociétés (`nasa_10_nf_tr`) | **Dividendes versés, impôts acquittés, rémunération des salariés, EBE, valeur ajoutée** | Secteur institutionnel × année | **1971 →** | API JSON-stat | idem | **chargé** |
| [CNAF](https://data.caf.fr/) | Foyers allocataires du RSA | National × mois | 2016 → | API Opendatasoft | Licence Ouverte | **chargé** |
| [HATVP](https://www.hatvp.fr/livraison/merge/declarations.xml) | Déclarations d'intérêts et de patrimoine : activités professionnelles, mandats, rémunérations déclarées | Déclarant | continue | XML (87 Mo) | Licence Ouverte | **chargé** |
| [INPI / BCE — ratios financiers](https://data.economie.gouv.fr/explore/dataset/ratios_inpi_bce/) | CA, marge, EBE, EBIT, résultat net par SIREN et exercice | Société × exercice | 2016 → | API Opendatasoft | Licence Ouverte v2.0 | **chargé** |
| [GLEIF](https://api.gleif.org/api/v1/lei-records) | LEI ↔ **SIREN** (`registeredAs`) | Entité juridique | continue | API JSON | CC0 1.0 | **chargé** |
| [Sénat — répertoire des sénateurs](https://data.senat.fr/data/senateurs/ODSEN_GENERAL.csv) | Matricule, **date de naissance, date de décès**, groupe, circonscription, profession | Sénateur | historique | CSV (ISO-8859-1) | Licence Ouverte | **chargé** |
| [ESMA FIRDS](https://registers.esma.europa.eu/) | Instruments admis à la négociation, LEI de l'émetteur, place de cotation | Instrument | 2017 → | Solr + ZIP/XML | Ouvert | identifié, non chargé |

**Ce qui n'existe dans aucune source ouverte**, après recherche documentée
(D-034, D-035) : la composition du CAC 40 — indice propriétaire d'Euronext ; les
**dividendes versés par une société nommée** — seulement dans les rapports
annuels en PDF ; l'**impôt sur les sociétés payé par une société nommée** —
secret fiscal ; la **masse salariale par société** — même raison. Les trois
derniers existent en agrégat dans les comptes nationaux, et c'est à ce niveau
que le projet les documente.

### 4.5 Partis et affiliations — ce qui est officiel et ce qui ne l'est pas

Il n'existe **aucun registre national des adhérents** d'un parti, et il serait illégal
d'en constituer un : l'appartenance partisane est une opinion politique au sens de
l'article 9 du RGPD. Ce qui existe :

| Objet | Disponible | Source | Qualité |
|---|---|---|---|
| Liste des partis | ✅ | [CNCCFP — comptes des partis](https://www.data.gouv.fr/datasets/comptes-des-partis-et-groupements-politiques) | Identifiant CNCCFP par parti, comptes annuels. C'est le registre de fait |
| Groupe parlementaire d'un député | ✅ daté | AN — Acteurs & Organes | Excellent |
| Groupe d'un sénateur | ✅ daté | data.senat.fr | Excellent |
| **Parti d'un parlementaire** | ✅ **annuel** | Rattachement au titre de la seconde fraction de l'aide publique, déclaré en novembre et **publié au JO en décembre** par les bureaux des deux assemblées | Officiel mais à granularité annuelle : un changement en cours d'année n'apparaît qu'au décembre suivant |
| Groupe + parti national d'un eurodéputé | ✅ | Parlement européen | Excellent |
| **Étiquette d'un maire** | ⚠️ | Résultats électoraux (Intérieur) — nuance de la liste majoritaire ; **jamais le RNE, qui n'en publie aucune** | **La nuance qualifie une LISTE, pas une personne, et c'est une qualification préfectorale, pas une adhésion.** Absente dans 90,6 % des communes, « divers » dans 85,9 % du reste |
| Parti d'un ministre | ❌ | — | Les décrets de nomination ne portent pas d'appartenance. Déclaratif uniquement |

Le rattachement publié au JO est la seule affiliation partisane individuelle officielle
en France, et elle est très peu exploitée.

**Conséquence directe sur les dossiers par organisation :** il n'existe pas un objet
« parti » unique permettant un dossier homogène du national au local. Il en existe
**trois**, qui ne doivent jamais être fusionnés :

1. le **groupe parlementaire** — le mieux fondé, daté au jour près ;
2. le **parti** au sens du rattachement JO — fiable, granularité annuelle ;
3. la **nuance** attribuée par la préfecture à une liste candidate — administrative,
   seuillée en population, sans équivalence avec le parti.

Écrire « les communes RN » est donc un raccourci. La formulation défendable est
« les communes où la liste arrivée en tête en sièges portait la nuance LRN à
l'élection de mars 2026, circulaire de millésime Y ».
Le schéma impose déjà cette distinction : `core.affiliation` (adhésion déclarée) et
`core.nuance_assignment` (qualification administrative) sont deux tables séparées.

### 4.4 Hors matrice

**Délibérations municipales.** Le [schéma SCDL Délibérations](https://schema.data.gouv.fr/scdl/deliberations/)
existe et est référencé, mais son adoption reste marginale — quelques centaines de
collectivités sur 34 800, essentiellement de grandes villes et des intercommunalités.
Aucun agrégateur national, aucune obligation de format. Ingérer ce qui est publié au
format SCDL est possible à coût faible, mais **aucune comparaison ne peut en être tirée** :
la couverture est biaisée vers les collectivités les mieux dotées, ce qui invaliderait
mécaniquement toute comparaison inter-partis.

---

## 5. Les quatre briques produit

### 5.1 La fiche vérifiable

Unité atomique, un permalien immuable par objet : un scrutin, un amendement,
une commune × indicateur × année.

Contient obligatoirement :
- l'objet, décrit factuellement ;
- le lien vers la source officielle et **le snapshot horodaté et haché** ;
- la date de consultation ;
- une section **« ce que cette fiche ne dit pas »**.

Cette dernière section est ce qui rend l'outil utilisable de façon contradictoire : elle
permet de contester une insinuation sans servir à en produire une autre.

### 5.2 La statistique reproductible

« X a voté N % avec Y » est l'affirmation la plus fréquente et la plus manipulable, parce
que le chiffre dépend entièrement du jeu de scrutins retenu.

**Le périmètre est encodé dans l'URL** : législature, bornes de dates, thèmes, type d'objet,
seuil de participation, traitement des absents. Chaque calcul est persisté avec ses
paramètres, sa `method_version` et une empreinte de ses entrées
(`derived.stat_query`). Les deux camps contestent alors le périmètre, pas le chiffre, et
chacun peut publier sa propre version avec un permalien.

Règles non négociables :
- les absents ne sont **jamais** une position — ce sont des données manquantes ;
- tout score s'affiche avec son `n` exploitable et un intervalle de confiance ;
- la redondance entre scrutins d'un même dossier est corrigée, sinon un dossier à huit
  scrutins écrase le reste.

### 5.3 Le résolveur d'affirmation

Le point d'entrée est une phrase entendue, pas une arborescence. Il faut mapper
« la loi immigration », « la loi Duplomb », « le budget 2025 » vers le bon dossier
législatif.

Cœur du dispositif : **une table d'alias maintenue à la main** (`ref.alias`), quelques
centaines d'entrées, indexée en trigrammes. C'est le composant au meilleur rapport
valeur/effort du projet, et personne ne le maintient aujourd'hui.

Une aide par modèle de langage est acceptable **en amont** du résolveur, pour proposer des
candidats. Elle est marquée `AI_*` et n'est jamais la source d'un fait publié.

### 5.4 Le contradictoire

- **Registre public des corrections**, lisible par machine, avec version précédente conservée.
- **Canal de contestation tracé** : un élu ou un journaliste conteste une fiche, la
  contestation et la réponse sont publiques.

C'est le seul mécanisme de neutralité qui tienne devant une accusation de parti pris, et il
coûte beaucoup moins cher qu'un comité éditorial.

---

## 6. Hors périmètre

### 6.1 Définitivement exclu

| Exclu | Raison |
|---|---|
| Décisions municipales / délibérations comme objet comparable | Pas d'agrégateur national, couverture biaisée (§4.4) |
| Suivi annonce → réalisation | Suppose les délibérations |
| Verdict vrai/faux | §2.1 |
| Déclarations hors Parlement | Aucun corpus |
| Statistiques de présence des parlementaires | Techniquement faisable, structurellement trompeur |
| Comparaison inter-communes en médiane brute par étiquette | Confondants massifs : taille, strate, revenu, région, compétences EPCI |

### 6.2 Reporté

| Reporté | Condition de réouverture |
|---|---|
| Quiz de proximité | Sous-produit gratuit du corpus, jamais un axe. Déjà occupé par au moins cinq acteurs |
| Parlement européen | Consommer l'API HowTheyVote.eu, ne pas réingérer |
| Ingestion de presse | Seulement comme signal de détection, jamais comme source d'un fait |
| Programmes et promesses | Nécessite un corpus manuel maintenu |

---

## 7. Architecture des données

Trois couches, plus un schéma de référentiels.

```
raw      Copie exacte de ce qui a été récupéré. Jamais écrasée, jamais corrigée.
         Octets dans l'object storage, métadonnées et empreintes en base.
ref      Nomenclatures externes stables et millésimées (COG, nuances, indicateurs).
core     Données normalisées, historisées, rattachées à une preuve.
derived  Indicateurs, statistiques, appariements. Reconstructibles, versionnés par méthode.
```

Invariants :

1. `core` est **intégralement reconstructible** à partir de `raw` par une fonction
   idempotente. Rejouer l'ingestion deux fois doit produire un état identique — c'est
   testé en intégration continue, et c'est le meilleur test de non-régression du projet.
2. Toute ligne de `core` porte une preuve (`core.evidence`) remontant à un document `raw`.
3. Toute ligne de `derived` porte une `method_version` et une empreinte de ses entrées.
4. Les données brutes ne sont jamais modifiées ni supprimées.

Convention de stockage objet : `raw/{source}/{yyyy}/{mm}/{dd}/{sha256}.{ext}`, bucket
versionné.

---

## 8. Jalons

### Socle — préalable à tout

Ingestion AN (acteurs, organes, dossiers, textes, lectures, amendements, scrutins
nominatifs) dans `raw` → `core`. Archive scellée opérationnelle dès le premier document.
Référentiel d'identités (`core.person_identifier`) publié.

### V1 — le vérificateur parlementaire

Résolveur d'affirmation et table d'alias · fiche scrutin avec « ce que ça ne dit pas » ·
votes individuels et mises au point · statistique reproductible par URL · « non vérifiable »
typé · mesure et publication du taux de couverture sur un corpus réel de débats.

### V2 — le panneau communal

RNE + COG + BANATIC + OFGL + DGFiP + SSMSI + RPLS/SRU + DECP, assemblés par commune et
alignés sur les mandatures. Périmètre de compétences EPCI affiché. Comparaison
**uniquement par appariement** (strate, région, revenu médian), jamais en médiane brute.

### V3 — le contradictoire

Contestations publiques · registre de corrections · API de citation.

---

## 9. Contraintes juridiques

- **RGPD.** Les données des élus relèvent de l'action publique et ne posent pas de
  difficulté particulière. En revanche, **toute réponse d'un visiteur exprimant une
  opinion politique est une donnée sensible au sens de l'article 9** : pas de compte,
  pas d'identifiant persistant, pas d'IP associée, pas d'analytics tiers sur ces pages.
  Si un quiz revient un jour, le calcul se fait côté client.
- **Licences.** Licence Ouverte sur l'ensemble du socle retenu, usage commercial permis,
  attribution et date de mise à jour obligatoires. Le bandeau d'attribution est généré
  automatiquement depuis `raw.source`. **Aucune source NC n'entre dans le pipeline.**
- **HATVP.** Conditions de réutilisation spécifiques, à vérifier avant toute ingestion.
- **Période électorale.** Un outil de ce type attire une attention particulière à
  l'approche d'un scrutin. Page de méthodologie exhaustive et point de contact
  identifiable obligatoires avant toute mise en ligne publique.

---

## 10. Décisions ouvertes

1. **Qui rédige et qui répond aux contestations ?** Un outil destiné à être opposé à des
   journalistes et à des élus ne tient pas sans une personne identifiable et un mécanisme
   de réponse. C'est le goulot réel du projet, et il n'est pas technique.
2. **Financement et indépendance** — à rendre publics dès la mise en ligne.
3. **Profondeur historique du volet parlementaire** : 14e législature (2012) ou 16e (2022) ?
   Détermine le volume de normalisation à écrire.
4. **Reconstitution des mandatures municipales antérieures à 2026** : nécessaire pour
   comparer des mandats complets, coûteuse (résultats électoraux Intérieur + RNE historisé).
