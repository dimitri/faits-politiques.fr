# SCI et holding : quels impôts, lesquels peuvent être évités, et comment

> **Dossier** · version 3 · 17 septembre 2026
>
> Une SCI ou une holding ne signifie jamais « absence d'impôt » — mais peut en
> déplacer le moment, le niveau, ou le rythme. Ce dossier montre le mécanisme
> légal de chacune, avec un exemple chiffré à chaque étape : ce qui est payé,
> ce qui est déplacé, et ce qui reste dû, tôt ou tard.

---

## Contexte

<!-- faits:CONTEXTE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Dans les comptes rendus de séance de l'Assemblée nationale chargés (du 18 juillet 2024 au 21 juillet 2026), les interventions qui emploient les mots du dossier :

| expression | interventions | orateurs distincts | première | dernière |
|---|---:|---:|---|---|
| holding, pacte Dutreil | 453 | 102 | 21 octobre 2024 | 15 juillet 2026 |
| IFI | 114 | 46 | 14 octobre 2024 | 28 mai 2026 |

Une mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).

<!-- faits:CONTEXTE:fin -->

## Enjeux

<!-- faits:ENJEUX:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **Remonter des dividendes d'une filiale vers une holding est imposé à 1,25 % au maximum, pas 0 %** (1er décembre 2025). Le CPO chiffre le régime mère-fille : la quote-part pour frais et charges (5 % du dividende, taxée à l'IS) fait que le transfert de la filiale vers la holding est imposé à un taux effectif de 1,25 % au maximum — 0,25 % seulement en cas d'intégration fiscale. — Conseil des prélèvements obligatoires (Cour des comptes), Corriger les principales distorsions de l'imposition du patrimoine · [source](https://www.ccomptes.fr/sites/default/files/2025-12/20251201-Corriger-les-principales-distorsions-de-l-imposition-du-patrimoine.pdf) · *officiel*

<!-- faits:ENJEUX:fin -->

## Cadre

<!-- faits:CADRE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

- **La loi PME de 2005, qui porte l'exonération du pacte Dutreil à 75 %** (2 août 2005). Son article 28 porte l'exonération de droits de mutation à titre gratuit du pacte Dutreil de la moitié à 75 % de la valeur des titres transmis. — Parlement (loi n° 2005-882) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000000452052) · *officiel*
- **La loi de finances pour 2018, qui crée l'impôt sur la fortune immobilière (IFI)** (30 décembre 2017). Son article 31 institue l'IFI (seuil d'assujettissement de 1 300 000 €) et exonère les biens immobiliers affectés à l'activité professionnelle réelle du redevable, y compris via une société à l'IS sous conditions de fonction et de détention. — Parlement (loi n° 2017-1837) · [source](https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000036339197) · *officiel*

<!-- faits:CADRE:fin -->

### 1. Ce que SIRENE dit déjà : combien de structures, réellement

Ce dépôt charge déjà le répertoire SIRENE en entier (`ref.unite_legale`, 13
millions d'unités légales) — jamais surfacé jusqu'ici sous cet angle. Parmi
les structures **actives** aujourd'hui :

| Forme juridique | Actives aujourd'hui |
|---|---:|
| SARL | 2 233 470 |
| SCI (dont SCI de construction-vente) | 1 950 199 |
| SAS | 1 824 501 |
| Associations déclarées | 1 240 302 |

Une SCI sur cinq environ des structures actives recensées ici est une société
civile immobilière — un ordre de grandeur, pas un chiffre sur les seules SCI
« à but d'optimisation » : la plupart portent un bien familial ou
professionnel unique, sans aucun montage fiscal particulier.

### 2. La SCI : transparence fiscale (IR) ou société opaque (IS)

Une SCI est **par défaut soumise à l'impôt sur le revenu** : c'est une
société de personnes au sens de l'article 8 du code général des impôts (CGI).
Elle peut opter pour l'impôt sur les sociétés (IS), une option en principe
irrévocable.

**SCI à l'IR — la transparence fiscale, pas une exonération.** Les revenus
fonciers ne sont pas imposés au niveau de la société : chaque associé déclare
sa quote-part et la paie à son propre taux marginal d'impôt sur le revenu,
plus les prélèvements sociaux (17,2 %). Aucune magie d'exonération — l'impôt
existe toujours, seulement réparti entre associés selon leurs parts. Exemple
pédagogique : un bien loué 15 000 € par an, 3 000 € de charges déductibles,
soit 12 000 € de résultat imposable ; pour un associé dans la tranche à
30 %, l'imposition combinée (IR + PS) est de 47,2 %, soit 5 664 € pour cet
exemple.

**SCI à l'IS — l'amortissement réduit l'impôt annuel, mais le reporte à la
revente.** Le bâti (hors valeur du terrain) peut être amorti comptablement
sur sa durée d'usage (25 à 30 ans en général), ce qui réduit le résultat
imposable chaque année. Sur le même exemple : un bâti de 240 000 € amorti
sur 25 ans (9 600 € par an), le résultat imposable tombe à 2 400 €, taxé au
taux réduit d'IS de 15 % (jusqu'à 42 500 € de bénéfice, sous conditions —
article 219 du CGI) : 360 € d'impôt, contre 5 664 € à l'IR sur le même
exemple.

**Ce que cet avantage annuel déplace, il ne l'efface pas.** À la revente, la
plus-value professionnelle se calcule sur la **valeur nette comptable**
(prix d'acquisition moins les amortissements déjà déduits), pas sur le prix
d'acquisition réel — et ne bénéficie d'aucun abattement pour durée de
détention, contrairement au régime des particuliers. Sur l'exemple : un bien
acheté 300 000 €, amorti de 96 000 € sur dix ans, revendu 350 000 € :

| | SCI à l'IR (régime des particuliers) | SCI à l'IS (régime professionnel) |
|---|---:|---:|
| Base de la plus-value | 350 000 − 300 000 = **50 000 €** | 350 000 − 204 000 (valeur nette comptable) = **146 000 €** |
| Abattement pour durée de détention | Oui (progressif depuis la 6ᵉ année, exonération totale de l'IR à 22 ans — article 150 VC du CGI) | Aucun |

Chaque euro d'amortissement qui a réduit l'impôt annuel réapparaît, des
années plus tard, dans une plus-value professionnelle plus élevée — l'IS ne
supprime pas l'impôt, il le déplace dans le temps et lui retire l'abattement
dont bénéficie un particulier.

### 3. SCI et transmission : le mécanisme du fractionnement dans le temps

La transmission d'un bien immobilier bénéficie, en ligne directe (parent à
enfant), d'un abattement de **100 000 € par parent et par enfant**, renouvelable
tous les **15 ans** (articles 779 et 784 du CGI), puis d'un barème progressif
de 5 % à 45 % au-delà (article 777 du CGI). Transmettre le bien **directement**
ne mobilise cet abattement qu'**une seule fois**. Transmettre des **parts de
SCI**, progressivement, permet de fractionner la donation en plusieurs vagues
espacées de 15 ans, mobilisant l'abattement à chaque cycle plutôt qu'une
seule fois.

**Exemple pédagogique** — un couple transmettant à deux enfants (quatre
couples parent-enfant, chacun avec son propre abattement de 100 000 €),
comparant une transmission directe en une fois contre deux donations de
parts espacées de 15 ans :

| Valeur du bien | Droits, transmission directe (une fois) | Droits, via SCI (deux donations à 15 ans d'écart) | Écart |
|---|---:|---:|---:|
| 300 000 € | 0 € | 0 € | 0 € |
| 800 000 € | 72 777 € | 0 € | 72 777 € (-100 %) |
| 2 000 000 € | 312 777 € | 225 555 € | 87 223 € (-28 %) |

**L'effet de seuil est net, et honnête à montrer tel quel** : sur un bien
d'une valeur inférieure au total des abattements mobilisables (300 000 €
ici), le montage ne change rien — les droits sont déjà nuls dans les deux
cas. Sur un bien dont la valeur correspond à peu près à deux fois le total
des abattements (800 000 €), l'effet est total. Sur un bien très supérieur
(2 000 000 €), l'écart existe mais se réduit en proportion : le barème
progressif finit par s'appliquer dans les deux cas, aucun montage ne
supprime l'impôt au-delà d'un certain montant. **Le mécanisme ne fait
disparaître aucun impôt : il change le rythme et le découpage de la
transmission**, un avantage qui grandit avec le temps disponible avant la
succession, pas avec la seule volonté du contribuable.

**Un facteur non chiffré ici** : la valorisation des parts de SCI fait
parfois l'objet d'une **décote** (illiquidité, occupation du bien) par
rapport à la valeur du bien détenu en direct — une pratique reconnue par la
jurisprudence (Cour de cassation, chambre commerciale, 9 février 2022,
n° 19-22.861) mais sans taux fixé par la loi, au cas par cas : non intégrée
au tableau ci-dessus, qui reste construit sur la valeur pleine du bien.

### 4. La holding : remonter les bénéfices sans les distribuer

Une holding est une société qui détient des participations dans d'autres
sociétés (ses « filiales »). Le mécanisme central, le **régime mère-fille**
(articles 145 et 216 du CGI), permet à une holding détenant au moins 5 % du
capital d'une filiale depuis au moins deux ans de recevoir ses dividendes
**presque intégralement exonérés d'impôt sur les sociétés** — mais pas
totalement : une **quote-part de frais et charges** de 5 % du dividende reçu
est réintégrée au résultat imposable de la holding et taxée normalement.

**Le point le plus souvent absent du débat public : un dividende n'est
jamais de l'argent qui n'a pas encore été taxé.** Une filiale ne peut
distribuer que ce qu'il lui reste après avoir payé l'impôt sur les
sociétés sur son bénéfice — le régime mère-fille s'applique à cet argent
déjà amputé de l'IS, pas au bénéfice brut. C'est l'impôt déjà acquitté à
ce premier étage, presque jamais cité quand on parle du « taux effectif de
1,25 % » du régime mère-fille, qui change entièrement la lecture du
mécanisme.

<!-- schema:holding-mere-fille -->

**Ce n'est pas une exonération totale, et le chiffrage exact en est
vérifiable, à chaque étage.** Exemple pédagogique : une filiale réalise
**100 000 € de bénéfice avant impôt**. Elle paie d'abord l'IS au taux
normal de 25 % (une filiale de cette taille dépasse le seuil de 42 500 €
de bénéfice au-delà duquel le taux réduit de 15 % réservé aux PME ne
s'applique plus) — soit **25 000 € d'impôt sur les sociétés, déjà
acquittés avant qu'un seul euro ne puisse être distribué**. Il ne reste
que **75 000 €** à distribuer. Sur ces 75 000 € remontés vers la holding,
5 % (3 750 €, la quote-part) sont taxés à l'IS (25 %), soit 937,50 € —
un taux effectif de 1,25 % **sur la somme remontée**, pas sur le bénéfice
initial. **En cumulant les deux étages, l'impôt déjà payé avant même que
l'argent ne soit dans la holding s'élève à 25 937,50 €, soit près de 26 %
du bénéfice initial** — très loin du 1,25 % isolément cité. Si la holding
et sa filiale optent pour l'**intégration fiscale** (article 223 A du CGI,
détention d'au moins 95 %), la quote-part tombe à 1 %, ramenant le cumul à
25 187,50 €, environ 25,2 % — la Cour des comptes chiffre elle-même cette
mécanique dans un rapport de décembre 2025 sur la fiscalité du patrimoine
(voir Enjeux ci-dessus).

**Le point pédagogique central : la holding déplace QUAND l'impôt est dû, pas
si.** Tant que l'argent reste dans la holding (une pratique parfois qualifiée
de « cash box »), l'impôt déjà payé (les ~26 % ci-dessus) reste le seul dû.
L'impôt plein (prélèvement forfaitaire unique de 30 % sur les dividendes, au
niveau de la personne physique) n'intervient qu'au moment où l'argent est
effectivement distribué à l'associé — pas avant. Sur le même exemple, si les
74 062,50 € restants dans la holding après la quote-part étaient
intégralement distribués à l'associé personne physique, le PFU ajouterait
22 218,75 € — portant le total cumulé, du bénéfice initial de la filiale
jusqu'au revenu net de l'associé, à **48 156,25 €, soit un peu plus de 48 %
des 100 000 € de départ**. Une holding qui ne distribue jamais rien à son
actionnaire personne physique ne lui a, par construction, procuré aucun
revenu taxable — mais ne lui a pas non plus procuré de revenu du tout.

**Le Pacte Dutreil**, pour la transmission de l'entreprise elle-même
(article 787 B du CGI, exonération portée à **75 %** de la valeur transmise
par la loi du 2 août 2005 — voir le Cadre ci-dessus), s'applique aussi aux
holdings dites « animatrices » de leur groupe. Un durcissement réel et récent
mérite d'être signalé : la loi de finances pour 2026 (loi n° 2026-103 du
19 février 2026) a porté la durée de l'**engagement individuel de
conservation** de 4 à **6 ans**, portant la durée totale minimale
(engagement collectif de 2 ans puis individuel) à **8 ans** — pas 6 ans comme
le régime l'imposait depuis 2008, pour les transmissions à compter du
21 février 2026.

