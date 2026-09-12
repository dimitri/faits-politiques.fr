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
