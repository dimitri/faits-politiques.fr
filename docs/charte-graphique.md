# Charte graphique

> Revue des sites comparables, principes retenus, et palette.
> Établie le 11 septembre 2026, refondue le 12 septembre 2026.
> Maquette de la refonte : [maquette-refonte.html](maquette-refonte.html).

---

## 1. Ce que font les sites de la même famille

Revue fondée sur une connaissance de ces sites et sur une inspection de leurs
pages, non sur un audit systématique — c'est une limite à connaître avant d'en
tirer des règles.

| Site | Ce qu'on lui prend | Ce qu'on ne lui prend pas |
|---|---|---|
| **GOV.UK** | La discipline de rédaction du *Content Design manual* : phrases courtes, mots ordinaires, un seul accent. | L'austérité, mal adaptée à une navigation exploratoire. |
| **Our World in Data** | Le **graphique est l'unité citable**, pas la page : source, téléchargement et « comment c'est mesuré » sont attachés à l'objet. | Une densité qui suppose un lecteur motivé. |
| **HowTheyVote.eu** | Le voisin le plus proche, et déjà une source de ce site : résultat, seuil, ventilation par groupe, grille des sièges, relevé filtrable et partage **en un écran**. | — |
| **TheyWorkForYou** | Les permaliens stables, et la page « comment nous calculons » liée depuis chaque chiffre. | Ses **résumés de vote** (« a généralement voté pour X »), qui agrègent des scrutins hétérogènes en une phrase d'opinion. |
| **ProPublica**, **The Markup** | La méthodologie en page de plein droit ; la couleur employée uniquement pour encoder ; le monospace pour les identifiants. | Une éditorialisation forte. |
| **Full Fact**, **Chequeado** | L'honnêteté sur ce qui **n'est pas** vérifiable ; le format court et citable, publié vite. | — |
| **Abgeordnetenwatch.de**, **GovTrack** | Le cycle de vie d'un texte rendu lisible ; le financement à côté du vote. | Les scores idéologiques de GovTrack. |
| **PolitiFact** | — | Le *Truth-O-Meter* : une échelle de verdicts qui transforme un travail de sourçage en notation. Contre-modèle fondateur. |
| **data.gouv.fr / DSFR** | Ses composants accessibles (RGAA). | Son identité : le bleu-blanc-rouge suggérerait une caution officielle qu'on n'a pas. |
| **Légifrance**, **INSEE** | L'exhaustivité, les permaliens. | Une hiérarchie visuelle faible : tout a le même poids, donc rien n'en a. |
| **NosDéputés.fr**, **Datan** | Fonctionnels, données réelles, antériorité. | La couleur employée décorativement, qui brouille les couleurs porteuses de sens. |
| **VoteWatch Europe** | — | Fermé en 2022. Un service de données meurt avec son financement : c'est l'argument du statique, de l'AGPL et de l'archive scellée. |

## 2. Ce qu'on en retient

1. **La couleur n'encode que du sens.** Les quatre positions de vote — pour,
   contre, abstention, non-votant — sont les seules couleurs sémantiques.
2. **Un résultat n'est pas une position.** « Adopté » et « rejeté » restent en
   **encre** : les colorer en vert et en rouge dirait qu'adopter est bien et
   rejeter est mal.
3. **Un seul accent**, pour les liens et les actions. Neutre partout ailleurs.
4. **Pas de bleu-blanc-rouge.** Ce site n'est pas officiel et ne doit pas en
   avoir l'air.
5. **Aucune couleur de parti** dans l'identité.
6. **La typographie fait la hiérarchie**, pas les cadres ni les ombres.
7. **Chiffres en chasse tabulaire**, identifiants et empreintes en monospace.
8. **Les sources sont dans le flux**, jamais derrière un clic.
9. **Contraste AA au minimum**, vérifié sur les **trois** surfaces — papier,
   papier creusé, carte — et dans les deux thèmes. Pas seulement sur le fond du
   corps, où l'en-tête et les cartes échappaient au contrôle.
