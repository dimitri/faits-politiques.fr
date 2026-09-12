# Base de données

Schéma Postgres du socle. Il traduit en contraintes les règles de
[docs/perimetre.md](../docs/perimetre.md) — l'objectif est qu'une règle éditoriale
qui compte soit impossible à violer, pas seulement documentée.

## Couches

| Schéma | Rôle | Règle |
|---|---|---|
| `raw` | Copie exacte de ce qui a été récupéré | Aucun `UPDATE`, aucun `DELETE` |
| `ref` | Nomenclatures externes | Toujours millésimées |
| `core` | Données normalisées et historisées | Toute ligne porte une preuve |
| `derived` | Indicateurs et statistiques | Toute ligne porte sa `method_version` |
| `selection` | Filtres nommés sur les faits | Aucune prose. Ne référence que `core` et `derived`, jamais l'inverse |

`core` doit être **intégralement reconstructible** depuis `raw` par une fonction
idempotente. Rejouer l'ingestion deux fois doit produire un état identique.

## Migrations

Format [goose](https://github.com/pressly/goose), appliquées dans l'ordre numérique.

```bash
goose -dir db/migrations postgres "$DATABASE_URL" up
```

## Tests

Deux fichiers, exécutés dans une transaction annulée à la fin. Ils ne laissent aucune
trace et peuvent tourner sur une base existante.

```bash
psql -v ON_ERROR_STOP=1 -d "$DATABASE_URL" -f db/tests/constraints_test.sql
psql -v ON_ERROR_STOP=1 -d "$DATABASE_URL" -f db/tests/selection_test.sql
psql -v ON_ERROR_STOP=1 -d "$DATABASE_URL" -f db/tests/mapping_test.sql
psql -v ON_ERROR_STOP=1 -d "$DATABASE_URL" -f db/tests/classifications_test.sql
```

Attendu : 20, 15, 21 puis 14 garanties vérifiées — **70 au total**.

## Garanties couvertes par le schéma

Ces points ne sont pas des conventions : ils échouent à l'écriture.

1. **Aucune source non commercialement réutilisable** n'entre dans le pipeline
   (`raw.source.commercial_use`) — ce qui exclut les dumps CC BY-NC-SA au profit des
   sources primaires en Licence Ouverte.
2. **Un vote nominatif est impossible sur un scrutin de granularité `GROUP`**, et
   réciproquement. C'est la traduction SQL de « ne jamais projeter la position d'un
   groupe sur un individu » — le cas du Sénat.
3. **Une mise au point est indissociable de sa date.** Le relevé officiel et le
   relevé rectifié sont stockés séparément : agréger l'un et afficher l'autre est la
   seule façon de ne publier ni un vote faux ni un résultat faux.
4. **Une preuve porte exactement un sujet** et pointe toujours vers un document
   scellé (`sha256`) et une récupération datée.
5. **Un document ne peut pas être attaché à une récupération HTTP en échec.**
6. **Pas de mandats du même type qui se chevauchent** pour une même personne ;
   un mandat national et un mandat local simultanés restent possibles.
7. **Un seul groupe parlementaire à la fois**, mais une double appartenance
   partisane reste permise.
8. **Une attribution `UNRESOLVED` ne peut désigner aucune organisation** — on
   n'écrit « le groupe X a proposé ceci » que lorsque l'attribution est univoque.
9. **Un résumé de provenance IA ne peut pas être marqué vérifié humainement.**
10. **Une contestation close doit porter une réponse.**
11. **Aucune clé étrangère ne remonte de `core`/`derived` vers `selection`.** La
    subordination des filtres aux faits est vérifiée par introspection du catalogue,
    pas par convention.
12. **Un filtre ne se publie pas sans sélectivité calculée.** Sans prose, le filtre est
    l'argument : il doit déclarer combien de faits il retient sur combien d'éligibles.
    La sélectivité est calculée par le système, jamais saisie par l'auteur.
13. **Un dossier par critères ne peut pas contenir de faits choisis à la main**, et
    réciproquement un dossier manuel déclare toujours l'univers dont il extrait.
14. **Un calcul comparatif ne se rattache qu'à un protocole scellé** et ne peut pas
    être antérieur à son scellement.
15. **Un résultat publié ne peut citer qu'une révision de carte gelée**, jamais une tête
    encore modifiable — sinon il devient irreproductible dès que la carte évolue.
16. **Une seule carte de rattachement de référence**, listée et sans jeton d'édition
    anonyme. Toutes les autres sont non listées par défaut.
17. **Une seule tête modifiable par lignée de carte** ; une révision gelée est en
    lecture seule, insertion, modification et suppression comprises.
18. **Seule une attribution `EXACT` rattache une commune à un parti.** La colonne
    `aggregatable` est calculée, pas laissée à la vigilance de qui écrit la requête.
19. **Une nuance ne désigne exactement qu'un parti par révision**, alors qu'une nuance
    de coalition peut légitimement en recouvrir plusieurs.
20. **Une attribution `EXACT` exige un code de justification** pris dans un vocabulaire
    fermé — ce qui rend deux cartes comparables ligne à ligne.
21. **Le même indicateur coexiste sur deux périmètres budgétaires** (`COMMUNE`, `CCAS`)
    sans collision, mais reste unique par périmètre.
22. **Une année d'élection est comptée au prorata** entre deux mandats.
23. **Une source restreinte est ingérable mais reste hors du périmètre redistribuable.**
    `reuse_class` trace la restriction au lieu de la dissoudre, et `commercial_use` en
    est calculée — on ne peut pas la forcer. Une absence de licence explicite vaut
    `RESTRICTED`, jamais autorisation.
24. **Une classification tierce est catégorielle ou numérique, jamais les deux**, et
    deux référentiels qui divergent sur un parti sont exposés côte à côte plutôt que
    moyennés.
25. **Un codage de sens exige une justification codée** et une révision non gelée — il
    suit exactement le régime des cartes de rattachement, puisque c'est le même genre
    d'objet : une décision, pas un fait.
26. **Un objet composite n'est jamais scoré.** Un texte est un assemblage de
    dispositions pouvant aller en sens contraires ; seul l'objet réellement soumis au
    vote se code sans ambiguïté.

## Points d'attention

- `core.f_unaccent` est un wrapper `IMMUTABLE` autour de `unaccent()` : l'original est
  `STABLE` et donc inutilisable dans un index.
- Les clés primaires sont des `bigint identity` internes ; les permaliens publics
  passent par les colonnes `slug`, immuables. Ne jamais exposer une clé technique.
- `core.scrutin.objet` contient l'intitulé officiel transcrit sans reformulation.
  Toute reformulation destinée à l'affichage vit dans `core.scrutin_resume`, avec son
  rédacteur et ses relecteurs — c'est le principal point d'entrée du biais éditorial,
  il est donc isolé et tracé.
