# Hébergement

> Version 1 — 13 septembre 2026.
> Décide où tournent l'ingestion, la base et le site, et ce que ça coûte.
> Tous les chiffres de dimensionnement sont **mesurés sur l'installation actuelle** ;
> les prix sont ceux de Scaleway au 13 septembre 2026, hors taxes.

---

## 1. Ce qu'il faut porter

| | mesuré |
|---|---|
| Base Postgres | **7,3 Go** (`commune_delinquance` 2,1 Go, `ballot` 821 Mo, `commune_indicator` 771 Mo) |
| Ingestion complète | **1 h 22** cumulées, dont 47 min pour `an-exposes` seul |
| `site/` généré | **3,8 Go / 40 293 fichiers** — ~0,6 Go une fois les actifs partagés extraits |
| Archive `raw/` | **1,81 Go, 3 400 documents** — médiane 41 Ko, maximum 283 Mo, **croissance monotone** |

Deux propriétés commandent toute l'architecture :

- **Le site est statique.** Rien n'interroge la base à la lecture. La base n'a donc
  besoin d'exister *que pendant le build*.
- **Tout est reconstructible sauf l'archive.** La base se régénère depuis `raw/` et le
  dépôt. Les seuls actifs irremplaçables sont l'archive scellée — des documents saisis
  à une date, dont certains disparaîtront du web — et Git. Il n'y a donc **pas de
  stratégie de sauvegarde** au-delà de la réplication de l'archive.

## 2. Pourquoi pas GitHub Pages

Pas pour la raison qu'on croit. Après extraction des actifs partagés, le site tombe à
~0,6 Go et **passerait** sous la limite de 1 Go de Pages. Trois autres raisons tranchent :

1. **Le nombre de fichiers.** 40 293 aujourd'hui, ~78 000 si les votes nominatifs
   sortent en JSON par scrutin. Pages ne documente pas de plafond, mais **le
   déploiement expire à 10 minutes** — c'est un pari à chaque publication.
2. **Le build ne tient pas dans un runner hébergé.** 14 Go de disque garantis contre
   ~14 Go de pic (base 7,3 + archive 1,9 + site 3,8 + chaîne Go 1), et 45 minutes de
   délai déclaré contre 1 h 22 d'ingestion mesurée. **Il faut une machine de toute
   façon** — et dès lors, servir depuis un bucket coûte 1 € de plus par mois.
3. **Cloudflare Pages est exclu d'office** : 20 000 fichiers par déploiement.

## 3. La forme retenue

```
                    ┌──────────────────────────────┐
   ssh / Claude ───►│  VM de build (à la demande)  │
                    │  Postgres · Go · cmd/build   │
                    └───────┬──────────────┬───────┘
                            │              │
              rclone sync   │              │  rclone sync
                            ▼              ▼
                  ┌──────────────┐  ┌──────────────┐
                  │ bucket       │  │ bucket       │
                  │ archive      │  │ site         │
                  │ Multi-AZ     │  │ One Zone     │
                  └──────────────┘  └──────┬───────┘
                                           │
                                    Edge Services
                                    (HTTPS, cache, domaine)
                                           │
                                           ▼
                                        lecteurs
```

La VM est **éteinte hors des builds**. Scaleway suspend la facturation du calcul à
l'arrêt ; le volume racine et l'IPv4 restent facturés.

## 4. Dimensionnement

**VM de build : 4 vCPU / 16 Go / volume racine 40 à 60 Go.**

Les 16 Go ne sont pas du luxe : Postgres construit des index sur plusieurs millions de
lignes, et `cmd/build` rend 40 000 pages. Surtout, **le confort est presque gratuit dans
ce modèle** — passer de 8 à 16 Go coûte 12,55 €/mois en fonctionnement continu, mais
**1,57 €/mois à trois heures par jour**. Il n'y a aucune raison de se serrer.

Le volume racine porte l'OS, la chaîne Go et les données Postgres. Ni l'archive ni le
site n'y résident durablement : c'est ce qui permet de rester à 40 Go plutôt qu'à 80.

## 5. Coût

| Poste | 3,8 Go de site, 3 h/j | 0,6 Go de site, 2 h/j |
|---|---|---|
| Calcul BASIC2-A4C-16G | 6,29 | 4,19 |
| Volume racine (0,095 €/Go/mois) | 5,70 (60 Go) | 3,80 (40 Go) |
| IPv4 flexible | 3,65 | 3,65 |
| Bucket site (One Zone, 0,00803 €/Go) | 0,03 | 0,00 |
| Bucket archive (Multi-AZ, 0,01606 €/Go) | 0,03 | 0,03 |
| Edge Services Starter (100 Go de cache) | 0,99 | 0,99 |
| **Total** | **16,69 €/mois HT** | **12,67 €/mois HT** |

