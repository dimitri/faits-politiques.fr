# L'immigration en France : ce que les données permettent de dire

> Note de synthèse. Version 2 — 14 septembre 2026.
> Cette note répond à une question d'inventaire : quels chiffres existent sur
> l'immigration en France, avec quelle précision, et où s'arrête l'open data
> pour laisser place à des rapports de recherche ou à des trous documentés.
> Comme le reste du projet, elle ne rend pas de verdict sur l'immigration —
> elle établit ce qui est mesuré, par qui, et ce qui ne l'est pas.
>
> **Version 2** ajoute la profondeur historique qui manquait à la version 1 :
> un siècle de recensements (§ 8) et les flux annuels d'immigration et de
> naturalisation (§ 8.1), pas seulement des stocks récents.

---

## 1. La distinction qui commande tout le reste

Deux mots, deux populations qui se recoupent **partiellement** :

- **Immigré** : une personne née **étrangère à l'étranger**, résidant en
  France — qu'elle ait ou non acquis la nationalité française depuis son
  arrivée. Le statut d'immigré est **définitif** : on ne cesse jamais d'être
  immigré, même naturalisé.
- **Étranger** : une personne de **nationalité étrangère actuelle**, quel que
  soit son lieu de naissance. Un enfant né en France de parents étrangers est
  étranger, pas immigré, tant qu'il n'a pas acquis la nationalité française.

**Ce que la base mesure, France entière, 15 ans ou plus, 2023** (`core.population_statut_migratoire`, Insee, recensement de la population) :

| | effectif |
|---|---:|
| **Immigrés** | 6 742 713 |
| **Étrangers** | 4 428 447 |
| Population totale (15 ans ou plus) | 54 960 108 |

**L'écart entre les deux nombres — environ 2,3 millions de personnes — est très
exactement la population des immigrés ayant acquis la nationalité française.**
C'est le fait à retenir avant tout autre chiffre sur le sujet : compter « les
étrangers » et compter « les immigrés » ne répond pas à la même question, et
les deux nombres ne sont substituables dans aucune phrase.

**France entière, tous âges, 2023** (le champ le plus large chargé) : 7 109 318
immigrés sur 66 165 815 résidents, soit **10,7 %** de la population — cohérent
avec le chiffre publié par l'Insee (7,2 millions hors Mayotte, 10,6 %).

## 2. Origines géographiques

Population immigrée par pays ou zone de naissance, France entière, 2023
(`core.population_immigree_origine`, regroupement Insee — pas la liste
complète des pays du monde, celle que l'Insee choisit de publier à ce niveau
de détail) :

| origine | effectif | part des immigrés |
|---|---:|---:|
| Autres pays d'Afrique | 1 270 704 | 17,9 % |
| Reste du monde | 1 163 271 | 16,4 % |
| Algérie | 919 578 | 12,9 % |
| Maroc | 867 096 | 12,2 % |
| Autres pays de l'Union européenne | 635 903 | 8,9 % |
| Portugal | 585 169 | 8,2 % |
| Autres pays d'Europe | 544 646 | 7,7 % |
| Tunisie | 347 890 | 4,9 % |
| Italie | 280 808 | 3,9 % |
| Turquie | 254 012 | 3,6 % |
| Espagne | 240 241 | 3,4 % |

Les trois pays du Maghreb réunis (Algérie, Maroc, Tunisie) représentent
**30,0 %** des immigrés — la plus grosse part identifiable individuellement,
mais pas une majorité : « le reste du monde » et « autres pays d'Afrique »,
deux catégories fourre-tout, pèsent ensemble davantage (34,3 %).

## 3. Emploi et catégorie socioprofessionnelle

Statut d'emploi des immigrés de 15 ans ou plus, France entière, 2023 :

| statut | effectif |
|---|---:|
| Actif occupé | 3 363 636 |
| Retraité | 1 336 166 |
| Chômeur | 736 118 |
| Autre inactif | 512 412 |
| Au foyer | 466 565 |
| Étudiant | 327 815 |

Catégorie socioprofessionnelle, immigrés contre non-immigrés (15 ans ou plus,
2023, `core.population_statut_migratoire_csp`) :

