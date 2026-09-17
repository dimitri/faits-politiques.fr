# Fraude fiscale : ce qu'on sait chiffrer, ce qu'on ne sait pas

> **Dossier** · version 1 · 17 septembre 2026
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

### 2. Le contrôle fiscal : aucun jeu de données ouvert trouvé

Le nombre de contrôles fiscaux, les droits rappelés et les pénalités
appliquées chaque année sont des chiffres réels, publiés — mais **pas en
open data**. Vérifié directement : le jeu de données « Tableaux
statistiques » de la DGFiP sur data.gouv.fr (17 fichiers, 2004-2022) ne
couvre que l'assiette et les recettes fiscales (IR, IS, TVA, CFE-CVAE, IFI,
DMTO), jamais le contrôle fiscal ; une recherche en texte intégral sur
data.gouv.fr pour « contrôle fiscal » et pour « fraude fiscale » ne retourne
aucun jeu de données. Ces chiffres existent uniquement sous forme d'articles
de presse institutionnelle (economie.gouv.fr) et d'indicateurs de
performance dans les annexes budgétaires (programme 156), publiés en HTML
ou PDF, pas en tableau exploitable. **Non chargé, faute de source
structurée — pas un oubli.**

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

- **Le contrôle fiscal réel** (§ 2) : nombre de contrôles, droits rappelés,
  pénalités — aucune source structurée trouvée en open data à ce jour.
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
- Conseil des prélèvements obligatoires, rapport 2007 sur la fraude fiscale.
- Solidaires Finances Publiques, rapport 2018 sur le coût social de la
  fraude fiscale et sociale.

## Versions

- **Version 1** (17 septembre 2026) : l'écart de TVA (6 à 10 Md€, DGFiP,
  méthode validée par contrôles aléatoires) distingué explicitement du
  chiffre médiatique 30-100 Md€, jamais établi par une institution ;
  absence documentée de données ouvertes sur le contrôle fiscal.
