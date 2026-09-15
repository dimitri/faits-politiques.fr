# Journal des décisions

Une entrée par décision structurante : ce qui a été décidé, pourquoi, et ce que cela
coûte. Les entrées ne sont jamais réécrites ; une décision qui change fait l'objet d'une
nouvelle entrée qui cite la précédente.

---

## D-001 — Vérification plutôt que quiz

**Décidé.** Le produit est un outil de vérification factuelle, pas une application de
proximité politique.

**Pourquoi.** Le quiz aveugle sur votes réels existe déjà au moins cinq fois en France
(Datan, LegiTest, LegiWatch, VoteMatch, Leurs Votes). La vérification post-débat n'a pas
d'équivalent.

**Coût.** On renonce au moteur d'audience le plus efficace.

---

## D-002 — Aucun verdict, jamais

**Décidé.** L'outil ne produit pas de « vrai / faux ». Il produit un objet factuel, sa
source primaire et ses limites explicites.

**Pourquoi.** Une seule erreur de verdict détruit la crédibilité de l'ensemble. Un
document sourcé n'est pas attaquable de la même façon.

---

## D-003 — Le « non vérifiable » est une réponse de plein droit

**Décidé.** Sept raisons typées (`ref.unverifiable_reason`), affichées.

**Pourquoi.** Un outil bien fait résout 25 à 35 % des affirmations d'un débat télévisé.
Afficher l'absence protège mieux que la taire.

---

## D-004 — Indicateurs communaux, pas décisions municipales

**Décidé.** Le volet local documente ce que les producteurs publics **mesurent** chaque
année, jamais ce qu'une commune **a décidé**.

**Pourquoi.** Aucun agrégateur national des délibérations n'existe, l'adoption du schéma
SCDL reste marginale, et une couverture partielle serait biaisée vers les collectivités
les mieux dotées — ce qui invaliderait toute comparaison inter-partis.

**Coût.** Les tarifs de cantine, exemple le plus cité du débat, ne sont disponibles que
par collecte manuelle.

---

## D-005 — Pré-enregistrement obligatoire des comparaisons

**Décidé.** Population, indicateurs et méthode scellés avant le premier calcul.
Protocoles [001](pre-enregistrement-001.md) et [002](pre-enregistrement-002.md).

**Pourquoi.** Sans cela, tout écart observé est attribuable au choix des indicateurs, et
l'objection est irréfutable même quand elle est fausse.

**Coût.** On s'engage à publier des résultats contraires à ses attentes.

---

## D-006 — Symétrie par construction

**Décidé.** Mêmes indicateurs, même méthode, même mise en avant pour toutes les
organisations. Un gabarit est un filtre paramétré par sujet.

**Pourquoi.** Une page qui n'existerait que pour une organisation est un outil de
campagne, pas un outil de référence.

---

## D-007 — Un dossier est un filtre, pas un texte

**Décidé.** Aucune prose dans le produit. Le schéma `editorial` est supprimé au profit
de `selection`.

**Pourquoi.** Un filtre est de la donnée structurée ; il peut donc être créé et partagé
sans compte.

**Conséquence.** Le biais se déplace : sans prose, **le filtre est l'argument**. La
garantie centrale devient « toute sélection déclare sa sélectivité », calculée par le
système et non par l'auteur.

---

## D-008 — Aucun compte utilisateur

**Décidé.** Ni pour consulter, ni pour créer, ni pour publier. Édition par jeton de
capacité haché, objets non listés par défaut.

**Pourquoi.** Un compte stockant des préférences politiques serait un profil d'opinion —
donnée sensible au sens de l'article 9 du RGPD. En supprimant les comptes, cette donnée
n'existe jamais.

---

## D-009 — Seul `EXACT` agrège

**Décidé.** Une commune n'est rattachée à un parti que par une nuance qualifiée `EXACT`.
`COALITION`, `BROADER` et `NOT_MAPPABLE` sont affichées et comptées dans le taux
d'exclusion.

**Pourquoi.** La nuance RNE est une qualification préfectorale, pas une adhésion. Trois
objets distincts coexistent — groupe parlementaire, parti au sens du rattachement JO,
nuance — et les fusionner produit des chiffres faux.

---

## D-010 — Cartes et codages forkables

**Décidé.** Les rattachements et le codage de sens vivent dans des lignées nommées avec
révisions gelées. Un résultat publié ne cite qu'une révision gelée.

**Pourquoi.** Ce sont des décisions éditoriales, pas des faits. La parade la plus forte
n'est pas « faites-nous confiance » mais « voici le nôtre, publiez le vôtre ».

---

## D-011 — Projet non commercial, mais restriction tracée

**Décidé.** La contrainte binaire `commercial_use` est remplacée par `reuse_class` par
source, avec la vue `raw.source_redistribuable`.

**Pourquoi.** Le non-commercial se propage aux sorties : un journaliste d'un média
commercial ne pourrait plus réutiliser les données dérivées, alors que c'est un des
publics visés. Tracer la restriction plutôt que la dissoudre préserve l'option d'un
export ouvert.

**Note.** PopuList v4.0 est en CC BY 4.0. CHES ne publie **aucune licence explicite** —
enregistré en `RESTRICTED` : une absence de licence n'est pas une autorisation.

---

## D-012 — Hébergement : conteneur serverless et Postgres managée sur Scaleway

**Décidé le 11 septembre 2026.** Le site statique n'est retenu que s'il tient dans
l'offre gratuite de GitHub Pages. À défaut, application dynamique en conteneur serverless
Scaleway, avec Database for PostgreSQL managée.

**Pourquoi.** Si une infrastructure payante est nécessaire de toute façon, autant qu'elle
apporte des capacités — contestation par formulaire sans compte GitHub, édition des
cartes en interface, publication incrémentale sans rebuild complet, aucun plafond de
pages.

**Seuil de bascule.** GitHub Pages plafonne à 1 Go de site et échoue au-delà de 10
minutes de déploiement. Les ~34 900 pages de communes représentent l'essentiel du
volume : servies depuis le SQLite côté client, le site retombe autour de 15 000 à 20 000
pages, soit environ 600 Mo — sous la limite. **Le risque résiduel est le timeout de
déploiement**, à mesurer avant de trancher.

**Ce qu'il faut reconstruire délibérément**, parce que le statique le donnait
gratuitement :

1. **La reproductibilité.** Elle n'est plus une propriété du déploiement. Il faut un
   pipeline de reconstruction depuis `raw` exécuté en tâche planifiée, une commande
   publique permettant à un tiers de rejouer le build, et des dumps complets publiés
   périodiquement. Sans cela, l'argument fondateur du projet s'affaiblit.
2. **Le scellement des pages.** `core.page_version` redevient porteur : en statique,
   l'historique git le portait.
3. **La protection contre les pics.** Un lien cité dans un débat télévisé frapperait
   l'application directement. CDN devant, en-têtes de cache longs — la quasi-totalité des
   pages est de fait immuable.
4. **Le pool de connexions.** Piège classique du scale-to-zero : `MaxConns` de 2 à 4 par
   instance et `max_scale` plafonné, sous peine de saturer la base managée au scale-up.

**Ce qui ne change pas.**

- `data/` reste dans git : les décisions éditoriales doivent rester des diffs relisibles,
  et la base les charge au déploiement. Git continue de jouer le rôle de
  `mapping_lineage` / `mapping_revision`.
- L'export SQLite reste produit et publié — même s'il n'est plus le moteur de requête, il
  reste ce qui permet à un tiers de vérifier autrement que par vos pages.
- **L'ingestion passe par des Serverless Jobs, pas par des conteneurs** : les conteneurs
  sont déclenchés par HTTP et plafonnés en durée de requête.
- File de travaux adossée à Postgres (River), pas de courtier de messages.

**Cible probable à terme.** Application dynamique derrière un CDN à TTL long : statique
pour les lecteurs, dynamique pour les écritures. C'est ce qui combine les deux jeux de
propriétés, et rien dans le schéma actuel ne s'y oppose.

---

## D-013 — Mesure réelle : le site statique tient dans l'offre gratuite

**Mesuré le 11 septembre 2026**, sur le socle Assemblée nationale seul.

| | Prévu (D-012) | Mesuré |
|---|---|---|
| Pages | ~50 000 | **9 094** |
| Poids | — | **287 Mo** |
| Durée du build | — | **1 min 32 s** |

L'écart vient des communes : les ~34 900 fiches communales représentaient l'essentiel du
volume prévu, et le volet local n'est pas encore ingéré. Sur le périmètre parlementaire
seul, **les deux plafonds qui avaient écarté les hébergeurs gratuits sont respectés** :
1 Go pour GitHub Pages, 20 000 fichiers pour Cloudflare Pages.

Le risque résiduel identifié en D-012 — le **timeout de déploiement de 10 minutes** de
GitHub Pages — reste à mesurer sur un déploiement réel. C'est désormais le seul critère
qui décide entre statique gratuit et conteneur serverless.

À l'ajout du volet communal, le seuil sera franchi : les fiches communales devront alors
être servies depuis le SQLite côté client plutôt qu'en pages, ou l'hébergement devra
changer. La décision n'a pas à être prise avant.

---

## D-014 — Le groupe d'un votant est transcrit, jamais reconstitué

**Décidé le 11 septembre 2026**, après un bug détecté en production locale.

Reconstituer le groupe parlementaire d'un député à la date d'un scrutin à partir de ses
mandats plaçait **les 577 députés dans « Non inscrit »** : les fichiers de mandats publiés
par l'Assemblée ne portent pas les groupes de la 17e législature.

Or chaque scrutin publie sa **ventilation par groupe** : la source indique donc, pour
chaque votant, le groupe sous lequel son vote a été enregistré ce jour-là.
`core.ballot.organization_id` stocke cette valeur (migration 0014). C'est une
transcription, pas une inférence.

**Portée générale.** Quand la source publie directement le fait, on le transcrit ; on ne
le recalcule pas à partir d'autres tables, même quand le recalcul paraît plus élégant.
Un calcul intermédiaire est une occasion supplémentaire de se tromper, et il masque
l'erreur au lieu de la révéler.

Deux contrôles de `cmd/verify` gardent désormais cette propriété : la ventilation doit
faire apparaître au moins huit groupes significatifs, et plus de 95 % des votes doivent
porter un groupe.

---

## D-015 — Retrait des condamnations CEDH

**Décidé le 12 septembre 2026.** La section n'apportait rien à l'objectif du produit :
documenter ce que les responsables politiques votent, proposent et décident. Un arrêt de
la Cour condamne un État, pas une personne ni un parti — il ne se raccroche à aucun des
acteurs que ce site suit.

Le connecteur et les migrations restent au dépôt (on ne supprime pas une migration
appliquée), mais plus rien ne les appelle.

---

## D-016 — Le Sénat publie bien les votes individuels

**Corrigé le 12 septembre 2026.** Ce projet a affirmé pendant toute sa conception que le
Sénat ne publiait que des positions **de groupe**, avec les seules exceptions nommées.
**C'est faux.** La base Dosleg contient la table `votsen` : **1 647 612 positions
nominatives** de 971 sénateurs sur 4 764 scrutins, de 2006 à 2026.

L'erreur venait de la présentation des scrutins sur le site public du Sénat, pas de ses
données ouvertes. Elle figurait dans le périmètre, dans les pages du site et dans les
commentaires du schéma. La granularité `GROUP` reste utile au modèle, mais elle ne
s'applique pas au Sénat tel qu'il publie aujourd'hui.

**Ce que la leçon coûte.** Une caractéristique de source affirmée sans avoir été vérifiée
dans les données s'est propagée pendant toute la conception, jusqu'à justifier de ne pas
écrire un connecteur. Une affirmation sur ce qu'une source contient doit être vérifiée
dans la source, pas déduite de son site.

---

## D-017 — Une classification thématique officielle existe pour le Parlement français

**Constaté le 12 septembre 2026.** Ce projet a longtemps posé qu'aucune classification
thématique officielle des textes français n'existait, et que toute taxonomie serait donc
une décision éditoriale exigeant un protocole scellé.

**C'est vrai pour l'Assemblée, faux pour le Sénat.** La base Dosleg publie 30 thèmes —
Police et sécurité, Société, Environnement, Questions sociales et santé, Pouvoirs publics
et Constitution, Éducation, Énergie, Justice, Famille… — et les rattache aux textes de
loi : **17 660 affectations sur 8 412 lois**, attribuées par les services du Sénat.

C'est une **transcription**, pas un classement de ce site. Elle ouvre une voie qui n'était
pas envisagée : classer thématiquement des textes français sans produire soi-même de
jugement, en reprenant la classification du Sénat et, pour l'Europe, celle d'EuroVoc
(1 808 concepts, 68 823 affectations).

**Reste ouvert.** Les scrutins du Sénat ne sont pas rattachables à ses lois dans ce dump :
ni `corscr` ni `amescr` ne portent de référence de texte. La classification porte donc sur
les dossiers, pas sur les votes. Rapprocher les deux demanderait un travail supplémentaire
qui n'a pas été fait, et aucun lien n'a été fabriqué.

---

## D-018 — Anomalies de source : borner plutôt que masquer

**Décidé le 12 septembre 2026.** Deux scrutins du Sénat sur 4 764 comptent une voix
« pour » de plus dans le relevé nominatif que dans le décompte publié — 2020/9 et 2024/93 —
sans qu'aucune correction figure dans la table `corscr`. L'incohérence est dans la source.

Le contrôle de publication ne l'ignore pas et ne la corrige pas : il **borne l'écart à une
voix**. Un défaut d'ingestion produirait des écarts massifs, pas des écarts d'une unité.
Le seuil est donc une sonde qui distingue les deux, et l'anomalie est nommée dans le code.

## D-019 — Les thèmes du Sénat remontent aux scrutins de l'Assemblée par la navette

**Date** : 2026-09-12
**Statut** : acté

La classification thématique officielle (30 thèmes, D-017) est posée par le Sénat
sur ses **lois** (`senat_raw.loithe`), pas sur ses scrutins — et le dump Dosleg
n'offre aucun moyen de relier un scrutin sénatorial à une loi (D-016). Les thèmes
du Sénat semblaient donc inutilisables pour analyser des votes.

Ils le deviennent par un troisième chemin, mesuré et non fabriqué :

    core.scrutin (AN)
      -> core.dossier (AN).senat_chemin        916 dossiers sur 3 069
      -> signet « ppl24-125 »                  extrait de l'URL
      -> senat_raw.loi.signet -> loicod        916/916 appariés (100 %)
      -> core.dossier (SENAT).source_uid       12 429/12 429 appariés
      -> core.topic_assignment                 thèmes officiels

Rendement : **2 577 scrutins AN** portent un thème officiel, soit 98,8 % des
2 608 scrutins AN rattachés à un dossier. 4 806 couples (scrutin, thème), 28 des
30 thèmes représentés.

L'appariement repose sur une **égalité exacte de clés publiées** (le signet du
Sénat, cité tel quel par l'Assemblée dans son propre dossier), jamais sur un
rapprochement de titres. Aucun appariement flou n'est admis ici.

**Limite à afficher** : la fenêtre couverte est 2025-04-07 → 2026-07-21, parce que
le connecteur dossiers de l'AN ne charge que les dossiers ouverts récemment. Le
thème n'est pas « le thème du scrutin » mais « le thème de la loi dont ce scrutin
est une étape » — deux scrutins opposés sur le même texte portent le même thème.

## D-020 — Aucun pont parti ↔ groupe n'existe en base

**Date** : 2026-09-12
**Statut** : dette identifiée, à corriger avant toute analyse thématique

`core.party_group_link`, `core.ep_national_party_link` et `core.nuance_party_link`
sont **vides**. Le rattachement d'un parti à son groupe parlementaire n'existe que
dans la colonne `groupe_an_uid` de `data/organisations.csv`, appliquée en Go au
moment du build.

