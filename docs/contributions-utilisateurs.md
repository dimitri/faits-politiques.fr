# Contributions sans compte — note de conception

> **Méthode** · version 2 · 15 septembre 2026
>
> Décision : **il n'y a aucun compte utilisateur dans ce produit.**
> Ni pour consulter, ni pour créer, ni pour publier.

---

## 1. La ligne de partage

Ce n'est pas « compte ou pas compte », c'est **« donnée structurée ou prose »**.

Une carte de rattachement est de la donnée structurée sur deux vocabulaires fermés :
codes nuance du RNE d'un côté, identifiants CNCCFP de l'autre. Elle n'énonce rien sur
personne, elle n'est pas diffamable, elle n'a pas d'auteur au sens éditorial.

Un dossier est **un filtre** : une liste de faits réunis pour être relus facilement dans
un contexte donné, sans avoir à parcourir tout le corpus. Aucune rédaction, aucune
affirmation, aucun commentaire.

Les deux sont donc de même nature, et **les deux peuvent être anonymes**. Il n'y a nulle
part de prose sur des personnes nommées ; il n'y a donc ni responsabilité éditoriale de
tiers, ni régime d'hébergeur à assumer, ni profil d'opinion à protéger.

## 2. Ce que cela évite

Un compte qui stockerait « les partis que je considère équivalents » ou « les dossiers
que j'ai constitués sur tel parti » serait un **profil d'opinion politique** — donnée
sensible au sens de l'article 9 du RGPD. En supprimant les comptes, cette donnée n'existe
jamais, sous aucune forme, à aucun moment.

## 3. Le mécanisme : jeton de capacité

Créer une carte ou un filtre retourne deux URL : une publique et une secrète. Qui détient
la secrète peut modifier. Aucune identité, aucune session, et **seul le haché du jeton est
stocké** (`edit_token_hash`).

Jeton perdu, objet figé — sans gravité : une révision gelée reste citable pour toujours.

## 4. Non listé par défaut

C'est ce qui rend l'anonymat viable. N'importe qui crée et partage son URL ; **seule
l'équipe référence un objet dans un index**. Le vandalisme perd tout intérêt puisqu'il
n'apporte aucune audience, sans rien retirer à l'usage visé : un contradicteur publie sa
carte et en donne le lien dans une contestation.

Une seule exception verrouillée par le schéma : il existe **exactement une** carte de
référence, listée, tenue par l'équipe, sans jeton d'édition anonyme.

## 5. « Faire évoluer » sans casser la reproductibilité

Une lignée porte une **tête modifiable** et des **révisions gelées**. Un résultat publié
ne peut citer qu'une révision gelée — jamais la tête. Sinon un chiffre publié deviendrait
faux dès que quelqu'un modifie la carte sous ses pieds.

Un fork démarre une nouvelle lignée pointant vers la révision d'origine.

## 6. Le biais ne disparaît pas, il se déplace

Sans prose, **le filtre est l'argument**. Retenir 12 scrutins sur 900 est un acte
rhétorique même sans un mot de commentaire.

La garantie centrale n'est donc pas « toute affirmation est citée » mais **« toute
sélection déclare sa sélectivité »** :

- tout filtre déclare un **univers** (des critères), même quand la sélection est
  manuelle — sans lui, « 12 faits » ne veut rien dire ;
- la **sélectivité est calculée par le système**, jamais saisie par l'auteur, et un
  filtre ne peut être ni gelé ni listé sans elle ;
- le lecteur voit « 12 faits retenus sur 214 éligibles » sans que personne n'ait eu à
  l'écrire — y compris sur les filtres de l'équipe.

Deux modes, explicitement distingués parce qu'ils n'ont pas la même valeur
épistémique :

| Mode | Nature | Propriété |
|---|---|---|
| `CRITERIA` | Le filtre **est** sa définition | Reproductible, se met à jour seul quand de nouveaux faits arrivent |
| `MANUAL_SUBSET` | Choix explicite dans un univers déclaré | Figé, et affiché comme tel |

## 7. La symétrie devient structurelle

Un **gabarit** est un filtre paramétré par sujet. Une définition unique, instanciée pour
tous les groupes ou toutes les nuances : la symétrie cesse d'être une discipline à tenir
page par page pour devenir une propriété de construction. `selection.template_coverage`
rend visible tout sujet pour lequel une instance manque.

## 8. Le vocabulaire fermé de justification

Le seul champ par lequel de la prose pouvait entrer dans une carte était sa
justification. Il est remplacé par un **code** pris dans une liste fermée
(`ref.rationale_code`), avec une note libre facultative.

Trois bénéfices : la surface de texte libre s'effondre, deux cartes deviennent
**comparables ligne à ligne**, et une divergence entre deux cartes se lit d'un coup
d'œil au lieu de se discuter.

## 9. Ce qui reste à traiter hors schéma

- **Limitation de débit sans compte**, et sans journaliser d'adresse IP : compteur en
  mémoire, pas de stockage persistant.
- **Nommage** : « carte politique » est parlant mais ambigu en français — on pense à une
  carte géographique ou à un positionnement idéologique. « Table de rattachement » est
  exact mais moins engageant. Arbitrage produit.
- **Les contestations** restent le seul endroit où du texte libre arrive du public. Elles
  ne sont pas publiées automatiquement : l'équipe y répond, et la réponse est publique.

## Versions

- **Version 2** (15 septembre 2026) : en-tête commun des documents de méthode (perimetre.md § 2.8, D-066).
- **Version 1** (12 septembre 2026) : note de conception des contributions sans compte.