10. **La couleur n'est jamais le seul porteur d'information** : chaque position
    porte aussi son libellé écrit **et une forme** — carré plein, carré ouvert,
    carré pointillé.
11. **Une barre dit une magnitude.** Une barre en pourcentage occupant toute la
    largeur fait lire 4 voix comme 72 : les barres de groupe sont à l'échelle
    absolue, la largeur disant l'effectif et le remplissage la répartition.
12. **Pas d'imagerie décorative.** Le seul motif de fond est la grille des
    sièges — la même géométrie que la marque —, sous 4 % d'opacité, confinée au
    hero, et **jamais derrière une donnée**.

## 3. Palette

Rapports de contraste calculés par la formule WCAG 2.1, en retenant le **pire**
des trois fonds de chaque thème. Minimum constaté : **4,86:1**.

### Base

| Rôle | Clair | Sombre | Emploi |
|---|---|---|---|
| Papier | `#FAF8F3` | `#131419` | fond du corps |
| Papier creusé | `#F1EEE6` | `#0E0F14` | bandes en retrait — évite de dessiner un cadre |
| Carte | `#FFFFFF` | `#1B1C22` | surfaces posées |
| Filet | `#E4E0D6` | `#2A2B33` | séparateurs — jamais du texte |
| Encre | `#17181D` | `#EEEBE4` | texte courant (14,27:1) |
| Encre atténuée | `#5A5850` | `#9C988F` | métadonnées (5,91:1) |

### Accent

| Rôle | Clair | Sombre | Contraste min. |
|---|---|---|---|
| Accent (liens, actions) | `#125863` | `#74C3D3` | 6,96:1 |

Pétrole sombre. Choisi parce qu'il n'est la couleur d'aucun parti français,
qu'il se distingue des quatre couleurs de position, et qu'il tient le contraste
dans les deux thèmes.

### Positions de vote — les seules couleurs sémantiques

| Position | Clair | Sombre | Contraste min. | Forme |
|---|---|---|---|---|
| Pour | `#1C6B45` | `#58C089` | 5,59:1 | carré plein |
| Contre | `#A32C2A` | `#E87D76` | 6,14:1 | carré plein |
| Abstention | `#7E5C0E` | `#DAAE45` | 5,29:1 | carré plein |
| Non-votant | `#6B675F` | `#918D85` | 4,86:1 | carré **ouvert** |
| Sans position enregistrée | `#E4E0D6` | `#2A2B33` | — | carré **pointillé** |

Le non-votant est volontairement **désaturé et sans graisse** : ce n'est pas une
position politique mais une donnée manquante, et son traitement visuel doit le
dire avant même qu'on lise la légende.

« Sans position enregistrée » est un **cinquième état**, distinct du non-votant
recensé. Sur une motion de censure, la procédure ne fait voter que ses
soutiens : les autres députés n'ont aucune ligne au relevé. C'est un fait sur la
donnée, pas une opinion prêtée à quiconque.

### Sur le vert et le rouge

Un site qui refuse tout verdict encode « pour » en vert et « contre » en rouge,
c'est-à-dire dans la paire la plus moralement chargée du répertoire occidental.
L'objection est réelle. La convention est **gardée** pour une raison qui prime :
le lecteur visé est pressé, et une convention connue lui coûte zéro
apprentissage. Trois garde-fous la rendent tenable — les teintes sont désaturées
vers l'encre plutôt que vers le feu tricolore ; chaque position porte son
libellé écrit et une forme distincte ; et **aucun tri, aucun classement du site
n'ordonne jamais par « pour »**.

Une variante non morale existe — sarcelle contre argile — et se substitue par
quatre jetons.

## 4. Typographie

Trois familles, **hébergées en propre** dans `web/fonts/` : aucune requête vers
un tiers, aucun traceur, aucun décalage au chargement. C'est une différence de
nature avec un CDN de fontes, pas de degré. Licences OFL, redistribuables —
cohérent avec l'AGPL-3.0 du dépôt.