Conséquence directe : les scores CHES 2024 (73 codages, 5 dimensions, 10 partis
français) ne sont **joignables à aucun vote** en SQL. Toute question de la forme
« comment votent les partis de droite sur tel thème » est aujourd'hui sans réponse
calculable, non par manque de données mais par absence de ce pont.

Ce que la reprise doit charger : 9 liens parti→groupe AN (les 9 partis de
`organisations.csv` ayant à la fois un `ches_nom` et un `groupe_an_uid`), qui
couvrent 1 202 334 des 1 270 476 bulletins AN (94,6 %). Restent non rattachés
LIOT, UDR et les non-inscrits — à laisser explicitement hors analyse plutôt qu'à
rattacher d'office.

### D-020 — résolution (2026-09-12)

Le chargement a révélé une difficulté que D-020 n'avait pas vue : un même parti
existe en base sous **trois lignes disjointes**, une par source, et aucune ne
cite l'identifiant des autres.

    registre CNCCFP   « Rassemblement national »   CNCCFP 40    org 136
    CHES 2024         « RN »                       CHES 610     org 1013
    organes de l'AN   « Rassemblement national »   PO761239     org 1696

`core.organization_identifier` ne peut pas les réunir : `(scheme, value)` y est
UNIQUE — et doit le rester, un identifiant ne désignant qu'une chose. Le pont
parti→groupe seul n'aurait donc rien débloqué : le score CHES serait resté sur
une quatrième ligne, inatteignable depuis un bulletin.

D'où **`core.party_referential_link`** (migration 0024) : la décision éditoriale
« cette entrée de référentiel tiers désigne ce parti », portée par la même
révision de cartographie, gelable et forkable comme les autres rattachements. Le
parti canonique est l'entrée du registre CNCCFP, seule liste centrale officielle
des partis politiques français.

Chargé par `internal/carto` (`go run ./cmd/ingest -only=carto`), idempotent :
**10 liens parti→groupe**, **18 liens parti→référentiel** (CHES et PopuList).
La chaîne score → bulletin se parcourt désormais en SQL pur, sur **9 groupes**
et 1 202 334 bulletins. Deux contrôles de `cmd/verify` la gardent, dont un qui
tombe à zéro si n'importe quel maillon se rompt.

Le 10e lien parti→groupe est l'UDR, qui a un groupe mais aucune entrée CHES. Il
est chargé quand même : la cartographie décrit ce qui est, pas ce qui arrange
l'analyse. C'est à l'analyse d'exclure ce cas explicitement.

## D-021 — Le thème d'un scrutin est dérivé, jamais réinjecté dans core

**Date** : 2026-09-12
**Statut** : acté

`derived.scrutin_topic` (migration 0025) porte le thème applicable à un scrutin,
sous deux origines :

| Origine | Apport | Ce que c'est |
|---|---|---|
| `DIRECT` | 68 823 | le thème publié par la source sur le scrutin (EuroVoc, au PE) |
| `NAVETTE` | 4 806 | le thème de la loi du Sénat citée par le dossier de l'AN (D-019) |

Le résultat n'est **jamais** reversé dans `core.topic_assignment` : une ligne de
`core` est une affirmation de la source, une ligne de `derived` est un calcul.
Les confondre rendrait indiscernable ce que le Sénat a écrit de ce que nous
avons déduit. La table se recalcule intégralement et porte son `method_version`
(`theme-v1-navette`), pour qu'un chiffre publié reste rattachable à la méthode
qui l'a produit. Un thème hérité conserve `via_dossier_id`, afin que le lecteur
puisse remonter jusqu'à la loi.

Limite à afficher partout où ces thèmes servent : le thème est celui de la
**loi**, pas du scrutin. Deux scrutins de sens opposés sur le même texte portent
le même thème. Et les scrutins du Sénat n'en portent aucun (D-016).

## D-022 — La nuance politique d'un maire ne vient pas du RNE

**Date** : 2026-09-12
**Statut** : acté — correction d'une erreur répétée dans `perimetre.md`

Le périmètre attribuait au Répertoire national des élus une colonne « code
nuance ». Vérification faite sur le fichier publié le 11 août 2026
(`elus-maire-mai.csv`, 34 826 maires, Licence Ouverte) : **quatorze colonnes,
aucune politique** — département, commune, nom, prénom, sexe, date de naissance,
catégorie socio-professionnelle, dates de mandat et de fonction. Rien d'autre.

La nuance est attribuée par les préfectures aux **listes candidates**, et n'est
publiée que dans les fichiers de résultats du ministère de l'Intérieur. Elle
qualifie donc une liste, pas une personne — troisième décalage, après le fait
qu'elle qualifie administrativement et non par adhésion.

Conséquence : relier un maire à une nuance demande une décision supplémentaire,
« la couleur d'une commune est celle de la liste ayant obtenu le plus de sièges
au conseil municipal ». Cette décision doit être écrite, versionnée et
contestable comme les autres rattachements, pas enfouie dans une requête.

Même classe d'erreur que D-016 : une affirmation sur une source, répétée dans
tout le document, que personne n'avait vérifiée contre le fichier lui-même.

## D-023 — Les mairies : neuf communes sur dix n'ont aucune étiquette

**Date** : 2026-09-12
**Statut** : acté

Mesuré sur les résultats des 15 et 22 mars 2026 (Intérieur, Licence Ouverte,
34 835 communes pourvues) :

| | communes | part |
|---|---|---|
| au moins une liste nuancée | 3 282 | 9,4 % |
| aucune nuance | 31 553 | 90,6 % |

Le seuil est de population : minimum 1 064 inscrits chez les nuancées, maximum
4 343 chez les autres. Et parmi les 3 282 communes nuancées, la majorité élue
est « divers » dans 85,9 % des cas (LDVD 34,0 %, LDVG 18,3 %, LDIV 16,9 %,
LDVC 16,8 %).

Les majorités municipales d'extrême droite représentent **61 communes** :
LRN 42, LUXD 11, LEXD 6, LUDR 2. Soit 1,9 % des communes nuancées, et 0,18 % des
communes françaises.

**Ce que ce chiffre interdit.** Une comparaison « communes RN contre les autres »
porte sur 61 unités, dont la moitié sous 10 000 habitants, face à un groupe de
contrôle 50 fois plus grand et de composition entièrement différente. Aucun écart
observé ne pourra être attribué à l'étiquette plutôt qu'à la taille, à la région
ou au niveau de revenu. C'est une limite de puissance statistique, pas un choix
éditorial : elle vaudrait à l'identique pour n'importe quelle autre étiquette de
même effectif.

**Ce que ce chiffre autorise.** Le cas par cas : ces 61 communes sont peu
nombreuses, nommées, et leurs comptes sont publiés commune par commune et année
par année. Les décrire une à une, sans moyenne, est à la fois faisable et plus
informatif qu'un écart agrégé que les confondants rendraient ininterprétable.

## D-024 — L'EPCI est la clé de lecture d'un budget communal

**Date** : 2026-09-12
**Statut** : acté

Un EPCI — établissement public de coopération intercommunale — est une
structure à laquelle des communes transfèrent des compétences : l'eau, les
déchets, les transports, l'urbanisme, l'action sociale. Communauté de communes,
communauté d'agglomération, communauté urbaine, métropole et syndicats en sont
les formes.

Il ne s'agit pas d'un raffinement : **sans lui, un montant communal est
illisible**. Deux communes voisines peuvent afficher des dépenses de
fonctionnement du simple au double parce que l'une a transféré la collecte des
déchets et l'autre non. Les comparer sans le savoir, c'est comparer deux
périmètres en croyant comparer deux politiques.

Le périmètre prévoyait déjà un code `EPCI_COMPETENCE` pour écarter un
indicateur quand la compétence n'est pas communale. Il lui manquait la donnée.

**Source** : BANATIC (DGCL), API `consultation/api`. Deux appels suffisent —
la nomenclature des compétences en JSON (125 entrées) et l'export national. Ce
dernier est un tableur de 76 Mo compressés, **1,4 Go de XML une fois ouvert** :
une ligne par couple (groupement, membre), 180 colonnes dont 125 de compétences
en OUI/NON.

Trois décisions de mise en œuvre :

1. **Lecture en flux, sans bibliothèque.** Les lecteurs XLSX courants
   construisent le classeur entier en mémoire ; un flux de jetons n'en garde
   qu'une ligne. Le projet n'a qu'une dépendance (pgx) et n'en prend pas une
   deuxième pour cela.
2. **Colonnes repérées par position, titres vérifiés.** Si la mise en page
   changeait sans qu'on le voie, on chargerait des compétences fausses sous des
   codes justes. Le chargement échoue si l'en-tête ne correspond pas.
3. **Appariement par identifiant.** BANATIC désigne ses membres par SIREN, tout
   le reste de la base par code INSEE. L'OFGL publie les deux sur la même
   ligne : c'est lui qui fait le pont, jamais un rapprochement de noms. Les
   125 libellés de compétence, eux, s'apparient exactement entre les deux
   produits du même producteur (125 sur 125).

La vue `core.commune_competence` répond à la seule question qui compte :
sur ce sujet, le budget de cette commune mesure-t-il encore une politique
municipale ?

**Chargé** : 9 282 groupements (3 979 SIVU, 1 901 syndicats mixtes fermés,
1 225 SIVOM, 987 communautés de communes, 769 syndicats mixtes ouverts, 230
communautés d'agglomération, 21 métropoles, 14 communautés urbaines),
129 022 adhésions de communes, 51 006 compétences exercées. 3 568 membres qui
ne sont pas des communes — autres groupements, départements, régions — sont
écartés plutôt qu'inventés en communes.

**Et le résultat renverse la lecture des comptes communaux.** Une commune a
transféré **38 compétences en moyenne** (de 6 à 89). Sur les cinq indicateurs
que le projet publie :

| Compétence transférée | Communes concernées | Part |
|---|---|---|
| Collecte des déchets ménagers | 34 503 | **98,9 %** |
| Eau (production, distribution) | 31 485 | 90,3 % |
| Assainissement collectif | 26 807 | 76,9 % |
| Plan local d'urbanisme | 21 779 | 62,5 % |
| Voirie communale | 12 013 | 34,4 % |

Autrement dit : les « dépenses de fonctionnement » d'une commune française
n'incluent presque jamais la collecte des déchets, rarement l'eau, et souvent
ni l'assainissement ni l'urbanisme. Le budget communal est un **résidu**, et sa
composition varie d'une commune à l'autre. Publier un classement de communes
sur ce total, sans dire ce que chacune a gardé, produirait un palmarès des
périmètres intercommunaux déguisé en palmarès de gestion.

## D-025 — Le RNE couvre tous les niveaux ; il ne couvre aucun passé

**Date** : 2026-09-12
**Statut** : acté

Les sept fichiers du Répertoire national des élus sont chargés, soit
**615 000 lignes** : 511 225 conseillers municipaux, 62 129 conseillers
communautaires, 34 826 maires, 4 037 conseillers départementaux, 1 745
conseillers régionaux, 577 députés, 348 sénateurs, 81 représentants au
Parlement européen.

C'est la seule source qui couvre tous les niveaux simultanément, donc la seule
qui permette d'établir la liste des mandats actuels d'une personne sans la
recomposer source par source.

**Deux limites, à afficher partout où ces mandats servent** : aucune nuance
politique (D-022), et **aucune profondeur historique** — le RNE ne publie que
la mandature en cours, sans aucune date de fin. La frise de carrière est donc,
pour l'instant, une frise du présent. Une page qui la présenterait comme une
carrière complète mentirait par omission.

**Réconciliation des identités.** Une même personne arrivait en double :
conseiller municipal au RNE, député chez l'Assemblée — 189 doublons sur les
seules données déjà chargées avant ce chantier. Le rapprochement se fait sur le
triplet **exact** (nom, prénom, date de naissance), insensible aux accents et à
la casse, et sur rien d'autre. Sans date de naissance, aucun rapprochement n'est
tenté : deux homonymes restent deux personnes. Un mandat manquant vaut mieux
qu'un mandat attribué à quelqu'un d'autre.

**Chargement ensembliste, après une première version jetée.** La version
initiale interrogeait la base par ligne : plus d'un million d'allers-retours
pour un demi-million de conseillers municipaux, soit des heures. Tout passe
désormais par un COPY vers une table temporaire puis des requêtes qui ne
touchent la base qu'une fois chacune. C'est la même leçon que pour le Sénat, et
elle avait déjà été donnée une fois.

## D-026 — La présidence est un repère chronologique, pas une imputation

**Date** : 2026-09-12
**Statut** : acté

Les présidences de la Ve République entrent en base comme des mandats de type
`PRESIDENT_REPUBLIQUE`, chargées depuis `data/presidents.csv` (source Élysée),
intérims du président du Sénat compris — les omettre laisserait des trous dans
la chronologie, et un mandat tombant dans un trou n'aurait aucun contexte.

Jusqu'ici le fichier n'était lu qu'au moment du rendu : aucune requête ne
pouvait demander « quels mandats se sont déroulés sous telle présidence ».

Deux vues (migration 0029) :

- `core.mandat_contexte` — chaque mandat avec la ou les présidences qu'il
  traverse, et la période d'intersection. Un mandat à cheval produit deux
  lignes : c'est le fait, pas un défaut.
- `core.mandat_actuel` — les mandats dont la période **contient la date du
  jour**. Le critère est explicite et non « dont la date de fin est nulle »,
  qui confondrait un mandat courant avec un mandat dont la fin n'a pas été
  publiée — exactement le cas du RNE.

**Un ministre n'est pas responsable des actes du président, ni l'inverse.** La
concomitance situe, elle n'impute pas, et toute page qui affiche la frise doit
l'écrire.

## D-027 — Les 29 702 mandats de l'Assemblée sont normalisés

**Date** : 2026-09-12
**Statut** : acté

Quatre types de mandats sur trente étaient exploités (ASSEMBLEE, MINISTERE, GP,
PARPOL). Les autres décrivaient où un député travaille réellement, et dormaient
dans `raw.record`.

**28 441 appartenances** sont désormais en base, contre 3 033 :

| Organe | Appartenances | Personnes |
|---|---|---|
| Groupes d'amitié (GA) | 6 463 | 411 |
| Groupes d'études (GE) | 5 640 | 418 |
| Commissions permanentes (COMPER) | 5 632 | 435 |
| Commissions mixtes paritaires (CMP) | 2 074 | 326 |
| Groupes politiques (GP) | 2 056 | 576 |
| Missions d'information | 1 052 | 278 |
| Organismes extraparlementaires | 842 | 331 |
| Partis politiques (PARPOL) | 774 | 431 |

Elles remontent à 1988 pour une poignée de députés de longue date, mais
l'essentiel commence en 2010 : **32 appartenances seulement précèdent cette
date** (voir D-032, qui corrige une première rédaction trop flatteuse). L'apport
réel est de 2 431 appartenances antérieures à 2017, là où le reste du jeu de
l'Assemblée ne dépasse pas la législature en cours.

Deux décisions de modèle : la valeur `PARLIAMENTARY_BODY` est ajoutée à
`core.organization_kind`, parce qu'un groupe d'amitié n'est pas une commission
et le ranger comme telle serait faux ; et `core.organization.organ_type` conserve
le code de l'Assemblée sans regroupement, un regroupement étant une décision
éditoriale.

## D-028 — Une personne n'appartient plus à un seul connecteur

**Date** : 2026-09-12
**Statut** : acté — corrige un défaut de conception

Le connecteur de l'Assemblée détruisait puis recréait les personnes qu'il
possède. C'était juste tant qu'il en était le seul producteur. Depuis que le
RNE y rattache des mandats locaux et la HATVP des déclarations, effacer une
personne parce qu'elle est députée emporterait son mandat de maire — et la clé
étrangère l'a refusé, ce qui a au moins rendu le problème visible.

Les personnes sont désormais **mises à jour en place**, jamais détruites, et
retrouvées dans cet ordre : l'identifiant de l'Assemblée, puis le triplet exact
(nom, prénom, date de naissance), puis le slug. Le deuxième chemin est celui qui
rattrape une personne créée d'abord par le RNE ; sans lui, un député par
ailleurs conseiller municipal existerait en double.

## D-029 — Un DELETE non borné a détruit les dossiers du Sénat

**Date** : 2026-09-12
**Statut** : réparé, et gardé par un contrôle

`DELETE FROM core.dossier`, sans clause, dans la remise à zéro du connecteur de
l'Assemblée. Il a emporté les **8 412 dossiers du Sénat** et les **17 660
assignations de thèmes** qui s'y rattachaient. L'héritage de thèmes par la
navette (D-019) est tombé de 4 806 à **0**, et rien ne l'a signalé — le
chargement s'est terminé en annonçant « 0 hérités » comme si c'était un résultat.

C'est la troisième fois qu'une remise à zéro dépasse son périmètre : les comptes
CNCCFP, puis les scrutins et leurs 1,27 million de votes, maintenant les
dossiers du Sénat. Le motif est toujours le même — une portée qui était juste
quand le connecteur était seul, et qui ne l'est plus quand un autre connecteur
écrit dans la même table.

Toutes les suppressions de ce bloc sont désormais bornées par
`institution = 'ASSEMBLEE_NATIONALE'`, et `DELETE FROM core.nuance_assignment`
est retiré : l'Assemblée n'en produit aucune.

Deux contrôles ajoutés à `cmd/verify`, dont celui qui aurait suffi :
« les dossiers du Sénat survivent à une renormalisation de l'Assemblée ».

## D-030 — HATVP : ce qui n'est pas publié doit rester visible comme tel

**Date** : 2026-09-12
**Statut** : acté

La Haute Autorité publie 13 687 déclarants : déclarations d'intérêts (DI, DIA
et modificatives) et déclarations de situation patrimoniale (DSP, DSPM, DSPFM).
C'est la seule source publique qui donne le **parcours** d'un responsable —
activités professionnelles des cinq dernières années, mandats détenus,
rémunérations déclarées année par année — là où le RNE n'a aucune profondeur
historique (D-025).

