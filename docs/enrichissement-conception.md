# Enrichir le site : ce que la base permet, et ce qu'elle ne permet pas encore

> Plan du 13 septembre 2026. Audit de la base, confrontation de six demandes à la
> donnée réelle, nouvelle architecture, ordre des chantiers.
> Maquette : [maquette-refonte.html](maquette-refonte.html).

**Ce document explore.** Aucune règle n'y est définitive, sauf une : ne montrer
que des faits, jamais des opinions. Tout le reste — découpage, forme, ordre — se
discute et se révise. Ce qui suit dit ce que la donnée permet, pas ce qu'il
faudrait penser.

La base a beaucoup grossi : 5,2 millions de faits de délinquance, 1,7 million
d'indicateurs communaux, 341 931 lignes de déclarations d'intérêts, 260 778
interventions en séance, 27 séries macroéconomiques, 34 875 communes. Les six
demandes sont réalisables — mais **aucune telle qu'elle est formulée**, et les
écarts sont ce que ce document a de plus utile.

L'audit a été fait **pendant une ingestion** (`cmd/ingest -only=normalize`) : les
comptages de l'Assemblée ont été transitoirement nuls puis rétablis. Les volumes
ci-dessous sont ceux des connecteurs stables.

---

## 1. Ce que la base contient vraiment

| Domaine | Volume | Couverture | Verdict |
|---|---:|---|---|
| Indicateurs communaux (OFGL) | 1 678 562 | 34 869 / 34 875 communes, 2018-2025, 6 indicateurs | quasi totale |
| Délinquance enregistrée | 5 231 250 | 34 875 communes, 2016-2025, 15 indicateurs | totale |
| Associations (RNA) | 1 184 622 | avec code commune | totale |
| Mandats (RNE) | 612 704 | dont 508 788 conseillers municipaux, 34 743 maires | totale |
| Déclarations HATVP | 341 931 lignes | 2 776 personnes, 6 525 déclarations | bonne |
| Interventions en séance | 260 778 | 674 personnes, 07/2024 → 07/2026 | deux ans |
| Bilan alimentaire (FAO) | 10 770 | 116 produits, 2010-2023, 9 éléments | bonne |
| Budgets des collectivités | 55 110 | 1 381 entités, 2018-2025 | bonne |
| Séries macroéconomiques | 947 points | 27 séries, 1971-2025 selon la famille | tardive |
| Contours administratifs | 126 | 18 régions, 101 départements, 7 collectivités | totale |
| Agriculture — usage des sols | 315 | 1961-2024 | la plus longue série du site |
| Nuances municipales | 43 270 | **3 269 communes nuancées sur 34 875** en 2026 | 9,4 % |
| Gouvernements | 36 | **1959 → mars 2014 seulement** | s'arrête en 2014 |
| Mandats ministériels | 115 | 80 personnes, 2002-2010 | huit ans |
| Pont nuance → parti | **0** | table vide | inexistant |
| Pont CHES → groupe | **0** | intersection des identifiants nulle | inexistant |
| Comptes de campagne | 6 290 | aucun relié à une personne | non relié |
| Amendements, textes | 0 | tables créées, non chargées | en cours |

**La leçon de ce tableau.** Les données *territoriales* et *financières* sont
quasi complètes ; les données *politiques* le sont beaucoup moins. C'est
l'inverse de ce qu'on attend d'un site politique, et c'est ce qui décide de
l'ordre des chantiers : les vues les plus solides à construire ne sont pas celles
qu'on imaginerait.

---

## 2. La frise de la V<sup>e</sup> République

### 2.1 Le repère chronologique est autorisé

**Décision du 13 septembre 2026.** Une frise qui associe les présidents à des
chiffres n'est pas une affirmation de causalité : c'est de l'histoire avec des
nombres. Le principe n° 3 interdit d'écrire qu'une politique a produit un
résultat ; il n'interdit pas de situer un chiffre dans le temps, ce que
`data/presidents.csv` fait déjà pour les mandats ministériels — « sert uniquement
à **situer** un mandat dans le temps ».

La frise est donc construite, avec trois garde-fous qui restent nécessaires :

