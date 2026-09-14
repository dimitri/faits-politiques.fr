# La France est-elle un paradis fiscal ? — données

Migration 0082, paquet `internal/fiscalite`, `-only=fiscalite` (ou
`fiscalite-listes`, `-ocde`, `-ide`, `-fats`, `-twz`, `-filiales`, `-comptes`).
Contrôles : `cmd/verify/fiscalite.go`. Décision : D-059.

## 1. Il n'existe pas UNE définition, mais trois grilles officielles

| Grille | Critères | Ce qu'elle examine | Où c'est dans la base |
|---|---|---|---|
| OCDE, *Harmful Tax Competition* (1998), encadré I | (a) impôt nul ou symbolique — condition de départ ; (b) pas d'échange effectif d'informations ; (c) manque de transparence ; (d) aucune exigence d'activité substantielle. Encadré II : régimes préférentiels dommageables (taux effectif faible, cantonnement aux non-résidents, opacité, pas d'échange). | Tout pays, régime par régime | Taux : `core.fiscalite_pays` (`CIT.*`, `ETR.*`) ; régimes PI : `IPR.*` (statut du Forum sur les pratiques fiscales dommageables) |
| Conseil de l'UE, conclusions du 5 décembre 2017, annexe V | 1. transparence (échange automatique CRS, échange sur demande « largement conforme », convention multilatérale, bénéficiaires effectifs) ; 2. fiscalité équitable (pas de régime dommageable au sens du code de conduite de 1997, pas de structures offshore sans activité réelle) ; 3. normes minimales BEPS. | **Pays tiers seulement** : aucun État membre ne peut y figurer | `ref.juridiction_non_cooperative` liste `UE_ANNEXE_I`, 23 versions (déc. 2017 → fév. 2026) |
| France, art. 238-0 A du CGI (ETNC) | Refus d'échange d'informations (a et b du 2), inscription sur la liste UE (2 bis 1° et 2°) | Pays tiers | Même table, liste `ETNC_FR`, arrêtés 2010, 2016, 2020 → 2025 lus dans le corpus du JO |

S'y ajoute une mesure économique, sans valeur juridique mais la plus directe :
**où les multinationales déclarent leurs bénéfices au regard de leurs salariés et
de leur chiffre d'affaires** — déclarations pays par pays agrégées par l'OCDE
(`core.cbcr_agregat`, vue `derived.cbcr_juridiction`) et estimations
Tørsløv-Wier-Zucman (`core.transfert_benefices_estimation`).

Les indices du Tax Justice Network (Corporate Tax Haven Index, Financial Secrecy
Index) sont une quatrième grille, militante et documentée. Ils sont **cités, pas
chargés** (D-059).

## 2. Sources

| Source | Slug | Réutilisation | Contenu chargé |
|---|---|---|---|
| Commission européenne (PDF d'historique de la liste) | `ue-liste-juridictions-non-cooperatives` | ATTRIBUTION | annexe I de chaque version, transcrite et contrôlée contre le nombre annoncé |
| JORF (corpus déjà chargé) | `jorf` | — | arrêtés ETNC (tableaux HTML, motifs en rowspan) |
| OCDE Corporate Tax Statistics | `ocde-statistiques-impot-societes` | CC BY 4.0 | CbCR 2016-2023 (tous sièges × 35 juridictions dont `WXD` reste du monde et `STLS` apatrides) ; taux légaux 2000-2026 ; taux effectifs moyens et marginaux 2017-2025 ; régimes PI |
| OCDE FDI statistics (BMD4) | `ocde-investissements-directs` | CC BY 4.0 | revenus d'IDE de la France par pays de contrepartie immédiat, 2013-2024, entrants et sortants, en USD et en euros |
| Eurostat FATS | `eurostat-filiales-etrangeres` | CC BY 4.0 | entreprises sous contrôle étranger en France, par pays de contrôle ultime, 2008-2020 (`fats_g1b_08`) et 2021-2023 (`fats_ctrl`) |
| missingprofits.world | `missing-profits-twz-wz` | RESTRICTED | WZ2022 Table A (2015-2019), TWZ2022 Table 3 (2015) |
| GLEIF Golden Copy | `gleif-lei-niveau2` | CC0 | sociétés françaises (SIREN) déclarant une mère ultime étrangère |
| Ratios INPI/BCE | `inpi-bce-ratios-financiers` | Licence ouverte | CA, EBE, résultat courant avant impôt (reconstitué), résultat net |

## 3. Pièges

1. **La liste UE ne peut pas contenir la France**, ni l'Irlande, ni le Luxembourg,
   ni les Pays-Bas. « La France n'est sur aucune liste » est vrai et ne prouve rien
   à lui seul ; l'argument doit passer par les critères.
2. **Le document de la Commission omet la révision du 17 octobre 2023.** Les 23
   versions chargées sont celles qu'il publie. La liste ETNC n'est reconstituée que
   pour les arrêtés qui l'écrivent en entier (tableau) : 2011-2015 ne font qu'ajouter
   ou retirer des noms, non reconstitués.
3. **CbCR : les totaux ne s'additionnent pas entre sièges.** Chaque pays transmet
   les déclarations de SES groupes ; certains transmettent des données non
   consolidées (dividendes intragroupe comptés en bénéfice) ; l'effectif américain
   « en Chine » varie du simple au décuple d'une année à l'autre selon le
   déclarant. Seules les lignes d'UN siège sont comparées entre elles — les groupes
   américains, qui déclarent le plus régulièrement, servent de référence.
4. **CbCR : sous-groupes bénéficiaires + déficitaires ≠ total.** Écart jusqu'à
   150 Md$ sur le reste du monde des groupes américains en 2022 : panels et totaux
   sont compilés séparément. Le contrôle retenu est une inégalité (les bénéficiaires
   seuls déclarent au moins le total).
5. **CbCR : le rapport impôt / bénéfice n'est pas un taux effectif.** Tous groupes
   confondus, pertes comprises ; un « taux » négatif ou supérieur à 100 % signale des
   pertes, pas une anomalie (Luxembourg, Royaume-Uni certaines années).
6. **IDE : pays de contrepartie immédiat.** Un dividende versé par la filiale
   française d'un groupe américain à sa holding luxembourgeoise est compté vers le
   Luxembourg. La France déclare zéro revenu d'entités à vocation spéciale (SPE),
   ce qui la distingue des pays-relais (Pays-Bas, Luxembourg).
7. **FATS : rupture en 2021** (nomenclature et champ), 2018 absent de la série
   ancienne, secteur financier exclu. Les deux séries sont gardées côte à côte.
8. **GLEIF : « mère étrangère » ≠ « groupe étranger ».** Le pays est celui
   d'immatriculation de la société de tête : Stellantis et Airbus, dont les sociétés
   de tête sont néerlandaises, apparaissent comme groupes « NL » : leurs 18 sociétés
   françaises pèsent 113 Md€ des 401 Md€ de chiffre d'affaires repérés (derniers
   comptes). Toute agrégation GLEIF est donc présentée aussi hors ces deux groupes.
   Les relations sont déclaratives et incomplètes (parmi les GAFAM, seules Google
   France et Google Cloud France en déclarent une) : repérage, pas recensement.
   Seules les autorités d'enregistrement dont l'identifiant est le SIREN sont lues
   (RA000189 SIRENE, RA000192 RCS) ; RA000190 (fonds AMF) est écarté.
