# Documentation — faits-politiques.fr

Index des documents. Chaque fichier a une fonction précise ; le journal des
décisions fait foi en cas de contradiction, parce qu'il est daté.

## À lire d'abord

| Document | Ce qu'il contient |
|---|---|
| [decisions.md](decisions.md) | **Le journal des décisions, D-001 à D-058.** Toute affirmation de ce projet doit pouvoir s'y rattacher, y compris les erreurs corrigées. |
| [perimetre.md](perimetre.md) | Ce que le projet couvre, la matrice des sources avec leurs licences, et ce qui est hors de portée. |
| [architecture.md](architecture.md) | Le choix statique / dynamique et ses conséquences. |

## Conception par domaine

| Document | Question traitée |
|---|---|
| [themes-conception.md](themes-conception.md) | Que permettent les thèmes officiels du Sénat sur les votes ? Et corrèlent-ils avec la classification gauche-droite ? |
| [mairies-conception.md](mairies-conception.md) | Communes, nuances, comptes, intercommunalités : ce qui est comparable et ce qui ne l'est pas. |
| [securite-conception.md](securite-conception.md) | Délinquance enregistrée et mandats municipaux : ce qui est vérifiable, et pourquoi l'attribution reste fautive. |
| [agriculture-carte-conception.md](agriculture-carte-conception.md) | Cartographier l'agriculture : où trouver le recensement communal, et trois pièges — le vide n'est pas un zéro, la surface est au siège, une exploitation n'est pas un paysan. |
| [candidats-donnees.md](candidats-donnees.md) | Frise de carrière, mandats actuels, réconciliation des identités. |
| [gouvernement-donnees.md](gouvernement-donnees.md) | Onglet GOUVERNEMENT : grandes séries nationales, comptes des sociétés, et ce qui n'existe pas (manifestations). |
| [budget-donnees.md](budget-donnees.md) | Budget de l'État et budget de la Sécurité sociale : comment ils s'articulent, et quelles sources donnent le voté et l'exécuté par année. |
| [dette-donnees.md](dette-donnees.md) | La dette publique : quatre définitions, qui la détient, taux de marché contre taux apparent, comparaison européenne, la Suisse ; à quoi sert l’emprunt (compte de capital), niches fiscales et exonérations en regard des dividendes. |
| [cotisations-et-droits.md](cotisations-et-droits.md) | Ce qu'une cotisation achète : répartition ou capitalisation, droit contributif ou non, et le poids de chacun. Section « pour aller plus loin » sur un socle universel. |
| [monnaie-et-inflation.md](monnaie-et-inflation.md) | Francs, euros, inflation : ce que `core` stocke, ce que `derived` calcule, et pourquoi une valeur déflatée n'est pas un fait. |
| [entreprises-perimetre.md](entreprises-perimetre.md) | `core.entreprise` n'est pas le CAC 40 : trois populations d'entreprises à ne pas confondre, et la réserve à écrire sur les dividendes. |
| [violences-policieres-donnees.md](violences-policieres-donnees.md) | Le piège d'étiquette « par » contre « contre », ce que la France et l'Europe ne publient pas, et pourquoi les arrêts CEDH sont un majorant. |
| [recherche-jo.md](recherche-jo.md) | Chercher dans le Journal officiel : configuration française, colonne stockée contre index fonctionnel, plein texte contre trigrammes, thésaurus des élus. |
| [scrutins-et-bulletins.md](scrutins-et-bulletins.md) | Le modèle des scrutins et des votes nominatifs. |
| [contributions-utilisateurs.md](contributions-utilisateurs.md) | Cartographies alternatives sans compte utilisateur. |
| [charte-graphique.md](charte-graphique.md) | Palette, contrastes, typographie. |

## Protocoles scellés avant calcul

| Document | Objet | État |
|---|---|---|
| [pre-enregistrement-001.md](pre-enregistrement-001.md) | Comparaison municipale par étiquette | **brouillon, non scellé** |
| [pre-enregistrement-002.md](pre-enregistrement-002.md) | Axe INSTITUTIONS, page d'accueil 2027 | **brouillon, non scellé** |

Aucune statistique comparative ne doit être publiée avant que le protocole
correspondant soit scellé. C'est ce qui garantit qu'un résultat sera publié
qu'il aille dans un sens ou dans l'autre.

## Les règles qui ne se négocient pas

Elles sont dispersées dans le journal ; les voici rassemblées.

1. **Aucun rapprochement par nom quand un identifiant existe** (D-025, D-033,
   D-035). Trois résolutions automatiques du CAC 40 ont produit de mauvaises
   entités avant qu'on cherche le pont par identifiant — qui existait.
2. **Aucun DELETE ni TRUNCATE dont la portée n'est pas bornée** par une clause
   sur l'institution ou sur une source (D-029, D-031). Quatre destructions de
   données ont eu ce motif. Un commentaire n'empêche rien ; seul un contrôle
   empêche.
3. **Ce qui n'est pas publié reste visible comme tel** (D-030, SSMSI) :
   « non publié », « non diffusé » et « zéro » sont trois faits différents.
4. **Une absence de licence n'est pas une autorisation** (CHES, municipales
   2020). Toute exception est datée et nominative (D-036).
5. **La concomitance n'est jamais une imputation** (D-026, D-037). Une courbe
   sous une présidence ne dit pas que cette présidence l'a produite.
6. **Avant de comparer un chiffre entre deux collectivités, vérifier qu'elles
   décident de ce qu'on mesure** (D-024, securite-conception §3).

## État du chargement

`go run ./cmd/verify` vérifie la cohérence avant toute publication. Les étapes
d'ingestion sont listées par `go run ./cmd/ingest -h`.
