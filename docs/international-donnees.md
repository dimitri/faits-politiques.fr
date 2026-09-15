# La France en contexte : Europe, G8, monde

> **Dossier** · version 2 · 15 septembre 2026
>
> Comment la France se situe-t-elle en Europe, au sein du G8 et dans le monde, sur des sujets
> qui ont chacun leur source, leur définition et souvent une couverture géographique incomplète ?
> Le dossier charge deux volets — le salaire minimum, le PIB et sa lecture au regard de
> l'épuisement des ressources — et décrit les sources candidates des dix autres.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->



<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->



<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun texte n'est encore chargé pour ce dossier.

<!-- faits:CADRE:fin -->

### 1. Méthode : comparer des pays

Chaque comparaison internationale de ce dossier respecte trois règles, déjà
appliquées ailleurs dans ce dépôt (`docs/dette-donnees.md`,
`docs/pauvrete-donnees.md`) :

1. **Une même mesure, une même définition, pour tous les pays comparés** —
   jamais un chiffre construit autrement pour un pays glissé à côté d'un
   chiffre construit autrement pour un autre.
2. **L'absence d'un pays n'est jamais complétée par une estimation.** Un pays
   manquant dans une source reste manquant dans ce dossier.
3. **Aucun classement moral.** Une comparaison chiffrée décrit un écart, elle
   ne dit jamais qui « fait mieux » — `docs/perimetre.md` § 2 s'applique ici
   comme ailleurs, avec une acuité particulière puisque le sujet invite plus
   qu'un autre au classement.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. Le salaire minimum : une comparaison possible, une lecture à ne pas forcer

`core.salaire_minimum` (Eurostat `earn_mw_cur`, mensuel brut, semestriel) —
seul jeu identifié qui réunit l'Europe et un pays du G8 hors Union dans la
même définition :

| Pays | Dernier montant (EUR/mois) |
|---|---:|
| Luxembourg | 2 771 |
| Irlande | 2 391 |
| Allemagne | 2 343 |
| Pays-Bas | 2 338 |
| Belgique | 2 234 |
| France | 1 867 |
| **États-Unis** | **1 103** |
| Roumanie | 825 |
| Ukraine | 169 |

**Le chiffre américain est le plancher fédéral, pas le salaire minimum
effectif de la plupart des travailleurs.** Les États-Unis n'ont pas relevé
leur minimum fédéral (7,25 $/heure) depuis 2009 ; de nombreux États et villes
appliquent un minimum bien supérieur (au-delà de 16 $/heure en Californie,
par exemple). Ce dossier ne charge que le chiffre fédéral, seul publié par
Eurostat dans cette série — comparer 1 103 € à la France sans cette réserve
donnerait une fausse image de l'écart réel.

**Sept pays n'ont pas de salaire minimum légal, et n'apparaissent donc pas
dans cette table** : l'Autriche, l'Italie, le Danemark, la Suède, la
Finlande, la Norvège et l'Islande — l'Allemagne, elle, y figure depuis sa loi
de 2015. Dans ces sept pays, le niveau plancher résulte de la seule
négociation collective, branche par branche. Leur absence n'est pas un
chiffre à zéro : c'est un système différent, que cette table ne peut pas
réduire à un seul nombre.

**Le Royaume-Uni s'arrête en 2020**, dernière valeur publiée dans cette
série par Eurostat après sa sortie du dispositif statistique européen —
comme pour le taux de pauvreté (`docs/pauvrete-donnees.md` § 3), un fait à
signaler plutôt qu'à masquer en présentant une valeur ancienne comme
actuelle.

**Le Japon et le Canada n'ont pas de minimum national unique** — le Japon
fixe un minimum par préfecture, le Canada par province — ce qui explique
leur absence de cette même série Eurostat sans qu'il s'agisse d'une lacune
de collecte.

### 3. Le PIB, et ce qu'il ne retranche jamais

Le PIB compte l'extraction d'une ressource naturelle comme une production,
sans jamais retrancher l'épuisement du stock. Deux pays qui vendent le même
baril de pétrole affichent la même croissance — que l'un réinvestisse la
rente dans une économie diversifiée, ou que l'autre épuise un gisement fini
sans rien construire à la place. Le PIB seul ne fait pas la différence.

`core.indicateur_mondial` (Banque mondiale, dix pays de comparaison — G8
historique, Chine, Arabie saoudite) :

| Pays | PIB 2023 (Md$) |
|---|---:|
| États-Unis | 27 812 |
| Chine | 18 270 |
| Allemagne | 4 562 |
| Japon | 4 385 |
| Royaume-Uni | 3 421 |
| France | 3 056 |
| Italie | 2 317 |
| Canada | 2 197 |
| Russie | 2 046 |
| Arabie saoudite | 1 219 |

