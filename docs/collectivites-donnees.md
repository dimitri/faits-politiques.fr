# Collectivités : d'où vient l'argent, et ce qu'un euro par habitant ne dit pas

> **Dossier** · version 2 · 15 septembre 2026
>
> Communes, intercommunalités, départements, régions : d'où vient l'argent que les
> collectivités dépensent ? Le dossier sépare deux origines — la fiscalité qu'elles votent et
> les transferts de l'État, dont la dotation globale de fonctionnement (DGF) — et ce qu'un
> montant par habitant ne dit pas.

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

### 1. Deux origines : fiscalité propre et transferts de l'État

Une collectivité locale finance ses dépenses de deux façons principales :
**la fiscalité qu'elle vote elle-même** (taxe foncière, part locale de
cotisation économique territoriale, etc. — regroupée ici sous « impôts et
taxes ») et **les transferts de l'État**, dont le plus connu est la
**dotation globale de fonctionnement (DGF)**. Confondre les deux revient à
confondre une collectivité qui s'autofinance et une collectivité sous
perfusion de l'État — une différence politique et budgétaire réelle, que
seule la ventilation permet de voir.

Cette série et cette série chargent maintenant trois agrégats OFGL par habitant pour
les quatre niveaux — **recettes totales**, **DGF**, **impôts et taxes** — aux côtés
des cinq indicateurs de dépense déjà documentés sur la page `/collectivites/`.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Situation chiffrée

### 2. La photographie 2025

Cette série (vue qui somme les indicateurs par habitant en masse totale, niveau par
niveau) :

| Niveau | Recettes totales | dont DGF | dont impôts et taxes |
|---|---:|---:|---:|
| Communes (34 772) | 121,1 Md€ | 10 % | 54 % |
| Intercommunalités (1 262) | 51,1 Md€ | 13 % | 46 % |
| Départements (97) | 96,8 Md€ | 8 % | 63 % |
| Régions (17) | 41,8 Md€ | 1 % | 62 % |

