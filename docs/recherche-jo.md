# Chercher dans le Journal officiel : ce que chaque index sait faire

> Mesures du 13 septembre 2026, sur le corpus complet chargé en base :
> 1 236 284 actes, 3 809 558 blocs de texte, 1861 → 2025.
> Processeur Intel Xeon E5-2620 v2 à 2,10 GHz, PostgreSQL 17.

---

## 1. La configuration française

`french`, livrée avec PostgreSQL, sait désuffixer le français mais ignore que
« Élysée » et « Elysee » sont le même mot. Le Journal officiel écrit les deux — un
siècle et demi de saisies, dont des reprises de documents papier, le garantit.

```sql
CREATE TEXT SEARCH CONFIGURATION fr (COPY = french);
ALTER TEXT SEARCH CONFIGURATION fr
  ALTER MAPPING FOR hword, hword_part, word, asciiword, asciihword
  WITH unaccent, french_stem;
```

L'ordre compte : **déaccentuer puis désuffixer**. L'inverse laisserait deux
radicaux différents pour le même mot.

| | `french` livrée | `fr` |
|---|---|---|
| `to_tsvector(…, 'Élysée présidence Élysee')` | `'élys':1 'président':2 'élyse':3` | `'elyse':1,3 'president':2` |

Ce que cela change en pratique : **60 actes trouvés contre 10**, sur les seuls
titres, pour une recherche « Elysee » saisie sans accent.

### Faut-il un dictionnaire plutôt qu'un désuffixeur ?

`french_stem`, le désuffixeur Snowball, tronque les mots selon des règles
mécaniques. Sur le vocabulaire du Journal officiel, il se trompe **dans les deux
sens** :

| | `french_stem` | `fr_hunspell` |
|---|---|---|
| `ministre` | `ministr` | `ministre` |
| `ministères` | `minister` | `ministère` |
| `nomination` | `nomin` | `nomination` |
| `nommés` | `nomm` | `nommé, nommer` |
| `abrogés` | `abrog` | `abroger` |
| `budgétaire` | `budgétair` | `budgétaire` |
| `retraites` | `retrait` | `retrait, retraiter, retraire` |
| `retrait` | `retr` | `retrait, retraire` |

