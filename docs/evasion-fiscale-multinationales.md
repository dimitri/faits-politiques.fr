# Évasion fiscale des multinationales : l'impôt que la France ne perçoit pas — données

Migrations 0082 et 0087, paquet `internal/fiscalite`, `-only=fiscalite` (ou
`fiscalite-listes`, `-ocde`, `-ide`, `-fats`, `-twz`, `-filiales`, `-comptes`,
`-marches`, `-faits`). Contrôles : `cmd/verify/fiscalite.go`. Décisions : D-059
(grilles et filiales), D-061 (le dossier et ses nouvelles données).

> Ce document s'appelait « La France est-elle un paradis fiscal ? ». La réponse
> (non, § 6) reste établie ; le dossier porte désormais sur la question qu'elle
> ouvre : pourquoi l'impôt des multinationales échappe-t-il en partie à la France, et
> que leur verse l'État dans le même temps ?

## 1. La question, et les mots

| mot | sens | exemple dans ce dossier |
|---|---|---|
| **Fraude fiscale** | infraction : dissimulation, montage fictif ; jugée ou transigée | Google (2019), McDonald's (2022) : conventions judiciaires d'intérêt public |
| **Évasion fiscale** | contournement de l'esprit de la loi, que l'administration peut requalifier (abus de droit, établissement stable non déclaré) | facturation des clients français depuis une société irlandaise |
| **Optimisation fiscale** | usage de règles légales pour réduire l'impôt | prix de transfert conformes, régimes de faveur (brevets, holdings) |

Les estimations de transfert de bénéfices (§ 2) **ne distinguent pas** ces trois
catégories : elles mesurent où les bénéfices sont déclarés, pas si c'est légal. Le
titre du dossier emploie « évasion » dans ce sens large ; chaque fait (§ 4) porte sa
qualification juridique propre.

## 2. Ce que la France ne perçoit pas

- **Estimations Tørsløv-Wier-Zucman** (`core.transfert_benefices_estimation`) :
  42,6 Md$ de bénéfices transférés hors de France en 2019, soit 21,8 % de l'impôt sur
  les sociétés collecté ; 32,1 Md$ en 2015.
- **Déclarations pays par pays des groupes américains** (`derived.cbcr_juridiction`),
  2023 : la France accueille 2,2 % de leurs salariés hors États-Unis et 1,4 % de leurs
  bénéfices ; l'Irlande 0,9 % des salariés et 17,3 % des bénéfices ; 221,6 Md$ sont
  logés dans des entités « apatrides », sans résidence fiscale.
- **Ce qu'aucune donnée ne dit** : l'impôt qu'une multinationale « devrait » payer en
  France. Le dossier montre des écarts (bénéfice par salarié, lieu de facturation),
  jamais un manque à gagner par entreprise.

## 3. Comment : où le chiffre d'affaires est facturé, où partent les redevances

Trois mécanismes, chacun établi par une source officielle au moins une fois :

1. **Facturer les clients français depuis l'étranger.** Les annonceurs français de
   Google contractaient avec Google Ireland Limited ; le parquet national financier y
   voyait une activité imposable en France (convention de 2019). Les abonnés de
   Netflix contractaient avec une société néerlandaise jusqu'en 2021. Le contrat
   Microsoft du ministère de la Défense a été conclu avec la société irlandaise du
   groupe (Sénat, 2017).
2. **Payer des redevances à une société mère peu imposée.** McDonald's France versait
   à sa mère luxembourgeoise une redevance jugée artificiellement gonflée (convention
   de 2022).
3. **Des prix de transfert qui ramènent le résultat à zéro.** Les entités françaises
   de McKinsey versaient à la maison mère du Delaware des frais qui annulaient leur
   bénéfice imposable : aucun impôt sur les sociétés de 2011 à 2020 pour 329 M€ de
   chiffre d'affaires en 2020 (commission d'enquête du Sénat, 2022).

Les comptes des filiales (`derived.filiale_etrangere_comptes`) montrent le résultat
de ces choix : un chiffre d'affaires de prestataire, une marge faible et stable, et des
ruptures quand le lieu de facturation change (Netflix Services France : 47 M€ en 2020,
1,2 Md€ en 2021).

## 4. Ce que l'État leur verse

### 4.1 Marchés publics (`core.marche_public_cible`, `derived.multinationale_marches`)

