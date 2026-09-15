# Pipeline CI : stockage des données brutes, orchestration du build, publication

> **Méthode** · version 2 · 15 septembre 2026
>
> Note d'étude.
> Complète [INFRA.md](../INFRA.md), qui tranche déjà où tourne le build (une VM
> à la demande) et comment le site est servi (bucket `fp-site` derrière Edge
> Services). Cette note répond à deux questions plus étroites, posées
> séparément : comment le pipeline GitHub Actions doit orchestrer
> stockage/ingestion/build à travers plusieurs jobs, et si découper la
> construction du site par lot de pages (concepts, scrutins AN, scrutins
> Sénat, collectivités…) est une bonne idée. Les chiffres cités viennent de
> mesures déjà faites ailleurs dans le dépôt (INFRA.md, docs/decisions.md
> D-012) plutôt que d'être refaits ici.

---

## 1. Ce que fait le pipeline aujourd'hui

`.github/workflows/build.yml` : un seul job, sur runner hébergé GitHub, avec
Postgres en conteneur `services:`, `timeout-minutes: 45`. Séquence : restaurer
`raw/` depuis `actions/cache`, `go run ./cmd/ingest`, deux portes (garanties
structurelles SQL, puis `go run ./cmd/verify`), `go run ./cmd/build`, publier
`site/` en artefact, et un déploiement encore non branché
(`echo "cible d'hébergement à trancher — voir docs/decisions.md D-012"`).

Deux choses à corriger avant d'ajouter quoi que ce soit :

- **Le cache `actions/cache` n'est pas un stockage durable pour l'archive.**
  GitHub plafonne le cache à 10 Go par dépôt et l'évince (LRU) sous pression ou
  après sept jours sans usage. L'archive scellée fait déjà 1,8 à 3,4 Go et
  **croît de façon monotone** (INFRA.md § 1) : elle finira par pousser
  d'autres caches dehors, ou se faire évincer elle-même — silencieusement, un
  run redevient un re-téléchargement complet sans qu'on l'ait décidé.
- **Les « 45 minutes » d'INFRA.md § 2 ne sont pas une limite de la
  plateforme.** C'est la valeur que ce fichier lui-même déclare
  (`timeout-minutes: 45`) ; GitHub autorise jusqu'à 360 minutes par job sur un
  dépôt public. La vraie contrainte dure des runners hébergés, c'est le
  **disque : 14 Go garantis**, contre un pic mesuré à ~14 Go (INFRA.md § 2) —
  zéro marge, et qui ne peut que s'aggraver avec la croissance de l'archive et
  de la base. Ce point ne change rien à la conclusion d'INFRA.md (une VM est
  nécessaire), mais la raison est le disque, pas le chronomètre.

## 2. Le stockage des données brutes (3,4 Go)

INFRA.md § 7-8 a déjà tranché : bucket `fp-archive`, classe Multi-AZ, jamais
monté en système de fichiers, alimenté par `rclone sync` après chaque
ingestion. Ce qui manque, c'est de le brancher dans le pipeline CI à la place
d'`actions/cache` :

```
1. rclone copy scw:fp-archive/ raw/      # restaurer ce qui est déjà scellé
2. go run ./cmd/ingest                   # ne récupère que le nouveau/changé
3. go run ./cmd/verify                   # porte existante
4. rclone sync raw/ scw:fp-archive/      # publier le delta
```

Ce que ça change par rapport au cache GitHub :