| CSP | immigrés | non-immigrés | part des immigrés dans la CSP |
|---|---:|---:|---:|
| Ouvriers | 1 120 507 | 5 150 514 | 17,9 % |
| Retraités | 1 354 389 | 14 096 960 | 8,8 % |
| Autres inactifs | 1 484 035 | 6 986 050 | 17,5 % |
| Employés | 1 166 135 | 7 133 375 | 14,1 % |
| Professions intermédiaires | 692 976 | 7 164 262 | 8,8 % |
| Cadres et professions intellectuelles supérieures | 747 645 | 5 454 226 | 12,1 % |
| Artisans, commerçants et chefs d'entreprise | 313 969 | 1 761 970 | 15,1 % |
| Agriculteurs | 10 998 | 361 989 | 2,9 % |

**Les immigrés sont sous-représentés chez les agriculteurs et les professions
intermédiaires, sur-représentés chez les ouvriers** — une structure par CSP
nettement différente de celle des non-immigrés, dans les deux sens. Aucune
lecture unique (« les immigrés font les métiers que personne ne veut faire »,
« les immigrés sont cadres comme les autres ») ne couvre ce tableau en entier.

## 4. Prestations sociales et impôts : ce qui manque, et pourquoi

**Aucune administration ne publie, en open data répétable, le montant des
prestations sociales versées ou des impôts payés en fonction du statut
migratoire ou de la nationalité du bénéficiaire.** Ce n'est pas un oubli de ce
projet, c'est une limite documentée des sources elles-mêmes :

- **Impôts.** Le droit fiscal français impose sur la **résidence fiscale**,
  pas la nationalité. La DGFiP ne trace ni nationalité ni pays de naissance
  dans ses statistiques publiques (fichier POTE, données ouvertes
  impots.gouv.fr). Un montant « payé par les immigrés » ou « par les
  étrangers » n'existe dans aucune source administrative — quiconque
  l'affirme calcule, il ne cite pas.
- **TVA.** Impôt sur la dépense, jamais imputable à une personne précise,
  quel que soit son statut : aucune administration, nulle part, ne peut
  produire ce chiffre par construction.
- **Prestations sociales.** La Cnaf publie une part agrégée (fin 2022, 11 %
  des foyers allocataires toutes prestations confondues sont de nationalité
  étrangère, pour environ 13 % de la masse versée) — un chiffre cité dans la
  presse et par des associations de vérification, **pas un jeu de données
  ouvert et répétable**, donc non chargé ici. La Cnav, qui gère l'essentiel de
  l'Aspa (minimum vieillesse), compte ses bénéficiaires par **pays de
  naissance**, pas par nationalité — un changement de classification au
  milieu du sujet qui rend toute comparaison directe avec les chiffres Cnaf
  trompeuse.

**La conséquence pour ce projet : les seuls chiffres de coût ou de
contribution nette qui suivent (§ 6) viennent de la littérature de recherche,
pas de comptes administratifs.** C'est une différence de nature, pas de degré,
avec les tableaux des sections 1 à 3.

## 5. Comparaison européenne (Eurostat)

Eurostat publie la même distinction sous un autre nom — **citoyenneté**
(l'équivalent européen d'« étranger ») et **pays de naissance** (l'équivalent
européen d'« immigré ») — ce qui permet de vérifier que la distinction n'est
pas une particularité française. Chargé dans
`core.eurostat_population_migratoire`, France, 2023 :

| | citoyenneté | pays de naissance |
|---|---:|---:|
| Nationaux / natifs | 62 648 487 | 59 313 899 |
| Union européenne (hors France) | 1 541 776 | 1 990 953 |
| Hors Union européenne | 4 086 947 | 6 972 358 |
| **Total** | **68 277 210** | **68 277 210** |

**8,96 millions de personnes nées à l'étranger** selon Eurostat (pays de
naissance), contre **7,11 millions d'immigrés** selon l'Insee la même année.
L'écart (environ 1,85 million) n'est pas une erreur : les deux instituts ne
mesurent pas exactement le même champ (l'Insee exclut par exemple les
personnes nées françaises à l'étranger de la catégorie « immigré », qu'Eurostat
peut classer différemment selon la source nationale transmise). **Deux
sources sérieuses sur le même sujet, la même année, le même pays, et deux
nombres qui ne coïncident pas d'1,85 million** : un rappel utile avant de
citer un seul chiffre comme s'il allait de soi.