1. **Les bandes présidentielles ne sont jamais colorées par parti.** Un rail
   neutre, alterné pour la lisibilité, rien d'autre.
2. **Aucun titre de la forme « bilan de X ».** La bande dit qui était en fonction,
   pas qui est responsable.
3. **Les courbes traversent les alternances sans rupture visuelle**, ce qui est
   précisément ce que la donnée montre : il ne se passe rien de particulier au
   changement de président. C'est un fait, et il vaut d'être vu.

### 2.2 Les séries commencent bien après 1958

C'est la contrainte qui gouverne le dessin.

| Famille | Début | Ce que ça couvre |
|---|---|---|
| Entreprises (VA, EBE, dividendes, salaires, impôts payés) | **1971** | à partir de Pompidou |
| Comptes publics (dette, solde, recettes, dépenses COFOG) | **1995** | à partir de Chirac |
| Chômage BIT (taux) | **2003** | à partir de Chirac II |
| Pauvreté en nombre | **2004** | idem |
| RSA (foyers) | **2016** | à partir de Hollande |

De Gaulle et Pompidou n'ont donc **aucun** chiffre ; Giscard et Mitterrand n'ont
que le partage de la valeur ajoutée. Commencer la frise en 1995 masquerait que la
République est plus vieille que ses statistiques : **la frise part de 1958 et
montre le bord**. Le vide est la première information.

### 2.3 Deux pièges de dessin, et leur parade

**Les euros courants.** Comparer 1971 et 2024 en euros courants est un mensonge,
et **aucune série de déflateur n'est en base**. 3 425 M€ de dividendes en 1971 et
301 932 M€ en 2024 ne sont pas comparables. Par défaut, la frise affiche donc des
**parts et des ratios** — dette en % du PIB, solde en % du PIB, dividendes
rapportés à l'excédent brut d'exploitation. Les euros courants restent
accessibles, étiquetés comme tels.

**La double échelle.** Superposer dette (% PIB) et chômeurs (milliers) sur deux
axes verticaux fabriquerait une corrélation absente de la donnée. La frise est
donc faite de **petits multiples** : une colonne par indicateur, chacune avec sa
propre échelle, toutes alignées sur le même axe du temps, une seule teinte. Les
séries ne sont pas des identités à distinguer, ce sont des mesures d'un même pays.

### 2.4 Et la Sécurité sociale

Il n'y a **pas** de budget de la Sécurité sociale en base. Le plus proche est
`depense.GF10` — « protection sociale » au sens COFOG, toutes administrations
confondues — qui n'est pas le budget de la Sécu et ne doit pas être étiqueté
ainsi. Le chantier est ouvert côté ingestion (`docs/budget-donnees.md`) : la
colonne existera, elle n'existe pas encore.

---

## 3. Gouvernement

Réalisable pour 1959-2014, et **vide exactement là où le reste du site travaille**.

- `core.gouvernement` : 36 gouvernements, du 8 janvier 1959 au 31 mars 2014.
- **Rien depuis avril 2014** — Valls, Cazeneuve, Philippe, Castex, Borne, Attal,
  Barnier, Bayrou, Lecornu. Le jeu de données amont s'arrête là.
- 10 gouvernements sur 36 ont un Premier ministre relié à une personne ; les
  mandats `MINISTRE` couvrent 2002-2010, 80 personnes. Une composition de
  gouvernement n'est donc pas affichable.

**Conséquence.** Une entrée « Gouvernement » ouverte aujourd'hui afficherait une
liste qui s'arrête douze ans avant le présent, sur un site dont l'Assemblée
couvre 2024-2026 : une section qui a l'air complète et ne l'est pas. Il faut le
connecteur d'abord — la composition des gouvernements récents est publiée au
*Journal officiel*, et `core.acte_jo` (4 590 actes) est le point d'entrée. À
défaut, la section s'ouvre sur son propre trou, avec un
`GOUVERNEMENT_APRES_2014_NON_INGERE`.

---

## 4. Agriculture et alimentation

La demande la mieux servie par la donnée, et la seule sans trou gênant. Elle
porte déjà sa question : *le pays arrive-t-il à nourrir ses habitants ?*

