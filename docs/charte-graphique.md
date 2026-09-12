# Charte graphique

> Revue des sites comparables, principes retenus, et palette. 11 septembre 2026.

---

## 1. Ce que font les sites de la même famille

Revue fondée sur une connaissance de ces sites, non sur un audit systématique —
c'est une limite à connaître avant d'en tirer des règles.

| Site | Ce qui marche | Ce qui ne marche pas |
|---|---|---|
| **GOV.UK** | La référence de la clarté de service public : noir sur blanc, un seul accent, échelle typographique franche, aucune décoration. On lit une page sans apprendre à s'en servir. | Austérité assumée : peu adapté à une navigation exploratoire. |
| **data.gouv.fr / DSFR** | Système d'État accessible (RGAA), contrastes tenus, composants cohérents. | Le bleu-blanc-rouge est **institutionnel** : sur un site politique indépendant, il suggérerait une caution officielle qu'on n'a pas. |
| **ProPublica**, **The Markup** | Typographie forte, couleur employée **uniquement pour encoder du sens**, monospace pour les identifiants. Les sources sont dans le flux, pas en note de bas de page. | Fortement éditorialisé : justement ce qu'on ne veut pas. |
| **Our World in Data** | Le graphique d'abord, la source toujours visible, l'incertitude affichée. Un chiffre y est toujours accompagné de son périmètre. | Densité qui suppose un lecteur motivé. |
| **Légifrance**, **INSEE** | Exhaustivité, permaliens stables. | Hiérarchie visuelle faible : tout a le même poids, donc rien n'en a. |
| **NosDéputés.fr**, **Datan** | Fonctionnels, données réelles, antériorité. | Couleur employée **décorativement**, ce qui brouille les couleurs qui portent du sens. Mise en page datée. |

## 2. Ce qu'on en retient

1. **La couleur n'encode que du sens.** Les quatre positions de vote — pour,
   contre, abstention, non-votant — sont les seules couleurs sémantiques du site.
   Aucune couleur décorative ne doit entrer en concurrence avec elles.
2. **Un seul accent**, pour les liens et les actions. Neutre partout ailleurs.
3. **Pas de bleu-blanc-rouge.** Ce site n'est pas officiel et ne doit pas en avoir
   l'air. Le piège est réel : un lecteur qui le prendrait pour une publication de
   l'État lui accorderait une autorité qu'il n'a pas.
4. **Aucune couleur de parti** dans l'identité. L'accent retenu est un pétrole
   sombre, associé à aucune formation française.
5. **La typographie fait la hiérarchie**, pas les cadres ni les ombres.
6. **Chiffres en chasse tabulaire**, identifiants et empreintes en monospace.
7. **Les sources sont dans le flux**, jamais derrière un clic.
8. **Contraste AA au minimum** : 4,5:1 pour le texte, 3:1 pour les éléments
   d'interface. Vérifié, pas supposé.
9. **La couleur n'est jamais le seul porteur d'information** : chaque position
   porte aussi son libellé écrit. Un lecteur daltonien lit la même chose.
10. **Pas d'imagerie décorative.** Au plus un motif de fond très discret, jamais
    derrière des données.

## 3. Palette

Rapports de contraste calculés sur le fond papier `#FBFAF8` (thème clair) et
`#15151A` (thème sombre).

### Base

| Rôle | Clair | Sombre | Contraste |
|---|---|---|---|
| Papier | `#FBFAF8` | `#15151A` | — |
| Encre | `#16161A` | `#ECEAE5` | 16,8:1 / 15,1:1 |
| Encre atténuée | `#5C5A55` | `#9C9890` | 6,4:1 / 6,1:1 |
| Filet | `#E2DED7` | `#2C2C33` | — |
| Carte | `#FFFFFF` | `#1C1C22` | — |

### Accent

| Rôle | Clair | Sombre | Contraste |
|---|---|---|---|
| Accent (liens) | `#175A6B` | `#66B8CC` | 6,5:1 / 8,2:1 |

Pétrole sombre. Choisi parce qu'il n'est la couleur d'aucun parti français,
qu'il se distingue nettement des quatre couleurs de position, et qu'il tient le
contraste dans les deux thèmes.

### Positions de vote — les seules couleurs sémantiques

| Position | Clair | Sombre | Contraste |
|---|---|---|---|
| Pour | `#1B6E3C` | `#4FBF80` | 5,6:1 / 8,4:1 |
| Contre | `#A32A2A` | `#E27272` | 6,1:1 / 7,3:1 |
| Abstention | `#8A6510` | `#D4A73C` | 5,2:1 / 9,1:1 |
| Non-votant | `#6E6A63` | `#8F8B83` | 5,1:1 / 5,4:1 |

Le non-votant est volontairement **désaturé et sans graisse** : ce n'est pas une
position politique mais une donnée manquante, et son traitement visuel doit le
dire avant même qu'on lise la légende.

## 4. Typographie

- **Texte** : pile système (`ui-sans-serif`, `system-ui`, `Segoe UI`, `Roboto`).
  Aucune police distante : pas de requête vers un tiers, pas de décalage au
  chargement, pas de traceur.
- **Chiffres** : `font-variant-numeric: tabular-nums` partout où des nombres
  s'alignent. Sans cela, une colonne de montants est illisible.
- **Identifiants, empreintes, codes** : monospace système.
- **Échelle** : 1 rem de base, titres à 1,65 / 1,2 / 1,02 rem. Trois niveaux
  suffisent ; au-delà la hiérarchie cesse d'être lisible.

## 5. Composants

- **Tableaux** : filets horizontaux seulement, en-têtes en petites capitales
  atténuées, alignement à droite pour les nombres, défilement horizontal isolé
  dans son conteneur — le corps de page ne défile jamais latéralement.
- **Cartes** : bord 1 px, rayon 10 px, aucune ombre. L'ombre suggère une
  profondeur qui n'apporte rien à un tableau de données.
- **Avertissements** (`ce que cette fiche ne dit pas`) : filet vertical, pas de
  fond coloré. Ils doivent se lire comme une précision, pas comme une alerte.
- **Absences** : filet vertical gris et texte atténué. Une donnée manquante
  s'affiche, elle ne se cache pas, mais elle ne crie pas non plus.

## 6. Identité

- **Marque** : trois barres verticales de hauteurs inégales, aux couleurs des
  positions, dans un carré à coins arrondis. Elle dit ce que fait le site —
  compter des votes — sans figurer d'hémicycle, dont la forme suggérerait un
  classement gauche-droite que ce site refuse de produire.
- **Motif de fond** : grille fine, 2 % d'opacité, réservée à l'en-tête de
  l'accueil. Évocation du papier millimétré et du tableau de données.
- Aucune photographie dans l'identité.

## 7. Accessibilité

- Cible RGAA / WCAG AA.
- Focus visible sur tous les éléments interactifs, jamais supprimé.
- Thème clair et sombre par tokens, tous deux définis explicitement.
- `prefers-reduced-motion` respecté — il n'y a de toute façon aucune animation.
- Images décoratives en `alt=""`, images porteuses de sens décrites.