Repères pour arbitrer : **+2,10 €/mois** par heure de build quotidienne supplémentaire,
**+0,95 €/mois** par tranche de 10 Go de volume racine. L'egress d'Object Storage est
offert jusqu'à 75 Go/mois, puis facturé 0,01 €/Go — mais le trafic passe par Edge
Services, dont le forfait Starter inclut 100 Go de cache.

**Variante sans IPv4 : 9,02 €/mois.** Les instances reçoivent une IPv6 gratuite ; si
l'accès SSH se fait en IPv6, l'adresse IPv4 flexible — facturée 3,65 €/mois même machine
éteinte — devient inutile. À ne retenir que si la connexion depuis laquelle on
administre est effectivement en IPv6.

## 6. Ce qui reste payé quand la machine dort

Il faut être clair : **le volume racine ne peut pas disparaître.** Une instance a besoin
d'un disque de démarrage, et Postgres a besoin d'un vrai système de fichiers. Le
plancher est donc de 3,80 à 5,70 €/mois, plus l'IPv4.

Ce que déplacer l'archive et le site vers les buckets fait gagner : **5,4 Go de volume
en moins, soit ~0,51 €/mois** aujourd'hui — mais l'archive croît indéfiniment, et c'est
là que le calcul devient intéressant. À 20 Go d'archive, le bucket coûte 0,32 €/mois
contre 1,90 € de disque bloc ; à 100 Go, 1,61 € contre 9,50 €. **Le gain n'est pas dans
le prix d'aujourd'hui, il est dans la pente.**

Deux fausses bonnes idées écartées :

- *Supprimer le volume entre deux builds et le recréer depuis un instantané.* Les
  instantanés coûtent 0,036 €/Go/mois, moins cher que le bloc, mais la restauration
  prend du temps à chaque démarrage et ajoute une pièce mobile à un système qui n'en a
  pas besoin pour économiser deux euros.
- *Reconstruire la base à chaque build.* Techniquement possible — tout est
  reconstructible — mais c'est 1 h 22 d'ingestion pour republier une page.

## 7. Les deux buckets

| | classe | contenu | motif |
|---|---|---|---|
| `fp-archive` | **Multi-AZ**, 0,01606 €/Go/mois | l'archive scellée | seul actif irremplaçable du projet ; trois zones de disponibilité pour trois centimes par mois |
| `fp-site` | **One Zone**, 0,00803 €/Go/mois | le site généré | intégralement reconstructible en une commande ; payer la redondance n'aurait aucun sens |

Le choix de classe n'est pas cosmétique : il découle directement du § 1. Ce qui se
reconstruit va en One Zone, ce qui ne se reconstruit pas va en Multi-AZ.

## 8. Monter le bucket comme un système de fichiers

La question se pose naturellement : le code écrit dans un répertoire, autant lui donner
un répertoire. **Des clients existent, et ce serait ici le mauvais outil.**

Ce qui existe sous Linux :

| Client | Caractère | Réserve |
|---|---|---|
| **rclone mount** | FUSE, multi-fournisseurs, plusieurs modes de cache VFS, actif | le plus lent sur les métadonnées, mais **le plus insensible à la latence** |
| **s3fs-fuse** | le plus mature, sémantique POSIX la plus complète | **un aller-retour HTTP synchrone à chaque fermeture de fichier** ; ×2,6 de ralentissement entre un backend proche et un backend lointain |
| **goofys** | le plus rapide en lecture, pas de cache local | POSIX très partiel, maintenance irrégulière |
| **JuiceFS** | vrai système de fichiers POSIX sur objet | exige un moteur de métadonnées séparé (Redis, Postgres…) — une pièce de plus à exploiter |

**Pourquoi c'est le mauvais outil pour cette archive.** `internal/archive/fetchOnce`
fait exactement trois choses sur le disque :

```go
tmp, _ := os.CreateTemp(a.Root, ".dl-*")   // le téléchargement transite par a.Root
...
if _, err := os.Stat(dst); err == nil {    // un HEAD par document
    cached = true
} else if err := os.Rename(tmp.Name(), dst) // pas d'équivalent atomique en objet
```