### 5. L'IFI : la SCI ne protège pas, la holding professionnelle peut exonérer

L'**impôt sur la fortune immobilière** (IFI, créé par la loi de finances pour
2018 — voir le Cadre ci-dessus) frappe chaque année les actifs immobiliers
nets dont la valeur dépasse **1 300 000 €**, quel que soit le montage utilisé
pour les détenir. C'est là que les deux mécanismes de ce dossier divergent
nettement.

**La SCI ne protège de rien face à l'IFI.** Que le bien soit détenu en
direct ou via une SCI (à l'IR ou à l'IS), sa valeur — ou celle des parts
sociales qui le représentent — entre dans l'assiette taxable de l'associé
personne physique, à due proportion de sa quote-part. Interposer une SCI ne
change ni le montant, ni le redevable de l'IFI : c'est un impôt qui regarde
à travers la structure, par construction (article 965 du CGI).

**La holding, à une condition précise, le peut.** L'article 975 du CGI
exonère un bien immobilier d'IFI quand il est affecté à l'activité
professionnelle réelle de son propriétaire — y compris via une société
soumise à l'IS, à condition que le redevable y exerce une fonction de
direction effective et rémunérée, et détienne au moins 25 % des droits de
vote. Un immeuble logé dans une holding qui exploite réellement une
entreprise peut donc échapper à l'IFI ; le même immeuble logé dans une
« cash box » qui ne fait qu'accumuler des liquidités, non — l'exonération
suit l'activité réelle, pas la seule forme juridique.

