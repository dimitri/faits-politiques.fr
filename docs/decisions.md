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