Elle comble aussi un angle mort : **2 513 déclarants au titre d'un EPCI**, dont
environ 1 014 présidents. Ces personnes arbitrent des budgets sans avoir été
élues directement par personne.

Deux décisions :

1. **`[Données non publiées]` est transcrit, pas traduit en NULL.** « Non
   publié » et « néant » sont deux faits différents, et les confondre ferait
   dire à la base ce que la source ne dit pas. La colonne `non_publie` porte la
   distinction.
2. **Un seul tableau pour tous les blocs**, `core.declaration_item`, avec le nom
   d'élément de la source dans `bloc`. Quinze tables auraient exigé de décider
   ce qui compte dans chacune, c'est-à-dire d'interpréter. Rien n'est perdu, et
   les champs d'adresse, de contact et de pièce d'identité sont traversés sans
   être lus — ce sont précisément ceux que la Haute Autorité ne publie pas.

Le patrimoine est chargé parce qu'il est demandé et qu'il est public là où la
loi le prévoit. La règle de publication reste à trancher : ce projet documente
ce que font les élus ; ce qu'ils possèdent n'éclaire une décision que lorsqu'un
conflit d'intérêts est en jeu.


## D-031 — Un TRUNCATE oublié a détruit les votes du Parlement européen

**Date** : 2026-09-12
**Statut** : réparé, et gardé par un contrôle

`TRUNCATE core.ballot`, sans portée, dans le chargeur de scrutins de
l'Assemblée. Il a emporté les **1 970 025 votes du Parlement européen**.

C'est la **quatrième** fois qu'une remise à zéro déborde son périmètre : les
comptes CNCCFP, les scrutins de l'Assemblée et leurs 1,27 million de votes, les
dossiers du Sénat (D-029), maintenant les votes européens. Et cette fois le
fichier voisin portait déjà, en commentaire, l'explication de pourquoi il ne
faut pas de TRUNCATE — le commentaire était là, l'instruction aussi.

Ce que cela dit de la méthode : un commentaire n'empêche rien, seul un contrôle
empêche. Les trois premières destructions avaient produit des seuils de volume
dans `cmd/verify` ; celle-ci a été trouvée par le même moyen, un contrôle qui
est passé de vert à rouge. C'est le dispositif qui fonctionne, pas la vigilance.

**Règle, désormais sans exception : aucun connecteur n'écrit un DELETE ou un
TRUNCATE dont la portée n'est pas exprimée par une clause sur l'institution ou
sur un identifiant de source.** Toute remise à zéro non bornée est un défaut,
même quand elle est correcte au moment où on l'écrit — parce qu'elle cessera de
l'être dès qu'un second connecteur écrira dans la même table.

## D-032 — La profondeur historique de l'Assemblée est plus faible qu'annoncé

**Date** : 2026-09-12
**Statut** : correction de D-027

D-027 affirmait que les appartenances aux organes de l'Assemblée « remontent à
1988 ». C'est vrai au sens strict et trompeur en pratique. La distribution
réelle :

| Période de début | Appartenances |
|---|---|
| 1985-1989 | 16 |
| 1990-2009 | 16 |
| 2010-2014 | 1 912 |
| 2015-2019 | 7 332 |
| 2020-2026 | 19 165 |

**32 appartenances seulement commencent avant 2010**, et 2 431 avant 2017. La
raison est mécanique : le jeu couvre les mandats des députés ACTUELS, et peu
d'entre eux siégeaient dans les années 1990.

L'apport reste réel — 2 431 appartenances débordent la législature en cours, là
où le reste du jeu n'en offrait aucune — mais il se compte en années, pas en
décennies. Un premier contrôle posé au jugé (« 1 000 appartenances avant 2000 »)
a échoué, et il a bien fait : il a révélé que la description était fausse avant
qu'elle ne serve à quoi que ce soit. Le seuil est désormais calé sur ce qui
existe, non sur ce qu'on aurait aimé trouver.

## D-033 — Les sénateurs : un identifiant valait mieux qu'un rapprochement

**Date** : 2026-09-12
**Statut** : acté

971 personnes du Sénat n'avaient aucune date de naissance : le dump Dosleg n'en
publie pas — `senat_raw.auteur` porte le matricule, le nom, le prénom, le groupe
et les dates de mandat, rien d'autre.

Le rapprochement par nom contre les sénateurs du RNE a été mesuré avant d'être
tenté : **342 correspondances uniques, 0 homonyme** sur les 348 sénateurs
actuels. Sûr, donc, mais partiel — 629 anciens sénateurs seraient restés sans
rien.

La recherche d'une autre source a mieux valu : le Sénat publie **son propre
répertoire** (`ODSEN_GENERAL.csv`, Licence Ouverte), indexé par **matricule**,
c'est-à-dire par l'identifiant déjà stocké. Aucun appariement de noms, donc
aucun homonyme possible par construction.

**Résultat : 971 personnes complétées, 0 restante.** La couverture des dates de
naissance passe à **99,99 %** (514 351 sur 514 410) ; les 59 manquantes sont
31 acteurs de l'Assemblée, 13 eurodéputés et 15 présidents créés depuis le
fichier éditorial.

Leçon de méthode : quand un rapprochement par nom paraît acceptable parce que
l'ensemble est petit, il reste préférable de chercher d'abord s'il existe une
source portant l'identifiant. Ici elle existait, elle était publique, et elle
couvrait le double du périmètre.

Le répertoire apporte aussi la **date de décès**, ce qui a révélé un défaut dans
la vue d'âge écrite le jour même : sans elle, un sénateur né en 1930 et décédé
en 2001 se serait vu attribuer 96 ans. L'âge se calcule désormais à la date du
décès quand il y en a une (migration 0034).

## D-034 — Le CAC 40 ne se résout pas automatiquement en SIREN

**Date** : 2026-09-12
**Statut** : acté

Trois stratégies automatiques ont été essayées pour relier une société cotée à
son numéro SIREN. **Les trois ont produit de mauvaises entités.**

| Stratégie | Résultat |
|---|---|
| Numéros de mémoire, vérifiés ensuite | 6 justes sur 21 — 552081317 désignait EDF et non Renault, 662042449 BNP Paribas et non L'Oréal |
| Égalité exacte de raison sociale | pour Renault, un SIREN sans aucun compte déposé, alors que Renault SA en dépose |
| SIREN ayant les plus gros comptes | « TOTALENERGIES LUBRIFIANTS » au lieu du groupe, « LVMH FRAGRANCE BRANDS » au lieu de LVMH |

Si la première liste avait été écrite sans vérification, le site aurait attribué
les comptes d'EDF à Renault — exactement le genre d'erreur que cet outil existe
pour permettre de contester.

`data/cac40.csv` est donc un **fichier éditorial**, vérifié ligne à ligne, où
chaque SIREN porte dans une colonne `verification` l'exercice et le chiffre
d'affaires qui ont servi à le retenir. Un lecteur peut refaire la vérification
au lieu de nous croire.

**Le piège de fond, et il est grave.** Une même marque dépose tantôt des comptes
sociaux, tantôt des comptes consolidés. TotalEnergies SE affiche 7 Md€ de
chiffre d'affaires en social contre environ 195 Md€ pour le groupe. Sur les
39 lignes retenues : **19 de portée GROUPE, 16 SOCIALE, 4 filiales françaises de
sociétés cotées à l'étranger** (Airbus et Stellantis aux Pays-Bas, ArcelorMittal
et Eurofins au Luxembourg — leurs groupes ne déposent rien au greffe français).
Additionner ou classer des sociétés de portées différentes produirait un
palmarès qui ne veut rien dire ; la colonne `portee` rend la distinction
obligatoire à la lecture.

**Ce qui reste hors d'atteinte** : les dividendes, qui ne figurent que dans les
rapports annuels en PDF société par société, et l'impôt sur les sociétés payé,
couvert par le secret fiscal — la ligne existe dans la liasse déposée, mais son
accès en masse passe par une API de l'INPI qui exige un compte.

## D-035 — Le CAC 40 n'est pas une donnée publique : une règle remplace la liste

**Date** : 2026-09-12
**Statut** : acté — annule et remplace `data/cac40.csv` (D-034)

Le CAC 40 est un **indice propriétaire d'Euronext Paris** : composition arrêtée
par un comité d'indice, révisée trimestriellement, publiée sous les conditions
d'Euronext. Vérifié : rien sur data.gouv.fr (sociétés cotées, AMF, LEI,
émetteurs — zéro résultat), pas d'open data à l'AMF, et la page d'Euronext est
une application JavaScript derrière un dispositif anti-robot.

Toute liste « CAC 40 » dans ce projet serait donc soit une copie d'un produit
commercial, soit une reconstitution de mémoire. La seconde avait été écrite
(D-034) ; **elle contenait des erreurs** : TotalEnergies y figurait avec
7,02 Md€ de chiffre d'affaires — son compte SOCIAL — alors que son compte
CONSOLIDÉ déposé la même année affiche **214,6 Md€**.

### La règle

> Sociétés ayant déposé un **compte consolidé** (`type_bilan = 'K'`) pour
> l'exercice 2024, avec un chiffre d'affaires supérieur à 20 Md€.

**43 sociétés** — ordre de grandeur comparable au CAC 40, obtenu par une requête
que le lecteur peut rejouer sur la base ouverte de l'INPI. Le critère est
stocké dans `core.entreprise.critere` ; l'exercice et le seuil sont des
constantes du connecteur, donc discutables et modifiables.

Ce que la règle fait apparaître et que l'indice masquait : **EDF (118,7 Md€), la
SNCF (43,4 Md€) et Les Mousquetaires (42,9 Md€)**, non cotés, pèsent plus que la
moitié des membres du CAC 40. Pour un outil qui documente le pouvoir
économique, c'est un gain.

Deux points laissés ouverts, documentés plutôt qu'arbitrés seuls : les holdings
apparaissent en doublon (Agache et Financière Agache au-dessus de LVMH, toutes
trois à 84,7 Md€), et le seuil de 20 Md€ est un choix.

### Le pont qui supprime la devinette

**GLEIF** (Global Legal Entity Identifier Foundation) publie en **CC0** le champ
`registeredAs` : l'identifiant de registre national déclaré par l'entité, soit
le SIREN pour la France. Vérifié sur trois sociétés — LVMH → 775670417,
Sanofi → 395 030 844, TotalEnergies SE → 542 051 180.

Et **ESMA FIRDS** (18 719 fichiers publics) recense tous les instruments admis
à la négociation dans l'Union, avec le LEI de l'émetteur et la place de
cotation. La chaîne `ESMA → GLEIF → INPI` est entièrement ouverte et ne repose
sur aucun appariement de noms.

Le connecteur interroge désormais le répertoire des entreprises **du SIREN vers
le nom**, jamais l'inverse : c'est le sens de la requête qui produisait de
mauvaises entités.

### Ce qui reste indisponible

Les **dividendes versés** (uniquement dans les rapports annuels, en PDF, société
par société), l'**impôt sur les sociétés payé** (secret fiscal) et la **masse
salariale**. Les deux derniers figurent dans la liasse fiscale déposée, mais son
accès en masse passe par `opendata-rncs.inpi.fr`, qui s'ouvre sur un écran de
connexion. C'est la seule porte, et elle demande un compte.

### La leçon

Quand une donnée n'existe pas en open data, la réponse n'est pas de la
reconstituer de mémoire : c'est de **changer de critère pour un critère
ouvert**. Ici le critère ouvert s'est révélé meilleur que l'original — plus
reproductible, plus large, et exempt de l'erreur social/consolidé que la liste
écrite à la main avait introduite.

## D-036 — Autorisation explicite d'utiliser les résultats municipaux de 2020

**Date** : 2026-09-12
**Statut** : acté sur décision du responsable du projet

Les fichiers de résultats des municipales de 2020 sont publiés par le ministère
de l'Intérieur **sans licence déclarée** (`notspecified` sur data.gouv.fr). La
règle du projet — qu'une absence de licence n'est pas une autorisation — les
écartait jusqu'ici (D-023), et c'est ce qui rendait la mandature 2020-2026
inanalysable alors qu'elle est la seule que nos séries couvrent.

L'exception a été demandée, la réserve exposée, et la décision prise par le
responsable du projet le 2026-09-12. Elle est appliquée ainsi :

- la source est enregistrée en **`RESTRICTED`**, pas en `OPEN` ;
- elle est affichable avec attribution au ministère de l'Intérieur ;
- elle **n'entre dans aucun export ouvert** du projet ;
- la décision est datée et nominative dans ce journal, pour qu'un tiers sache
  d'où vient l'autorisation.

### Ce que cela débloque

Toutes nos séries communales — délinquance enregistrée 2016-2025, comptes
communaux 2018-2025 — se déroulent **pendant la mandature 2020-2026**. Sans les
couleurs de 2020, elles ne pouvaient être rattachées qu'au scrutin de mars 2026,
qui leur est postérieur : elles décrivaient un héritage. Avec elles, la colonne
`periode` de `derived.commune_securite` bascule sur `PENDANT_MANDAT` et la
comparaison avant/après devient calculable.

### Deux pièges propres au millésime 2020

1. **`LNC` n'est pas une nuance.** Le code signifie « nuance non communiquée »
   et couvre les communes sous le seuil d'attribution. Il apparaît 10 889 fois,
   dans des communes dont la médiane est de 1 219 inscrits et le maximum de
   4 447 — contre 4 975 de médiane pour les communes réellement nuancées. Il est
   transcrit comme une ABSENCE de nuance.
2. **Le seuil d'attribution a changé entre 2020 et 2026.** Comparer la couleur
   d'une commune d'un scrutin à l'autre suppose de vérifier qu'elle franchissait
   le seuil aux deux dates. C'est ce que `circulaire_millesime` sert à
   distinguer, et la comparaison doit l'utiliser plutôt que de supposer une
   continuité.