Sur un point de montage, cela donne : le téléchargement d'un fichier de **283 Mo** est
écrit *dans le bucket*, puis « renommé », c'est-à-dire recopié côté serveur puis
supprimé. **Le trafic double et l'opération la plus fréquente du connecteur devient la
plus coûteuse du protocole.** Et rien ne le justifie, parce que — c'est le point
décisif — **l'archive n'est jamais relue**. `fetchOnce` télécharge toujours, calcule
l'empreinte, et ne consulte le disque que pour éviter de réécrire. C'est un registre en
écriture seule.

**Ce qu'il faut faire à la place**, par ordre de coût :

1. **`rclone sync raw/ scw:fp-archive/` après chaque ingestion.** Zéro ligne de Go,
   l'archive locale reste la référence pendant le run, le bucket est la copie durable.
   C'est le point de départ.
2. **Écrire le fichier temporaire sur le disque local et téléverser l'objet final.**
   Une quinzaine de lignes dans `fetchOnce` : `os.CreateTemp` dans `os.TempDir()`, et un
   `PutObject` à la place du `os.Rename`. Le `os.Stat` devient un `HeadObject`, qui est
   exactement ce qu'il faut. L'archive quitte alors le disque pour de bon.
3. Un montage FUSE : seulement si un outil externe doit parcourir l'archive comme un
   répertoire. Dans ce cas, **rclone mount**, pour son insensibilité à la latence.

## 9. Fonctionnement

**Cycle de build.** Allumer la VM (API Scaleway ou `scw instance server start`),
`fpctl ingest data`, `fpctl verify data`, `fpctl build site`,
`rclone sync site/ scw:fp-site/`, `rclone sync raw/ scw:fp-archive/`, éteindre. Le tout
scriptable en une commande, déclenchable depuis une machine tierce.

**Session Claude à distance.** Claude Code est un binaire en ligne de commande : il
tourne sur la VM comme ailleurs. `ssh` puis `tmux` — et tmux n'est pas un confort ici,
une ingestion d'1 h 22 ne survit pas à une coupure de connexion. L'authentification se
fait une fois sur la machine. Conséquence sur le modèle de coût : **une session de
travail allume la machine**, donc la facturation suit les heures réellement passées, ce
qui est la bonne forme.

**Un point à trancher** : machine éteinte, on ne peut pas s'y connecter. Soit on
l'allume depuis l'extérieur avant chaque session, soit on garde une seconde petite
instance permanente (DEV1-S, ~12 €/mois avec son disque et son IP) qui sert de point
d'entrée et d'orchestrateur. La première option est plus simple et moins chère ; la
seconde évite de dépendre de la console Scaleway.

## 10. GitHub Actions : le runner auto-hébergé

Un runner auto-hébergé sur la VM de build lève d'un coup les deux limites du § 2 — plus
de plafond de disque, plus de délai de 45 minutes — et rend inutile le cache de 1,9 Go
que le workflow actuel reconstruit à chaque exécution.

**Mais GitHub déconseille explicitement les runners auto-hébergés sur les dépôts
publics.** N'importe qui peut forker, ouvrir une pull request et faire exécuter du code
arbitraire sur la machine, avec accès aux secrets et au `GITHUB_TOKEN`. Le runner n'est
pas isolé par défaut. Or `.github/workflows/build.yml` se déclenche aujourd'hui sur
`pull_request:`.

Conditions minimales avant d'installer un runner :

- retirer `pull_request:` des déclencheurs, ne garder que `workflow_dispatch`,
  `schedule` et `push` sur `main` ;
- exiger l'approbation manuelle pour tout contributeur extérieur ;
- faire tourner le runner en **mode éphémère**, dans un conteneur détruit après chaque
  job, pour qu'une exécution ne puisse pas contaminer la suivante ;
- ne stocker aucun secret durable sur la machine — les clés du bucket appartiennent au
  compte de service du build, pas au runner.

Alternative plus sobre : **pas de runner du tout.** Le workflow GitHub se contente de
valider (`go build`, `go vet`, `gofmt`, tests SQL sur une base vide), et le build réel
est déclenché par `cron` sur la VM. On perd le tableau de bord, on gagne de ne pas
exposer la machine.

## 11. Ce qui reste à trancher

1. **L'extraction des actifs partagés** (1,53 Go de CSS, SVG et JS recopiés 38 042 fois)
   n'est pas une optimisation d'hébergement mais elle change tous les chiffres de ce
   document. À faire avant de dimensionner définitivement.
2. **Runner auto-hébergé ou cron local** (§ 10).
3. **IPv4 ou IPv6 seul** (§ 5) — 3,65 €/mois, mais surtout une question d'accès.
4. **Domaine et certificat** : Edge Services génère un Let's Encrypt pour un
   sous-domaine personnalisé. Le nom de domaine lui-même n'est pas dans ce budget.
