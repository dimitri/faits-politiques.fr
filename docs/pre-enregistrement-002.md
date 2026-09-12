# Pré-enregistrement 002 — Axe INSTITUTIONS et page d'accueil 2027

> **Protocole scellé.** Fixe le corpus, le codage et les règles d'affichage **avant**
> de regarder le moindre résultat. Horodaté, son empreinte est publiée.
>
> | | |
> |---|---|
> | Version | 1 |
> | Rédigé le | 11 septembre 2026 |
> | Statut | **BROUILLON — non scellé** |
> | Scellé le | — |
> | Empreinte SHA-256 | — |
> | Premier calcul autorisé | après scellement uniquement |

---

## 1. Objet

Construire une page d'accueil qui, en vingt secondes, montre **ce que les candidats
à l'élection présidentielle de 2027 ont fait**, et non ce qu'on dit d'eux.

Ce protocole n'énonce aucune hypothèse à tester. C'est un **protocole de construction
d'indicateur** : il fige le corpus et la méthode pour qu'aucun résultat ne puisse être
attribué au choix des scrutins ni au codage.

Il ne remplace pas le protocole [001](pre-enregistrement-001.md), qui porte sur l'action
municipale, et ne partage avec lui ni population ni méthode.

## 2. Pourquoi « institutions » et pas « gauche-droite »

Un positionnement gauche-droite dérivé des votes ne mesure pas l'idéologie : dans un
régime parlementaire à groupes disciplinés, la première dimension extraite d'une matrice
de votes sépare la **majorité de l'opposition**. Le RN et LFI s'y retrouveraient du même
côté — résultat exact, conclusion absurde.

L'axe institutionnel porte au contraire sur des objets précis et vérifiables :
indépendance de la justice, liberté de la presse, contrôle constitutionnel, autorités
indépendantes, conventions relatives aux droits fondamentaux, révisions
constitutionnelles.

Il substitue à une question inarbitrable — « ce candidat est-il fasciste ? » — une
question documentable : **qu'a-t-il voté, sur ces scrutins-là ?**

## 3. Population : qui est candidat

Le seul registre officiel est la liste des parrainages validés par le Conseil
constitutionnel, publiée vers mars 2027. D'ici là, critère d'inclusion explicite :

- **déclaration publique de candidature**, sourcée et datée, archivée et scellée ;
- **ou** investiture formelle par une organisation ayant déposé des comptes à la CNCCFP.

La liste est publiée avec ses sources, et **révisée uniquement par ajout** : un candidat
retiré reste affiché comme retiré, avec sa date. Retirer silencieusement une ligne serait
indistinguable d'un tri.

Dès la publication de la liste officielle, elle **remplace** ce critère, et l'écart entre
les deux listes est publié.

## 4. Corpus de scrutins

### 4.1 Critères d'inclusion, figés avant sélection

1. Rattachement au thème `INSTITUTIONS` de la taxonomie v1, affectation au **dossier**
   et non au scrutin isolé.
2. Scrutin public à positions nominatives — l'Assemblée nationale et le Congrès
   qualifient ; le Sénat, dont les scrutins sont publiés par groupe, **ne qualifie pas**.
3. Objet auto-portant : ensemble d'un texte, motion, résolution, article entier, ou
   amendement dont l'objet est identifiable sans lecture du dossier complet.
4. Clivage minimal : les scrutins quasi unanimes sont exclus, ils ne portent aucune
   information.
5. Période : 17e législature, plus les scrutins antérieurs nécessaires pour couvrir les
   candidats ayant siégé avant 2024.

### 4.2 Taille

**8 à 12 scrutins** pour la grille d'accueil. Le corpus complet, plus large, reste
accessible et publié ; la grille en est un sous-ensemble **tiré selon les critères
ci-dessus, pas choisi**.

### 4.3 Publication

Le corpus est un jeu de données ouvert et versionné, avec la liste des scrutins
**écartés et le motif de chaque exclusion**.

## 5. Codage de sens

### 5.1 C'est un acte éditorial, et il est traité comme tel

Transformer un vote en position sur un axe suppose de décider quel côté renforce et quel
côté affaiblit le contre-pouvoir concerné. **Ce n'est pas un fait.**

Le codage vit donc dans un jeu **forkable**, exactement comme une carte de rattachement :
une lignée nommée, des révisions gelées, un code de justification par ligne. Quiconque
n'est pas d'accord publie son propre codage, et les deux s'affichent côte à côte.