Données essentielles de la commande publique, consolidées (plus de 3,2 millions de
lignes, 2,1 millions de marchés dans leur dernière version). Rattachement au groupe par
le SIREN du titulaire, sa dénomination, ou l'objet du marché. Au chargement du
14 septembre 2026 :

| groupe | rattachement | marchés | acheteurs publics | montants publiés dédoublonnés |
|---|---|---:|---:|---:|
| Microsoft | objet (licences, souvent via revendeur) | 745 | 357 | jusqu'à 713 M€ |
| Microsoft | SIREN (Microsoft France, LinkedIn France) | 12 | 9 | 1,7 M€ |
| Accenture | SIREN | 166 | 45 | jusqu'à 949 M€ |
| Oracle | SIREN (Oracle France) | 66 | 41 | jusqu'à 373 M€, dont 300 M€ d'accord de support avec le Service des achats de l'État (2023) |
| Apple | objet (iPad, Mac) | 195 | 81 | jusqu'à 254 M€ |
| IBM | SIREN | 34 | 18 | jusqu'à 105 M€ |
| Cisco | nom et objet | 129 | 75 | jusqu'à 200 M€ |
| Google | objet (Google Workspace, via intégrateurs) | 35 | 19 | jusqu'à 21 M€ |
| Amazon | SIREN et nom | 2 | 2 | 0,1 M€ |
| **Capgemini** (groupe **français**) | SIREN | 653 | 134 | jusqu'à 3,3 Md€ |
| **Capgemini** (groupe **français**) | nom (autres sociétés du groupe, Sogeti) | 334 | 171 | jusqu'à 98 M€ |

« Jusqu'à » : ces montants additionnent des **maxima d'accords-cadres**. Les licences
Microsoft passent presque toujours par des revendeurs (SCC, Crayon, Computacenter,
Econocom) : le titulaire n'est pas Microsoft, et le montant couvre souvent d'autres
produits.

**Amazon Web Services est presque absent des données essentielles**, alors que le
Sénat chiffre ses ventes à l'État : les services d'hébergement passent par le marché
cloud de l'UGAP, dont le titulaire est un distributeur (Crayon, après Capgemini), ou par
des contrats hors obligation de publication (Bpifrance). Les deux lignes rattachées ne
concernent pas le cloud (logistique, casiers de retrait).

**Capgemini n'est pas une multinationale étrangère** : sa société de tête est à Paris et
il est imposé en France. Il est suivi parce qu'il est, de loin, le plus présent des groupes suivis dans ces données, qu'il a été titulaire du premier marché cloud de l'UGAP (2020) et qu'il
porte, avec Orange, l'offre Bleu bâtie sur les technologies de Microsoft.

**Le marché Microsoft de l'Éducation nationale.** Le ministère le présente à l'Assemblée
comme un accord-cadre « avec Microsoft » plafonné à 152 M€ HT (2025). La commission
d'enquête du Sénat en donne les titulaires : le revendeur Crayon France pour les
licences (64 M€ et 6 M€ estimés) et Open SAS pour le support (4,72 M€), soit 74,72 M€
estimés sur quatre ans. Les données essentielles publiées ne portent, pour ce marché,
que des montants partiels attribués à Crayon.

### 4.2 Faits documentés (`ref.fait_multinationale`)

