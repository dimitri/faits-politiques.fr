# La santé : FINESS comme clé pivot, et comment les médecins sont payés

> Note de synthèse. Version 1 — 14 septembre 2026.
> Le système de santé français est le domaine le plus éclaté en sources que
> ce projet ait chargé : FINESS (établissements), SAE (activité), PMSI
> (séjours hospitaliers), SNDS/Open Damir (remboursements), RPPS
> (professionnels), HAS (qualité) répondent chacune à une question
> différente, sans identifiant commun garanti d'un jeu à l'autre — sauf
> FINESS, précisément pourquoi cette note commence par lui plutôt que par un
> sujet. **Ce premier chargement couvre le référentiel des établissements et
> la rémunération des médecins par secteur conventionnel — les autres
> sources restent à charger, explicitement, § 4.**

---

## 1. FINESS : le référentiel, sans aucune activité ni finances

`ref.finess_etablissement`, 103 022 établissements sanitaires et sociaux
(48 760 entités juridiques distinctes — une entité gère souvent plusieurs
sites). Source : ANS (Agence du numérique en santé), via data.gouv.fr.

**Réserve à poser dès l'ouverture** : le jeu de données utilisé
(`etalab_cs1100502`) est signalé comme remplacé par de nouveaux flux
quotidiens de l'ANS depuis le 20 juillet 2026, mais reste publié et à jour au
moment du chargement (édition du 12 mai 2026). Utilisé faute d'avoir confirmé
une URL stable pour le nouveau flux — à revoir dès qu'elle sera identifiée.

**Ce que la table ne dit pas, et qu'il ne faut pas lui faire dire** : le
champ de statut (`code_sph`/`libelle_sph`, censé distinguer public / privé
d'intérêt collectif / privé) n'est renseigné que pour **12 % des
établissements** (11 970 sur 103 022) — il ne concerne pleinement que les
établissements de santé au sens strict (hôpitaux, cliniques), pas le
médico-social (EHPAD, établissements pour personnes handicapées) qui
compose l'essentiel du fichier. En déduire une répartition public/privé
exhaustive serait une extrapolation non fondée.

| Catégorie (agrégat) | Établissements |
|---|---:|
| Commerce de biens à usage médical (pharmacies, etc.) | 21 035 |
| Établissements et services multi-clientèles | 12 998 |
| Établissements d'hébergement pour personnes âgées (EHPAD, etc.) | 9 894 |
| Établissements et services d'hébergement pour adultes handicapés | 5 305 |
| Laboratoires de biologie médicale | 4 562 |

**Ce que cette table permet pour la suite** : `nofinesset` devient la clé sur
laquelle toute source secondaire (SAE, PMSI, DAMIR, RPPS) pourra se joindre
sans dupliquer l'identification d'un établissement — le même rôle que
`ref.commune` pour le volet territorial. `code_insee` (département +
commune FINESS concaténés par ce connecteur, à ne pas confondre avec un code
INSEE publié tel quel par la source) prépare une jointure géographique
future.

## 2. Comment les médecins sont rémunérés : le secteur conventionnel

Source : Cnam (Assurance Maladie), `data.ameli.fr`, « Démographie secteurs
conventionnels » — 177 720 lignes, toutes professions de santé libérales
(pas seulement les médecins), 2010-2024.

**Ensemble des médecins, France entière, 2024** :

| Secteur | Effectif | Part |
|---|---:|---:|
| Secteur 1 (tarifs fixés par convention) | 76 910 | 68,6 % |
| Secteur 2 avec Optam/Optam-CO | 17 293 | 15,4 % |
| Secteur 2 sans Optam/Optam-CO | 17 029 | 15,2 % |
| Non conventionnés | 927 | 0,8 % |
| **Total** | **112 159** | |

**Ce que ces quatre catégories signifient concrètement** : en secteur 1, le
médecin applique le tarif fixé par la convention avec l'Assurance Maladie,
remboursé intégralement à ce tarif (hors participation forfaitaire). En
secteur 2, le médecin fixe librement ses honoraires ; l'Assurance Maladie ne
rembourse que sur la base du tarif conventionnel, jamais sur le dépassement.
L'Optam (option pratique tarifaire maîtrisée) est un engagement à modérer ce
dépassement, en échange d'un remboursement complémentaire plus favorable —
un médecin de secteur 2 adhérent à l'Optam n'est donc pas un troisième
secteur au sens strict, mais un sous-ensemble du secteur 2. Les non
conventionnés (0,8 %) fixent librement leurs honoraires et l'Assurance
Maladie rembourse sur une base forfaitaire très inférieure.

**Une rupture de série réelle, pas une lacune** : avant 2013, la donnée ne
distingue que trois catégories (secteur 1, secteur 2, non conventionné) —
l'Optam (sous son nom d'origine, le contrat d'accès aux soins) n'existe pas
avant cette date. `cmd/verify` l'accepte explicitement plutôt que de
signaler une anomalie à chaque millésime antérieur à 2013.

**Évolution 2010-2024** : l'effectif total de médecins recule légèrement
(118 133 → 112 159, soit −5,1 %) sur la période — un chiffre à mettre en
regard, sans le faire ici, de la démographie et des capacités de formation,
hors périmètre de cette note.

## 3. Ce que cette note ne fait pas encore

Elle ne dit pas combien un médecin gagne en euros — seulement dans quel
système de tarification il exerce. Le montant des dépassements et des
honoraires perçus existe dans un jeu de données distinct identifié
(`honoraires` sur `data.ameli.fr`), non chargé à ce stade : ses champs
« moyens » codent l'absence de donnée par la valeur littérale « NS » (non
significatif, secret statistique sur petit effectif) mêlée à des valeurs
numériques dans la même colonne — un traitement plus délicat que le
chargement fait ici, laissé à une prochaine itération plutôt que bâclé.