**Le reste des recettes totales — 30 à 40 % selon le niveau — n'est pas
détaillé par cette seule agrégation** : subventions diverses, produits des
services (cantines, droits d'entrée), dotations autres que la DGF, emprunts
le cas échéant. Ce dossier cite ce qu'il mesure, pas un total reconstitué.

**N'additionnez jamais ces montants entre niveaux** : une subvention
départementale à une commune, ou l'attribution de compensation qu'une
intercommunalité reverse à ses communes membres, est comptée en recette chez
celui qui la reçoit et en dépense chez celui qui la verse — le même
avertissement que pour les cinq indicateurs de dépense déjà publiés.

### 3. Les régions, un cas à part : la DGF a presque disparu, pas les transferts de l'État

**1 % de DGF pour les régions, contre 8 à 13 % pour les autres niveaux — un
fait réel, pas une lacune de chargement.** Depuis la loi de finances pour
2018, la DGF des régions a été remplacée par une fraction du produit de la
TVA, une recette de l'État reversée sans que la région en vote le taux.

**Où se loge cette fraction de TVA dans cette agrégation reste une question
ouverte, honnêtement non tranchée.** La part d'« impôts et taxes » des
régions (62 %) est comparable à celle des départements (63 %) — un indice
que l'OFGL compte vraisemblablement cette recette de substitution dans cette
catégorie plutôt que dans une case à part, mais ce dossier n'a pas ouvert la
méthodologie détaillée de l'OFGL pour le confirmer. **Une région à faible DGF
n'est donc pas une région abandonnée par l'État** : c'est un changement de
tuyau budgétaire de 2018, pas un désengagement.

## Ce que les données ne disent pas

### 4. Ce que ce chargement ne permet pas encore de faire

**Le pont vers la santé et l'éducation nationale, posé mais pas chiffré.** Les
départements financent une part de l'action sociale et des collèges, les régions les
lycées et une part de la formation professionnelle, et certains établissements de
santé publics sont des établissements territoriaux — mais aucune ligne budgétaire
chargée ici n'isole la part d'un budget départemental ou régional qui va
spécifiquement à un collège, un lycée ou un hôpital. Cette série documentent déjà
quelles intercommunalités déclarent une compétence donnée (voir
[docs/bassins-versants-donnees.md](bassins-versants-donnees.md) pour le même exercice
appliqué à l'eau) ; le chiffrage financier de ce pont reste un chantier séparé, non
commencé.

**La fiscalité locale par mécanisme et par payeur (DGFiP REI), chargée
partiellement.** `core.fiscalite_directe_locale` (migration 0119,
`internal/communes/fiscalite_locale.go`) porte le produit réel national de
quatre impôts du bloc communal — foncier bâti, foncier non bâti (ménages),
CFE, TASCOM (entreprises) — agrégé côté serveur (group_by) plutôt que
téléchargé commune par commune : le REI publie ~35 millions de lignes par
commune × variable × année, et une seule variable de tête par dispositif ×
destinataire (jamais un sous-total ET son détail, qui ne s'additionnent
pas — un premier calcul naïf avait donné 21,9 Md€ de CFE intercommunale
contre 7,3 Md€ réels). **Ce que ce chargement NE donne PAS** : le détail
commune par commune (taux voté, base imposable) qu'évoquait la version
précédente de cette note — seul l'agrégat national par mécanisme est
chargé, la question posée ici étant « qui paie, au total » plutôt que la
fiscalité de telle commune précise. IFER, taxe d'habitation résiduelle,
TEOM et les surtaxes GEMAPI/TSE/CHAMBRE ne sont pas chargés — IFER en
particulier répète la même valeur régionale sur chaque commune membre de
la région (vérifié directement), ce qu'un agrégat national naïf prendrait
pour une vraie ventilation territoriale et fausserait de plusieurs ordres
de grandeur.

## Sources

- OFGL (Observatoire des finances et de la gestion publique locales) /
  DGCL, données par habitant par niveau de collectivité, 2018-2025.
- DGFiP, Registre des éléments d'imposition (REI), diffusion OFGL —
  fiscalité directe locale, 2024-2025 (§ 4).
- [docs/mairies-conception.md](mairies-conception.md), pour ce qui est
  comparable et ce qui ne l'est pas entre communes.
- [docs/bassins-versants-donnees.md](bassins-versants-donnees.md), pour un
  exemple déjà traité de compétence territoriale partagée entre niveaux.

## Annexe technique

### 5. Ce qui est chargé

| # | Source | Volume |
| --- | --- | --- |
| 1 | OFGL / DGCL, budgets communaux | 34 256 à 34 772 communes, 2018-2025 |
| 2 | OFGL / DGCL, budgets région/département/groupement | mêmes trois codes, 2018-2025 |
| 3 | Vue dérivée | déjà en place, étendue automatiquement aux trois nouveaux codes |
| 4 | DGFiP (REI), diffusion OFGL — fiscalité directe locale | 4 dispositifs × 2 destinataires × 2 millésimes (2024-2025), agrégats nationaux |

**Non chargé, et pourquoi** :
- **Détail de la fraction de TVA versée aux régions** : non isolée dans la
  nomenclature OFGL utilisée ici — § 3.
- **Fiscalité locale, détail commune par commune (taux voté, base
  imposable)** : seul l'agrégat national par mécanisme est chargé — § 4.
- **IFER, taxe d'habitation résiduelle, TEOM, surtaxes GEMAPI/TSE/CHAMBRE** :
  hors du périmètre ménages/entreprises visé, IFER de surcroît affecté d'un
  piège de comptage vérifié (valeur régionale répétée par commune) — § 4.
- **Chiffrage du financement territorial de la santé et de l'éducation** :
  compétences identifiées, montants non isolés — § 4.

## Versions

- **Version 3** (16 septembre 2026) : qui paie, via quel mécanisme fiscal
  (DGFiP REI, foncier bâti/non bâti, CFE, TASCOM) — ménages contre
  entreprises, en plus de la distinction déjà chargée fiscalité propre
  contre transferts de l'État.
- **Version 2** (15 septembre 2026) : plan commun des dossiers.
- **Version 1** (15 septembre 2026) : dotation globale de fonctionnement et fiscalité propre.