- **Usage des sols** : 1961 → 2024, 64 points par série (terres agricoles,
  arables, cultivées, prairies). Soixante-trois ans continus.
- **Bilan alimentaire** : 116 produits, 2010 → 2023, avec production,
  importation, exportation, disponibilité intérieure, alimentation animale, et
  l'apport en kcal/habitant/jour.
- **Emploi** : agricole 1991 → 2025, agroalimentaire 2000 → 2023.

### 4.1 Humains ou bétail : le partage est dans la donnée

Le bilan FAO distingue **`Food`** (alimentation humaine) et **`Feed`**
(alimentation animale). Le partage est donc publié, pas déduit. En 2023, sur les
produits élémentaires :

| Produit | Humain (kt) | Bétail (kt) | Part au bétail |
|---|---:|---:|---:|
| Blé | 6 866 | 6 695 | 49 % |
| Maïs | 756 | 7 291 | **91 %** |
| Orge | 51 | 2 521 | **98 %** |
| Autres céréales | 38 | 2 194 | 98 % |
| Pommes de terre | 3 545 | 851 | 19 % |
| Lait | 17 188 | 2 437 | 12 % |

C'est une des choses les plus parlantes que la base sache dire : **l'essentiel de
la sole céréalière française ne nourrit pas des humains.** Le fait est publié par
la FAO, il ne demande aucun modèle, et il change la lecture de « la France
produit assez de céréales ».

**Deux précautions, l'une méthodologique, l'autre technique.**

`Food + Feed` ne fait pas la disponibilité intérieure : 161 207 kt contre
290 572 kt en 2023. Le reste — semences, transformation, pertes, usages non
alimentaires — existe mais **n'est pas publié dans ce jeu**. Le graphique doit
montrer les deux barres et dire que le total leur échappe, plutôt que de laisser
croire à un partage exhaustif.

`ref.produit_alimentaire.agregat` **est incomplet** : 4 lignes sur 116 sont
marquées comme agrégats (Animal Products, Vegetal Products, Grand Total,
Population), alors que « Cereals — Excluding Beer », « Meat », « Vegetables »,
« Fruits », « Starchy Roots » en sont aussi. Sommer sans les écarter compte le
blé deux fois. À corriger dans la table de référence avant toute somme publiée.

Les libellés sont par ailleurs ceux de la FAO, en anglais. Les traduire est une
décision éditoriale de plus, à consigner.

### 4.2 Auto-approvisionnement

**Ce que ça permet sans rien inventer** : un taux d'auto-approvisionnement par
produit et par année, production ÷ disponibilité intérieure. Une division entre
deux colonnes publiées par la même source, pas un modèle.

**Forme** : une matrice produits × années, triée par taux — 116 lignes lisibles
d'un coup là où 116 courbes seraient illisibles. Échelle **divergente** centrée
sur 100 %, seul cas du site où une divergente se justifie : il y a un vrai point
neutre. Deux teintes opposées, gris au milieu, et **surtout pas** le vert et le
rouge des positions de vote, qui diraient « bien / mal » d'un fait agronomique.

**Ce que ça ne dit pas** : un taux supérieur à 100 % n'est pas l'autonomie. La
France exporte du blé et importe du soja pour nourrir ses animaux ; le bilan par
produit ne se somme pas en une souveraineté. La disponibilité intérieure inclut
l'alimentation animale et les usages non alimentaires.

---

## 5. La fiche individuelle

C'est là que la nouvelle donnée change le plus la vie du lecteur.

| À ajouter | Volume | Portée | Précaution |
|---|---:|---|---|
| Intérêts déclarés (HATVP) | 341 931 | 2 776 personnes | Le bloc `non_publie` existe : une case vide n'est pas un zéro, c'est une rétention légale. |
| Participations de dirigeant | 189 343 | 173 718 avec montant | Une participation n'est pas un conflit d'intérêts. Le mot « conflit » n'apparaît nulle part. |
| Activités des cinq ans précédents | 39 679 | 31 218 avec montant | Déclaratif, non vérifié à la ligne par la HATVP. |
| Interventions en séance | 260 778 | 674 personnes | **Le nombre d'interventions est un fait ; le « temps de parole » n'en est pas un.** |
| Déports | 59 | Assemblée | 59 lignes ne font pas une statistique. |
| Patrimoine | 594 | 67 déclarations de situation patrimoniale | Publier un patrimoine pour 67 personnes et rien pour les autres crée une asymétrie qui se lit comme un jugement. |