*Réserve technique : la décomposition national/UE27/hors UE27 n'est complète
pour la France, dans la source Eurostat, qu'à partir de 2015 — avant, seul le
total est fiable (`cmd/verify` le contrôle).*

## 6. Ce que dit la recherche sur la contribution nette aux finances publiques

Trois méthodes coexistent dans la littérature économique (résumées par le CAE,
voir ci-dessous) : l'hypothèse de l'« aimant social » (test d'une dépendance
différentielle aux prestations), l'« approche comptable » (imputation de
profils moyens de taxes et transferts par âge et qualification, immigrés
contre natifs, une année donnée) et l'« approche dynamique » (valeur actuelle
nette sur le cycle de vie).

**Sources, par ordre de solidité méthodologique :**

- **[Conseil d'analyse économique, *Focus* n° 072-2021, « Immigration et
  finances publiques »](https://cae-eco.fr/static/pdf/cae-focus072.pdf)**
  (Lionel Ragot, novembre 2021). La synthèse la plus citable : la contribution
  nette de la population immigrée aux finances publiques des pays développés
  est généralement comprise **entre ± 0,5 % du PIB**, portée pour l'essentiel
  par une structure par âge favorable (les immigrés sont, en moyenne, plus
  jeunes) qui compense une contribution individuelle nette plus faible aux
  âges actifs. L'effet est très sensible à la structure par qualification —
  d'où la recommandation, récurrente dans cette littérature, de politiques
  migratoires plus sélectives.
- **[France Stratégie, « L'impact de l'immigration sur le marché du travail,
  les finances publiques et la
  croissance »](https://www.strategie-plan.gouv.fr/files/files/Publications/2024/The%20impact%20of%20immigration%20on%20the%20labour%20market/fs-rapport-2019-immigration-juillet-2019_1.pdf)**
  (juillet 2019, republié en 2024). Revue de littérature commandée par le
  Comité d'évaluation et de contrôle des politiques publiques de l'Assemblée
  nationale : conclut à un impact **« ni de grande ampleur ni univoque »**, et
  souligne que les deux seules études disponibles sur le solde des finances
  publiques (OCDE, CEPII) convergent sous 0,5 % du PIB.
- **[Assemblée nationale, Comité d'évaluation et de contrôle, rapport
  « Évaluation des coûts et bénéfices de l'immigration en matière économique
  et sociale »](https://www2.assemblee-nationale.fr/content/download/231587/2274069/version/1/file/CEC+immigrationV2.pdf)**
  (Stéphanie Do et Pierre-Henri Dumont, janvier 2024) : l'actualisation
  parlementaire de 2024 mentionnée en demande. Fondée sur trente auditions
  d'experts et d'administrations, elle mobilise la contribution de France
  Stratégie ci-dessus et formule vingt-deux propositions.
- **OCDE, *International Migration Outlook*** (édition annuelle) : la
  contribution fiscale nette des immigrés dans les pays de l'OCDE est restée
  « constamment faible » sur la période 2006-2018, entre **-1 % et +1 % du
  PIB** pour la plupart des pays — un ordre de grandeur cohérent avec le CAE.

**Ce que ces sources NE sont pas : des jeux de données.** Ce sont des travaux
de recherche et d'évaluation parlementaire, cités avec leur lien, jamais
chargés en base — il n'y a rien à y charger, ce sont des synthèses, pas des
tableaux de séries.

## 7. Flux administratifs : titres de séjour (DGEF)

`core.titre_sejour_stock` : stock de titres et documents de séjour valides au
31 décembre, ressortissants de pays tiers hors Britanniques (suivis à part
depuis le Brexit), 2013-2023 :

| zone | 2013 | 2023 |
|---|---:|---:|
| France métropolitaine | 2 603 554 | 3 876 967 |
| DOM | 87 324 | 120 572 |
| COM | 6 217 | 6 179 |
| **Total** | **2 697 095** | **4 003 718** |

**+48,5 % en dix ans**, à comparer avec prudence à l'effectif immigré du § 1 :
un titre de séjour n'est pas une personne (une même personne détient parfois
plusieurs titres successifs dans l'année) et le champ exclut les personnes
naturalisées, les mineurs entrés avec leurs parents avant l'âge de la carte,
et les ressortissants de l'Union européenne (dispensés de titre de séjour).

**Le détail par motif (économique, familial, étudiant, humanitaire) n'est pas
chargé.** Le fichier CSV correspondant mélange plusieurs tableaux, des colonnes
d'années marquées « (provisoire) »/« (définitif) » et des notes de bas de page
dans le même fichier — une structure trop instable pour un connecteur fiable
sans reconstruction manuelle à chaque publication. Il reste consultable
directement sur
[data.gouv.fr](https://www.data.gouv.fr/fr/datasets/titres-de-sejour-publication-du-27-juin-2024/).
**La publication elle-même s'est arrêtée après juin 2024** dans le catalogue
consulté : aucune édition plus récente n'y figure.

---

## 8. L'évolution historique : un siècle de recensements

Toutes les sections précédentes portent sur un ou deux millésimes récents. Ce
que l'Insee publie de plus long, chargé dans
`core.population_historique_nationalite` — trente-deux recensements ou
estimations, 1921 à 2025 :

| année | immigrés (%) | étrangers (%) | Français par acquisition (milliers) |
|---|---:|---:|---:|
| 1921 | 3,7 | 3,9 | 254 |
| 1931 | 6,6 | 6,6 | 361 |
| 1946 | 5,0 | 4,4 | 853 |
| 1975 | 7,4 | 6,5 | 1 392 |
| 1999 | 7,3 | 5,5 | 2 376 |
| 2010 | 8,5 | 5,9 | 2 822 |
| 2020 | 10,2 | 7,6 | 3 057 |
| **2025 (p)** | **11,6** | **9,1** | **3 336** |

**Trois faits que le seul millésime 2023 ne montre pas :**

- **La part d'immigrés n'a jamais été stable dans le temps, et la hausse
  récente n'est pas sans précédent.** Elle double presque entre 1921 (3,7 %)
  et 1931 (6,6 %) — l'immigration de l'entre-deux-guerres, moins présente dans
  la mémoire collective que celle des Trente Glorieuses — puis reflue jusqu'à
  1946 (guerre, expulsions), avant de remonter pour se stabiliser autour de
  7,3-7,4 % de 1975 à 1999. **La croissance continue de 1999 à 2025 (7,3 % →
  11,6 %) est donc la plus longue de la série, mais son AMPLEUR sur vingt-cinq
  ans reste comparable à celle du seul début des années 1920.**
- **La part d'étrangers augmente moins vite que celle d'immigrés**, et
  l'écart entre les deux se creuse continûment depuis 1999 (1,8 point d'écart
  en 1999, 2,5 points en 2025) : c'est la trace directe des naturalisations
  (§ 8.1) — une part croissante des immigrés devient française sans cesser
  d'être immigrée.
- **Le nombre de Français par acquisition a été multiplié par treize depuis
  1921** (254 000 → 3 336 000), pas par un simple effet mécanique de la hausse
  de l'immigration : c'est un STOCK qui s'accumule tant que les personnes
  naturalisées restent en vie, contrairement au flux annuel du § 8.1.

*Réserves posées par la source elle-même : le champ change en 1990 (métropole
→ hors Mayotte) et 2014 (Mayotte incluse), et une rupture de série affecte
2024-2025 (protocole de collecte du recensement revu) — `cmd/verify` compare
chaque millésime à lui-même, pas à un lissage qui masquerait ces ruptures.*

### 8.1 Les flux, pas seulement le stock

`core.flux_migratoire` (Eurostat, France) donne, année par année, ce que le
tableau ci-dessus ne peut pas montrer : **combien de personnes entrent, et
combien acquièrent la nationalité, chaque année** — deux flux, pas des stocks :

| | 2010 | 2024 |
|---|---:|---:|
| Immigration (entrées) | 307 111 | 438 626 |
| Naturalisations | 143 261 | 103 661 |

**Les deux séries évoluent en sens contraire sur la période récente** :
l'immigration augmente (+43 % entre 2010 et 2024, avec un pic à 490 655 en
2022), les naturalisations reculent (−28 %). Un stock d'immigrés qui augmente
peut donc coexister avec un flux de naturalisations en baisse : ce sont deux
mécanismes indépendants, et aucun des deux ne se déduit de l'autre. Ce constat
est un fait démographique, pas une explication : cette note n'attribue le
recul des naturalisations à aucune cause précise, faute de données sur les
motifs des refus ou des non-demandes.

---

## 9. Ce qui est chargé

Migration `0071_immigration.sql`, connecteur `internal/immigration/`, commande
`go run ./cmd/ingest -only=immigration`.

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | Insee, recensement, `DS_RP_TD_IMMI_AGESEXEMPSTA_PRINC` + `DS_RP_TD_NAT_AGESEXEMPSTA_PRINC` (Melodi) | `core.population_statut_migratoire` | 333 lignes, France entière, 2023 |
| 2 | Insee, recensement, `DS_RP_TD_IMMI_AGESEXPCS_COMP` + `DS_RP_TD_NAT_AGESEXPCS_COMP` (Melodi) | `core.population_statut_migratoire_csp` | 162 lignes |
| 3 | Insee, recensement, `DS_RP_TD_IMMI_AGESEX_PAYSNAISS_R_PRINC` (Melodi) | `core.population_immigree_origine` | 180 lignes |
| 4 | Eurostat `migr_pop1ctz` + `migr_pop3ctb` | `core.eurostat_population_migratoire` | 123 lignes, France, 1999-2025 |
| 5 | DGEF/MIOM, stock de titres de séjour | `core.titre_sejour_stock` | 33 lignes, 2013-2023 |
| 6 | Insee, population immigrée et étrangère depuis 1921 | `core.population_historique_nationalite` | 32 millésimes, 1921-2025 |
| 7 | Eurostat `migr_imm1ctz` + `migr_acq` | `core.flux_migratoire` | 19 + 27 ans |

**Non chargé, et pourquoi :**

- Répartition des prestations sociales et des impôts par statut migratoire ou
  nationalité — aucune source administrative ouverte et répétable (§ 4).
- Détail des premiers titres de séjour par motif — fichier DGEF au format
  instable (§ 7).
- Travaux de recherche (CAE, France Stratégie, OCDE) — synthèses, pas des
  jeux de données (§ 6).
- Demandes d'asile (OFPRA) : identifiées, pas encore explorées pour leur
  format — un flux administratif distinct des titres de séjour et de
  l'immigration au sens du recensement, qui compléterait le § 8.1.

**Prolongement documenté, non réalisé** : les mêmes jeux Melodi publient la
population immigrée jusqu'au département et à l'EPCI (population ≥ 50 000
habitants pour les tableaux les plus fins) — le même mécanisme que
`internal/macro/menages_effectif.go` pour les ménages permettrait de
descendre à cette maille si un besoin géographique se précise.

## Sources

- Insee, *Immigrés et descendants d'immigrés en France*, Insee Références,
  édition 2023.
- Insee, *Entre 2006 et 2023, le nombre d'immigrés entrés en France augmente
  et leur niveau de diplôme s'améliore*, Insee Première n° 2051 (2024).
- Insee, recensement de la population, diffusion API Melodi (`api.insee.fr/melodi`),
  jeux de données cités au § 8.
- Eurostat, `migr_pop1ctz` (population par citoyenneté) et `migr_pop3ctb`
  (population par pays de naissance).
- Conseil d'analyse économique, *Focus* n° 072-2021, *Immigration et finances
  publiques* (Lionel Ragot, novembre 2021).
- France Stratégie, *L'impact de l'immigration sur le marché du travail, les
  finances publiques et la croissance* (juillet 2019).
- Assemblée nationale, Comité d'évaluation et de contrôle des politiques
  publiques, *Évaluation des coûts et bénéfices de l'immigration en matière
  économique et sociale* (janvier 2024).
- OCDE, *International Migration Outlook*, édition annuelle.
- Ministère de l'Intérieur, Direction générale des étrangers en France,
  *Titres de séjour, publication du 27 juin 2024*, data.gouv.fr.
- Insee, *Population immigrée et étrangère en France*, série 1921-2025.
- Eurostat, `migr_imm1ctz` (immigration par citoyenneté) et `migr_acq`
  (acquisitions de la nationalité par ancienne citoyenneté).