<!-- schema:carte-ifi -->

**Ce que les chiffres montrent, commune par commune.** La DGFiP publie
chaque année le nombre de redevables et le patrimoine moyen à l'IFI, mais
seulement pour les communes de plus de 20 000 habitants comptant plus de
50 redevables — 237 communes en 2025, loin des 34 875 communes de France.
Parmi elles, 106 230 foyers redevables sont comptés, avec un patrimoine
moyen qui varie de 1,8 à 3,8 M€ selon la commune. Sans surprise, les
arrondissements de l'ouest parisien dominent : Paris 16ᵉ (10 159 redevables),
Paris 7ᵉ (4 558) et Neuilly-sur-Seine (4 091) concentrent, à eux seuls, plus
de redevables que la plupart des départements entiers.

## Contrôles et évaluations

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier.

<!-- faits:CONTROLE:fin -->

## Ce que les données ne disent pas

### 6. Ce qui reste hors de portée de cette version

- **L'IFI hors des communes publiées** (§ 5) : la DGFiP ne publie ce détail
  que pour 237 communes sur 34 875 — la répartition nationale complète des
  redevables et de leur patrimoine reste hors de portée.
- **Le lien entre une commune IFI et les structures qui y détiennent des
  biens** (SCI, holdings) : IFICOM compte des foyers redevables, pas des
  structures juridiques — impossible de savoir, depuis cette seule source,
  quelle part du patrimoine immobilier taxé est logée dans une SCI plutôt
  que détenue en direct.