9. **Comptes : pas d'impôt sur les sociétés dans le jeu INPI/BCE.** Le résultat
   courant avant impôt est reconstitué (ratio publié à trois décimales × CA, erreur
   d'arrondi ≤ 0,0005 % du CA) ; l'écart au résultat net mêle impôt, résultat
   exceptionnel et participation. **Jamais présenté comme « l'impôt payé ».** Un
   écart très faible (Euro Disney Associés, exercice 2025 : 5 M€ pour 265 M€ de
   résultat courant) ou négatif (McDonald's France 2022 : 891 M€ de résultat net pour
   520 M€ de résultat courant) peut venir d'un exceptionnel, de déficits antérieurs imputés ou d'un
   produit d'impôt : le jeu ne permet pas de trancher.
10. **Comptes : la filiale n'est pas le chiffre d'affaires du groupe en France.**
    Les ventes aux clients français peuvent être facturées depuis une autre
    juridiction (Irlande notamment) ; la filiale française est alors rémunérée comme
    prestataire (marketing, support) avec une marge faible et stable. Le jeu ne permet
    de mesurer ni les ventes réelles en France, ni les redevances versées.
11. **Estimations TWZ : ce sont des estimations** (hypothèses sur la rentabilité
    « normale », la liste des paradis et la clé de répartition). Le classeur WZ2022
    répète une ligne « India » portant les valeurs de l'Afrique du Sud : la seconde
    occurrence est écartée.

## 4. Chiffres de référence (chargement du 14 septembre 2026)

- Taux légal combiné de l'IS, France : 36,13 % en 2025-2026 (contribution
  exceptionnelle comprise ; 25,83 % en 2022-2024). Taux effectif moyen (EATR,
  composite, OCDE) 2025 : France 23,6 % ; Allemagne 26,8 ; Pays-Bas 24,5 ;
  Royaume-Uni 22,5 ; Suisse 18,4 ; Irlande 12,4 ; Hongrie 12,0 ; Chypre 11,4 ;
  Bulgarie 9,1.
- Régime PI français : 10 %, statut OCDE « Not harmful (amended) ». Irlande 6,25,
  Luxembourg 4,99, Pays-Bas 7, Belgique 3,76, Malte 0.
- Groupes américains, 2023 : France 484 621 salariés (2,2 % de leurs effectifs hors
  États-Unis), 11,7 Md$ de bénéfice (1,4 %), 24 k$ par salarié, impôt dû / bénéfice
  33,9 %. Irlande : 206 268 salariés (0,9 %), 142,8 Md$ (17,3 %), 692 k$ par salarié,
  14,5 %. Bermudes : 987 salariés, 5,7 Md$, 3,2 %.
- Groupes français, 2023 : Singapour 207 k$ de bénéfice par salarié, Luxembourg
  202 k$, Irlande 131 k$ ; France 20 k$.
- TWZ / WZ : bénéfices transférés hors de France 32,1 Md$ (2015) → 42,6 Md$ (2019),
  soit 21,8 % de l'IS collecté perdu en 2019.
- Entreprises sous contrôle étranger en France, 2023 : 18 292 entreprises, 2,48 M de
  personnes (11,9 %), 239,5 Md€ de valeur ajoutée (15,4 %) ; contrôle américain
  483 671 personnes ; contrôle « offshore » 27 521.
- Dividendes versés par les filiales en France à leurs investisseurs directs
  étrangers, 2024 : 37,6 Md€ (Luxembourg 22,8 %, Pays-Bas 16,0 %, Suisse 12,5 %,
  Allemagne 12,1 %) ; reçus par les investisseurs français de leurs filiales à
  l'étranger : 111,4 Md€.