**Sur le temps de parole.** `core.intervention.instant_s` est un *instant*, pas
une durée. En déduire un temps de parole exige une dérivation datée, avec sa
`method_version` — et une décision sur ce qu'on fait des interruptions. Tant
qu'elle n'est pas écrite, la fiche compte des interventions et ne parle pas de
minutes.

**Le principe d'agencement.** La fiche répond déjà à « comment a-t-il voté ».
Elle doit maintenant répondre à « qu'a-t-il dit » et « quels intérêts a-t-il
déclarés » — en gardant ces trois questions **séparées**. Les mêler produirait
l'insinuation que le site refuse : un vote à côté d'une participation financière
suggère un lien que la donnée n'établit pas.

**Couverture.** 2 776 personnes ont une déclaration HATVP pour 2 126 fiches, mais
l'intersection n'est pas totale : beaucoup de fiches n'auront aucun intérêt
déclaré, et cette absence doit être typée (`HATVP_SANS_DECLARATION`).

---

## 6. Les cartes

### 6.1 Ce qui est en place

PostGIS 3.5 est installé, et `geo.contour` porte **18 régions, 101 départements
et 7 collectivités d'outre-mer** issus d'OpenStreetMap (ODbL), soit 3 Mo une fois
simplifiés. Les 101 codes de département joignent exactement le COG.

Le rendu se fait **en SVG par la base elle-même** (`ST_AsSVG`), sans bibliothèque
de cartographie, sans serveur de tuiles et sans requête vers un tiers : la règle
du site tient donc aussi pour les cartes. En contrepartie, l'ODbL impose que
« © les contributeurs OpenStreetMap » accompagne **chaque carte affichée**, et
non la seule page des sources.

Six rattachements sont écrits plutôt que devinés
(`data/geo-rattachements.csv`) : la Martinique et la Guyane ne sont plus des
départements depuis 2015, OSM distingue le Rhône de la Métropole de Lyon, et
Tuamotu-Gambier est hors périmètre.

### 6.2 Outre-mer : une projection par territoire

La France ne tient pas dans une seule projection. Chaque contour porte donc sa
`srid_rendu` (`data/geo-projections.csv`), et les outre-mer se dessinent en
**cartons séparés** : RGAF09 aux Antilles, RGFG95 en Guyane, RGR92 à La Réunion,
RGM04 à Mayotte, Lambert NC en Nouvelle-Calédonie. Les cartons ne sont pas à la
même échelle entre eux, et la carte doit le dire.

Deux cas se déclarent comme problématiques plutôt que d'être résolus en silence :
la Polynésie s'étend sur 2 000 km et un carton unique en fausse les distances ;
les Terres australes sont dispersées de l'océan Indien à l'Antarctique et ne se
dessinent pas en un carton. Les sept collectivités n'ont par ailleurs **aucune
donnée communale en base** : leur contour existe pour que l'absence soit montrée.

### 6.3 Agréger change tout — et ne suffit pas

**Correction d'une conclusion précédente.** J'avais écrit que la couverture des
nuances était de 9,4 %, ce qui est vrai *par commune* et trompeur : le seuil de
nuançage est un seuil de population, donc les communes nuancées sont les
peuplées. Mesurée autrement :

| Mesure | Couverture |
|---|---|
| Par commune | **9,4 %** (3 269 / 34 875) |
| **Par habitant** | **69,1 %** (47,4 M / 68,5 M) |

Agréger n'est donc pas un pis-aller, c'est la bonne échelle. Par niveau :

| Niveau | Unités | Couverture min → max | Moyenne | Au-dessus de 50 % |
|---|---:|---|---:|---|
| **Régions** | 18 | 43 % → 100 % | 73 % | **17 / 18** |
| **Départements** | 101 | 18 % → 100 % | 58 % | 56 / 101 |
| **Agglomérations (EPCI)** | 4 809 | — | **56,3 % des sièges** | — |
| Communes | 34 875 | — | 9,4 % | — |