| groupe | fait | montant | établi par |
|---|---|---|---|
| Microsoft | Défense : droits d'usage des logiciels, procédure négociée sans mise en concurrence, cocontractant irlandais (2009-2021) | 82 M€ (2009-2013, presse) ; 120 M€ (2013-2017, repris par le Sénat) | Sénat (proposition de résolution, 2017 ; réponse ministérielle, 2020) |
| Microsoft | Éducation nationale et Enseignement supérieur : accord-cadre 2025-2029, titulaires Crayon France et Open SAS | plafond 152 M€ HT ; 74,72 M€ estimés | Assemblée nationale (réponse ministérielle, 2025) ; Sénat (rapport n° 830, 2025) |
| Microsoft | ventes de produits Microsoft par l'UGAP en 2024 | environ 230 M€ | Sénat (rapport n° 830) |
| Microsoft | audition sous serment : Microsoft France « ne peut pas garantir » que les données ne seront pas transmises à des autorités étrangères | — | Sénat (rapport n° 830) |
| Microsoft | plateforme des données de santé hébergée sur Azure | — | Conseil d'État (2020) |
| Oracle | ventes de produits Oracle par l'UGAP en 2024 | environ 100 M€ | Sénat (rapport n° 830) |
| Amazon (AWS) | marché cloud de l'UGAP, oct. 2020 - mai 2025 : 8 % des 146 M€ de commandes tous fournisseurs (Microsoft 19 %, OVHcloud 37 %) ; 2,2 M€ en 2024 | — | Sénat (rapport n° 830) |
| Amazon (AWS) | Bpifrance : plateforme des prêts garantis par l'État sur AWS, sans appel d'offres | non publié | Assemblée nationale (réponse ministérielle, 2022) |
| Amazon (AWS) | Doctolib, hébergé par AWS, pour les rendez-vous de vaccination : pas de suspension | — | Conseil d'État (2021) |
| Google | Éducation nationale : arrêt du déploiement de Google Workspace et d'Office 365 dans les établissements, contraires au RGPD | — | Assemblée nationale (réponse ministérielle, 2022) |
| Google | convention judiciaire : 500 M€ d'amende, 465 M€ d'impôts | 965 M€ | PNF, Agence française anticorruption (2019) |
| S3NS (Thales-Google Cloud) | qualification SecNumCloud de l'offre bâtie sur Google Cloud (déc. 2025) | — | communiqué de l'entreprise |
| Bleu (Orange-Capgemini) | « cloud de confiance » pour l'État bâti sur Microsoft 365 et Azure, sous licence | non public | communiqué des entreprises |
| Capgemini (français) | appui à la préfiguration de la plateforme des données de santé (2018-2019), finalement hébergée sur Azure | — | Sénat (rapport n° 830) |
| Palantir | renouvellement pour trois ans du contrat de la DGSI | non public | Sénat (question écrite, 2025) |
| McDonald's | convention judiciaire : 508 M€ d'amende, 737 M€ d'impôt | 1,245 Md€ | ministère de l'Économie (2022) |
| McKinsey | aucun impôt sur les sociétés en France de 2011 à 2020 | CA 2020 : 329 M€ | Sénat (commission d'enquête, 2022) |

### 4.3 Aides publiées par bénéficiaire (`derived.multinationale_aides`)

Les registres d'aides (TAM européen, ADEME, minimis) croisés avec les sociétés de
groupes étrangers : 235 groupes ont reçu 1 315 aides du registre européen, pour
3,5 Md€ d'équivalent-subvention (2016-2026, montants aberrants écartés), dont Airbus
(994 M€, société de tête néerlandaise), Rio Tinto (395 M€, dont la compensation des coûts indirects du carbone),
ArcelorMittal (158 M€), Stellantis (85 M€, société de tête néerlandaise) et Euro Disney (15 M€, surtout au titre du
Covid). Parmi les GAFAM et Microsoft, seul Amazon apparaît (0,7 M€, pour des camions à
hydrogène) ; IBM a reçu 6,6 M€ au titre de la recherche.

**Ce qui reste secret.** Le crédit d'impôt recherche (8,0 Md€ en 2026) et les autres
crédits d'impôt sont couverts par le secret fiscal entreprise par entreprise : seuls
leurs totaux sont publics. Aucune donnée ouverte ne dit ce qu'une multinationale en a
obtenu.

## 5. Pourquoi un marché public ne peut pas exiger l'impôt payé en France

Le droit de la commande publique interdit de choisir un titulaire selon le pays où il
paie ses impôts : égalité de traitement des candidats (code de la commande publique,
art. L. 3) et non-discrimination entre entreprises européennes (directive 2014/24/UE).
Un candidat peut être exclu s'il ne s'est pas acquitté de ses impôts **là où il est
établi** (directive 2014/24/UE, art. 57) ; une société irlandaise en règle en Irlande
remplit cette condition. Contracter avec la société irlandaise d'un groupe n'est donc
pas une irrégularité ; c'est ce que les sénateurs ont qualifié, en 2017, de défaut
d'**exemplarité** plutôt que de légalité.

## 6. La France n'est pas un paradis fiscal

| Grille | Critères | Ce qu'elle examine | Où c'est dans la base |
|---|---|---|---|
| OCDE, *Harmful Tax Competition* (1998), encadré I | (a) impôt nul ou symbolique — condition de départ ; (b) pas d'échange effectif d'informations ; (c) manque de transparence ; (d) aucune exigence d'activité substantielle. Encadré II : régimes préférentiels dommageables (taux effectif faible, cantonnement aux non-résidents, opacité, pas d'échange). | Tout pays, régime par régime | Taux : `core.fiscalite_pays` (`CIT.*`, `ETR.*`) ; régimes PI : `IPR.*` (statut du Forum sur les pratiques fiscales dommageables) |
| Conseil de l'UE, conclusions du 5 décembre 2017, annexe V | 1. transparence (échange automatique CRS, échange sur demande « largement conforme », convention multilatérale, bénéficiaires effectifs) ; 2. fiscalité équitable (pas de régime dommageable au sens du code de conduite de 1997, pas de structures offshore sans activité réelle) ; 3. normes minimales BEPS. | **Pays tiers seulement** : aucun État membre ne peut y figurer | `ref.juridiction_non_cooperative` liste `UE_ANNEXE_I`, 23 versions (déc. 2017 → fév. 2026) |
| France, art. 238-0 A du CGI (ETNC) | Refus d'échange d'informations (a et b du 2), inscription sur la liste UE (2 bis 1° et 2°) | Pays tiers | Même table, liste `ETNC_FR`, arrêtés 2010, 2016, 2020 → 2025 lus dans le corpus du JO |

