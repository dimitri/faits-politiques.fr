# Pré-enregistrement 003 — Comment les groupes votent ensemble

> **Protocole de construction d'indicateur, écrit APRÈS une exploration des mêmes données.**
> Il fige la fenêtre, les seuils, la mesure et l'affichage de la page
> « Comment les groupes votent ensemble » (Qui décide). Il n'est donc
> pas un test confirmatoire : les chiffres de cette première version sont exploratoires, et
> seules les mises à jour faites ensuite, avec ces règles inchangées, le sont.
>
> | | |
> |---|---|
> | Version | 1 |
> | Rédigé le | 7 novembre 2026 |
> | Statut | **Exploratoire — écrit après un premier calcul** |
> | Méthode | `votes-groupes-v1` |
> | Premier calcul avant ce protocole | oui, le 7 novembre 2026, hors site (voir § 8) |

---

## 1. Objet

Mesurer, pour chaque paire de groupes de l'Assemblée nationale et du Parlement européen
(délégation française), à quel point leurs votes vont dans le même sens, avec une
incertitude et sans conclusion.

Ce protocole n'énonce aucune hypothèse. Il fige un indicateur pour qu'aucun écart ne puisse
être attribué au choix des scrutins, des seuils ou du codage (D-005).

## 2. Ce que la page ne produit pas

Aucune projection à deux axes, aucun classement de groupes, aucun libellé « gauche » ou
« droite » : dans un régime à groupes disciplinés, la première dimension d'une matrice de
votes sépare souvent la majorité de l'opposition, et un hémicycle ou un axe suggère une
lecture que ce site refuse de produire (protocole [002](pre-enregistrement-002.md) § 2,
charte graphique § 5). Les groupes sont rangés par l'arbre de classification, dont l'ordre
des feuilles est une convention de dessin.

## 3. Population

- **Scrutins** : tous les scrutins publics de l'Assemblée nationale et du Parlement européen
  depuis le **18 juillet 2024**, ouverture de la 17<sup>e</sup> législature (les groupes y sont constitués), quel qu'en
  soit l'objet (textes, articles, amendements, motions). Aucun scrutin n'est écarté à
  la main.
- **Groupes** : ceux que la source transcrit sur chaque bulletin (décision D-014), via
  `mv.scrutin_groupe_vote`. Deux identifiants du même groupe sont réunis (l'UDR, renommé
  en septembre 2025).
- **Sénat : hors périmètre.** Ses bulletins ne portent aucun groupe, et ce site ne le
  reconstitue pas à partir des appartenances (D-014). Une version qui l'inclurait devrait
  l'écrire dans un nouveau protocole.