Couverture comparable malgré tout : **3 212 communes nuancées en 2020** contre
3 269 en 2026.

### Ce que cela n'autorise toujours pas

Le §3 de `docs/securite-conception.md` tient sans changement : **la commune ne
commande pas la police nationale ni la gendarmerie**. Disposer enfin des deux
bornes d'un mandat rend la comparaison calculable ; cela ne la rend pas
attribuable. Un écart mesuré entre communes de nuances différentes reste à
expliquer, et le pré-enregistrement reste requis avant tout calcul comparatif.

## D-037 — La corrélation demandée se fait en comptes nationaux, pas société par société

**Date** : 2026-09-12
**Statut** : acté

L'objectif énoncé : corréler les **dividendes distribués** par les grands groupes
avec les **impôts acquittés**, le **budget de l'État** et la **dette**.

Les trois recherches précédentes (D-034, D-035) cherchaient ces chiffres société
par société, et butaient sur le secret fiscal et sur des rapports annuels en PDF.
La question était mal posée : une corrélation ne demande pas des chiffres
nominatifs, elle demande des **agrégats comparables mesurés selon les mêmes
conventions sur la même période**.

Les comptes nationaux les publient. Eurostat les diffuse depuis **1971** —
cinquante-quatre points, contre trente pour les finances publiques.

| Série (2024) | Montant |
|---|---|
| Dividendes versés par les sociétés non financières (D.42) | 302 Md€ |
| Dividendes versés par les sociétés financières | 58 Md€ |
| Dividendes reçus par les ménages | 68 Md€ |
| Impôts sur le revenu payés par les sociétés non financières (D.51) | 65 Md€ |
| Impôt sur les bénéfices encaissé par l'État (D.51B) | 84 Md€ |
| Rémunération des salariés versée par les sociétés non financières (D.1) | 992 Md€ |
| Excédent brut d'exploitation | 487 Md€ |
| Valeur ajoutée | 1 513 Md€ |

Elles entrent dans `core.macro_value`, aux côtés de la dette et du solde public :
la jointure se fait par l'année, sans retraitement.

**La masse salariale, jugée non indispensable, arrive par le même chemin** : le
D.1 est publié depuis 1971 au même titre que les dividendes. Ce qui était
inaccessible par société l'est immédiatement par secteur.

### Trois mises en garde qui doivent accompagner l'affichage

1. **C'est un agrégat**, couvrant toutes les sociétés résidentes et non les
   quarante plus grandes. Il ne se rapporte à aucune société identifiable.
2. **Les dividendes versés ne vont pas tous à des actionnaires français** :
   l'écart entre 302 Md€ versés et 68 Md€ reçus par les ménages résidents mesure
   ce qui va aux autres sociétés et au reste du monde, une part étant du flux
   intra-groupe compté plusieurs fois dans la chaîne de détention.