S'y ajoute une mesure économique, sans valeur juridique mais la plus directe :
**où les multinationales déclarent leurs bénéfices au regard de leurs salariés et
de leur chiffre d'affaires** — déclarations pays par pays agrégées par l'OCDE
(`core.cbcr_agregat`, vue `derived.cbcr_juridiction`) et estimations
Tørsløv-Wier-Zucman (`core.transfert_benefices_estimation`).

Les indices du Tax Justice Network (Corporate Tax Haven Index, Financial Secrecy
Index) sont une quatrième grille, militante et documentée. Ils sont **cités, pas
chargés** (D-059).

S'y ajoute une mesure économique, sans valeur juridique mais la plus directe : **où
les multinationales déclarent leurs bénéfices au regard de leurs salariés et de leur
chiffre d'affaires** (§ 2). Sur tous les critères, la France est du côté des pays qui
perdent des bénéfices.

Les indices du Tax Justice Network (Corporate Tax Haven Index, Financial Secrecy
Index) sont une quatrième grille, militante et documentée. Ils sont **cités, pas
chargés** (D-059).

## 7. Sources

| Source | Slug | Réutilisation | Contenu chargé |
|---|---|---|---|
| Commission européenne (PDF d'historique de la liste) | `ue-liste-juridictions-non-cooperatives` | ATTRIBUTION | annexe I de chaque version, transcrite et contrôlée contre le nombre annoncé |
| JORF (corpus déjà chargé) | `jorf` | — | arrêtés ETNC (tableaux HTML, motifs en rowspan) |
| OCDE Corporate Tax Statistics | `ocde-statistiques-impot-societes` | CC BY 4.0 | CbCR 2016-2023 (tous sièges × 35 juridictions dont `WXD` reste du monde et `STLS` apatrides) ; taux légaux 2000-2026 ; taux effectifs moyens et marginaux 2017-2025 ; régimes PI |
| OCDE FDI statistics (BMD4) | `ocde-investissements-directs` | CC BY 4.0 | revenus d'IDE de la France par pays de contrepartie immédiat, 2013-2024, entrants et sortants, en USD et en euros |
| Eurostat FATS | `eurostat-filiales-etrangeres` | CC BY 4.0 | entreprises sous contrôle étranger en France, par pays de contrôle ultime, 2008-2020 (`fats_g1b_08`) et 2021-2023 (`fats_ctrl`) |
| missingprofits.world | `missing-profits-twz-wz` | RESTRICTED | WZ2022 Table A (2015-2019), TWZ2022 Table 3 (2015) |
| GLEIF Golden Copy | `gleif-lei-niveau2` | CC0 | sociétés françaises (SIREN) déclarant une mère ultime étrangère |
| Ratios INPI/BCE | `inpi-bce-ratios-financiers` | Licence ouverte | CA, EBE, résultat courant avant impôt (reconstitué), résultat net |
| Données essentielles de la commande publique, consolidées (decp.info) | `decp-consolidees` | Licence ouverte | marchés dont le titulaire ou l'objet se rattache à un groupe suivi, dernière version de chaque marché |
| Sénat (dont le rapport n° 830 de 2025 sur la commande publique), Assemblée nationale, Conseil d'État, AFA, ministère de l'Économie ; presse et entreprises, signalées | `faits-multinationales` | ATTRIBUTION | 24 faits : contrats, ventes via l'UGAP, règlements fiscaux, constats d'enquête, chacun avec sa qualité (officiel, presse, entreprise) |

## 8. Pièges

1. **La liste UE ne peut pas contenir la France**, ni l'Irlande, ni le Luxembourg,
   ni les Pays-Bas. « La France n'est sur aucune liste » est vrai et ne prouve rien
   à lui seul ; l'argument doit passer par les critères.
2. **Le document de la Commission omet la révision du 17 octobre 2023.** Les 23
   versions chargées sont celles qu'il publie. La liste ETNC n'est reconstituée que
   pour les arrêtés qui l'écrivent en entier (tableau) : 2011-2015 ne font qu'ajouter
   ou retirer des noms, non reconstitués.
3. **CbCR : les totaux ne s'additionnent pas entre sièges.** Chaque pays transmet
   les déclarations de SES groupes ; certains transmettent des données non
   consolidées (dividendes intragroupe comptés en bénéfice) ; l'effectif américain
   « en Chine » varie du simple au décuple d'une année à l'autre selon le
   déclarant. Seules les lignes d'UN siège sont comparées entre elles — les groupes
   américains, qui déclarent le plus régulièrement, servent de référence.
4. **CbCR : sous-groupes bénéficiaires + déficitaires ≠ total.** Écart jusqu'à
   150 Md$ sur le reste du monde des groupes américains en 2022 : panels et totaux
   sont compilés séparément. Le contrôle retenu est une inégalité (les bénéficiaires
   seuls déclarent au moins le total).
5. **CbCR : le rapport impôt / bénéfice n'est pas un taux effectif.** Tous groupes
   confondus, pertes comprises ; un « taux » négatif ou supérieur à 100 % signale des
   pertes, pas une anomalie (Luxembourg, Royaume-Uni certaines années).
6. **IDE : pays de contrepartie immédiat.** Un dividende versé par la filiale
   française d'un groupe américain à sa holding luxembourgeoise est compté vers le
   Luxembourg. La France déclare zéro revenu d'entités à vocation spéciale (SPE),
   ce qui la distingue des pays-relais (Pays-Bas, Luxembourg).
7. **FATS : rupture en 2021** (nomenclature et champ), 2018 absent de la série
   ancienne, secteur financier exclu. Les deux séries sont gardées côte à côte.
8. **GLEIF : « mère étrangère » ≠ « groupe étranger ».** Le pays est celui
   d'immatriculation de la société de tête : Stellantis et Airbus, dont les sociétés
   de tête sont néerlandaises, apparaissent comme groupes « NL » : leurs 18 sociétés
   françaises pèsent 113 Md€ des 401 Md€ de chiffre d'affaires repérés (derniers
   comptes). Toute agrégation GLEIF est donc présentée aussi hors ces deux groupes.
   Les relations sont déclaratives et incomplètes (parmi les GAFAM, seules Google
   France et Google Cloud France en déclarent une) : repérage, pas recensement.
   Seules les autorités d'enregistrement dont l'identifiant est le SIREN sont lues
   (RA000189 SIRENE, RA000192 RCS) ; RA000190 (fonds AMF) est écarté.