- **Les droits de succession et de donation réellement perçus, en série** :
  seulement des faits ponctuels (Cour des comptes, DGFiP) cités ici, pas une
  série chargée en base.
- **La décote de valorisation des parts de SCI** (§ 3) : une pratique réelle,
  mais sans taux fixé par la loi — non chiffrée dans le tableau de
  transmission, qui reste construit sur la valeur pleine du bien.
- **Le barème exact des prélèvements sociaux sur les plus-values
  immobilières** (§ 2) : l'exonération totale intervient à la trentième année
  de détention (contre vingt-deux ans pour l'impôt sur le revenu), sur un
  barème distinct de celui de l'article 150 VC — le texte exact qui le fixe
  aujourd'hui n'a pas été vérifié avec une précision suffisante pour être cité
  article par article ici.
- **L'apport-cession et le report d'imposition** (article 150-0 B ter du
  CGI), un autre mécanisme réel de la fiscalité des holdings, cité par la
  Cour des comptes mais pas encore détaillé dans ce dossier.
- **Le nombre réel de SCI et holdings utilisées à des fins d'optimisation**,
  par opposition à un usage patrimonial ordinaire (un bien familial, un local
  professionnel) : SIRENE compte les structures, pas leur intention.

## Sources

- INSEE, répertoire SIRENE (`ref.unite_legale`, déjà chargé dans ce dépôt).
- Légifrance, code général des impôts, articles 8, 145, 150 VC, 216, 219,
  223 A, 777, 779, 784, 787 B, 964, 965, 975.
- Légifrance, loi n° 2005-882 du 2 août 2005 en faveur des petites et
  moyennes entreprises, article 28 (JORFTEXT000000452052).
- Légifrance, loi n° 2017-1837 du 30 décembre 2017 de finances pour 2018,
  article 31, créant l'impôt sur la fortune immobilière (JORFTEXT000036339197).
- Conseil des prélèvements obligatoires (Cour des comptes), *Corriger les
  principales distorsions de l'imposition du patrimoine*, 1er décembre 2025.
- Cour de cassation, chambre commerciale, 9 février 2022, n° 19-22.861
  (décote de valorisation des parts sociales).
- DGFiP, IFICOM — répartition communale de l'impôt sur la fortune
  immobilière, millésimes 2021 à 2025 (`core.ifi_commune`, déjà chargé dans
  ce dépôt).

## Versions

- **Version 3** (17 septembre 2026) : l'IFI — la SCI ne protège pas
  (l'assiette regarde à travers la structure), la holding professionnelle le
  peut sous conditions précises (article 975 du CGI) ; une carte des 237
  communes où la DGFiP publie le détail (IFICOM, `core.ifi_commune`,
  millésimes 2021-2025 chargés).
- **Version 2** (17 septembre 2026) : l'exemple du régime mère-fille reparti
  du bénéfice avant IS de la filiale, pas du dividende déjà net — le point
  manquant identifié dans le débat public (25 000 € d'IS déjà payés avant
  toute distribution, cumul explicite jusqu'à l'associé personne physique).
- **Version 1** (17 septembre 2026) : la SCI (transparence fiscale à l'IR,
  amortissement à l'IS, transmission fractionnée) et la holding (régime
  mère-fille, intégration fiscale, Pacte Dutreil), avec des exemples chiffrés
  à chaque étape.
