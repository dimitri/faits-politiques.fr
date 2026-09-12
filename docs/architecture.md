# Architecture — site statique, dépôt public, build en CI

> Étude de faisabilité. 11 septembre 2026.
> Conclusion : **oui, et c'est un meilleur choix que l'architecture dynamique
> envisagée au départ** — pas d'abord pour le coût, mais parce que le site statique
> transforme l'exigence d'auditabilité en mécanisme de déploiement.

---

## 1. Pourquoi c'est le bon choix ici

### 1.1 La propriété décisive

Le README de la base pose un invariant :

> `core` doit être **intégralement reconstructible** depuis `raw` par une fonction
> idempotente.

Dans une architecture dynamique, cet invariant est une promesse qu'on teste en
intégration continue. Dans une architecture statique, **il devient le mécanisme de
publication lui-même**. Le site est une fonction pure de (données brutes + dépôt). Si le
build passe, l'invariant tient. S'il ne tient pas, il n'y a pas de site.

Conséquence directe, et c'est l'argument central : **n'importe qui peut reconstruire le
site entier et comparer sa sortie à la vôtre.** Un contradicteur qui vous accuse d'avoir
trafiqué un chiffre n'a pas à vous croire — il relance le build et il diffe. Aucune
architecture dynamique ne peut offrir cela.

### 1.2 Le reste des avantages

- **Les données sont lentes.** Scrutins par séance, RNE trimestriel, OFGL annuel, CRC au
  fil des contrôles. Rien n'est temps réel ; rien ne justifie un serveur permanent.
- **Les permaliens scellés sont natifs.** Un site statique est du contenu adressé par
  chemin, versionné dans git. `core.page_version` et son empreinte deviennent presque
  redondants — l'historique du dépôt les porte déjà.
- **Surface d'attaque nulle.** Ni injection, ni prise de contrôle applicative.
- **Résistance aux pics.** Un lien cité dans un débat télévisé n'a aucun effet sur un
  CDN, alors qu'il tuerait un Postgres serverless à 4 connexions.
- **Coût.** Quelques euros par mois, hors stockage des documents scellés.

### 1.3 Git implémente déjà ce que j'avais modélisé en SQL

C'est l'observation la plus utile de cette étude.

| Modèle SQL | Équivalent git |
|---|---|
| `mapping_lineage` | un dépôt, ou un répertoire |
| `mapping_revision` | un commit |
| révision gelée + `content_hash` | un tag, et le SHA du commit |
| `forked_from_revision_id` | un fork |
| contestation | une issue, ou une pull request |
| `edit_token_hash` | les droits du dépôt |

**Les décisions éditoriales deviennent donc des fichiers du dépôt**, pas des lignes en
base : cartes de rattachement, codage de sens, corpus pré-enregistrés, table d'alias,
titres neutres et sections « ce que ça ne dit pas ».

Chaque modification est une **diff relisible**. Contester le codage d'un scrutin, c'est
ouvrir une pull request sur une ligne de CSV. Le schéma SQL reste la représentation de
**build**, git devient la représentation **autoritative**.

---

## 2. Le chaînage

```
sources publiques
        │  cmd/ingest   (Go)
        ▼
  object storage        documents scellés, immuables, adressés par SHA-256
        │
        ▼
  Postgres éphémère     service du runner CI, monté puis jeté à chaque build
        │  migrations + décisions éditoriales de /data
        ▼
  cmd/build   (Go)      requêtes SQL + templates
        │
        ├──► HTML statique
        └──► corpus.sqlite   (le même socle, interrogeable dans le navigateur)
        │
        ▼
  object storage + CDN
```

Postgres n'existe **que pendant le build**. C'est un moteur de requête, pas une
infrastructure. Il n'y a rien à exploiter, rien à sauvegarder, rien à surveiller.

---

## 3. Le dimensionnement, qui décide de l'hébergement

### 3.1 Ce qui mérite une page, et ce qui n'en mérite pas