9. **Comptes : pas d'impôt sur les sociétés dans le jeu INPI/BCE.** Le résultat
   courant avant impôt est reconstitué (ratio publié à trois décimales × CA, erreur
   d'arrondi ≤ 0,0005 % du CA) ; l'écart au résultat net mêle impôt, résultat
   exceptionnel et participation. **Jamais présenté comme « l'impôt payé ».** Un
   écart très faible (Euro Disney Associés, exercice 2025 : 5 M€ pour 265 M€ de
   résultat courant) ou négatif (McDonald's France 2022 : 891 M€ de résultat net pour
   520 M€ de résultat courant) peut venir d'un exceptionnel, de déficits antérieurs imputés ou d'un
   produit d'impôt : le jeu ne permet pas de trancher.
10. **Comptes : la filiale n'est pas le chiffre d'affaires du groupe en France.**
    Les ventes aux clients français peuvent être facturées depuis une autre
    juridiction (Irlande notamment) ; la filiale française est alors rémunérée comme
    prestataire (marketing, support) avec une marge faible et stable. Le jeu ne permet
    de mesurer ni les ventes réelles en France, ni les redevances versées.
11. **Estimations TWZ : ce sont des estimations** (hypothèses sur la rentabilité
    « normale », la liste des paradis et la clé de répartition). Le classeur WZ2022
    répète une ligne « India » portant les valeurs de l'Afrique du Sud : la seconde
    occurrence est écartée.
12. **Commande publique : les accords-cadres mutualisés sont republiés par chaque
    membre.** Un accord-cadre de 160 M€ passé par un groupement apparaît chez chacune
    des communes membres avec le même maximum. Les montants ne sont comptés qu'une
    fois par titulaire, date et montant ; les montants signalés comme suspects par le
    consolidateur sont exclus des sommes.
13. **Commande publique : un rattachement par objet n'est pas un marché avec le
    groupe.** « Licences Microsoft » désigne un achat à un revendeur ; l'argent parvient
    à Microsoft pour une part inconnue. Les montants par objet sont des ordres de
    grandeur d'engagements possibles, pas des paiements à la multinationale.
14. **Commande publique : couverture partielle.** Publication obligatoire depuis 2019,
    inégalement respectée ; les marchés de défense et de sécurité en sont largement
    absents ; les contrats antérieurs n'y figurent pas. Une ligne déclarant la
    société titulaire comme acheteur (Microsoft France « achetant » une trottinette)
    est écartée.
15. **Sociétés étrangères immatriculées en France** (Microsoft Ireland Operations,
    Google Ireland, Amazon Web Services EMEA, catégories juridiques 31xx et 32xx) : elles
    ont un SIREN, peuvent être titulaires de marchés, mais ne déposent pas de comptes
    sociaux français.
16. **« AWS » et « Amazon » dans les noms et les objets.** « AWS » désigne aussi Avenue
    Web Systèmes, éditeur de plateformes de marchés publics, et « Amazon » des sociétés
    guyanaises : le rattachement exige la dénomination d'une société du groupe
    (Amazon Web Services, Amazon EU…).
17. **Faits de presse.** Le montant du contrat Microsoft de la Défense n'a jamais été
    publié par le ministère ; il vient de la presse et de parlementaires qui la citent.
    Il est présenté comme tel.

## 9. Chiffres de référence (chargement du 14 septembre 2026)

- Taux légal combiné de l'IS, France : 36,13 % en 2025-2026 (contribution
  exceptionnelle comprise ; 25,83 % en 2022-2024). Taux effectif moyen (EATR,
  composite, OCDE) 2025 : France 23,6 % ; Allemagne 26,8 ; Pays-Bas 24,5 ;
  Royaume-Uni 22,5 ; Suisse 18,4 ; Irlande 12,4 ; Hongrie 12,0 ; Chypre 11,4 ;
  Bulgarie 9,1.