L'échelon intercommunal se mesure autrement, et mieux : `municipal_list.sieges_cc`
donne les sièges communautaires, et **80 287 des 142 723 sièges (56,3 %) portent
une nuance**. L'unité y est le siège, pas l'habitant — plus propre, parce qu'un
siège est ce qui est réellement attribué.

**Conclusion : oui aux régions, oui aux agglomérations, oui aux départements à
condition d'afficher la couverture de chaque unité à côté de sa composition.**
Non aux communes prises une à une.

### 6.4 Mais ce ne sera pas une carte des *partis*

Deux raisons demeurent, et la seconde est la plus intéressante.

**Le pont nuance → parti est vide.** `core.nuance_party_link` et
`core.commune_party` comptent 0 ligne. Une nuance est une étiquette attribuée par
le ministère de l'Intérieur à une *liste*, pas un parti.

**Et surtout : 82,7 % des sièges portent une nuance « divers ».** Sur les
102 000 sièges municipaux nuancés de 2026 :

| Nuance | Sièges | Part |
|---|---:|---:|
| LDVD — divers droite | 32 701 | 31,9 % |
| LDVG — divers gauche | 19 155 | 18,7 % |
| LDIV — divers | 16 427 | 16,0 % |
| LDVC — divers centre | 16 420 | 16,0 % |
| *sous-total « divers »* | | **82,7 %** |
| *nuances nommant un parti* (LLR, LRN, LSOC, LFI, LCOM, LVEC…) | | **7,7 %** |

Même avec le pont construit, une carte des partis serait donc à 4/5 vide de
partis. Par département, la part des sièges dont la nuance nomme un parti est de
**7,4 % en moyenne**, 37 % au maximum (Alpes-Maritimes), et **13 départements
sont à zéro**.

Ce n'est pas un défaut de la donnée : c'est un fait sur la politique municipale
française, et il mérite d'être la carte elle-même plutôt que d'être caché
derrière une carte des partis qu'on ne peut pas faire. **Ce qu'on publie donc :
une carte des nuances, nommée comme telle**, et une carte de la part partisane —
qui dit, en une image, que les listes municipales ne sont majoritairement pas des
listes de parti.

**Un piège rencontré en la construisant.** Les départements sans aucun siège
partisan donnaient `NULL`, et `ntile` range les `NULL` en dernier : les treize
départements à **zéro** se retrouvaient dans la classe la plus foncée, soit
exactement l'inverse de la vérité. Un `coalesce` corrige ; la leçon est qu'une
absence doit être ramenée à zéro *explicitement* avant tout classement.

### 6.4 Ce qu'on fait à la place

| Carte | Couverture | Forme | Statut |
|---|---:|---|---|
| **Nuances par région** | 17/18 régions au-dessus de 50 % | composition, couverture affichée | prête |
| **Nuances par agglomération** | 56,3 % des 142 723 sièges | composition par EPCI | prête |
| **Nuances par département** | 56/101 au-dessus de 50 % | composition + couverture par unité | prête |
| **Part des sièges à nuance partisane** | 96 départements | choroplèthe séquentielle | prête |
| Finances communales (dette, investissement, épargne, masse salariale) | 34 869 / 34 875 | choroplèthe séquentielle | prête |
| Délinquance enregistrée, 15 indicateurs | 34 875 | choroplèthe, taux pour 1 000 habitants | prête |
| Budgets des collectivités | 1 381 entités | choroplèthe + tableau | prête |
| Densité associative | 1 184 622 associations | choroplèthe pour 1 000 habitants | prête |
| Couverture du nuançage | 3 269 / 34 875 | carte binaire : **le blanc est le sujet** | à cadrer |
| Nuances des municipales | 3 269 | **petits multiples** : une carte par nuance | à cadrer |
| ~~Carte des *partis*~~ | — | — | pas de pont nuance → parti, et 82,7 % de « divers » |