| Objet | Ordre de grandeur | Page statique ? |
|---|---|---|
| Communes | ~34 900 | ✅ |
| Scrutins (selon la profondeur retenue) | quelques milliers à ~15 000 | ✅ |
| Parlementaires, actuels et passés | ~3 000 | ✅ |
| Dossiers législatifs | quelques milliers | ✅ |
| **Amendements** | **plusieurs centaines de milliers** | ❌ |
| **Élus au RNE** | **~500 000** | ❌ |

Ordre de grandeur cible : **~50 000 pages**. Les amendements et les élus locaux sont
servis par recherche côté client sur un index, pas par une page chacun.

### 3.2 Conséquence sur l'hébergement

| Hébergeur | Limite bloquante | Verdict |
|---|---|---|
| [GitHub Pages](https://docs.github.com/en/pages/getting-started-with-github-pages/github-pages-limits) | **1 Go** de site, déploiement **> 10 min en échec** | ❌ 50 000 pages passent difficilement |
| [Cloudflare Pages](https://developers.cloudflare.com/pages/platform/limits/) gratuit | **20 000 fichiers** par déploiement | ❌ |
| Cloudflare Pages payant | 100 000 fichiers depuis janvier 2026 | ⚠️ passe, mais peu de marge |
| **Scaleway Object Storage + Edge Services** | pas de limite de nombre de fichiers | ✅ **recommandé** |

**Recommandation : build dans GitHub Actions, déploiement vers Scaleway Object
Storage derrière Edge Services.** Cela conserve la décision d'hébergement français prise
plus tôt, cohérente avec le positionnement du projet, sans plafond de fichiers, pour
quelques euros par mois.

GitHub Pages reste utile pour une prévisualisation de branche ou une version réduite.

---

## 4. Ce que le statique fait très bien

### 4.1 SQLite servi comme un actif statique

Le build produit, en plus du HTML, un fichier **SQLite du corpus complet**, interrogeable
depuis le navigateur par requêtes HTTP `Range` — seules les pages de la base réellement
nécessaires sont téléchargées.

Ce que cela débloque, sans un seul serveur :

- **La recherche**, y compris plein texte.
- **La statistique reproductible** dont le périmètre est encodé dans l'URL : le calcul se
  fait chez le lecteur, avec le même socle que le vôtre.
- **Les filtres nommés** : un dossier est une URL, calculée à la volée côté client, avec
  sa sélectivité recalculée localement.
- **Les requêtes libres**, pour un journaliste ou un chercheur qui veut vérifier un
  chiffre autrement que par vos pages.

Pour un outil de vérification, laisser le lecteur exécuter ses propres requêtes sur les
données publiées est l'argument de confiance le plus fort possible.

### 4.2 L'archive scellée

Les documents primaires (XML, PDF, pages archivées, programmes de campagne) **ne vont pas
dans git** : ils gonfleraient le dépôt sans bénéfice. Ils vont dans l'object storage,
adressés par leur empreinte, bucket versionné. Le dépôt ne porte que les empreintes et
les métadonnées — ce qui suffit à prouver qu'un document n'a pas changé.

---

## 5. Ce que le statique casse, et comment

### 5.1 La contribution anonyme

C'est le vrai coût. La décision « aucun compte nulle part » devient :

| Fonction | En statique |
|---|---|
| Filtre nommé, partagé | ✅ URL encodée, calcul côté client — **survit intact** |
| Carte de rattachement forkée | ⚠️ devient un **fork du dépôt** — public et signé, mais exige de savoir utiliser git |
| Codage de sens alternatif | ⚠️ idem |
| Contestation d'une fiche | ⚠️ une **issue** avec gabarit, tracée publiquement |

Pour les cartes et les codages, c'est acceptable : le public visé — journalistes,
collaborateurs parlementaires, chercheurs — sait ouvrir une pull request, et l'exigence
« public et signé » était de toute façon posée. **Pour la contestation citoyenne, la
barre monte réellement**, et il faudra à terme un formulaire adossé à un petit service
d'écriture. À assumer comme une limite de la V1, pas à masquer.

### 5.2 Le délai de publication

Corriger une coquille impose un rebuild complet. Mitigation : build incrémental, en ne
régénérant que les pages dont les données ou les gabarits ont changé.

### 5.3 Le temps de build

Le poste coûteux n'est pas le rendu — 50 000 pages en Go tiennent en quelques dizaines de
secondes — mais **le téléchargement et l'analyse des dumps**. D'où : ingestion
incrémentale, documents bruts conservés dans l'object storage entre deux builds, et
re-parsing uniquement de ce qui a changé. Les minutes d'exécution sont gratuites et
illimitées sur les runners standards d'un dépôt public.

---

## 6. Structure du dépôt

```
cmd/ingest/            connecteurs : sources -> raw -> core
cmd/build/             core -> HTML + corpus.sqlite
internal/              parsing, entity resolution, rendu
db/migrations/         schéma, 13 migrations
db/tests/              70 garanties structurelles
docs/                  périmètre, pré-enregistrements, architecture
data/                  LES DÉCISIONS ÉDITORIALES, en clair et relisibles
  mappings/reference/nuance-party.csv
  codings/institutions.csv
  corpus/preenr-002-scrutins.csv
  alias.csv
  resumes/<scrutin>.md        titre neutre + « ce que ça ne dit pas »
web/templates/
.github/workflows/build.yml
```

`data/` est le cœur politique du projet : **tout ce qui relève d'une décision y est un
fichier, et toute décision y est une diff.**

Règle de revue conseillée : une modification dans `data/` exige une relecture
contradictoire, une modification dans `internal/` une relecture technique.

---

## 7. Le workflow

```yaml
on:
  schedule: [{cron: '0 5 * * *'}]
  workflow_dispatch:
  pull_request:          # build sans déploiement, pour relire une diff de data/

jobs:
  build:
    services:
      postgres: {image: postgres:17}
    steps:
      - checkout
      - setup-go
      - restore cache des documents bruts
      - goose up
      - cmd/ingest      (incrémental)
      - db/tests        # les 70 garanties, avant toute publication
      - cmd/build
      - vérification : liens morts, pages sans source, filtres sans sélectivité
      - déploiement (uniquement sur la branche principale)
```

Deux portes avant publication :

1. **Les tests de garanties** tournent sur la base réellement chargée, pas sur une base
   vide. Une donnée qui violerait une règle éditoriale bloque le déploiement.
2. **Une vérification de cohérence** : aucune page publiée sans source, aucun filtre sans
   sélectivité calculée, aucun résultat citant une révision non gelée.

Sur une pull request, le build tourne **sans déployer** : on voit l'effet d'un
changement de codage avant de le fusionner.

---

## 8. Recommandation, et arbitrage retenu

L'analyse ci-dessus plaide pour le statique : il transforme l'exigence de
reproductibilité en mécanisme de déploiement, ce qu'aucune architecture dynamique ne peut
offrir.

**Arbitrage retenu le 11 septembre 2026** (voir [D-012](decisions.md)) : le statique
n'est conservé que s'il tient dans l'offre gratuite de GitHub Pages. À défaut,
application dynamique en conteneur serverless Scaleway avec base managée.

Le seuil est mesurable : les ~34 900 pages de communes font l'essentiel du volume. Si
elles sont servies depuis le SQLite côté client, le site retombe autour de 15 000 à
20 000 pages, soit environ 600 Mo — sous la limite du gigaoctet. **Le risque résiduel est
le timeout de 10 minutes au déploiement**, à mesurer sur un build réel avant de trancher.

Dans l'hypothèse dynamique, quatre choses que le statique donnait gratuitement doivent
être reconstruites délibérément : la reproductibilité (pipeline de reconstruction et
dumps publiés), le scellement des pages, la protection contre les pics de trafic, et la
maîtrise du pool de connexions. Le détail est dans [D-012](decisions.md).

Trois éléments survivent dans les deux cas, et méritent d'être posés dès maintenant :
`data/` versionné dans git, l'export SQLite publié, et l'ingestion en Serverless Jobs
plutôt qu'en conteneurs.

Séquence inchangée : dépôt public, schéma et `data/` d'abord, puis le connecteur AN, puis
le build et les gabarits. La décision d'hébergement peut être prise plus tard, sur des
chiffres réels, sans rien remettre en cause en amont.