**L'épargne nette ajustée** (*Adjusted Net Savings*, Banque mondiale) est
l'indicateur qui corrige précisément cet angle mort : elle retranche du
revenu national l'épuisement des ressources naturelles, l'usure du capital
humain et les dégâts environnementaux, en plus de la consommation de capital
fixe déjà retranchée du PIB net. Pour 2020, dernière année disponible pour
neuf des dix pays (Royaume-Uni non disponible depuis 2018 sur cette série
précise) :

| Pays | Épargne nette ajustée (% RNB) | dont épuisement des ressources (% RNB) |
|---|---:|---:|
| Arabie saoudite | 10,9 | 5,1 |
| Russie | 8,5 | 3,9 |
| Chine | 15,3 | 0,5 |
| Canada | 3,8 | 0,4 |
| États-Unis | 5,7 | 0,2 |
| Italie | 5,5 | 0,0 |
| Allemagne | 13,2 | 0,0 |
| Japon | 4,7 | 0,0 |
| France | 5,9 | 0,0 |

**L'Arabie saoudite et la Russie perdent, chaque année, plusieurs points de
leur revenu national en ressources non renouvelées épuisées** — une perte
que leur PIB ne montre jamais, puisqu'il compte l'extraction comme un
revenu plutôt que comme la vente d'un capital. **Ce n'est pas un jugement sur
leur politique économique** : l'épargne nette ajustée reste positive pour
les deux (elles réinvestissent plus qu'elles n'épuisent, au moins sur ce
critère), et ce dossier ne conclut pas au-delà de ce que la mesure dit.

**Ces deux pourcentages ne se comparent jamais en valeur absolue au PIB en
dollars du tableau précédent** — l'un est un montant, les deux autres des
ratios au revenu national brut, une grandeur différente du PIB elle-même
(voir le commentaire de `core.indicateur_mondial`).

## Ce que les données ne disent pas

### 4. Les dix autres sujets : scopés, non chargés

Chacun des dix sujets suivants a une source candidate identifiée dans le
plan de travail (`docs/decisions.md` et le plan de pivot éditorial), mais
aucune donnée n'est chargée à ce stade. Pour chacun, la raison précise plutôt
qu'une case vide :

| Sujet | Source candidate | Pourquoi non chargé |
|---|---|---|
| Dette publique hors UE (G8, Chine) | FMI *World Economic Outlook* | Format et licence non vérifiés à ce stade — la dette européenne est déjà chargée (`internal/dette/eurostat.go`) |
| Régime politique | V-Dem Institute, Freedom House | Score composite portant un jugement de valeur inhérent — à charger avec une attribution explicite, jamais comme un verdict du site |
| Liberté de la presse | Reporters sans frontières | Format d'export non vérifié |
| Représentativité des dirigeants | International IDEA (participation) | Part du vainqueur et mode de scrutin non harmonisés entre pays — compilation manuelle nécessaire |
| Mouvements sociaux | ILOSTAT (jours de grève), ACLED (événements) | Deux mesures hétérogènes, couverture pays inégale |
| Santé (comparaison internationale) | OCDE Health Statistics, OMS | Non explorées ; le terrain français est déjà couvert (chantier 8) |
| Heures travaillées vs PIB | OCDE (*Average annual hours actually worked*) | Couverture essentiellement OCDE — ne couvre pas la Chine, l'Inde, la Russie |
| Âge de départ à la retraite | OCDE *Pensions at a Glance* | Publication biennale, probablement en tableaux non structurés |
| Liens économiques avec des pays en guerre | Douanes françaises + UCDP (conflits) | Croisement délicat, prudence éditoriale maximale requise avant tout chargement |
| OTAN / opérations de maintien de la paix | OTAN (dépense de défense), ONU (contributeurs) | Formats PDF/HTML non vérifiés |

## Sources

- Eurostat, `earn_mw_cur` (salaire minimum national mensuel).
- Banque mondiale, `NY.GDP.MKTP.CD` (PIB courant), `NY.ADJ.SVNG.GN.ZS`
  (épargne nette ajustée), `NY.ADJ.DRES.GN.ZS` (épuisement des ressources
  naturelles).
- [docs/dette-donnees.md](dette-donnees.md), pour la comparaison européenne
  de la dette déjà chargée.
- [docs/pauvrete-donnees.md](pauvrete-donnees.md), pour la même méthode
  appliquée au taux de pauvreté.

## Annexe technique

### 5. Ce qui est chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Eurostat, `earn_mw_cur` | `core.salaire_minimum` | 4 573 lignes, 31 pays, 1999-2026 |
| 2 | Banque mondiale, *World Development Indicators* | `core.indicateur_mondial` | 682 lignes, 10 pays, 3 indicateurs, 2000-2024 |

## Versions

- **Version 2** (15 septembre 2026) : plan commun des dossiers (D-066).
- **Version 1** (15 septembre 2026) : salaire minimum, PIB et épuisement des ressources ; dix sujets décrits, non chargés.
