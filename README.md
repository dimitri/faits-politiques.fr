# faits-politiques.fr

Outil de vérification factuelle de l'action politique française. Il documente **ce qui a
été proposé, voté et décidé**, avec la source primaire de chaque donnée — et il ne
conclut jamais à la place du lecteur.

Cas d'usage : sortir d'un débat télévisé et contrôler une affirmation ; sourcer ou
contredire une affirmation sous contrainte de temps ; contester une erreur formulée par
un adversaire ou un commentateur.

## Principes

Quatre règles gouvernent tout le reste, et sont traduites en **contraintes de base de
données** plutôt qu'en bonnes intentions :

1. **Aucun verdict.** Ni « vrai », ni « faux ». Un objet factuel, sa source, ses limites.
2. **Le « non vérifiable » est une réponse de plein droit**, avec sa raison typée.
   Un outil bien fait résout 25 à 35 % des affirmations d'un débat, pas 90 %.
3. **Jamais de causalité.** Un indicateur décrit une évolution, pas l'effet d'une
   politique.
4. **Jamais un vote de groupe projeté sur un individu.** La granularité de chaque
   scrutin est une donnée stockée, pas une convention.

Voir [docs/perimetre.md](docs/perimetre.md), et [docs/decisions.md](docs/decisions.md)
pour l'historique des arbitrages.

## Démarrage

```bash
make db-up      # Postgres local (docker)
make ingest     # télécharge, scelle et charge les données de l'Assemblée nationale
make build      # génère le site statique dans ./site
```

Première exécution : environ 6 minutes, dont 40 Mo téléchargés.

## Architecture

```
sources publiques  ──►  raw/        archive scellée : octets + SHA-256, jamais écrasés
                        raw.*       métadonnées, récupérations datées
                        ref.*       nomenclatures externes, millésimées
                        core.*      données normalisées, historisées, sourcées
                        derived.*   indicateurs, avec method_version
                        selection.* filtres nommés, sans prose
                   ──►  site/       HTML statique
                        web/        gabarits, marque, fontes hébergées en propre
```

Sections publiées : Assemblée nationale, **Sénat**, Parlement européen,
**thèmes** (les 30 thèmes officiels du Sénat, rattachés aux scrutins de
l'Assemblée par la navette), candidats, partis, et **Comprendre** — les
documents de `docs/` rendus intégralement en pages du site.

Le site est **statique et sans dépendance distante** : aucune requête vers un tiers,
aucun traceur, aucune fonte de CDN. Le seul JavaScript est une recherche locale sur un
index statique, et ses déclencheurs restent masqués s'il ne s'exécute pas — le reste du
site fonctionne sans lui. Présentation : [docs/charte-graphique.md](docs/charte-graphique.md).

**Invariant central** : `core` est intégralement reconstructible depuis `raw` par une
fonction idempotente. `fpctl ingest data` reconstruit les tables dérivées à chaque
exécution plutôt que de les compléter — rejouer l'ingestion doit produire un état
identique.

## Commandes

Toutes routées par `fpctl` (`cmd/fpctl` ; voir `fpctl help`) — un seul point d'entrée,
un verbe puis un nom, comme `git` :

| Commande | Rôle |
|---|---|
| `fpctl ingest data` | connecteurs, archive scellée, `raw` → `core` |
| `fpctl verify data` | contrôles de cohérence des **données chargées** — porte de publication |
| `fpctl build site` | `core` → site statique |
| `fpctl list sources` | catalogue des sources ingérées |
| `fpctl generate dossiers`, `fpctl generate bulletin` | sections chiffrées des dossiers documentaires |

## Deux portes avant publication

1. **`db/tests/*.sql`** — 70 garanties structurelles, exécutées sur la base réellement
   chargée. Une donnée qui violerait une règle éditoriale bloque le déploiement.
2. **`fpctl verify data`** — concordance des décomptes chargés avec le relevé officiel
   publié par l'Assemblée. Cette porte a déjà servi : elle a détecté que le jeu de
   données « députés actifs » omettait les députés ayant quitté leur siège en cours de
   législature, dont les votes disparaissaient silencieusement.

```bash
make test
fpctl verify data
```

## `data/` — les décisions éditoriales

Tout ce qui relève d'un **choix** y est un fichier, et toute modification y est une diff
relisible : liste des candidats et leurs sources, cartes de rattachement, codage de sens,
corpus pré-enregistrés, alias, **seuils de majorité par type de scrutin**
(`data/seuils.csv`, avec sa règle et sa source — sans lui, « 197 pour · 0 contre →
rejeté » n'est pas lisible).

Une modification dans `data/` exige une relecture contradictoire ; une modification dans
`internal/` une relecture technique.

## Sources

Assemblée nationale — [open data](https://data.assemblee-nationale.fr/), Licence Ouverte.
Seuls les **scrutins publics** sont couverts : la majorité des votes ont lieu à main
levée et ne laissent aucune trace nominative. Ni cet outil ni aucun autre ne peut dire
qui a voté quoi sur ces textes.

Aucune source non librement redistribuable n'entre dans le pipeline sans que sa
restriction soit tracée (`raw.source.reuse_class`).

## Licence

Code sous AGPL-3.0. Données produites sous Licence Ouverte, dans la limite des licences
des sources amont.