3. **Deux mesures de l'impôt coexistent et ne coïncident pas** : 65 Md€ payés
   (point de vue des sociétés non financières) contre 84 Md€ encaissés (point de
   vue de l'État, sociétés financières incluses). Les deux sont justes ; l'écart
   est un fait à montrer.

### Et la corrélation ne sera pas une causalité

Que la part de l'excédent d'exploitation distribuée en dividendes soit passée de
18,8 % en 1980 à 62,1 % en 2024 pendant que la dette publique quadruplait
n'établit aucun lien entre les deux. La frise met les séries côte à côte parce
que le lecteur a le droit de les voir ensemble ; elle ne doit pas suggérer
qu'elles s'expliquent. C'est la même règle que pour les mandats replacés sous une
présidence (D-026).

### Sources alternatives à l'INPI, examinées

| Piste | Verdict |
|---|---|
| Comptes nationaux (Eurostat / INSEE) | **retenue** — agrégats, 1971 → |
| ESMA FIRDS | ouverte, donne les instruments cotés et le LEI de l'émetteur ; pas de données financières |
| GLEIF | ouverte (CC0), donne le pont LEI ↔ SIREN ; pas de données financières |
| ESEF / iXBRL (états consolidés balisés, obligatoires depuis 2021) | structurés et lisibles par machine, mais déposés émetteur par émetteur ; pas d'agrégateur ouvert en France |
| BODACC | publie les avis de dépôt des comptes, pas les montants |
| `opendata-rncs.inpi.fr` | seule porte vers la liasse fiscale déposée (IS, salaires par société) ; exige un compte |
| Banque de France (FIBEN) | non ouverte |

## D-038 — « stime » n'était pas une durée : 539 310 heures de parole en deux ans

Le connecteur des comptes rendus de séance (`internal/an/interventions.go`) a
chargé 260 778 prises de parole, chacune portant l'attribut `stime` du jeu
Syceron. La colonne a été nommée `duree_s` et la vue `derived.temps_de_parole`
l'a sommée. Le total : **539 310 heures de parole** pour les sessions 2024-2026,
dont 5 940 pour une seule ministre — deux heures et demie par intervention.

La Ve République n'a pas assez d'heures. La lecture était fausse.

`stime` est la **position** de la prise de parole dans la séance, en secondes
depuis son ouverture. « La séance est ouverte » y vaut 26,68 ; le maximum
observé en base vaut 25 771, soit 7 h 09 — la durée d'une séance, pas celle
d'une phrase.

### Ce qui est corrigé

- la colonne s'appelle `instant_s`, et son commentaire SQL dit ce qu'elle n'est
  pas (migration `0052_temps_parole.sql`) ;
- la durée est **dérivée** : l'écart avec la prise de parole suivante de la même
  séance, par `lead(...) OVER (PARTITION BY séance ORDER BY ordre)` ;
- les écarts négatifs ou supérieurs à un quart d'heure sont écartés comme
  anomalies de séquence, et leur nombre est publié dans la colonne
  `duree_indeterminee` plutôt que dissous dans le total.

Après correction : **1 780 heures de débat**, 1 174 interventions de durée
indéterminée sur 228 920. La première oratrice de la législature totalise 28,5
heures, ce qui est vérifiable.

### La règle

Le nom d'une colonne est une affirmation. `duree_s` affirmait une durée que la
source ne publie pas. Un attribut numérique sans unité documentée doit être
**confronté à un ordre de grandeur connu** avant d'être nommé, et le nom doit
être celui de la mesure brute, pas celui de l'usage qu'on espère en faire.

Corollaire déjà appliqué ailleurs : la même vue révélait « Thibault Bazin » deux
fois. Ce n'était pas un doublon de personne mais deux sessions — la vue groupe
par session. La leçon tient quand même : une sortie qui surprend se diagnostique
avant d'être publiée, y compris quand elle finit par être juste.

## D-039 — Le Sénat n'ayant pas d'identifiant commun, 982 élus existaient en double

Le répertoire du Sénat publie un matricule (`14263U`) qui n'existe nulle part
ailleurs : ni à l'Assemblée, ni au RNE, ni à la HATVP. Le connecteur créait donc
une personne par matricule inconnu. Résultat mesuré : **982 fiches en double**.

Patrick Abate y figurait deux fois — une fois comme sénateur avec 1 693 votes,
une fois comme conseiller municipal avec son mandat local. Deux fiches pour un
homme, aucune complète. Au total, les doublons portaient 1 646 481 votes, 9 442
affiliations de groupe et 417 mandats, tous invisibles depuis l'autre moitié de
la fiche.

### Le rapprochement, et pourquoi il est admis ici

Ce projet refuse le rapprochement par nom (D-033). L'exception est bornée :

1. **nom ET date de naissance au jour près** — le seul discriminant fiable dont
   nous disposons, et celui que le Sénat publie ;
2. **appariement unique des deux côtés** — une paire est retenue seulement si
   ni le sénateur ni la personne candidate n'apparaissent dans une autre paire ;
3. **l'ambiguïté est mesurée, pas supposée** : elle vaut 0 sur 982, et le
   connecteur publie ce compte à chaque exécution. S'il cesse de valoir 0, les
   paires ambiguës sont écartées, pas devinées ;
4. **population fermée** : 1 948 sénateurs depuis 1958.

Sur les 1 948 sénateurs, 982 sont désormais reliés à une autre source ; les 966
restants n'ont jamais siégé à l'Assemblée ni figuré dans un RNE, ce qui est
attendu pour des mandats antérieurs aux répertoires électroniques.

### Ce que la fusion déplace, et ce qu'elle refuse de perdre

Les quatorze tables qui référencent `core.person` sont déplacées explicitement,
et **la liste est confrontée au catalogue** : si le schéma gagne une table qui
référence une personne, la fusion échoue plutôt que de l'oublier. Une table
oubliée serait effacée en cascade ou orpheline — dans les deux cas sans un mot.

Trois mandats sénatoriaux existaient des deux côtés avec des périodes
différentes : le dump Dosleg couvre le mandat entier, le RNE seulement sa
portion courante. La règle retenue est de **garder la période la plus large**.

## D-040 — Le décodeur XML strict jetait 70 % des actes du Journal officiel

Le connecteur JORF annonçait « 60 archives, 1 349 actes ». Huit archives en
contenaient déjà 1 065 : le compte était faux d'un facteur sept, et rien ne le
signalait.

Cause : `xml.Unmarshal` en mode strict. Les actes du JO enferment du HTML dans
`<CONTENU>` — balises non fermées, entités de traitement de texte. Le décodeur
rejette le fichier **en entier**, métadonnées parfaitement valides comprises, et
le code passait au suivant par un `continue` silencieux.

Correctif : `xml.Decoder` avec `Strict = false`, `AutoClose = HTMLAutoClose` et
`Entity = HTMLEntity`. Le corps, lui, reste extrait par expression régulière
(`<BLOC_TEXTUEL><CONTENU>`), comme les exposés des motifs.

Second défaut du même connecteur : l'extrait de contexte d'une mention était
tronqué à 120 **octets**, ce qui coupait « é » en son milieu et produisait un
`0xc3` orphelin. PostgreSQL refusait l'insertion et le chargement s'arrêtait net
sur `invalid byte sequence for encoding "UTF8"`. La troncature se fait
désormais sur des **caractères**, avec `strings.ToValidUTF8` en dernier recours.
C'est la troisième fois que ce bug apparaît dans ce dépôt (HATVP, JORF, extraits).

### La règle qui manquait

Le connecteur tient maintenant un **bilan** : fichiers vus, décodés, en échec,
sans identifiant. Un fichier écarté est compté et affiché. Un `continue` qui ne
compte rien est une panne en attente — c'est le même enseignement que le
commentaire qui n'empêchait pas le `TRUNCATE` (D-031) : seul un contrôle
empêche, et seul un compteur révèle.

## D-041 — Deux jours de chevauchement effaçaient l'histoire des députés

L'Assemblée publie, dans `AMO30_..._historique`, **1 258 mandats de député**
remontant au 12 juin 1988. Notre base en contenait **64**, tous postérieurs à
2012, et rien ne le signalait.

La cause tient en deux dates. La quatorzième législature s'achève le 20 juin
2017 ; la quinzième débute le 18. Les deux sont justes — l'Assemblée sortante
siège jusqu'à ce que la nouvelle soit constituée — mais la contrainte
d'exclusion temporelle sur `core.mandate`, elle, voit deux mandats simultanés et
refuse le second. Chaque député ne gardait donc que sa première élection.

Le refus était même *documenté* comme un choix : « un refus est une anomalie de
source, signalée par l'absence, jamais comblée ». La règle est bonne ; son
application était aveugle. Ce chevauchement n'est pas une anomalie de source,
c'est une convention de publication, et il fallait la lire plutôt que la subir.

### Le recollement

`recoller()` clôt un mandat parlementaire la veille du suivant. Portée stricte :

- **sièges parlementaires seulement** (`ASSEMBLEE`, `SENAT`) — un siège ne se
  détient pas deux fois ;
- **jamais les ministères** — un ministre peut cumuler deux portefeuilles, et
  ses chevauchements restent des faits ;
- **jamais au-delà du raisonnable** — si la veille du mandat suivant tombe avant
  le début du mandat courant, c'est une vraie anomalie de source, et elle
  redevient signalée par l'absence.

### Ce que la source ne donne toujours pas

Le fichier de la 17e législature ne porte l'historique que des **acteurs
présents** : 577 personnes, celles qui siègent aujourd'hui. Les députés partis
en 2017, 2022 ou 2024 viennent des publications propres aux 15e et 16e
législatures, déclarées dans `internal/an/extract.go`. Au-delà, le dépôt de
l'Assemblée répond 404 : les législatures 14 et antérieures ne sont pas
publiées, et l'histoire de leurs élus n'existe dans notre base que par les
mandats que les députés d'aujourd'hui y ont exercés.

## D-042 — L'histoire des députés était dans le fichier, pas dans celui qu'on lisait

Suite immédiate de D-041. Le recollement des chevauchements de législature
rendait exploitables les 1 258 mandats de député publiés par l'Assemblée. Il
restait à comprendre pourquoi ces 1 258 mandats ne concernaient que **577
personnes** — exactement la promotion en exercice.

L'Assemblée publie ses mandats à deux endroits :

| Fichier | Forme | Mandats `ASSEMBLEE` | Députés | Remonte à |
|---|---|---|---|---|
| AMO50 « divisés » | objets autonomes, un par mandat | 1 258 | 577 | 2012 |
| AMO30 « tous acteurs » | tableau `mandats.mandat` DANS chaque acteur | **3 952** | **2 120** | **2002** |

Le connecteur lisait AMO50, et le commentaire justifiait ce choix : les objets
y sont autonomes, « ce qui évite de dépendre d'une sérialisation qui varie d'un
fichier à l'autre ». L'argument est bon. Il conduisait au mauvais fichier.
AMO50 est la publication « divisée », celle qui omet les députés partis en
cours de législature — le connecteur en avertissait d'ailleurs, quinze lignes
plus haut, à propos du téléchargement.

La structure imbriquée qu'on avait voulu éviter était l'endroit où se trouvait
l'histoire. Les deux sources sont désormais lues et réunies sur l'identifiant du
mandat (`uid`) : aucun rapprochement approximatif, aucun doublon. Cinq acteurs
n'ayant qu'un seul mandat, le JSON le sérialise pour eux comme un objet et non
comme un tableau ; les ignorer aurait perdu cinq carrières sans le dire.

### Ce qui a été vérifié et ne marche pas

Les publications propres aux 15e et 16e législatures (`repository/15`,
`repository/16`) ont été téléchargées et dépliées : elles apportent **deux**
acteurs que la 17e ne contenait pas. Le fichier « tous acteurs depuis la XIe
législature » porte bien ce qu'il annonce, dans chaque dépôt. Les charger ne
nuit pas ; il ne faut pas en attendre davantage. Les législatures 14 et
antérieures répondent 404 : ce n'est pas publié.

## D-043 — L'ordre de la chaîne d'ingestion faisait mentir un garde-fou

Le connecteur du RNE n'insère un mandat de député que si l'Assemblée n'en a pas
déjà publié un : « le RNE complète, il n'écrase pas ». Le garde-fou est écrit,
commenté, et il ne gardait rien — parce que `an.Normalize` s'exécutait **en fin
de chaîne**, après le RNE. Le RNE arrivait donc le premier avec sa version
pauvre (pas de circonscription, pas de date de fin), et la contrainte
d'exclusion faisait rejeter celle de l'Assemblée, en silence.

Résultat visible avant correction : 577 mandats de député sans institution ni
circonscription, 64 avec. La normalisation est remontée avant le Sénat et les
communes.

Deux autres défauts de la même remise à zéro, découverts en la rejouant :

1. **Elle n'était pas transactionnelle.** Chaque suppression s'exécutait par
   `pool.Exec`, donc validée séparément. Un échec à mi-parcours laissait la base
   à moitié vide — 1,27 million de votes effacés, les organisations encore là,
   et aucun moyen de savoir où on en était. Elle tient maintenant en une
   transaction.
2. **Deux exécutions concurrentes se sont marché dessus**, l'une reconstruisant
   ce que l'autre effaçait. Le message d'erreur parlait d'une clé étrangère ; la
   cause était une course. Une seule instance à la fois, et le point est noté.

Enfin, la remise à zéro ne libérait pas tout ce qui pointe les textes qu'elle
reconstruit : `core.amendement`, `core.lecture`, `core.intervention.dossier_id`
et `core.scrutin.amendement_id` la faisaient échouer. Les amendements sont donc
rechargés APRÈS la normalisation, et la chaîne complète les enchaîne désormais
elle-même — avec les exposés, les interventions, les comptes de campagne et le
Journal officiel, qui n'étaient accessibles que par `-only`.

### Et sept clés étrangères n'étaient pas indexées

`DELETE FROM core.amendement` tournait encore au bout de sept minutes.
PostgreSQL n'indexe pas le côté référençant d'une clé étrangère : chaque ligne
supprimée déclenchait un parcours complet de `core.scrutin`, de
`core.topic_assignment` et de `selection.dossier_item`. Même chose pour
`core.organization`, dont six tables référençantes n'avaient aucun index — dont
`core.organization_identifier`, par où passe tout rapprochement d'organisation.
Migrations `0054` et `0055`. Coût : onze index. Gain : une renormalisation qui
se termine.

## D-044 — La présentation des textes du Sénat était dans le dump depuis le début

Question posée : peut-on rapatrier les présentations des scrutins venus du Sénat,
plutôt que de ne présenter que ceux de l'Assemblée ? Réponse : oui, et sans rien
télécharger.

`senat_raw.loi`, table du dump Dosleg scellé le premier jour, porte trois
colonnes que nous n'avions jamais ouvertes :

| Colonne | Contenu | Rempli |
|---|---|---|
| `objet` | présentation rédigée du texte, jusqu'à 14 649 caractères | 1 454 |
| `motclef` | mots-clefs du service de la séance | 5 069 |
| `en_clair_url` | page « La loi en clair » du service des études | 234 |

Les 12 429 dossiers du Sénat se rattachent **tous** à une ligne de `loi` par
`loicod` : jointure sur identifiant, aucun rapprochement de titre.

`derived.scrutin_presentation` présente désormais les scrutins des deux
assemblées, avec un champ `origine` qui dit d'où vient la présentation —
`EXPOSE_DES_MOTIFS` (Assemblée, récupéré page par page) ou `OBJET_DU_DOSSIER`
(Sénat, repris du dump). Une présentation dont on ignore la provenance ne vaut
rien.

`senat_raw.rap.rapres` porte en outre **2 557 résumés de rapports**, non encore
repris : ils appartiennent au rapport, pas au texte, et leur rattachement demande
d'être établi avant d'être affiché.

### La leçon, pour la troisième fois

Ce dump s'est révélé plus riche que prévu trois fois : d'abord 1,65 million de
votes nominatifs, puis 30 thèmes officiels, maintenant les présentations. Avant
de chercher une source nouvelle, finir de lire celle qu'on a déjà.

## D-045 — Le balisage se lit avec un analyseur, pas avec une expression régulière

Question posée, et la réponse honnête était gênante : quatre endroits du dépôt
extrayaient du texte de documents balisés avec `regexp.MustCompile("<[^>]+>")`.

Tout le reste utilise de vrais analyseurs — `encoding/json` pour l'Assemblée,
`encoding/csv` pour les référentiels, `encoding/xml` pour Syceron et le Journal
officiel, `archive/zip` + `encoding/xml` pour les XLSX — mais le **contenu
textuel** des documents HTML était découpé au motif.

Un motif ne sait pas ce qu'est une balise ; il sait reconnaître une forme. Sur
`<p title="a>b">Texte</p>`, `<[^>]+>` s'arrête au premier `>` et laisse `b">`
dans le texte publié. L'artefact existait déjà sous une autre forme :
`<span>M</span>esdames` a été publié « M esdames ».

Le paquet `internal/balisage` fait ce découpage avec `xml.Decoder` — un vrai
analyseur lexical, qui connaît les attributs, les guillemets, les entités et les
sections CDATA — réglé en mode tolérant pour le HTML enfermé dans du XML
(`Strict = false`, `AutoClose = HTMLAutoClose`, `Entity = HTMLEntity`). Les
balises de bloc deviennent un saut de ligne, les balises en ligne disparaissent
sans laisser d'espace. Onze cas de test, dont les deux artefacts ci-dessus.

Quatre connecteurs y sont passés : le Journal officiel (dont le corps était en
plus *localisé* par une expression régulière sur `<BLOC_TEXTUEL>`, ce qui est
indéfendable pour du XML et se fait maintenant par la structure de décodage),
les exposés des motifs, les déclarations de déport, et les décisions du Conseil
constitutionnel.

Reste une expression régulière assumée : celle qui repère les **noms de
personnes dans la prose juridique** du Journal officiel. Là, il n'y a aucune
structure à analyser — l'acte ne balise pas les personnes qu'il nomme. C'est
précisément pourquoi ces mentions restent `CANDIDAT` et ne sont jamais publiées
comme des faits.

## D-046 — Une carrière est un multirange, pas N lignes

`core.mandate` porte une ligne par période, et c'est juste : chaque période a sa
circonscription, sa qualité, son institution. Mais la question « depuis combien
de temps siège-t-il ? » n'a alors de réponse sur aucune ligne, et un sénateur
élu en 2001, battu en 2011, réélu en 2014 se lit en trois morceaux.

PostgreSQL dit cela en un seul type. `derived.mandat_serie` agrège les périodes
par (personne × type de mandat × institution) avec `range_agg`, ce qui donne un
**`datemultirange`** : `{[2001-10-01,2011-10-01), [2014-10-01,)}`. L'ensemble
**porte l'interruption**, là où trois lignes obligent le lecteur à la
reconstituer.

Deux précisions de type et de place :

- `datemultirange` et non `tstzmultirange` : un mandat commence un jour, pas à
  une heure. Le fuseau horaire n'a rien à faire dans une date d'élection.
- une **vue dérivée** et non une colonne : fondre les périodes dans un
  multirange perdrait les faits propres à chacune. La série se déduit des
  lignes, ce qui est le sens du schéma `derived`.

## D-047 — Le RNE écrasait ce qu'il devait compléter

La règle est écrite dans `internal/communes/rne.go`, en toutes lettres : « le RNE
complète, il n'écrase pas ». Elle est appliquée à l'INSERTION, par un `NOT
EXISTS` qui refuse d'ajouter un mandat de député quand l'Assemblée en a déjà
publié un. Elle ne l'était pas à la SUPPRESSION :

```sql
DELETE FROM core.mandate m
 WHERE m.mandate_type IN ('MAIRE', …)
    OR EXISTS (SELECT 1 FROM core.person_identifier i
               WHERE i.person_id = m.person_id AND i.scheme = 'RNE'
                 AND m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN'))
```

Ce `EXISTS` efface les mandats parlementaires de **toute personne portant un
identifiant RNE** — c'est-à-dire de la plupart des députés, qui ont presque tous
un mandat local. Mesure du dégât, prise juste après la correction de D-042 :
3 270 mandats de député remontant à 2002, ramenés à 1 851. Mille quatre cent
dix-neuf carrières parlementaires détruites et remplacées par la version pauvre
du RNE, qui ne connaît ni circonscription, ni date de fin.

La correction tient en une clause : `m.institution IS NULL`. Un mandat
parlementaire porte l'institution qui l'a publié ; ceux que le RNE crée n'en
portent aucune. La suppression est donc bornée à ce que ce connecteur possède —
la même discipline que D-029 et D-031, appliquée une fois de plus.

### Ce que ces trois décisions ont en commun

D-029 (`DELETE FROM core.dossier` sans clause), D-031 (`TRUNCATE core.ballot`),
et celle-ci : à chaque fois une suppression dont la portée dépassait ce que le
connecteur produisait, et à chaque fois la perte a été **silencieuse**. Aucune
n'a fait échouer quoi que ce soit ; toutes ont été découvertes en regardant un
chiffre qui avait baissé.

D'où le contrôle ajouté à `cmd/verify` : « les mandats de député remontent avant
2017 ». Il ne mesure pas la source, il mesure ce qui reste après la chaîne. Un
commentaire n'empêche rien ; une clause borne ; seul un contrôle révèle.

## D-048 — Le budget social est le plus lourd des deux et le moins documenté

Sept sources budgétaires repérées, six chargées. Le constat qui les traverse toutes
est asymétrique : l'État publie une exécution mensuelle, des balances de comptes et
des jeux par millésime ; **la Sécurité sociale ne publie pas ses comptes**. Une
recherche « comptes de la sécurité sociale » sur data.gouv.fr renvoie zéro jeu, et le
seul jeu rattaché à la loi de financement, les REPSS, est gelé depuis janvier 2022.
Le budget le plus lourd des deux — 803,5 Md€ de dépenses en 2025 contre 680,8 Md€
pour l'administration centrale — est le moins documenté en données ouvertes.

### Ce que le schéma refuse, et pourquoi il le refuse

Trois confusions traversent le débat budgétaire français. Elles produisent des
phrases fausses avec des chiffres justes, et elles ne se corrigent pas dans une note
de bas de page : `ref.budget_comptabilite`, `ref.budget_perimetre` et
`ref.budget_stade` sont des clés étrangères obligatoires sur toute valeur chargée.

1. **Trois comptabilités.** Budgétaire, générale, nationale. Le solde de la loi de
   finances et le déficit public ne sont pas le même objet et ne se comparent pas.
2. **Quatre périmètres côté social.** Régime général, régime général + FSV, tous
   régimes obligatoires de base, et protection sociale au sens de la DREES et
   d'Eurostat — ce dernier incluant l'assurance chômage et les retraites
   complémentaires, que la LFSS ne couvre pas. La colonne `hors_lfss` marque cette
   ligne de partage, qui vaut des dizaines de milliards.
3. **« Voté » n'est pas un état stable.** Le solde de la LFSS 2026 vaut −17,5 Md€ au
   dépôt et −19,4 Md€ à l'adoption. Sans colonne `stade`, ces deux chiffres justes se
   contredisent dans la base.

S'y ajoute une distinction juridique que l'affichage ne doit pas pouvoir escamoter :
`core.solde_vote.est_objectif` sépare l'ONDAM — un objectif révisable, dont le
dépassement n'est pas une irrégularité — des crédits limitatifs de l'État, dont le
dépassement est illégal.

Enfin `derived.budget_rapprochement` n'apparie un solde voté et un solde constaté que
lorsque la comptabilité ET le périmètre coïncident. Un rapprochement vide n'est pas un
défaut de la vue : c'est la source qui dit que ces deux chiffres ne se comparent pas.

### Le fait que la comptabilité nationale établit

C'est le seul cadre où l'État et la Sécurité sociale se mesurent sur la même règle, et
il renverse la phrase courante. En 2025, les administrations de sécurité sociale
dépensent **803,5 Md€** contre **680,8 Md€** pour l'administration centrale, et le
besoin de financement est celui de l'État : **−130,2 Md€** contre **−6,7 Md€**. En
2023 et 2024, les administrations de sécurité sociale étaient même **en excédent** au
sens de la comptabilité nationale, pendant que le débat public parlait du « trou de la
Sécu ».

Les deux affirmations peuvent coexister sans qu'aucune soit fausse — elles ne portent
ni sur le même périmètre ni sur la même comptabilité. C'est exactement pour cela que
les colonnes existent.

ESSPROS, de son côté, permet de **dater** la fiscalisation du financement social au
lieu de l'affirmer : la part des impôts affectés passe de 3,5 % en 1990 à 29,8 % en
2023, celle des cotisations de 79,9 % à 54,7 %.

### Ce qui n'est pas chargé, et ne peut pas l'être

`core.solde_vote` est **vide**. Les tableaux d'équilibre que le Parlement vote
n'existent que dans le texte de loi et dans des PDF. `ref.loi_financiere` sème donc la
LISTE des textes — numéro, date, référence au *Journal officiel* — et pas un seul
montant. C'est le patron de `ref.pdr_proclamation` : une table vide se voit, un
chiffre saisi à la main dans une migration ne se voit plus jamais.

### Trois pièges de source, mesurés et non supposés

- **Un titre qui promet plus que le contenu.** Le jeu
  `situations-mensuelles-budgetaires-series-longues` s'annonce « exercices 2013 à nos
  jours » ; son schéma ne porte que 31 arrêtés, de janvier 2024 à juillet 2026. La
  table stocke ce qu'elle reçoit, et un contrôle de fraîcheur dira si cela change.
- **Une table pivotée.** Ce même jeu publie 26 lignes de postes et *une colonne par
  date d'arrêté*. Le dépliage **refuse** une colonne dont le nom ne se parse pas en
  date, plutôt que de l'ignorer : une colonne ignorée, c'est un mois perdu sans trace.
  Le contrôle va plus loin qu'une expression régulière — « 31_02_2024 » a la bonne
  forme et n'existe pas au calendrier.
- **`offset + limit > 10 000` est refusé** par les portails Opendatasoft, avec un
  HTTP 400. Paginer les 15 654 lignes de la DREES échoue à mi-parcours, et une
  pagination mal contrôlée s'arrêterait en silence sur les 10 000 premières. Tout
  passe par `/exports/json`. Le piège vaut pour tous ces portails, `data.caf.fr`
  compris.

### Une sonde qui affirmait plus que la source ne promet

Le contrôle « les actes du Journal officiel portent un corps de texte » (D-040)
exigeait **zéro** acte sans corps. Il échouait sur cinq actes de 1958 à 1988 qui ne
portent aucun `<BLOC_TEXTUEL>` : la DILA n'en publie dans JORFSIMPLE que les
métadonnées, leur texte vivant dans le fonds consolidé LEGI. Ce sont 0,6 % des actes
nominatifs, et c'est un état de la source, pas un défaut de chargement.

Le seuil est passé à 5 %, ce qui garde ce qui compte : une régression du décodeur
ferait passer cette part à 100 %, pas à 1 %. Une sonde qui échoue sur un fait normal
finit par être désactivée, et c'est ainsi qu'on perd un contrôle utile.

## D-049 — Les gouvernements s'arrêtaient en 2014 ; la source de droit les continue

Notre base croyait Manuel Valls Premier ministre en septembre 2026. Le fichier
`data/gouvernements.csv` transcrit le jeu officiel des services du Premier
ministre, « Composition des gouvernements de la Vème République », et ce jeu
**s'arrête en 2014** : dernière mise à jour le 18 juin 2014, aucun successeur au
catalogue. Douze ans de gouvernements manquaient, ministres compris.

### Quatre pistes, une seule tient

| Piste | Verdict |
|---|---|
| data.gouv.fr — composition des gouvernements | **gelée en 2014**, aucun successeur au catalogue |
| Légifrance, site et API | **HTTP 403** derrière une protection anti-robot ; l'API exige un compte PISTE |
| Annuaire de service-public.fr | les ministères d'aujourd'hui, **aucun historique** |
| Open data de l'Assemblée (AMO30) | **partiel et contaminé** : seulement les ministres qui furent députés, et 398 lignes sur 1 103 sont des « parlementaires en mission » — qui ne sont pas membres du Gouvernement |
| **DILA — Journal officiel, base complète** | **retenue** : le décret lui-même |

La source de droit était la seule réponse, et elle était à portée : le décret
relatif à la composition du Gouvernement est publié au *Journal officiel*, la
DILA le diffuse en open data sous Licence Ouverte, et le connecteur qui lit ce
flux existait déjà.

**Sa prose est réglée parce que c'est du droit.** La même phrase depuis 1959 :
« Sont nommés ministres : M. X, ministre de Y ; … ». Le repère qui rend la
découpe possible est une convention typographique — **le patronyme est en
capitales, le prénom ne l'est pas** — et c'est elle qui sépare « Amélie de
MONTCHALIN » et « Anne Le HÉNANFF » sans heuristique de position.

Coût : la base complète du Journal officiel, 1,1 Go, **1 236 284 fichiers XML**.
Le connecteur la traverse en flux et n'en retient que les décrets de
gouvernement. Charger la totalité du Journal officiel pour répondre à cette
question serait disproportionné ; c'est une décision séparée.

### Deux pièges, tous deux silencieux

**La date sentinelle.** Quand la date d'un texte est inconnue, le Journal
officiel ne laisse pas le champ vide : il écrit **`2999-01-01`**. Quatre cent
dix-neuf actes de notre corpus la portent, dont dix-huit décrets de composition
des années 1990. Prise au mot, elle datait ces décrets du trentième siècle et les
rangeait après tout le reste — or la déduction des périodes ministérielles
repose entièrement sur l'ordre chronologique des décrets. Une sentinelle n'est
pas une date : elle devient NULL, et c'est la date de publication qui sert alors
de repère.

**Le chemin figé.** `<BLOC_TEXTUEL>` ne se trouve pas à la même profondeur selon
la publication : les livraisons quotidiennes le placent directement sous
`<TEXTE>`, la base complète le range sous `<STRUCT><ARTICLE>`. Le champ
`xml:"BLOC_TEXTUEL>CONTENU"` lisait donc les quatre décrets récents et rendait
vides les deux cents autres — **sans erreur**, puisqu'un acte sans corps est un
cas possible (D-040). Le corps est désormais collecté par un parcours à
profondeur libre, qui prend tout `<CONTENU>` dont le parent est
`<BLOC_TEXTUEL>`, où qu'il soit. Les `<CONTENU>` de `<NOTICE>`, `<VISAS>` et
`<ABRO>` restent écartés : ce sont la notice, les visas et les abrogations, pas
le dispositif.

C'est la deuxième fois dans ce connecteur qu'une hypothèse de structure fait
disparaître des données en silence. La règle qui s'en dégage : **quand une source
publie le même objet sous deux formes, ne pas choisir — parcourir.**

### Ce qui est chargé n'est pas attribué

`core.gouvernement_membre` contient ce que le décret DIT : une civilité, un
prénom, un patronyme, une fonction, un portefeuille. Pas une personne de notre
base. Le décret ne porte aucune date de naissance, et 41,9 % de nos élus ont un
homonyme exact en nom et prénom : le rapprochement reste donc `CANDIDAT`, et
`AMBIGU` dès qu'il y a plusieurs porteurs du nom. Même échelle que
`core.acte_jo_mention` (D-040), même règle — rien de `CANDIDAT` ne se publie
comme un fait.

Les PÉRIODES, elles, sont dans `derived.mandat_ministeriel` avec leur
`method_version` : un décret nomme, il ne dit pas jusqu'à quand. Une fonction
s'achève à la première cessation nominative, ou au premier décret qui recompose
un gouvernement entier sans reprendre la personne. Ce second critère — plus de
dix ministres de plein exercice nommés — est imparfait, et c'est précisément
pourquoi il est dans `derived` et non dans `core`.

### D-049 (suite) — Un contrôle en pourcentage dérive ; un contrôle daté, non

Le contrôle « les actes du Journal officiel portent un corps de texte » a été
écrit trois fois, et les deux premières étaient fausses :

1. **zéro exception** — plus que la source ne promet : cinq actes de 1958 à 1988
   ne portent que des métadonnées, la DILA ne publiant pas leur texte ;
2. **moins de 5 %** — un pourcentage, donc une valeur qui dépend de la
   composition du corpus. En ajoutant les 231 décrets de gouvernement, dont 42
   antérieurs à 1990, la part est passée à **5,07 %** et le contrôle a échoué
   sans que rien ne soit cassé ;
3. **tout acte postérieur à 1998 porte un corps** — le fait réel, qui est daté
   et non statistique. Le dernier acte nominatif sans texte de notre corpus est
   du 4 juin 1997 ; au-delà, un corps vide ne peut être qu'un défaut de lecture.

La troisième formulation est aussi la plus sensible : une régression du décodeur
ferait tomber ce contrôle sur les **934** actes concernés, là où le seuil en
pourcentage attendait d'être franchi. Un contrôle doit énoncer la propriété que
la source garantit, pas une tolérance autour de ce qu'on a observé.

## D-050 — Le chargement du Journal officiel : une traversée, quatre flux COPY

1,24 million d'actes de 1861 à 2025, 3,8 millions de blocs de texte, 1,1 Go
compressé. Trois façons de charger cela, et la troisième n'a pas le défaut
qu'on lui prête.

|  | Coût |
|---|---|
| Une seule table large, avec un discriminant | un seul flux, débit maximal — mais on écrit une forme qu'il faudra démêler |
| Plusieurs passes, une par table | chaque passe coûte une décompression complète |
| **Une traversée, N flux COPY parallèles** | **retenue** |

`COPY` monopolise une connexion : on ne peut pas alimenter plusieurs tables sur
la même. La troisième voie ouvre donc une connexion par table cible, et un
lecteur unique distribue les lignes par canaux.

**La question de l'ordre des COMMIT ne se pose pas**, parce qu'il n'y a rien à
ordonner : les tables d'atterrissage n'ont ni clé étrangère ni index. Un bloc qui
arrive avant l'acte qu'il désigne n'est pas une violation, c'est une ligne. La
clé étrangère est posée **après**, en quatre secondes, et vérifie tout d'un coup —
1 236 284 identifiants distincts, zéro lien orphelin.

### Pourquoi c'était lent, et ce qui l'a corrigé

La première version mettait 1 497 s. Décomposition mesurée :

| Étape | Temps | Part |
|---|---|---|
| Décompression gzip + parcours tar | 117 s | 8 % |
| Analyse XML | ~1 020 s | 68 % |
| Canaux + COPY | ~360 s | 24 % |

Et le micro-banc d'essai, sur 3 000 actes réels (17,6 Mo) :

| | Débit | Allocations |
|---|---|---|
| Tokenisation seule, **sans rien construire** | 20,4 Mo/s | 2,60 M |
| Décodage des métadonnées | 16,9 Mo/s | 2,74 M |
| Découpage en blocs | 15,0 Mo/s | 2,81 M |
| Les deux enchaînés | **8,0 Mo/s** | 5,56 M |

`encoding/xml` alloue **une fois tous les sept octets**. C'est le prix d'un
décodeur générique — espaces de noms, entités, XML arbitraire — sur un format qui
est en réalité plat, régulier et produit par machine. Et le prototype parsait
chaque acte **deux fois** : 16,9 et 15,0 séparément donnent exactement 8,0
enchaînés.

Le parcours de l'archive ne se parallélise pas — gzip est un flux séquentiel, et
il impose un plancher de 117 s. L'analyse, si. Le lecteur ne fait plus que lire
et distribuer, vingt-quatre ouvriers décodent : **1 497 s → 187 s**, à comptes
identiques. La même propriété qui dispensait d'ordonner les COMMIT — des tables
sans contraintes — dispense d'ordonner les sorties du pool.

### Un routage sur le chemin, et 1,2 million d'actes au mauvais endroit

Un texte est rangé **dans le répertoire de son sommaire** :

```
…/JORFCONT000000016676/JORFTEXT000000339141.xml
```

`strings.Contains(chemin, "JORFCONT")` est donc vrai pour les deux. Les 1 236 284
textes sont partis dans la table des sommaires, où ils se sont décodés **sans
erreur** — un JORFTEXT a lui aussi une balise `<ID>`. Seul un compteur resté à
zéro l'a dit. Le routage se fait sur le nom de base.

## D-051 — Le vecteur de recherche : la mesure du chargement conduisait à la faute

Deux façons d'indexer 2,9 Go de prose : une colonne `tsvector` générée et
stockée, ou un index fonctionnel sur l'expression. Les deux mesurées, dans cet
ordre, donnent des verdicts opposés.

**Au chargement**, l'index fonctionnel gagne : 187 s + 1 687 s contre 1 832 s +
211 s, et trois gigaoctets économisés. Le calcul de `to_tsvector` coûte le même
prix des deux côtés — environ 1 650 s — et il est sériel dans les deux cas :
`COPY` ne parallélise pas les colonnes générées, et PostgreSQL 17 ne parallélise
pas un index GIN.

J'ai donc retiré la colonne. C'était une faute, et la mesure suivante l'a dit :

| Requête | Vecteur stocké | Index fonctionnel |
|---|---|---|
| Recherche de phrase | **15 ms** | **50 057 ms** |
| Sur la même table : phrase | — | 1 807 ms |
| Sur la même table : conjonction | — | 12,6 ms |

**GIN ne stocke pas les positions des lexèmes.** Une conjonction se résout dans
l'index seul ; une recherche de phrase doit vérifier l'adjacence sur chaque
candidat. Avec une colonne stockée, cette vérification LIT un vecteur ; avec un
index fonctionnel, elle le RECALCULE, sur des blocs de plusieurs mégaoctets.

Conséquence concrète : la reconnaissance des 3 401 noms du thésaurus, à 1,8 s par
nom, demandait plus d'une heure et demie. Sur la colonne stockée, elle rentre
dans le budget d'un traitement de nuit.

### La leçon

Mesurer une seule phase, c'est optimiser contre soi. **Un corpus se charge une
fois et s'interroge indéfiniment** : vingt-huit minutes et trois gigaoctets de
plus au chargement sont le bon prix pour trois ordres de grandeur à la requête.
La colonne est restaurée.

## D-052 — Notre propre image PostgreSQL, et ce que le volume ne peut pas traverser

Quatre extensions sont nécessaires et aucune image publiée ne les réunit :
**postgis** (les contours administratifs sont dessinés par la base elle-même, en
SVG), **vector** (recherche par voisinage), **rum** (positions des lexèmes dans
l'index, ce que GIN ne fait pas — voir D-051) et **unaccent** (la configuration
de recherche `fr`). `postgis/postgis` n'a ni pgvector ni rum ;
`pgvector/pgvector` n'a pas PostGIS.

L'image est bâtie sur `debian:bookworm-slim` plus le dépôt PGDG, sur le modèle du
laboratoire TAOP : nativement multi-architecture, paquets à jour, et l'outillage
d'amorçage copié depuis l'image officielle plutôt que réécrit.

### Le volume ne traverse pas, et c'est le point

L'image précédente était Alpine, donc **musl** ; celle-ci est Debian, donc
**glibc**. Les collations diffèrent, donc l'ordre des index diffère. Un
répertoire de données recopié d'une image à l'autre **démarrerait et répondrait
faux** — c'est la pire des pannes, celle qui ne se signale pas. Le passage se
fait par `pg_dump -Fc` puis `pg_restore`, jamais par le volume.

Mesuré : dump de 1,26 Go en 321 s depuis une base de 16 Go ; restauration
vérifiée dans l'image neuve.

### Le dictionnaire français

`french_stem`, le désuffixeur Snowball, tronque selon des règles mécaniques. Sur
le vocabulaire du Journal officiel il se trompe **dans les deux sens** : il
sépare `ministre` (`ministr`) de `ministères` (`minister`), et il confond
`retraites` avec `retrait`. Le dictionnaire Hunspell français — Grammalecte v7.0,
MPL 2.0, 86 491 entrées, variante « toutes variantes » parce que le corpus va de
1861 à 2025 — ramène les formes fléchies à leur lemme.

Il répare le premier défaut, pas le second : quand une forme est ambiguë, il rend
*tous* ses lemmes. Il rend la confusion explicite plutôt que de la supprimer.

Son coût, mesuré : un dictionnaire Ispell est chargé en mémoire au premier usage
**de chaque session** — 175 à 278 ms, puis 0,17 ms. Avec un pool de connexions,
c'est négligeable ; sans pool, ce serait deux cents millisecondes sur chaque
première requête.

## D-053 — Le vecteur de recherche : quatre formes, et deux prévisions démenties

Le vecteur doit être stocké (D-051). Restait à savoir sous quelle forme. Quatre,
mesurées sur 200 000 blocs :

| Forme | Construction | Index | Dump | Restauration |
|---|---|---|---|---|
| Colonne générée | 89,8 s | 13,5 s | 14,1 s / 55,8 Mo | **105,1 s** |
| Colonne ordinaire, `INSERT…SELECT` | 90,3 s | 12,8 s | 28,8 s / 136,8 Mo | 35,8 s |
| Table `CREATE TABLE AS` | **30,4 s** | 11,6 s | 27,3 s / 140,8 Mo | **31,3 s** |
| **Vue matérialisée** | 31,5 s | 11,6 s | **0,3 s / 1,6 Ko** | 44,1 s |

**Première prévision démentie.** L'écart entre 90 s et 30 s à la construction
n'oppose pas la vue à la table : il oppose `INSERT … SELECT` à
`CREATE TABLE AS`. Une relation créée dans la transaction courante évite une
partie du travail d'écriture. Et le contournement évident — `TRUNCATE` puis
`INSERT` dans la même transaction — ne retrouve rien : 86,6 s mesurés, parce
qu'avec `wal_level = replica` l'optimisation ne s'applique pas.

**Seconde prévision démentie.** J'attendais que la vue matérialisée allège le
dump du corpus entier de près de trois gigaoctets. Elle ne l'allège pas du tout :
1 352 937 088 octets contre 1 351 402 765 avec la colonne générée. C'est logique
après coup — **une colonne générée n'est pas dumpée non plus.** Seule la colonne
*ordinaire* aurait emporté les vecteurs, au prix de 2,7 Go.

### Ce qui a décidé

Le temps de **restauration**, et la dépendance déclarée. Un `REFRESH` en bloc va
plus vite qu'un calcul ligne à ligne pendant `COPY` — 44,1 s contre 105,1 s — et
surtout PostgreSQL **connaît** la dépendance de la vue vers sa source : elle ne
peut pas dériver ligne à ligne, elle est fraîche ou périmée. Une colonne
ordinaire aurait demandé une sonde pour vérifier qu'elle ne ment pas ; la vue
rend cette sonde inutile.

L'index `UNIQUE` sur la vue conditionne `REFRESH … CONCURRENTLY` : sans lui, tout
rafraîchissement prend un verrou exclusif et coupe la recherche.

### Deux réglages qui traînaient

`maintenance_work_mem` restait au défaut de 64 Mo pendant la construction des
index GIN. L'écart avec 1 Go est de 2 % sur l'échantillon — l'index y tient
presque en mémoire — mais laisser un défaut sur une construction de quatre
gigaoctets n'est pas un choix, c'est un oubli. Et un index GIN maintenu pendant
l'insertion coûte 8 % de plus que le même construit à la fin : avec la vue
matérialisée la question disparaît, les index ne portant plus sur les tables que
`COPY` remplit.

## D-054 — La bascule vers l'image Debian : trois défauts que seul un vrai démarrage montrait

La base a été basculée le 14 septembre 2026 sur `faits-politiques/postgres:17`.
Protocole, dans cet ordre :

1. décompte exact des 200 tables et vues avant toute opération ;
2. `pg_dump -Fc` par le `pg_dump` 17 du conteneur d'origine — 325 s, 1,29 Go
   pour une base de 20 Go ;
3. relecture intégrale de l'archive (`pg_restore -f /dev/null`, 34 s) avant de
   toucher au conteneur ;
4. démarrage de l'image neuve sur un **volume neuf** ;
5. restauration, puis comparaison des décomptes.

**L'ancien volume n'est pas supprimé.** Il a été initialisé par une image musl ;
ses index de texte sont rangés selon d'autres collations que celles de glibc.
Démarrer l'image Debian dessus aurait fonctionné en apparence et rendu des
résultats faux. Il reste sur le disque comme retour arrière, et sa suppression
est une décision à prendre explicitement plus tard.

### Trois défauts, aucun visible avant le démarrage réel

**Le cluster fantôme.** Le paquet Debian `postgresql-17` crée à l'installation un
« cluster » `/etc/postgresql/17/main`, ici sur le port 5433. Les binaires de la
version étaient placés en **queue** de `PATH` : `pg_isready` se résolvait donc
vers le `pg_wrapper` de Debian, qui adopte le port de ce cluster. Le contrôle de
santé interrogeait 5433 pendant que le serveur écoutait sur 5432, et le conteneur
restait « unhealthy » en fonctionnant parfaitement. Le même symptôme était apparu
dans le conteneur d'essai, où je l'avais attribué à tort à une variable
d'environnement de mon shell — une erreur d'interprétation qui a laissé passer le
défaut. Corrigé à la racine : binaires en tête de `PATH`, `create_main_cluster =
false` posé avant l'installation, et deux garde-fous qui font échouer la
construction si l'un ou l'autre revient.

**La période de grâce.** Le premier démarrage d'un volume neuf lance `initdb` puis
crée dix extensions, PostGIS comprise : plus d'une minute. Le contrôle de santé
abandonnait au bout de soixante secondes et `docker compose up --wait` déclarait
le conteneur défaillant pendant qu'il s'initialisait normalement.
`start_period: 180s`.

**`time` dans le Makefile.** C'est un mot-clé de bash ; `make` lance ses recettes
avec `/bin/sh`, dash sous Debian. `make db-restore` échouait après avoir supprimé
et recréé la base — sans dommage ici, la base du volume neuf étant vide, mais une
recette qui détruit avant d'échouer est exactement celle qu'il faut corriger avant
qu'elle ne serve sur une base pleine. La durée est désormais mesurée à la main, et
le code de retour de `pg_restore` est propagé plutôt qu'avalé.

## D-055 — La dette : un modèle long, et trois accès choisis plutôt que forcés

Le bloc dette (`docs/dette-donnees.md`, migration 0073, paquet `internal/dette`)
charge sept sources dans **un seul modèle long** : `ref.dette_serie` porte les
dimensions (définition, détenteur, échéance, instrument), `core.dette_observation`
les valeurs à l'unité. Une table par source aurait été plus simple ; elle aurait
interdit les contrôles croisés qui ont justement trouvé deux défauts réels des
sources (§ 9 de la note) : en octobre 2017, une ventilation de la dette négociable
reprend le total du mois précédent (+23,7 Md€) ; avant 1998, l'INSEE et Eurostat
divergent de plusieurs milliards. Les deux sont laissés en base tels que publiés,
et exclus **nommément** des contrôles — une tolérance élargie les aurait cachés.

**Trois accès, aucun contournement.**

- **L'Agence France Trésor** ferme tout son site derrière une protection
  anti-robot. Ses chiffres mensuels sont pris chez l'INSEE, qui les republie sous
  licence ouverte en citant l'AFT ; la détention chez la Banque de France, qui la
  produit ; la performance des émissions dans le rapport au Parlement
  (programme 117). Ce qui n'existe qu'à l'AFT — l'échéancier titre par titre — n'est
  pas chargé, et la note le dit.
- **Le DataMapper du FMI** renvoie 403 à un client qui s'identifie honnêtement ;
  il répond à un User-Agent de navigateur. Se déguiser aurait fonctionné et aurait
  été un contournement. L'API SDMX du FMI accepte le client tel qu'il est, et
  apporte ce que le DataMapper n'a pas : la dernière année **observée** de chaque
  pays. Les projections du FMI, que le DataMapper mêle aux données sans les
  distinguer, restent ainsi hors de la base.
- **La Banque de France** exige une clé d'API. Elle vit dans `.env`, non versionné,
  et passe dans un **en-tête** (`archive.FetchEntetes`), jamais dans l'URL :
  `raw.retrieval` conserve les URL, et une clé qui y figurerait serait publiée avec
  la provenance. Vérifié après chargement : aucune URL archivée ne la contient.

**La valeur de marché n'est pas le nominal.** La détention publiée par la Banque de
France est en valeur de marché (2 602 Md€ au T1 2026), la dette négociable de l'AFT
en nominal (2 824 Md€ en mars 2026) ; en 2019, l'écart était de sens inverse. Toute
comparaison entre les deux se fait en **parts**, jamais en montants.

## D-056 — « À quoi sert la dette » : une identité comptable, et des aides juxtaposées, jamais additionnées

La question « à quoi sert la dette » est traitée par le **compte de capital** des
administrations publiques (`derived.dette_compte_capital`) : le besoin de financement
se décompose exactement en épargne brute négative, investissement et transferts en
capital nets. C'est une identité, vérifiée au million près, pas une affectation :
l'argent public est fongible, et la page le dit avant de montrer le chiffre. Les deux
conventions — brute et nette de l'usure des équipements — donnent des lectures
opposées certaines années ; les deux sont publiées, chacune nommée.

Les aides sont **juxtaposées** dans `derived.dette_aides_dividendes`, jamais sommées,
parce que les données elles-mêmes l'interdisent : le CICE apparaît à la fois dans les
exonérations de l'URSSAF (2013-2018), dans les subventions de la comptabilité
nationale et dans les dépenses fiscales. Un total « aides aux entreprises » construit
par addition l'aurait compté trois fois.

La nature du bénéficiaire des dépenses fiscales (entreprises, ménages) n'existe en
données ouvertes que pour le PLF 2023 ; elle est appliquée aux autres millésimes par
numéro de mesure, et la part non classée est contrôlée (moins de 5 %). Les autres
annexes Voies et moyens sont sur budget.gouv.fr, derrière une protection Incapsula :
non contournée, non chargées.

L'argument politique « les aides aux grandes entreprises sont financées par les
ménages et endettent le pays » est examiné maillon par maillon, sans verdict : aucune
source ne ventile les aides par taille d'entreprise, l'incidence d'un impôt relève
d'un modèle et non d'une donnée, et « sans ces aides le déficit aurait été moindre »
est un contrefactuel.

## D-057 — La taille des bénéficiaires : trois sources, trois notions de « grande entreprise »

Trois ajouts (migration 0077, paquet `internal/aides`) pour dire qui reçoit les aides :

- **Quatre millésimes de l'annexe Voies et moyens** (PLF 2020 à 2023), trouvés en
  pièces jointes des jeux de la Direction du budget sur data.economie.gouv.fr, alors
  que budget.gouv.fr les tient derrière un défi anti-robot. Les niches fiscales
  exécutées remontent à 2018, avec la nature du bénéficiaire. La vue retenue prend
  **un seul millésime par année** : la première version choisissait mesure par
  mesure et comptait deux fois les mesures renumérotées (2019 : 103,0 Md€ au lieu de
  99,9). L'erreur a été vue en rapprochant le total de l'exécution publiée.
- **Les exonérations de cotisations et la masse salariale par tranche d'effectif**
  (URSSAF). La tranche est celle de la **société**, pas du groupe : la part des grands
  groupes est minorée, dans une proportion que la source ne permet pas de mesurer.
  C'est écrit sur la page, pas en note.
- **La catégorie d'entreprise de l'INSEE** (PME, ETI, GE) pour chaque personne morale
  de SIRENE, calculée au niveau du **groupe** : la bonne notion pour « grandes
  entreprises », chargée en attendant une source d'aides nominatives à croiser. Les
  entrepreneurs individuels (catégorie juridique 1000) ne sont pas chargés : leur
  SIREN désigne une personne physique, et aucun usage du projet ne le demande.

Les trois notions ne se convertissent pas l'une dans l'autre. Aucune table de
passage « tranche d'effectif → PME/ETI/GE » n'est construite : elle donnerait une
précision que les données n'ont pas.

## D-058 — Les aides nominatives : trois registres, un croisement, et le nom des seules personnes morales

Migration 0078, `internal/aides` (`-only=aides-nominatives`). Trois sources publient
les aides bénéficiaire par bénéficiaire : le registre européen de transparence des
aides d'État (TAM), les aides de l'ADEME, le registre public des aides de minimis.
Toutes trois sont rapprochées de SIRENE par le SIREN publié, jamais par le nom
(D-025).

**Le registre européen, avec accord explicite.** La Commission ne publie ni API ni
fichier complet du TAM ; la recherche publique passe par un formulaire dont la page
de résultats propose un export CSV lié à la session. Le responsable du projet a
autorisé explicitement, le 14 septembre 2026, que le connecteur reproduise ce
parcours. Il le fait en visiteur identifié (User-Agent du projet), une pause de deux
secondes entre les requêtes, par périodes de date d'octroi d'au plus 800 aides environ :
au-delà d'un millier de lignes, le registre ne sert plus l'export et propose de l'envoyer
par courriel contre un nom et une adresse, formulaire que le connecteur ne remplit pas. Chaque export
est scellé ; son adresse archivée porte la recherche en fragment
(`#pays=FRA&octroi=…`), sans quoi « export?format=CSV » ne dirait pas ce qui a été
exporté. Le nombre de lignes est contrôlé contre la pagination de la page de
résultats ; un écart fait découper la période plutôt que charger un trimestre
incomplet. Le site ne présente aucune protection anti-robot ; s'il en présentait une,
le connecteur échouerait.

**Le nom et l'identifiant ne sont gardés que pour les personnes morales du
répertoire.** Les registres publient aussi des entrepreneurs individuels et des
exploitants agricoles. Leur SIREN est une donnée personnelle, et la question posée —
la taille des entreprises aidées — ne demande pas de les nommer : la ligne est
conservée (elle compte dans les totaux), son bénéficiaire ne l'est pas. Une
contrainte de la table l'impose.

**Le type « PME » du TAM est remplacé, pas corrigé.** Il est déclaré par l'autorité
qui octroie ; la vue `derived.aide_tam_type_declare` le confronte à la catégorie de
l'INSEE sans jamais le réécrire.

**Les sources se recouvrent** (une aide de l'ADEME notifiée figure au TAM) : aucune
vue ne les additionne.

## D-059 — « La France est-elle un paradis fiscal ? » : trois grilles officielles, une mesure, et des filiales nommées sans « impôt payé »

Migration 0082, `internal/fiscalite` (`-only=fiscalite`), docs/paradis-fiscal-donnees.md.

**Aucune définition unique n'est imposée.** Le projet ne tranche pas « paradis
fiscal : oui / non ». Il confronte la France à chacune des grilles officielles
existantes — facteurs de l'OCDE (1998), critères du Conseil de l'UE (2017), liste
française des ETNC — et à la mesure économique du transfert de bénéfices. Chaque
grille est présentée avec son angle mort : la liste européenne n'examine par
construction aucun État membre.

**Tax Justice Network : cité, pas chargé.** Le responsable du projet a autorisé
explicitement, le 14 septembre 2026, l'usage des indices du TJN (licence non
commerciale, compatible avec le projet). Leurs données détaillées ne sont
téléchargeables qu'après création d'un compte, ce que le projet ne fait pas ; les
pages publiques sont rendues par script. Les indices sont donc cités avec lien,
aucun chiffre du TJN n'est publié sans source primaire consultée.

**Tørsløv-Wier-Zucman : RESTRICTED.** Les classeurs de réplication ne déclarent
aucune licence. Ils sont chargés pour être cités avec attribution (quelques
agrégats par pays), pas redistribués.

**Les filiales : deux origines jamais confondues.** Le repérage systématique vient
de GLEIF (licence CC0), déclaratif et partiel. La sélection nommée (GAFAM, Disney,
et une trentaine d'autres groupes) est écrite dans le code avec, pour chaque SIREN,
le fondement du rattachement ; un SIREN absent de SIRENE fait échouer le chargement.

**Jamais « l'impôt payé » d'une filiale.** Le seul jeu ouvert de comptes sociaux
(ratios INPI/BCE) ne publie pas l'impôt sur les sociétés. L'écart entre résultat
courant avant impôt et résultat net est publié sous ce nom, avec ses composantes
possibles. Les régularisations connues (Google 2019, McDonald's 2022) sont citées
depuis leurs sources officielles, hors base.

**CbCR : on ne somme pas les sièges.** Les comparaisons de juridictions se font à
siège constant (groupes américains, groupes français) ; les parts sont calculées
sur le « reste du monde » publié par le siège.

## D-060 — Le bulletin de paie d'exemple : un barème en table, un calcul en vues, un budget par destinataire

Migration 0085, `internal/paie` (`-only=paie`), `cmd/figure-bulletin`,
docs/cotisations-et-droits.md § 3 bis.

**Un bulletin factice, mais aucun chiffre à la main.** Les taux, le plafond et les
paramètres de la réduction générale sont des lignes de référence
(`ref.taux_cotisation`, `ref.parametre_social`), chacune avec son fondement. Le
bulletin est recalculé par des vues ; la figure du document est régénérée depuis ces
vues et `cmd/verify` fige ses totaux, calculés indépendamment avant d'écrire les vues.
Les seules hypothèses propres au cas — taux accidents du travail de l'établissement,
taux d'impôt du foyer, dispense de mutuelle — sont écrites dans `ref.bulletin_cas`.

**Archiver ce qui peut l'être, citer le reste.** Les fiches de service-public.fr et la
page de l'INSEE sur le périmètre des administrations de sécurité sociale sont scellées,
et le connecteur échoue si elles ne contiennent plus les valeurs qu'on leur fait dire.
Le barème de l'URSSAF et Légifrance refusent l'accès automatisé : ils ne sont pas
contournés, leurs références sont écrites dans le fondement de chaque ligne.

**« L'État » ou « la Sécu » ne suffit pas.** Chaque destinataire porte le budget
dont il relève au sens du texte qui l'arrête : État (loi de finances), Sécurité
sociale dans le champ de la LFSS, régimes paritaires hors LFSS (comptés en
administrations de sécurité sociale par l'INSEE), opérateur de l'État, fonds de
l'État, collectivités, organismes privés, affectation choisie par l'employeur. Le
sous-secteur de comptabilité nationale n'est renseigné que lorsqu'une source l'établit
(France compétences : liste des ODAC de 2023 ; régimes paritaires et CADES : INSEE) ;
il reste vide pour l'AGS, le Fnal et le fonds paritaire du dialogue social.

**Deux répartitions restent des conventions, et le disent.** La réduction générale
est imputée sur la retraite complémentaire selon la règle officielle ; entre les
caisses de l'URSSAF, faute de clé publiée, elle suit les taux. La CSG est rangée
comme un seul destinataire, sa répartition légale entre caisses n'étant pas chargée.

## D-061 — Le dossier devient « évasion fiscale des multinationales » ; ce que l'État leur verse est mis en regard, sans être compensé

Migration 0087, `internal/fiscalite` (`-only=fiscalite-marches`, `-only=fiscalite-faits`),
docs/evasion-fiscale-multinationales.md (renommée depuis paradis-fiscal-donnees.md).

**Changement de question, pas de conclusion.** La réponse établie par D-059 tient :
au sens de toutes les grilles officielles, la France n'est pas un paradis fiscal. À la
demande du responsable du projet, le dossier porte désormais sur la question que ce
constat ouvre : pourquoi l'impôt des multinationales échappe-t-il en partie à la France,
et que leur verse l'État dans le même temps ? La grille « paradis fiscal » y devient une
section. La migration 0082 et la décision D-059 gardent leur nom d'origine : le journal
est daté.

**Les mots.** Fraude (illégale, jugée ou transigée), évasion (contournement de l'esprit
de la loi, que l'administration peut requalifier) et optimisation (usage de règles
légales) ne sont pas synonymes. Le titre emploie « évasion fiscale » au sens large que
lui donnent les estimations du transfert de bénéfices, qui ne distinguent pas ; chaque
fait porte sa qualification juridique propre (convention judiciaire, constat d'enquête,
contrat).

**Mettre en regard n'est pas compenser.** Les marchés publics, les aides et les faits
documentés sont présentés à côté des comptes et des impôts, jamais soustraits les uns
des autres : un plafond d'accord-cadre n'est pas une dépense, un chiffre d'affaires
n'est pas un bénéfice, et aucune donnée ne dit quel impôt une multinationale « devrait »
payer en France.

**Commande publique : rattacher prudemment.** Les données essentielles de la commande
publique sont rattachées à un groupe par le SIREN du titulaire, par sa dénomination, ou
par l'objet du marché (licences Microsoft achetées via un revendeur). Les trois modes ne
sont jamais confondus ; les montants par objet englobent souvent d'autres produits. Les
contrats antérieurs à l'obligation de publication (le contrat Microsoft de la Défense,
2009-2021) ne viennent que des sources parlementaires.

**Faits documentés : la qualité est une colonne.** Un fait OFFICIEL (Sénat, Assemblée,
Conseil d'État, PNF, ministère) a sa page scellée et vérifiée ; un fait PRESSE ou
ENTREPRISE est cité avec son lien et son statut, jamais présenté comme établi.

**Ce qui n'est pas dans le dossier, et pourquoi.** Le crédit d'impôt recherche et les
autres crédits d'impôt par entreprise sont couverts par le secret fiscal : seuls leurs
totaux sont publics. L'affirmation « telle filiale ne paie pas d'impôt » n'est reprise
que lorsqu'une source officielle l'établit (McKinsey) ; ailleurs, le dossier montre
l'écart entre résultat avant impôt et résultat net, et l'endroit où le chiffre d'affaires
français est facturé.

## D-062 — AWS, Google et Capgemini dans le dossier ; une correction sur le marché Microsoft de l'Éducation nationale

Extension de D-061 (`internal/fiscalite`, marchés et faits documentés).

**Correction.** Le dossier présentait l'accord-cadre de l'Éducation nationale comme
« conclu avec Microsoft », sur la foi de la réponse du ministère à l'Assemblée. Le
rapport n° 830 de la commission d'enquête du Sénat sur la commande publique (juillet
2025) en donne les titulaires : le revendeur Crayon France et Open SAS, pour 74,72 M€
estimés et 152 M€ au plus. Les deux sources sont citées ; le fait qui nomme les
titulaires l'emporte pour dire qui a signé.

**Capgemini est suivi, mais comme groupe français.** Sa société de tête est à Paris et
il est imposé en France : il n'entre ni dans la table des filiales de groupes étrangers
ni dans les agrégats sur l'évasion. Il figure dans les marchés publics et les faits
parce qu'il est, de loin, le plus présent des groupes suivis dans ces données et un acteur des
offres « cloud de confiance » bâties sur des technologies américaines (Bleu). S3NS
(Thales et Google Cloud) est suivi de la même façon que Bleu.

**AWS n'apparaît presque pas dans les données essentielles**, et ce n'est pas une
absence de dépense : ses services passent par le marché cloud de l'UGAP, dont le
titulaire est un distributeur, ou par des contrats hors publication. Les montants
viennent du Sénat. Le rattachement par nom et par objet exige désormais la
dénomination d'une société du groupe : « AWS » désigne aussi Avenue Web Systèmes,
éditeur de plateformes de marchés publics.

## D-063 — Palantir, Oracle, IBM et Accenture ; les montants « commandés »

Migration 0090, `internal/fiscalite/dossier.go` (`-only=fiscalite-faits`).

**Une nature de montant de plus : COMMANDE.** Le rapport n° 578 du Sénat sur les
cabinets de conseil chiffre des commandes émises sur des accords-cadres (5,34 M€ pour
Accenture pendant la crise sanitaire, 16,21 M€ au groupement McKinsey-Accenture). Ce ne
sont ni des plafonds de marché ni des paiements constatés ; les ranger sous l'une ou
l'autre étiquette aurait trompé le lecteur.

**Palantir : deux faits de presse, signalés comme tels.** Le montant du premier contrat
de la DGSI (environ 10 M€, 2016) et l'annonce de son remplacement par ChapsVision (juin
2026) ne viennent que de la presse ; la seule source officielle chargée reste la question
écrite de 2025. Les marchés de renseignement échappent à la publication : l'absence de
Palantir dans les données essentielles n'est pas une absence de dépense.

**Oracle et IBM : les données essentielles portent l'essentiel.** Les plus gros marchés
(300 M€ de support Oracle avec le Service des achats de l'État, licences et mainframes
IBM de la DGFiP et de la CNAV) viennent directement de la commande publique ; les faits
ajoutés situent ces technologies dans le système d'information de l'État (réponse
ministérielle de 2021, Cour des comptes 2019) et, pour Oracle, des dépenses annuelles
rapportées par la presse depuis des auditions parlementaires.

## D-064 — L'impôt payé en France : un statut fondé par groupe, un impôt théorique, les déclarations pays par pays publiques

Migration 0092, `internal/fiscalite/transparence.go` (`-only=fiscalite-transparence`),
docs/evasion-fiscale-multinationales.md § 4 bis.

**Contrats publics et évasion ne se confondent pas.** Chaque groupe suivi porte un statut
(`ref.groupe_statut_fiscal`) : fraude transigée, impôt nul constaté, facturation depuis
une société étrangère établie, groupe français, ou aucun constat public. Les trois
premiers exigent un fait OFFICIEL chargé ; le connecteur refuse tout autre fondement.
Oracle, IBM, Accenture et Palantir, dont les marchés publics figurent au dossier, sont
« aucun constat public » : la présentation doit le dire à côté de leurs contrats.

**L'impôt théorique est un point de comparaison, pas une estimation de l'impôt dû.** Il
applique au résultat courant avant impôt de chaque filiale le taux normal, la
contribution sociale et la contribution exceptionnelle de 2025, à partir de paramètres
sourcés (`ref.parametre_is`, BOFiP). Le constat qu'il permet — l'écart publié couvre au
moins l'impôt théorique — porte sur le bénéfice déclaré en France, jamais sur celui qui
ne l'est pas.

**La bourse ne donne pas la France ; la directive européenne, si.** Les rapports 10-K de
la SEC (API XBRL, domaine public) sont chargés pour le taux effectif des groupes et leur
part de bénéfice étranger ; ils ne ventilent pas par pays. Les déclarations pays par pays
publiques (directive (UE) 2021/2101) sont la seule source ouverte d'impôt dû en France
par groupe ; elles paraissent depuis mi-2026. Chaque rapport est transcrit du document
scellé et contrôlé contre le bénéfice du groupe déposé à la SEC. Les rapports roumains
anticipés ne portent que sur la Roumanie. Les comptes irlandais, payants, ne sont pas
chargés.

## D-065 — Souveraineté numérique : aucune ré-identification des sanctions de la CNIL, tous les marchés informatiques, les déclarations séparées des constats

Migration 0097, paquet `internal/numerique` (`-only=numerique`, ou `numerique-anssi`,
`-cnil`, `-faits`, `-marches`), docs/souverainete-numerique.md.

**Le dossier ne part pas d'une thèse.** Une première version de la note examinait une
phrase de départ (« l'État enrichit des sociétés américaines… ») affirmation par
affirmation ; elle a été retirée : la phrase servait à fouiller les données, pas à les
présenter. Le dossier suit désormais un ordre fixe — contexte, raisons avancées, normes et
souhaits de l'État, contrôles, situation chiffrée — et chaque partie dit ce que ses sources
ne permettent pas d'établir. Le contexte s'appuie sur les données déjà chargées :
intitulés des décrets d'attributions (corpus JORF) et prises de parole de l'Assemblée
nationale (`derived.dossier_mentions_an`, D-066), où une mention n'est jamais lue comme une
position.

**Ce projet ne rétablit pas le nom d'un organisme que la CNIL a retiré.** La publicité
nominative d'une sanction est une peine complémentaire limitée dans le temps ; à son
expiration, la CNIL désigne l'organisme par sa catégorie et retire ses communiqués. Les
noms restent trouvables dans la presse, mais les republier contournerait une décision de
l'autorité : `core.sanction_cnil` garde la catégorie publiée, et un contrôle bloque tout
nom de groupe dans cette table. Seules les sanctions que l'autorité nomme elle-même à la
date du chargement (Google, 325 M€, CNIL 2025 ; Meta, 1,2 Md€, autorité irlandaise 2023)
sont des faits nommés.

**Le RGPD et les contrats de l'État ne se confondent pas.** Aucune décision chargée ne
constate une violation du RGPD par un fournisseur *dans l'exécution d'un contrat
public* ; les sanctions nommées portent sur des services grand public. Le cadre de
transfert UE-États-Unis est en vigueur (décision de 2023, confirmée par le Tribunal en
septembre 2025). Le dossier documente donc l'exposition juridique (CLOUD Act, FISA,
arrêt Schrems II) et l'écart entre la règle française (loi SREN, doctrine « cloud au
centre ») et les achats constatés — pas une illégalité qui n'a pas été jugée.

**Tous les marchés informatiques, pas les groupes suivis.** Mesurer la part des groupes
étrangers sur les seuls groupes qu'on a choisis de suivre fabriquerait le résultat.
`core.marche_numerique` prend tous les marchés des codes CPV 48, 72 et 302. Il en ressort
surtout ce que la commande publique ne dit pas : le titulaire est le plus souvent un
revendeur ou un intégrateur français, et l'objet ne nomme un éditeur que dans une petite
minorité de marchés. La part de l'argent public qui revient à des éditeurs étrangers
n'est pas mesurable avec les données publiées ; le dossier donne les seuls chiffres
officiels partiels (ventes de l'UGAP, marché cloud de l'UGAP) et le dit.

**Une déclaration rapportée par le Sénat n'est pas un constat du Sénat.** Les faits
distinguent OFFICIEL (établi par l'institution, ou propos dont l'institution atteste
qu'il a été tenu, comme la réponse de Microsoft France sous serment) et DECLARATIF
(chiffre ou affirmation d'une partie : les 71 % de part de marché avancés par une
association, les surcoûts de 200 à 1 300 % avancés par un ministère et contestés par un
éditeur, la suspension de la messagerie du procureur de la CPI rapportée par la presse et
contestée par Microsoft).

**Le catalogue de l'ANSSI est lu par pdftotext.** Le projet n'avait pas de lecteur PDF ;
comme 7z pour la SAE, l'outil est exigé et son absence fait échouer le connecteur. Le
même outil relit les pages du rapport du Sénat n° 830 : chaque fait tiré d'un PDF est
désormais contrôlé contre le texte, et non plus seulement scellé.

## D-066 — Un même plan, une même échelle de qualité et des citations liées pour tous les dossiers

Migration 0098, paquet `internal/dossiers` (`-only=dossiers`), commande
`cmd/sections-dossiers`, perimetre.md § 2.8.

**Pourquoi.** Les dossiers avaient été écrits au fil des questions : cinq types d'en-tête,
sept noms pour « ce qui manque », des formules de verdict (« la prémisse est fausse »,
« idées reçues vérifiées », « Non, la France n'est pas un paradis fiscal ») contraires à
D-002, des « chiffres souvent cités » sans dire par qui, et deux échelles de qualité des
faits (OFFICIEL/PRESSE/ENTREPRISE pour l'évasion fiscale, OFFICIEL/DECLARATIF/PRESSE pour
la souveraineté numérique). Le dossier souveraineté, réécrit dans l'ordre contexte →
enjeux → cadre → contrôles → situation, a servi de modèle.

**Décidé.**

- **Un plan unique** (perimetre.md § 2.8). Les sections existantes des dossiers sont
  rangées dans ce plan en gardant leur numérotation, pour que les renvois « § 1.3 » du
  journal et du code restent valides.
- **Une table de faits pour tous les dossiers**, `ref.fait_dossier`, avec une preuve
  obligatoire relue au chargement : phrase attendue dans un document scellé, dans un texte
  du Journal officiel chargé (article compris), ou dans une prise de parole de l'Assemblée
  chargée. `ref.fait_souverainete` (migration 0097, jamais commitée) y est fondue.
- **Une échelle de qualité unique**, `ref.qualite_fait` : le communiqué d'entreprise
  (ENTREPRISE) devient DECLARATIF, comme la déclaration d'une association ou d'un élu en
  séance. Une liste unique de natures de montant, `ref.nature_montant`, référencée par les
  deux tables de faits.
- **Les citations de personnes publiques sont liées à leur fiche.** `person_id` relie le
  fait à `core.person` ; `derived.fait_dossier_personne` dit si la fiche existe sur le site
  (mêmes critères que `cmd/build`). Les prises de parole sont référencées par le slug du
  paragraphe (`intervention_slug`), sans clé étrangère : le connecteur des comptes rendus
  recharge sa table entière.
- **Les sections factuelles sont générées** par `cmd/sections-dossiers`, entre marqueurs
  `<!-- faits:SECTION:debut … -->`, comme la figure du bulletin de paie (D-060).

**Ce que cela coûte.** Les « enjeux » d'un dossier ne peuvent plus être écrits par le
projet : ils viennent d'une institution citée, ce qui rend certaines sections courtes. Les
rapports de la Cour des comptes sont lus dans leur PDF (la page d'accueil du site a renvoyé
une erreur 502, les fichiers se téléchargent) : le premier chargé est celui d'avril 2026 sur
les semi-conducteurs. Les faits de contexte tirés des débats
couvrent la seule Assemblée nationale depuis juillet 2024 (couverture des comptes rendus
chargés).

## D-067 — Des dossiers verticaux par mission budgétaire ; une source non officielle ouvre une piste sans porter de chiffre

Migration 0102, `internal/dossiers/verticales.go` et `souverainete_semiconducteurs.go`,
`cmd/sections-dossiers` (section BUDGET), perimetre.md § 2.8.

**Pourquoi.** Treize missions de l'État chargées dans `core.budget_programme` n'avaient
aucun dossier : justice, culture, recherche et enseignement supérieur, logement, outre-mer,
action extérieure, aide au développement, sport, fonction publique, économie, France 2030,
plan de relance, agriculture. Et le dossier souveraineté numérique affirmait qu'aucune
source chargée ne documentait la production de puces en France, alors que la Cour des
comptes, la Commission européenne, la DGE et le dossier de concertation de Crolles le font.

**Décidé.**

- **Onze dossiers verticaux**, au plan commun, les missions proches regroupées (culture et
  médias, action extérieure et aide au développement, économie et participations, France
  2030 et relance). Chaque dossier déclare ses missions dans `ref.dossier_mission` par un
  motif, parce que les libellés changent d'un projet de loi de finances à l'autre ; le
  chargement refuse un motif sans correspondance.
- **Le tableau des crédits est généré** (`derived.dossier_budget_programme`, section
  BUDGET) : crédits de paiement par programme des projets 2024 et 2025, total des
  autorisations d'engagement, et la mention « ni votés ni exécutés » sous chaque tableau.
  `cmd/verify` contrôle que la somme par programme retombe sur le total de la mission.
- **Les contrôles viennent des synthèses des rapports spéciaux du Sénat** sur le projet de
  loi de finances pour 2026 (rapport général n° 139, annexes 31 à 330), liées à la fiche du
  rapporteur spécial ; les cadres, des textes du Journal officiel chargé, article compris
  quand le fait cite un objectif chiffré.
- **Les articles de Laurent Bloch** (usine de Crolles en 2014, règlement européen sur les
  puces en 2022) ont servi de point de départ au volet semi-conducteurs. Ils sont cités
  comme analyses signées (*déclaratif*) ; tous les chiffres du volet viennent du rapport de
  la Cour des comptes d'avril 2026, de la communication COM(2022) 45, du règlement
  (UE) 2023/1781, de la DGE et du dossier de concertation publié par la CNDP (celui-ci
  *déclaratif*, puisque rédigé par l'entreprise).

**Ce que cela coûte.** Les tableaux comparent deux projets de loi de finances, quand le
Sénat compare la loi votée pour 2025 au projet pour 2026 : les chiffres ne se recoupent pas
directement, et le piège est écrit dans chaque dossier. L'exécution par mission n'est pas
chargée. Deux affirmations de Laurent Bloch — la dépendance de la défense aux composants
soumis à la réglementation américaine ITAR, et Crolles seule usine européenne de
processeurs de pointe en 2014 — restent les siennes : aucune source officielle chargée ne
les recoupe.