- Régime PI français : 10 %, statut OCDE « Not harmful (amended) ». Irlande 6,25,
  Luxembourg 4,99, Pays-Bas 7, Belgique 3,76, Malte 0.
- Groupes américains, 2023 : France 484 621 salariés (2,2 % de leurs effectifs hors
  États-Unis), 11,7 Md$ de bénéfice (1,4 %), 24 k$ par salarié, impôt dû / bénéfice
  33,9 %. Irlande : 206 268 salariés (0,9 %), 142,8 Md$ (17,3 %), 692 k$ par salarié,
  14,5 %. Bermudes : 987 salariés, 5,7 Md$, 3,2 %.
- Groupes français, 2023 : Singapour 207 k$ de bénéfice par salarié, Luxembourg
  202 k$, Irlande 131 k$ ; France 20 k$.
- TWZ / WZ : bénéfices transférés hors de France 32,1 Md$ (2015) → 42,6 Md$ (2019),
  soit 21,8 % de l'IS collecté perdu en 2019.
- Entreprises sous contrôle étranger en France, 2023 : 18 292 entreprises, 2,48 M de
  personnes (11,9 %), 239,5 Md€ de valeur ajoutée (15,4 %) ; contrôle américain
  483 671 personnes ; contrôle « offshore » 27 521.
- Dividendes versés par les filiales en France à leurs investisseurs directs
  étrangers, 2024 : 37,6 Md€ (Luxembourg 22,8 %, Pays-Bas 16,0 %, Suisse 12,5 %,
  Allemagne 12,1 %) ; reçus par les investisseurs français de leurs filiales à
  l'étranger : 111,4 Md€.
- Commande publique : 745 marchés dont l'objet nomme Microsoft, chez 357 acheteurs
  publics distincts ; 12 marchés seulement ont pour titulaire une société française
  du groupe. UGAP, 2024 : environ 230 M€ de ventes Microsoft, 100 M€ Oracle ; marché
  cloud : 146 M€ cumulés d'octobre 2020 à mai 2025, dont 8 % pour AWS.