- **Écartés, avec leur raison affichée** : les non-inscrits (ce n'est pas un groupe) et tout
  groupe qui n'a pas une position sur au moins 100 scrutins (ESN : un seul eurodéputé français).

## 4. Mesures

Pour un groupe *g* et un scrutin *s* : `pour`, `contre`, `abstentions` parmi ses membres,
rectifications comprises. **Les absents et les non-votants ne sont jamais une position.**

- **Position** : le groupe a une position si `pour + contre + abstentions ≥ 3`. Sa position
  majoritaire est la plus nombreuse des trois ; une égalité en tête n'est pas une position.
- **Score** : `s(g, s) = (pour − contre) ÷ (pour + contre + abstentions)`, de −1 à +1.
- **Corrélation** : le coefficient de Pearson entre les scores de deux groupes sur les scrutins
  où les deux ont une position, s'ils sont au moins 30.
- **Accord** : la part de ces scrutins où la position majoritaire est la même.
- **Arbre** : classification ascendante hiérarchique à lien moyen (UPGMA) sur `1 − r` ;
  une paire sans corrélation vaut la distance 1.

## 5. Incertitude

Intervalle à 95 % par rééchantillonnage avec remise des **jours de séance** (et non des
scrutins : les scrutins d'un même jour ne sont pas indépendants, et `perimetre.md` § 5.2 demande de corriger
cette redondance), 1 000 tirages, graine 20240718, bornes aux percentiles 2,5 % et 97,5 %.
La stabilité du voisin le plus proche est la part des tirages où il reste le même.

## 6. Rapprochement des partis et des groupes

Pour la comparaison d'une assemblée à l'autre, ce site rapproche un parti et le groupe où
ses élus siègent. Ce n'est pas une équivalence.

| Parti | Assemblée nationale | Parlement européen (élus français) |
|---|---|---|
| LFI | La France insoumise – Nouveau Front populaire | The Left |
| PS | Socialistes et apparentés | S&D |
| Écologistes | Écologiste et social | Greens/EFA |
| Renaissance | Ensemble pour la République | Renew |
| LR | Droite républicaine | EPP |
| RN | Rassemblement national | Patriots for Europe |

Un groupe européen réunit plusieurs partis français (Renew : Renaissance, MoDem, Horizons ;
S&D : PS et Place publique). C'est la délégation française qui est comparée, pas le groupe entier.
**Recoupé le 10 novembre 2026** avec les appartenances publiées par HowTheyVote (publication
du 7 novembre, fichiers `national_party_memberships` et `group_memberships`, 10ᵉ législature,
élus français) : les six correspondances ci-dessus se retrouvent, LFI→The Left (11 élus),
PS→S&D (10), Écologistes→Greens/EFA (6), Renaissance→Renew (6), LR→EPP (7), RN→Patriots for
Europe (32). Les autres élus français ne sont pas dans ce tableau : MoDem (4), Horizons (2) et
UDI (1) siègent à Renew, Place publique (3) à S&D, Identité-Libertés (5) et quelques divers à
l'ECR, Reconquête (1) à ESN. Un élu de l'UDR siège à l'EPP et un autre à Patriots for Europe, ce
qui laisse ce parti hors du tableau. Le contrôle est fait hors base : `core.ep_national_party_link`
reste vide (voir D-020), il n'est pas une donnée chargée du site.

## 7. Affichage

- Chaque valeur porte son intervalle et son nombre de scrutins ; une paire sous 30 scrutins
  n'a pas de chiffre, et le dit.
- Même indicateur, même méthode et même mise en avant pour tous les groupes (D-006).
- La matrice est un tableau : la valeur est écrite dans chaque case, la teinte ne porte que
  l'intensité (charte graphique : la couleur n'est jamais le seul porteur d'information).
- Les motions de censure sont montrées par le **nombre** de bulletins « pour » de chaque groupe,
  jamais par un oui ou un non : la procédure ne fait voter que les soutiens, et quelques
  députés d'un groupe ne sont pas la position du groupe.
- Les chiffres sont publiés avec leurs paramètres dans un fichier JSON à côté de la page.

## 8. Ce qui a été fait avant ce protocole

Le 7 novembre 2026, avant la rédaction de ce texte, les mêmes données ont été explorées hors
site : corrélations et accords par paire, classification, projection à deux axes, un sous-ensemble de
scrutins solennels, la dérive par semestre, et une version qui incluait le Sénat avec un groupe
reconstitué. Ce qui en a été retenu pour la page : la corrélation, l'accord, l'arbre et la
comparaison d'une assemblée à l'autre. Ce qui en a été écarté : la projection à deux axes (§ 2),
la dérive dans le temps (non conçue pour résister à un calendrier budgétaire) et le Sénat (§ 3).

Une première lecture des motions de censure avait attribué à un groupe des votes qui n'étaient
que ceux de quelques-uns de ses députés : elle a été corrigée, d'où l'affichage par nombre de bulletins (§ 7).

## 9. Hors protocole

Une évolution dans le temps, une analyse par thème de loi, le Sénat, et toute lecture
de ces chiffres comme un positionnement idéologique.

## Versions

- **v1 — 7 novembre 2026.** Première version, écrite après une exploration (§ 8).
- **v1.1 — 10 novembre 2026.** Le rapprochement du § 6 est recoupé avec HowTheyVote. Aucun
  changement de méthode ni de chiffre.