## 4. Ce qui est chargé, et ce qui reste identifié mais non chargé

| # | Source | Table | Volume |
|---|---|---|---|
| 1 | ANS, référentiel FINESS des établissements | `ref.finess_etablissement` | 103 022 lignes |
| 2 | Cnam, démographie par secteur conventionnel | `core.medecin_secteur_effectif` | 177 720 lignes, 2010-2024 |

**Identifié, non chargé — et pourquoi, sujet par sujet :**

- **SAE** (Statistique annuelle des établissements — lits, personnel,
  activité) : publiée sur `data.drees.solidarites-sante.gouv.fr`
  (`708_bases-statistiques-sae`), mais sous forme d'une quarantaine de
  fichiers `.7z` compressés (bases SAS/CSV par bordereau thématique), pas
  d'un jeu tabulaire directement interrogeable — la compression `.7z` n'est
  prise en charge par aucun outil de ce dépôt à ce jour.
- **PMSI** (activité hospitalière détaillée par séjour) : nécessite un accès
  spécifique (ATIH), non résolu à ce stade.
- **SNDS / Open Damir** (remboursements) : au-delà du secteur conventionnel
  chargé au § 2, les montants remboursés par pathologie ou par acte
  demandent le Système national des données de santé, à accès restreint pour
  le détail individuel — la version ouverte agrégée (Open Damir) reste à
  localiser précisément.
- **RPPS** (identification des professionnels, hors comptage global) :
  l'Annuaire Santé publie des extractions en libre accès
  (`annuaire.sante.fr`), identifiées mais pas encore explorées pour leur
  schéma exact.
- **HAS** (indicateurs qualité par établissement) : identifiée dans le
  catalogue data.gouv.fr de la Haute Autorité de Santé, pas encore chargée.
- **DECP** pour les fournisseurs des établissements publics de santé : même
  limite que pour Éducation et Défense — la table `core.public_contract`
  existe (`docs/perimetre.md` § 4.4, priorité P2) mais aucun connecteur ne
  l'alimente encore.
- **Honoraires et dépassements en euros** (§ 3) : jeu identifié, traitement
  du champ « NS » non encore fait proprement.

## Sources

- ANS (Agence du numérique en santé), *FINESS — extraction du fichier des
  établissements*, data.gouv.fr.
- Cnam (Caisse nationale de l'Assurance Maladie), *Démographie des
  professionnels de santé libéraux par secteur conventionnel*,
  data.ameli.fr.
