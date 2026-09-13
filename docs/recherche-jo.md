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

---

## 2. Où mettre le vecteur : la mesure contredit l'intuition

Deux façons d'indexer : une colonne `tsvector` **générée et stockée**, ou un
**index fonctionnel** sur l'expression. Mesurées dans cet ordre, elles donnent
des verdicts opposés.

### Au chargement, l'index fonctionnel gagne — de peu

| | Chargement | Index | Total | Stockage |
|---|---|---|---|---|
| Colonne générée | 1 832 s | 211 s | 2 043 s | **+3 Go** |
| Index fonctionnel | **187 s** | 1 687 s | **1 874 s** | — |

Le calcul de `to_tsvector` coûte le même prix des deux côtés, environ 1 650 s, et
il est **sériel dans les deux cas** : `COPY` ne parallélise pas les colonnes
générées, et PostgreSQL 17 ne parallélise pas la construction d'un index GIN.

### À la requête, la colonne stockée gagne par trois ordres de grandeur

| Requête | Vecteur stocké | Index fonctionnel |
|---|---|---|
| Recherche de **phrase** | **15 ms** | **50 057 ms** |
| Même table, un nom : phrase | **—** | 1 807 ms |
| Même table, même mots en conjonction | — | 12,6 ms |

**La raison tient en une phrase : GIN ne stocke pas les positions des lexèmes.**
Une conjonction se résout dans l'index seul. Une recherche de phrase ne le peut
pas — elle doit vérifier l'adjacence sur chaque candidat. Avec une colonne
stockée, cette vérification **lit** un vecteur ; avec un index fonctionnel, elle
le **recalcule**, sur des blocs qui font parfois plusieurs mégaoctets.

Un corpus se charge une fois et s'interroge indéfiniment : les trois gigaoctets
et les vingt-huit minutes sont le bon prix. La colonne générée avait été retirée
sur la foi du seul chargement ; elle est restaurée.

> C'est précisément le cas d'usage de l'extension **RUM**, qui range les
> positions dans les listes d'occurrences et rend phrase et classement sans accès
> au tas. Elle n'est pas dans l'image et reste une piste.

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
