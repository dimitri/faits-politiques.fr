# Fraude fiscale : ce qu'on sait chiffrer, ce qu'on ne sait pas

> **Dossier** · version 2 · 19 septembre 2026
>
> « 30 à 100 milliards d'euros » : le chiffre le plus cité sur la fraude
> fiscale en France n'est établi par aucune institution — les estimations
> disponibles varient du simple au quadruple selon qui les produit et avec
> quelle méthode. Ce dossier distingue ce qui est mesuré, et comment, de ce
> qui reste une estimation contestée.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| fraude fiscale | 337 | 93 | 1er octobre 2024 | 9 juin 2026 |

Une mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **L'écart de TVA est estimé entre 6 et 10 Md€, soit 4 à 5 % de la TVA collectée** (1er septembre 2024). La DGFiP chiffre le manque à gagner de TVA dû à la sous-déclaration des entreprises dans une fourchette de 6 à 10 milliards d'euros, soit 4 à 5 % du montant de TVA effectivement collecté — une méthode validée par une expérience de contrôles aléatoires. — DGFiP Analyses n°7, Le manque à gagner de TVA en France · [source](https://www.impots.gouv.fr/sites/default/files/media/9_statistiques/0_etudes_et_stats/0_publications/dgfip_analyses/2024/num07_09/dgfip_analyses_07_2024.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **La loi de 2018 contre la fraude, qui permet de publier le nom des fraudeurs les plus graves** (23 octobre 2018). Son article 18 crée dans le CGI la possibilité de publier, pour les manquements les plus graves (au moins 50 000 € de droits fraudés avec manœuvre frauduleuse), la nature et le montant des droits fraudés ainsi que l'identité du contribuable. — Parlement (loi n° 2018-898) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000037518803) · *officiel*

<!-- faits:CADRE:fin -->

### 1. Ce que la loi permet déjà : nommer les fraudeurs les plus graves

Depuis la loi du 23 octobre 2018 contre la fraude (voir Cadre ci-dessus),
l'administration peut publier — sur son propre site, un an au maximum,
après avis d'une commission — l'identité, l'activité et le montant des
droits fraudés d'un contribuable pour les manquements les plus graves
(au moins 50 000 € de droits fraudés, avec manœuvre frauduleuse). Un
mécanisme réel, distinct de la question du chiffrage global des § 3 et 4
ci-dessous : il dit ce qui est **sanctionné publiquement**, pas ce qui
**existe** en fraude non détectée.

### 2. Le contrôle fiscal réel : ce qui est notifié, ce qui est encaissé

<!-- schema:controle-fiscal -->

Le nombre de contrôles fiscaux n'existe dans aucune source ouverte trouvée
à ce jour (voir § 5) — mais les **montants**, eux, sont vérifiables :
vérifié directement, le jeu de données « Tableaux statistiques » de la
DGFiP sur data.gouv.fr ne couvre que l'assiette et les recettes fiscales
(IR, IS, TVA, CFE-CVAE, IFI, DMTO), jamais l'activité de contrôle, et
economie.gouv.fr bloque l'accès direct à ses communiqués (pare-feu). Les
montants ci-dessus viennent de trois rapports du Sénat (commission des
finances), recoupés entre eux sur les années communes.

**Deux chiffres, jamais confondus** : le **notifié** (droits et pénalités
mis en recouvrement, avant recours et négociation) et l'**encaissé**
(ce que l'État perçoit effectivement) ne mesurent pas la même chose — sur
2015-2021, l'écart entre les deux atteint en moyenne près de 30 %. **Le
notifié 2022 et 2023 n'a été retrouvé dans aucune des trois sources
consultées** : la ligne s'interrompt sur ces deux années sur le graphique
plutôt que d'être devinée ou interpolée. Le notifié 2024 (16,6 Md€) est
**déduit** de l'écart notifié/encaissé que le rapport 2025 publie
explicitement (11,4 + 5,2 Md€), pas cité tel quel par la source.

### 3. L'écart de TVA : un chiffrage réel, méthodologiquement validé

Une estimation officielle et sourcée existe pour un périmètre précis :
l'écart entre la TVA théoriquement due et la TVA réellement collectée. La
DGFiP l'a publiée dans son étude *DGFiP Analyses* n°7 (septembre 2024) :
le manque à gagner de TVA lié à la sous-déclaration des entreprises est
**compris dans une fourchette de 6 à 10 Md€**, soit **4 à 5 % du montant de
TVA effectivement collecté** (voir Enjeux ci-dessus). La méthode
(rééchantillonnage statistique et modèle de gradient boosting) a été
**validée par une expérience de contrôles aléatoires** menée par les
équipes de vérification de la DGFiP en 2022 sur deux sous-secteurs (610
entreprises tirées au sort, 22 % redressées).

**Ce chiffre ne couvre qu'un seul impôt** — pas l'impôt sur le revenu, pas
l'impôt sur les sociétés, pas les cotisations sociales — et seulement la
sous-déclaration des entreprises déjà identifiées comme redevables de la
TVA, pas la fraude à l'immatriculation ou le travail dissimulé. C'est un
chiffrage réel sur un périmètre étroit, pas une estimation de « la fraude
fiscale » en général.

### 4. Le chiffre « 30 à 100 Md€ » : hors de portée, et pourquoi

Le chiffre le plus répété dans le débat public sur la fraude fiscale totale
ne vient d'aucune institution qui l'aurait mesuré directement :

| Source | Estimation | Méthode |
|---|---:|---|
| Conseil des prélèvements obligatoires (2007) | environ 20,5 à 25,6 Md€ | reconstruction macroéconomique, méthodologie ancienne |
| Solidaires Finances Publiques (syndicat, 2018) | 80 à 100 Md€ | estimation syndicale, méthode non validée par une institution de contrôle |

Un écart du simple au quadruple entre les deux, selon qui produit
l'estimation et avec quelle méthode — la Cour des comptes elle-même
constate qu'aucune estimation fiable et consolidée de l'écart fiscal total
n'existe à ce jour. **Ce dossier ne retient aucun de ces chiffres comme un
fait établi** : ni 30, ni 100 Md€ n'ont la même nature de preuve que l'écart
de TVA du § 3, chiffré par une administration avec une méthode publiée et
validée par l'expérience.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Ce que les données ne disent pas

### 5. Ce qui reste hors de portée de cette version

- **Le nombre de contrôles fiscaux réalisés chaque année** (§ 2) : les
  montants notifiés et encaissés sont chargés, pas le nombre de contrôles
  (sur place, sur pièces) — aucune source structurée trouvée à ce jour.
- **Le notifié 2022 et 2023** (§ 2) : non retrouvé dans les trois rapports
  du Sénat consultés — un chiffre existe sans doute quelque part, pas
  vérifié directement au moment du chargement.
- **La part des contrôles ciblés par datamining/IA** : citée dans un des
  rapports (56 % des contrôles professionnels en 2024, contre 22 % en
  2019) mais seulement deux points de repère, pas une série continue —
  non chargée en table pour cette raison.
- **L'écart fiscal total, tous impôts confondus** : seul l'écart de TVA
  (§ 3) est chiffré avec une méthode publiée et validée ; rien d'équivalent
  n'a été trouvé pour l'impôt sur le revenu, l'impôt sur les sociétés ou les
  cotisations sociales.
- **Le travail dissimulé et la fraude à l'immatriculation** : des
  phénomènes réels, mais distincts de la sous-déclaration de TVA mesurée
  ici, et non chiffrés dans ce dossier.
- **La fraude aux prestations sociales** : un sujet voisin mais distinct,
  hors du périmètre de ce dossier centré sur la fiscalité.

## Sources

- Direction générale des finances publiques (DGFiP), *DGFiP Analyses* n°7,
  *Le manque à gagner de TVA en France*, septembre 2024.
- data.gouv.fr, jeu de données *Tableaux statistiques de la Direction
  générale des finances publiques (DGFiP)* — vérifié pour son absence de
  données de contrôle fiscal.
- Sénat, commission des finances, rapport d'information n° 72 (2022-2023),
  *Fraude et évasion fiscales : faire les comptes et intensifier la lutte*
  (§ 2, montants 2015-2021).
- Sénat, commission des finances, rapport n° l24-034-215-1 (2024), sur les
  résultats de la gestion et l'approbation des comptes de l'État pour 2023
  (§ 2, montants 2022-2023).
- Sénat, commission des finances, rapport n° l25-139-314, projet de loi de
  finances pour 2026 (§ 2, montants 2024).
- Conseil des prélèvements obligatoires, rapport 2007 sur la fraude fiscale.
- Solidaires Finances Publiques, rapport 2018 sur le coût social de la
  fraude fiscale et sociale.

## Versions

- **Version 2** (19 septembre 2026) : résultats du contrôle fiscal chargés
  (notifié et encaissé, 2015-2024, trois rapports du Sénat recoupés) — le
  premier vrai chiffrage de l'activité de contrôle dans ce dossier, au-delà
  de l'écart de TVA. Le notifié 2022-2023, non retrouvé dans une source
  primaire, reste explicitement absent plutôt que deviné.
- **Version 1** (17 septembre 2026) : l'écart de TVA (6 à 10 Md€, DGFiP,
  méthode validée par contrôles aléatoires) distingué explicitement du
  chiffre médiatique 30-100 Md€, jamais établi par une institution ;
  absence documentée de données ouvertes sur le contrôle fiscal.