- **IBM Plex Sans** — texte, données, interface. De vrais chiffres tabulaires,
  et une allure d'ingénierie plutôt que de campagne.
- **IBM Plex Mono** — identifiants, empreintes, surtitres, en-têtes de colonnes.
  Mêmes métriques que Plex Sans : un `VTANR5L17V1` ne fait pas sauter la ligne.
- **Newsreader** — titres. La voix éditoriale qui manquait, sans emphase.

Sous-jeux `latin` et `latin-ext` seulement. Plex Sans et Newsreader sont
variables ; Plex Mono ne l'est pas, d'où deux graisses par sous-jeu. Repli sur
la pile système, `font-display:swap`.

- **Chiffres** : `font-variant-numeric: tabular-nums` sur tout le corps.
- **Échelle** : 0,68 / 0,78 / 0,86 / 1 / 1,15 rem pour le texte, puis
  1,28 / 1,55 / 2,1 / 3,1 rem pour les titres. Au-delà, la hiérarchie cesse
  d'être perçue comme une hiérarchie.

## 5. Composants

- **Tableaux** : filets horizontaux seulement, en-têtes en petites capitales
  monospace, nombres à droite. Sous 52 rem, un tableau portant la classe `pile`
  **se replie en liste** — aucune colonne coupée, aucun défilement horizontal
  invisible.
- **Cartes** : bord 1 px, rayon 12 px, aucune ombre.
- **Bandeau de résultat** : la réponse d'abord. Quand une règle de seuil
  s'applique (`data/seuils.csv`), une **barre de seuil** porte la ligne de
  majorité requise. Sans elle, « 197 pour · 0 contre → rejeté » produit un
  contresens chez tout lecteur pressé.
- **Grille des sièges** : un carré par siège, groupé par groupe. L'ordre interne
  n'a pas de signification, et **il n'y a pas d'hémicycle** — sa forme
  suggérerait un axe gauche-droite que ce site refuse de produire.
- **Notes de procédure** (`.note.proc`) : filet d'accent, **toujours visibles**.
  Un fait sans lequel la page se lit de travers ne se replie pas.
- **Limites de portée** (`details.plus`) : repliables, ouvertes par défaut, à la
  même place sur toutes les pages, pour qu'on apprenne à les trouver.
- **Absences** (`.nodata`, `.note.abs`) : filet pointillé et texte atténué,
  toujours accompagnés de **leur raison typée**. Aucun état vide ne s'arrête
  avant le « parce que ».

## 6. Identité

- **Marque** : une gaufre de seize cases — 9 pour, 4 contre, 2 abstentions,
  1 non-votant. C'est une **forme de graphique réelle**, identique à la grille
  des sièges des pages de scrutin : la marque et la donnée sont le même objet.
  Pas d'hémicycle, pour la raison ci-dessus. Rendue en SVG inline
  (`Marque()`), pour que le mot soit du vrai texte dans la fonte du site.
- **Motif de fond** : la même grille, 3,5 % d'opacité, repliée vers la droite du
  hero où il n'y a pas de contenu.
- Aucune photographie dans l'identité.

## 7. Accessibilité

- Cible RGAA / WCAG AA, vérifiée par calcul et non supposée.
- **Focus visible sur tous les éléments interactifs, jamais supprimé.**
- Lien d'évitement vers le contenu en première position du document.
- Thème clair et sombre par jetons, tous deux définis explicitement, plus un
  forçage `[data-theme]`.
- `prefers-reduced-motion` respecté.
- La recherche est un `<dialog>` natif ; **ses déclencheurs restent masqués sans
  JavaScript** — on ne montre pas une commande morte. Tout le reste du site
  fonctionne sans JavaScript.
- Images décoratives en `alt=""`, motif de fond en `aria-hidden`.