**Il sépare ce qui est identique** : `ministre` donne `ministr` et `ministères`
donne `minister` — chercher l'un ne trouve pas l'autre. **Et il confond ce qui
diffère** : `retraites` (les pensions) et `retrait` (d'un texte) se rejoignent
sur `retrait`.

Le dictionnaire Hunspell ne tronque pas : il ramène une forme fléchie à son
**lemme**, par des règles morphologiques appuyées sur une liste de mots réels.
Il répare le premier défaut — `ministères` donne bien `ministère`.

Il ne répare **pas** le second, et il faut le dire : quand une forme est
ambiguë, il rend *tous* ses lemmes possibles. `retraites` reste rattaché à
`retrait`, non plus par accident de troncature mais parce que la forme est
réellement ambiguë hors contexte. Le dictionnaire rend la confusion explicite ;
il ne la supprime pas.

**Ce qui est installé.** `hunspell-fr-comprehensive` — le dictionnaire
Grammalecte v7.0 d'Olivier R., sous licence MPL 2.0, 86 491 entrées. Variante
« toutes variantes » et non « classique », parce que le corpus va de 1861 à 2025
et contient donc les orthographes d'avant et d'après la réforme de 1990. Les
deux fichiers sont copiés dans `tsearch_data` sous les noms que PostgreSQL
attend, `fr_fr.dict` et `fr_fr.affix` ; il lit le format Hunspell, `FLAG long`
compris.

**Son coût, mesuré.** Un dictionnaire Ispell est chargé en mémoire au premier
usage **de chaque session** :

| | Premier appel d'une session | Appels suivants |
|---|---|---|
| `fr_hunspell` | **175 à 278 ms** | 0,17 ms |
| `french_stem` | 1,8 ms | 0,17 ms |

Avec un pool de connexions — ce projet en a un — c'est deux cents millisecondes
payées une fois par connexion, donc négligeable. Sur une application qui ouvre
une connexion par requête, ce serait deux cents millisecondes sur chaque
première requête.

**Ce qui n'est pas fait.** La configuration `fr` en service reste
`unaccent + french_stem`. Basculer sur le dictionnaire demande de réindexer
3,8 millions de blocs — une demi-heure — et ce n'est pas un choix à faire sans
mesurer le gain de rappel sur des recherches réelles. La configuration est prête
à poser :

```sql
CREATE TEXT SEARCH DICTIONARY fr_hunspell (
  TEMPLATE = ispell, DictFile = fr_fr, AffFile = fr_fr, StopWords = french);

CREATE TEXT SEARCH CONFIGURATION fr_lemme (COPY = fr);
ALTER TEXT SEARCH CONFIGURATION fr_lemme
  ALTER MAPPING FOR hword, hword_part, word, asciiword, asciihword
  WITH unaccent, fr_hunspell, french_stem;
```

L'ordre `fr_hunspell, french_stem` compte : le désuffixeur sert de **repli** pour
ce que le dictionnaire ne connaît pas — noms propres, sigles, jargon juridique.
Sans lui, ces mots ne seraient pas indexés du tout.

---

## 2. Où mettre le vecteur : quatre formes mesurées

Le vecteur doit être **stocké** — c'est le premier résultat, et il est net. Une
recherche de phrase sur un index fonctionnel recalcule le vecteur de chaque
candidat :

| Recherche de phrase | Vecteur stocké | Index fonctionnel |
|---|---|---|
| | **15 ms** | **50 057 ms** |

**GIN ne range pas les positions des lexèmes.** Une conjonction se résout dans
l'index seul ; une phrase doit vérifier l'adjacence sur chaque candidat — en
*lisant* un vecteur stocké, ou en le *recalculant* sur des blocs de plusieurs
mégaoctets.

Restait à savoir **sous quelle forme** le stocker. Quatre formes, mesurées sur
200 000 blocs représentatifs :

| Forme | Construction | Index | Dump | Restauration |
|---|---|---|---|---|
| Colonne générée | 89,8 s | 13,5 s | 14,1 s / 55,8 Mo | **105,1 s** |
| Colonne ordinaire, `INSERT…SELECT` | 90,3 s | 12,8 s | 28,8 s / 136,8 Mo | 35,8 s |
| Table `CREATE TABLE AS` | **30,4 s** | 11,6 s | 27,3 s / 140,8 Mo | **31,3 s** |
| **Vue matérialisée** | 31,5 s | 11,6 s | **0,3 s / 1,6 Ko** | 44,1 s |

Trois enseignements, dans l'ordre où ils sont apparus :

1. **`pg_dump` ne transporte pas une colonne générée** : il la fait recalculer à
   la restauration. D'où les 105 s, trois fois le reste. Vérifié sur notre propre
   dump — l'entrée « MATERIALIZED VIEW DATA » de la table des matières contient
   en réalité un `REFRESH MATERIALIZED VIEW`, pas des données.
2. **L'écart entre 90 s et 30 s n'oppose pas la vue à la table** : il oppose
   `INSERT … SELECT` à `CREATE TABLE AS`. Une relation créée dans la transaction
   courante évite une partie du travail d'écriture. Et un `TRUNCATE` suivi d'un
   `INSERT` dans la même transaction ne le retrouve pas — 86,6 s mesurés :
   avec `wal_level = replica`, l'optimisation ne joue pas.
3. **Le dump d'une vue matérialisée ne contient que sa définition** : 1,6 Ko
   contre 140 Mo pour la table équivalente.

> **Une prévision démentie par la mesure.** J'attendais que la vue matérialisée
> allège le dump du corpus entier de près de trois gigaoctets. Elle ne l'allège
> pas du tout : 1 352 937 088 octets en 321 s, contre 1 351 402 765 en 320 s
> avec la colonne générée. C'est logique après coup — **une colonne générée
> n'est pas dumpée non plus**. Les deux formes excluent le vecteur ; seule la
> colonne *ordinaire* l'aurait emporté, au prix de 2,7 Go.

### La forme retenue : la vue matérialisée

Son avantage n'est donc pas la taille du dump mais **le temps de restauration**,
et pour la même raison qui fait gagner le `CREATE TABLE AS` : un `REFRESH` en
bloc va plus vite qu'un calcul ligne à ligne pendant `COPY`. Sur l'échantillon,
44,1 s contre 105,1 s.

Et elle apporte ce qu'aucune colonne ne donne : **PostgreSQL connaît la
dépendance**. La vue ne peut pas dériver ligne à ligne — elle est fraîche ou
périmée, jamais incohérente — et la source ne peut pas être modifiée sans que le
moteur le signale. Une colonne ordinaire, elle, repose sur la discipline du
chargement ; il aurait fallu une sonde pour vérifier qu'elle ne ment pas.

L'index `UNIQUE` sur la vue n'est pas décoratif : sans lui,
`REFRESH … CONCURRENTLY` est refusé, et tout rafraîchissement prend un verrou
exclusif — donc coupe la recherche pendant les quarante-quatre secondes qu'il
dure.

### `maintenance_work_mem`

Le défaut est de **64 Mo**, et le connecteur ne le relevait pas. Sur 200 000
blocs, la différence avec 1 Go est faible — 11 151 ms contre 10 908 ms, soit
2 % — parce que l'index tient presque en mémoire à cette taille. Il est relevé
explicitement tout de même : l'écart grandit avec le corpus, et laisser un
réglage par défaut sur une construction d'index GIN de quatre gigaoctets n'est
pas un choix, c'est un oubli.

### Ne pas maintenir l'index pendant le chargement

Un index GIN présent pendant l'insertion coûte **112,3 s** contre **89,8 + 13,5**
pour le même construit en bloc à la fin — 8 %. L'écart est modeste parce que la
liste d'attente de GIN amortit déjà les insertions, mais il est gratuit à
prendre : avec la vue matérialisée, la question ne se pose plus du tout, les
index ne portant pas sur les tables que `COPY` remplit.

---

## 3. Plein texte contre trigrammes, sur la même donnée

Les 1 236 284 titres d'actes, indexés **deux fois** : GIN sur `tsvector` et GIN
sur `gin_trgm_ops`. Même table, mêmes lignes, deux index.

### Taille

| Index | Taille |
|---|---|
| GIN plein texte | **52 Mo** |
| GIN trigrammes | **215 Mo** |

Quatre fois plus, pour indexer le même texte.

### Vitesse et résultats

| Recherche | Plein texte | Trigrammes | Commentaire |
|---|---|---|---|
| terme courant — `retraite` | **209 ms** / 42 285 | 329 ms / 42 073 | le désuffixage trouve « retraites », « retraité » |
| **expression** — `composition du gouvernement` | **4,3 ms** / 169 | 172 ms / 169 | **40× plus rapide**, même résultat |
| nom propre rare — `Nunez` | **0,9 ms** / 8 | 2,4 ms / 7 | |
| **faute de frappe** — `gouvernment` | 0,5 ms / **0** | **7 652 ms** / 1 | le trigramme est le seul à trouver — pour huit secondes |
| **accent absent** — `Elysee` | 1,2 ms / **60** | 1,6 ms / **10** | le plein texte trouve **six fois plus** |
| fragment — `commissaire` | **43 ms** / 7 959 | 75 ms / 7 960 | |
| conjonction — `budget` et `sécurité` | **4,3 ms** / **255** | 22 ms / 46 | |

### Ce que chacun sait faire

**Le plein texte gagne presque partout, et il gagne pour une raison
structurelle : il normalise une fois, à l'indexation.** Accents, suffixes, mots
vides sont résolus avant que la question ne soit posée. Le trigramme, lui,
compare des formes : c'est à l'appelant d'énumérer les variantes. Écrire
`ILIKE '%lysée%' OR ILIKE '%lysee%'` ramène bien les 60 actes — mais il faut y
avoir pensé.

**Le trigramme gagne sur un seul point, et il est décisif : la faute de frappe.**
`gouvernment` ne correspond à aucun lexème ; le plein texte rend zéro résultat
et n'a aucun moyen de faire mieux. Le trigramme trouve — en 7,6 secondes sur
1,2 million de titres.

D'où l'usage : **plein texte pour chercher, trigramme pour rattraper.** Une
recherche qui ne rend rien peut être rejouée en trigrammes pour proposer « vouliez-vous
dire… ». L'inverse — tout indexer en trigrammes — coûte quatre fois la taille et
perd le désuffixage, les phrases et les accents.

---

## 4. Le second index : les élus

Chercher « NUNEZ » dans 1,24 million d'actes rend tous les Nunez depuis 1861.
Chercher « Laurent » puis « Nunez » séparément rend en plus tous les actes où les
deux mots se croisent sans se toucher. **41,9 % de nos élus ont un homonyme
exact en nom et prénom.**

Le thésaurus ramène les graphies d'un nom à un jeton canonique — l'identifiant
de la personne — et la reconnaissance se fait par **recherche de phrase** :
le prénom doit précéder immédiatement le nom.

### Pourquoi pas un dictionnaire `thesaurus` de PostgreSQL

C'est l'outil prévu, et il ne convient pas ici, par ordre de gravité :

1. il se configure par un **fichier** déposé dans `$SHAREDIR/tsearch_data`. Sur
   une base gérée — Scaleway, où ce projet doit tourner — on n'écrit pas de
   fichier sur le serveur. Le schéma ne serait pas déployable ;
2. le fichier vit dans l'image, pas dans le dépôt. Il disparaît au premier
   changement d'image, et l'image vient d'en changer ;
3. la documentation de PostgreSQL avertit qu'un thésaurus est chargé en mémoire
   **à son premier usage dans chaque session**, et qu'il n'est pas prévu pour un
   grand nombre d'entrées.

Le thésaurus est donc une **table**, `ref.elu_recherche`, et la substitution se
fait à l'indexation plutôt qu'à l'analyse lexicale. Le résultat est le même — un
jeton canonique par personne — et il se réplique, se sauvegarde et se déploie.

### Population

3 401 personnes ayant exercé un mandat **national** (député, sénateur,
eurodéputé, ministre, président). Les 34 743 maires en fonction sont exclus : le
Journal officiel les nomme rarement, et les inclure multiplierait par dix le coût
de reconnaissance pour un gain proche de zéro. La règle est dans le code, pas
dans une liste.

### Ce que la reconnaissance dit, et ce qu'elle ne dit pas

Un acte qui nomme « Laurent NUNEZ » nomme **quelqu'un de ce nom**. Le Journal
officiel ne porte aucune date de naissance. D'où la colonne `homonymes` dans
`jo.acte_elu` : au-delà de 1, la citation est ambiguë et doit être présentée
comme telle. C'est la même discipline que `core.acte_jo_mention`.

---

## 5. pgvector : ce qui manque, et ce que cela mesurerait

**L'extension n'est pas dans l'image** (`postgis/postgis:17-3.5-alpine`), ni dans
les autres images disponibles localement. L'ajouter demande une image construite
sur mesure, combinant PostGIS — dont dépendent les cartes — et pgvector. C'est
une décision d'infrastructure partagée, pas un détail de schéma.

Mais le vrai obstacle est ailleurs, et il vaut d'être dit : **sans modèle
d'embedding, une comparaison pgvector ne mesurerait rien de ce qu'on en attend.**
Un index vectoriel ne trouve pas « le sens » ; il trouve des voisins dans l'espace
que le modèle a défini. Fabriquer ici des vecteurs par projection de sacs de mots
donnerait des chiffres — temps de réponse, taille d'index — mais ils
décriraient la mécanique d'IVFFlat ou de HNSW, pas la qualité sémantique. Les
présenter comme une comparaison avec le plein texte serait trompeur.

Ce qu'une vraie comparaison demanderait : un modèle d'embedding français, appliqué
à 3,8 millions de blocs, avec le coût de calcul correspondant. C'est un chantier
en soi, et il commence par choisir le modèle — pas par installer l'extension.