**Pourquoi des petits multiples et pas une carte arc-en-ciel.** Douze nuances sur
une carte, c'est douze classes de couleur porteuses de sens : au-delà de sept les
classes voisines se confondent, et sous daltonisme elles fusionnent. Une carte par
nuance, chacune en une teinte, se lit sans légende et se compare d'un coup d'œil.
C'est aussi la seule forme qui ne suggère pas que les nuances forment un spectre
ordonné.

### 6.5 Cartes de scores : une seule élection est cartographiable

Question posée : peut-on dessiner, pour chaque parti, une carte de ses derniers
scores — municipales, législatives ou présidentielle ? Réponse par élection :

| Élection | Ce qu'on a | Cartographiable ? |
|---|---|---|
| **Municipales 2020 et 2026** | voix par liste et par commune, avec nuance | **oui** |
| Présidentielle | `core.pdr_voix` : **22 lignes** — totaux nationaux du second tour, vainqueur et finaliste, de 1965 à 2022 | non : aucune géographie, aucun premier tour |
| Législatives | rien | non |

La présidentielle vient des proclamations du Conseil constitutionnel : c'est un
résultat national, pas un résultat par commune. Pour cartographier la
présidentielle ou les législatives, il faudrait le connecteur des résultats
détaillés du ministère de l'Intérieur — publiés par commune et par bureau de
vote. C'est un chantier d'ingestion, pas un chantier de rendu.

**Ce qui est faisable aujourd'hui**, et qui est fait : une carte par nuance,
score aux municipales 2026, agrégé au département.

- 26 059 311 voix au premier tour, dont **16 835 701 sur des listes nuancées
  (64,6 %)**, réparties sur 3 269 communes.
- Le score affiché est la part des voix **rapportée aux seules listes nuancées**,
  et doit être nommé ainsi : un tiers des voix se porte sur des listes que
  l'Intérieur ne nuance pas, et les ignorer silencieusement gonflerait tous les
  scores.

**Trois règles de dessin, apprises en le faisant.**

1. **Une carte par nuance, jamais une carte multicolore.** Douze nuances sur une
   carte, ce sont douze classes de couleur à distinguer ; six cartes en une
   teinte se lisent sans légende.
2. **Chaque carte a sa propre échelle.** Le RN plafonne à 21,8 % là où « divers
   droite » atteint 73,8 % : une échelle commune écraserait tout sauf les deux
   nuances majoritaires. Les couleurs ne se comparent donc pas d'une carte à
   l'autre — seules les **formes** le font, et c'est ce qu'on vient y chercher.
3. **L'absence n'est pas un zéro.** Un département où aucune liste LR ne s'est
   présentée n'est pas un département où LR fait 0 %. Il reçoit un gris neutre,
   distinct du bas de la rampe, et la légende le dit.

**Ce que les cartes montrent d'emblée** : les nuances qui nomment un parti sont
géographiquement clairsemées — Les Républicains présents dans 40 départements sur
96, le Parti socialiste dans 33, La France insoumise dans 69, le RN dans 83 —
tandis que « divers droite » et « divers gauche » couvrent la quasi-totalité du
territoire. La carte d'un parti aux municipales est d'abord la carte des endroits
où il présente des listes sous son nom.

### 6.6 Granularité

Le département d'abord : 101 polygones, 380 Ko simplifiés, instantané et sans
découpage en tuiles. Les 34 875 communes sont un autre problème — plusieurs
dizaines de mégaoctets bruts, à ne charger que par département, en second temps.

---

## 7. Architecture : accueil, menu, navigation

Le menu range aujourd'hui par institution. Avec le gouvernement, les territoires,
la macroéconomie et l'agriculture, il faudrait onze entrées : ranger par
institution ne tient plus.

**Ranger par question.** Le lecteur n'arrive pas en cherchant « l'Assemblée
nationale » ; il arrive avec une affirmation à contrôler. Quatre questions
couvrent tout ce que le site sait :

| Question | Ce qu'elle ouvre |
|---|---|
| **Qui décide** | Gouvernement, Assemblée, Sénat, Europe, Personnes |
| **Ce qui a été voté** | Scrutins, thèmes, dossiers |
| **Où** | Territoires, communes, collectivités, cartes |
| **Combien** | La frise, budget, dette, entreprises, agriculture |