### 5.2 Règles

- Le codage porte sur **l'objet soumis au vote**, jamais sur « la loi ». Un texte est un
  assemblage de dispositions qui peuvent aller en sens contraires.
- Un objet dont les dispositions vont en sens contraires est marqué **composite** :
  il est affiché, il n'est **pas** scoré.
- Chaque ligne porte un code de justification pris dans un vocabulaire fermé.
- **Relecture contradictoire obligatoire** : au moins deux relecteurs déclarant des
  sensibilités opposées, désaccords tracés et publiés.

### 5.3 Nommage des pôles

Les pôles sont décrits **littéralement**, jamais par une valeur.

| Interdit | Obligatoire |
|---|---|
| « autoritaire → démocratique » | « a voté contre / pour l'extension du contrôle juridictionnel » |
| « antidémocratique » | « a voté pour la restriction du champ de saisine » |

Un axe nommé par un jugement fait perdre le procès en neutralité ; un axe nommé par son
contenu le rend sans objet.

## 6. Affichage

### 6.1 Vue principale : la grille

Lignes = candidats. Colonnes = les scrutins du corpus. Cellules = la position réelle :
`POUR`, `CONTRE`, `ABSTENTION`, `ABSENT`, ou l'absence de mandat.

**Aucun codage de sens n'intervient dans cette vue.** Une cellule affiche un vote. Le
lecteur fait la synthèse. C'est la vue par défaut, et c'est celle qui doit tenir en
vingt secondes.

### 6.2 Vue secondaire : l'axe, ou la matrice

Score dérivé du codage, avec le jeu de codage cité et son empreinte, et le lien vers le
codage ligne à ligne. Un second axe ne peut venir que d'un autre thème pré-enregistré,
jamais d'une dimension fabriquée pour l'esthétique du graphique.

### 6.3 Candidats sans bilan délibératif

**Non positionnés.** Case vide typée avec sa raison (`OUT_OF_CORPUS`, `OUT_OF_PERIOD`,
absence de mandat). Placer au centre un candidat sans bilan serait une invention.

Les mandats européens sont affichés sur un **graphique distinct** : les votes du
Parlement européen ne vivent pas dans le même espace que ceux de l'Assemblée et ne sont
pas superposables.

### 6.4 Programmes

Affichés **en colonne parallèle**, jamais sur le même axe que les votes. Une déclaration
et un acte ne se moyennent pas. L'écart entre les deux colonnes est l'information.

Les engagements extraits d'un programme sont marqués `AI_EXTRACTED` jusqu'à vérification
humaine, et le programme lui-même est archivé et scellé au moment de sa publication —
ces documents disparaissent dans les mois qui suivent le scrutin.

## 7. Classifications tierces

Les positions de partis issues de PopuList, CHES ou d'autres référentiels sont affichées
**avec le vocabulaire et le millésime de leur source** — « far right selon PopuList
v4.0 », « 8,2 sur l'axe GAL-TAN selon CHES 2024 ».

Trois interdits :

1. **Aucune synthèse.** Moyenner deux référentiels fabrique un jugement et le présente
   comme un fait.
2. **Aucune traduction.** « Far right » n'est pas « extrême droite » : la littérature
   internationale distingue *radical* (rejette la démocratie libérale, accepte
   l'élection) et *extrême* (rejette la démocratie), distinction que l'usage français
   ignore.
3. **Aucune classification de notre fait.** Nous citons, nous ne classons pas.

Quand deux référentiels divergent sur un parti, **les deux sont affichés**. Le désaccord
est une information honnête.

## 8. Symétrie

La grille, l'axe et les fiches sont produits **à l'identique pour tous les candidats
retenus**, par la même requête. Une asymétrie est un défaut, pas un choix éditorial, et
elle est mesurée par la vue de couverture.

## 9. Ce qui invaliderait ce protocole

- **Moins de 6 scrutins** satisfaisant les critères → la grille n'est pas publiée.
- **Plus d'un tiers des candidats sans aucun bilan** → la vue axe est abandonnée, seule
  la grille et les fiches subsistent.
- **Désaccord persistant des relecteurs sur plus de 20 % du codage** → l'axe n'est pas
  publié, et le désaccord l'est.
- Découverte que la sélection produit une majorité de scrutins émanant d'une même
  origine politique → corpus repris **en entier**, et le fait publié.

## 10. Amendements

| Date | Amendement | Motif | Résultats déjà connus |
|---|---|---|---|
| — | — | — | — |