- **Aucune éviction, aucune limite de taille** — le calcul du § 5 d'INFRA.md
  (0,03 €/mois pour l'archive actuelle, 1,61 €/mois à 100 Go) reste valable
  quelle que soit la croissance.
- **L'archive devient un livrable, pas un détail d'implémentation.**
  docs/decisions.md D-012 pose comme condition à l'abandon du statique pur
  « des dumps complets publiés périodiquement » pour ne pas affaiblir
  l'argument de reproductibilité du projet. Un bucket accessible en lecture
  publique satisfait cette condition directement — un tiers peut rejouer
  l'ingestion sans dépendre de sources externes qui, elles, disparaissent
  (l'Ofpra 2020 qui répond 404 aujourd'hui en est l'illustration concrète,
  voir [docs/immigration-donnees.md](immigration-donnees.md) § 7.1).
- **La restauration peut être publique, la publication non.** Puisque
  l'archive est destinée à être un livrable public, `fp-archive` peut être en
  lecture publique — l'étape 1 n'a besoin d'aucun secret, y compris sur une
  pull request de fork. Seule l'étape 4 (écriture) a besoin d'une clé de
  service, et seulement sur les runs qui publient (voir § 5).

## 3. Faut-il plusieurs jobs pour construire le site ?

### 3.1 Ce que la mesure dit déjà

docs/decisions.md D-012 a chronométré un build complet à **1 min 32 s pour
9 094 pages**. INFRA.md § 1 chiffre l'ingestion à **1 h 22 cumulées, dont
47 minutes pour le seul connecteur `an-exposes`**. Le rendu des pages n'est
pas le goulot — la récupération réseau des données sources l'est, de très
loin.

### 3.2 Découper `cmd/build` par lot répond à la mauvaise question

`cmd/build/main.go` est aujourd'hui une fonction `run()` unique : elle ouvre
un pool Postgres, construit tout le site dans un répertoire `.construction`,
puis bascule en un `os.Rename` atomique
(`mettreEnPlace`, [cmd/build/main.go:118](../cmd/build/main.go)). Il n'existe
pas de `-only` comme sur `cmd/ingest` (`flag.String("only", ...)`), et
l'introduire pour de vrai découper le travail en jobs indépendants se heurte
à deux obstacles :

- **Des artefacts partagés par toutes les pages.** `assets.go` produit un
  paquet CSS/JS unique, sa signature (empreinte) référencée par chaque page
  — un pré-calcul bon marché mais commun, à faire une fois avant tout lot.
- **Un index qui agrège tout le corpus, pas un lot.** `cmd/build/recherche.go`
  construit l'index de recherche locale à partir de `persons`, `candidats`,
  `orgs`, `groupes`, `refs`, `themes` et `docs` réunis
  ([cmd/build/recherche.go:28](../cmd/build/recherche.go)) — députés, sénateurs
  et candidats 2027 dans le même fichier. Un lot « scrutins Sénat » isolé ne
  peut pas produire cet index seul ; il faudrait soit une étape d'agrégation
  finale qui retouche déjà toute chose, soit accepter un index partiel puis
  fusionné, ce qui est plus de code que le problème n'en vaut vu le temps réel
  en jeu (1 min 32 s pour l'ensemble).
- **La bascule atomique se paierait en cohérence perçue.** Aujourd'hui, un
  déploiement remplace le site entier d'un coup ; des lots publiés
  indépendamment (chacun vers son propre préfixe de bucket, par exemple)
  introduisent une fenêtre où les scrutins AN sont à jour et les scrutins
  Sénat encore de la veille — pas forcément grave pour ce site, mais c'est un
  renoncement, pas un gain gratuit.

### 3.3 Là où le parallélisme paierait réellement : les connecteurs d'ingestion

Le vrai temps est dans `cmd/ingest`, pas dans `cmd/build`. Les connecteurs
écrivent dans des tables indépendantes (chacun avec son propre `DELETE` puis
`COPY`, un pattern déjà appliqué pour éviter que deux connecteurs
s'effacent — voir par exemple `core.prime_activite_effectif` séparé de
`core.minima_sociaux_effectif`). Rien n'empêche *a priori* de lancer plusieurs
`Ingest*` indépendants en parallèle (un `errgroup` dans `cmd/ingest`, même
process, même pool de connexions avec un `MaxConns` suffisant) plutôt que de
les enchaîner séquentiellement — ce qui réduirait le temps mur sans toucher à
la topologie GitHub Actions ni au partage d'une base entre jobs séparés.

**Ce n'est pas vérifié dans cette note** : il faudrait relire chaque
connecteur pour écarter un état partagé (fichiers temporaires nommés en dur,
compteurs globaux) avant de paralléliser — un travail de revue à part, pas une
simple bascule de configuration.

### 3.4 Pourquoi le découpage par *job* GitHub Actions coûte cher spécifiquement

Un job GitHub Actions tourne sur sa propre machine : un conteneur
`services: postgres` ne survit pas à la fin de son job, donc deux jobs ne
peuvent pas partager la même base en cours de construction. La seule façon
propre de faire un lot par job est un relais `pg_dump` / `pg_restore` (ou un
dépôt sur bucket, comme pour l'archive) entre le job d'ingestion et chaque job
de build — soit **7,3 Go de base à retransférer autant de fois qu'il y a de
lots**. C'est ce coût-là, spécifique au découpage en jobs séparés (pas au
découpage en lui-même), qui rend l'option chère.

### 3.5 Recommandation

Garder `cmd/build` en un seul job avec sa bascule atomique actuelle — elle est
déjà correcte et rapide. Découper en jobs séparés ce qui l'est réellement et
sans coût : un job léger de vérification (`go vet`, `gofmt -l`, tests sur base
vide) qui répond en quelques secondes sur chaque pull request, distinct du job
lourd (ingestion + build + publication) qui ne tourne que sur `push: main` et
planifié. C'est découper pour la latence de retour aux contributeurs, pas pour
absorber les 3,4 Go / 1 h 22 — ce que ni le nombre de pages ni leur poids ne
justifient au vu des mesures ci-dessus.

## 4. Publication directe en bucket

L'intuition est la bonne, et c'est déjà la forme retenue par INFRA.md § 3 et
§ 7 : bucket `fp-site` en classe One Zone, alimenté par `rclone sync site/
scw:fp-site/`, exposé derrière **Edge Services** pour le domaine et le
certificat. Un point que ni INFRA.md ni cette question ne rendaient explicite :

**Scaleway Object Storage sait servir un bucket directement en site statique**,
sans aucun calcul devant : la fonctionnalité *Bucket Website* expose le
contenu sur `https://<bucket>.s3-website.<région>.scw.cloud`, avec document
d'index et page d'erreur configurables
([documentation Scaleway](https://www.scaleway.com/en/docs/object-storage/how-to/use-bucket-website/)).
L'activer applique automatiquement une politique de lecture publique à tout
le bucket — sans conséquence ici puisque `fp-site` ne contient que du contenu
déjà public.

Deux limites de cette route directe, qui ramènent exactement à ce qu'INFRA.md
a déjà budgété :

- **Le point de terminaison natif n'a pas de domaine personnalisé, et son
  certificat TLS ne couvre que `*.scw.cloud`** — pas
  `faits-politiques.fr`. Pour le vrai domaine avec HTTPS dessus, il faut Edge
  Services devant (certificat Let's Encrypt géré par Scaleway), déjà prévu et
  chiffré à 0,99 €/mois (offre Starter, INFRA.md § 5).
- **Rien d'autre à trancher côté hébergement** — l'instinct de « bucket
  servi en HTTP, très peu cher » et le plan déjà écrit dans INFRA.md sont la
  même architecture, pas deux options concurrentes.

**Sur « v1 sur dev-runner suffit »** : lu comme *démarrer avec la VM de build
d'INFRA.md et le cycle en quatre commandes (ingest, verify, build, deux
`rclone sync`), sans attendre le runner auto-hébergé ni le domaine
personnalisé*. C'est cohérent : rien dans ce cycle ne dépend de CI pour
exister — INFRA.md § 9 le décrit déjà comme déclenchable à la main ou par
`cron`. Ce qui peut légitimement attendre une v2 (runner auto-hébergé,
domaine, WAF) est déjà listé en INFRA.md § 11.

**Un détail à régler dès la v1, indépendamment du reste** : les métadonnées
`Cache-Control` par objet. `rclone`/`mc` déduisent le `Content-Type` de
l'extension automatiquement, mais pas le `Cache-Control` — à fixer
explicitement, et différemment selon la classe de fichier : les actifs
empreints (`assets.go` fingerprinte CSS/JS, un nouveau contenu change le nom
de fichier) supportent un `max-age` long et immuable ; les pages HTML, elles,
gardent le même nom d'une reconstruction à l'autre et ont besoin d'un TTL
court ou d'un `no-cache` — sans quoi un scrutin publié « au fil des séances »
(le motif même du cron actuel) resterait caché derrière une version en cache
le temps du TTL.

## 5. Secrets et déclenchement — ce qui change avec l'écriture sur bucket

Le workflow actuel se déclenche déjà sur `pull_request:`, mais sans secret
d'écriture à protéger — l'étape de déploiement n'est qu'un `echo`. Ça change
dès qu'une clé Scaleway avec droit d'écriture entre dans le pipeline :

- La clé de service doit être **scoped en écriture aux seuls buckets
  `fp-archive` et `fp-site`**, jamais au compte Scaleway entier.
- Les étapes d'écriture (`rclone sync` vers les deux buckets) doivent rester
  gardées par la même condition que l'étape de déploiement actuelle
  (`github.ref == 'refs/heads/main' && github.event_name != 'pull_request'`)
  — déjà correcte dans le fichier existant, à répliquer sur les nouvelles
  étapes plutôt qu'à réinventer.
- La restauration de l'archive (lecture) peut au contraire rester
  inconditionnelle et sans secret si `fp-archive` est en lecture publique
  (§ 2) : une pull request de fork en bénéficie sans jamais voir de clé.

## 6. Ce qui reste à trancher

1. **Paralléliser les connecteurs d'ingestion** (§ 3.3) : vérifier l'absence
   d'état partagé avant de lancer plusieurs `Ingest*` en `errgroup`.
2. **Le `-only` de `cmd/build`** n'est pas nécessaire pour la publication,
   mais resterait utile en développement local pour ne reconstruire qu'une
   famille de pages sans attendre le corpus entier — un besoin différent du
   découpage en jobs CI, à ne pas confondre.
3. **Domaine et certificat** restent hors budget de ce document, comme déjà
   noté en INFRA.md § 11.4.

## Versions

- **Version 2** (15 septembre 2026) : en-tête commun des documents de méthode (perimetre.md § 2.8, D-066).
- **Version 1** (14 septembre 2026) : note d'étude du pipeline.