Menu retenu : `Chercher · Qui décide · Ce qui a été voté · Où · Combien ·
Comprendre`. Six entrées, dont quatre ouvrent un panneau listant leurs sections
avec **leur volume réel** — un menu qui dit combien il y a derrière chaque porte
est déjà une réponse.

**L'accueil.** La recherche reste la porte d'entrée. Sous elle, **la frise devient
la colonne vertébrale du site**, en bandeau horizontal compact : cliquer une année
ouvre ce que le site sait de cette année. Puis les derniers scrutins, puis les
quatre questions, puis la méthode.

**Ce qui disparaît** : le bandeau de compteurs. « 8 434 scrutins » n'aide personne
à vérifier quoi que ce soit. Les volumes restent dans le menu et sur les pages de
section, là où ils informent un choix.

---

## 8. Règles de visualisation

Le site a déjà une discipline de couleur forte : quatre positions de vote, un
accent, rien d'autre. Les données continues, géographiques et longitudinales
demandent trois familles de plus, qui ne doivent pas empiéter sur les quatre
existantes.

1. **Jamais deux axes verticaux.** L'alignement des échelles est arbitraire : le
   graphique fabriquerait une corrélation absente de la donnée.
2. **Séquentielle = une teinte, clair → foncé.** Rampe pétrole vérifiée par
   calcul : clarté OKLab monotone, pas ≥ 9.
3. **Divergente seulement s'il y a un vrai zéro**, deux teintes opposées et un
   gris neutre au milieu. Jamais une teinte au point neutre.
4. **Les quatre couleurs de vote ne servent qu'aux votes.** Le vert « pour » sur
   un taux agricole dirait « bien ».
5. **Au-delà de sept classes, un tableau.**
6. **La couleur n'est jamais seule** : libellé, forme, et tableau équivalent.
7. **Montrer le bord des données.** Commencer un graphique là où la série commence
   masque que le sujet est plus ancien.

**À corriger dans l'existant** : les quatre couleurs de vote n'ont jamais été
vérifiées pour le daltonisme, seulement pour le contraste. Vert et rouge sont la
paire à risque. Le libellé écrit et la forme distincte rendent la paire
acceptable, mais la vérification devrait être faite et consignée, comme l'a été
le contraste.

---

## 9. Chantiers, par valeur rendue

| # | Chantier | Dépend de | Note |
|---|---|---|---|
| 1 | Cartes des territoires | rien | Géométrie chargée, couverture quasi totale, chaîne prouvée. |
| 2 | Agriculture et alimentation | rien | La donnée la mieux couverte du site. |
| 3 | Fiche individuelle enrichie | rien | Le plus gros gain pour le lecteur ; couverture inégale, donc absences typées. |
| 4 | Accueil, menu, navigation | rien | Six entrées par question ; la frise en colonne vertébrale. |
| 5 | La frise de la V<sup>e</sup> République | rien | Réalisable telle quelle, bord de données visible, ratios plutôt qu'euros courants. |
| 6 | Connecteur OSM scellé | ingestion | Remplacer le chargement manuel des contours par un connecteur qui scelle la source dans `raw`. |
| 7 | Gouvernements depuis 2014 | ingestion | Onze gouvernements manquants ; `core.acte_jo` est le point d'entrée. |
| 8 | Sécurité sociale | ingestion | `depense.GF10` n'est pas le budget de la Sécu. |
| 9 | Pont nuance → parti | **décision éditoriale** | Ne débloquerait que 7,7 % des sièges : à faire pour la complétude, pas pour la carte. |
| 10 | Résultats détaillés Intérieur (présidentielle, législatives, par commune) | ingestion | Débloquerait les cartes de score pour les deux élections nationales. Aujourd'hui seules les municipales sont cartographiables. |
| 11 | Communes en géométrie | ingestion lourde | 34 875 polygones, par département, après les cartes départementales. |

**La règle qui les ordonne** : publier d'abord ce dont la couverture est bonne.
Les territoires et l'agriculture sont mieux couverts que la politique — c'est
contre-intuitif pour un site politique, mais c'est ce que dit la base, et publier
dans cet ordre évite de mettre en avant des sections à trous.
