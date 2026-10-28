# Les candidats et la justice : décisions de condamnation

> **Dossier** · version 1 · 28 octobre 2026
>
> Les condamnations prononcées par une juridiction contre une personne qui a déclaré sa
> candidature à l'élection présidentielle de 2027 (la liste : `data/candidats.csv`), avec
> à chaque fois le stade de la procédure et la qualité de la source. Une liste
> **partielle**, qui dit exactement ce qu'elle couvre — et ce qu'elle ne couvre pas.

---

## Contexte

Ce dossier répond à une question simple : parmi les personnes qui se déclarent candidates,
lesquelles ont fait l'objet d'une condamnation par une juridiction, et où en est la
procédure ? Il ne juge personne. Il rapporte des décisions de justice, une par une, avec
leur date, la juridiction, ce que la source en dit, et — surtout — **à quel stade elles
en sont** : un jugement de première instance, une décision d'appel, un pourvoi en
cassation ou une condamnation devenue définitive ne disent pas la même chose, et la même
phrase peut être exacte pour l'un et trompeuse pour l'autre.

## Ce qui entre dans ce dossier, et ce qui n'y entre pas

**Entre** : une décision rendue par une juridiction, qui condamne une personne nommément,
quand cette personne figure dans la liste des candidats déclarés.

**N'entre pas** :

- les enquêtes, gardes à vue, mises en examen, plaintes, renvois devant un tribunal et
  procès en cours, tant qu'aucune décision n'a été rendue ;
- les décisions qui ne condamnent pas (relaxes, non-lieux, classements) — elles ne sont
  pas des condamnations, mais leur absence ici ne dit pas qu'elles n'existent pas : quand
  une relaxe fait partie de l'histoire d'une même affaire (une relaxe en appel
  qu'annule ensuite la Cour de cassation, par exemple), elle est rapportée dans la
  fiche de l'affaire ;
- les affaires qui visent un parti, une société ou un proche plutôt que la personne ;
- les propos de tiers, les commentaires et les réactions politiques.

## Comment lire un stade

| Stade | Ce que la source dit |
|---|---|
| Première instance | un tribunal a statué ; en principe, un recours est encore possible |
| Appel | une cour d'appel a statué ; un pourvoi en cassation est encore possible, ou en cours |
| Pourvoi | la Cour de cassation est saisie, ou vient de statuer |
| Définitive | la source dit que la condamnation est définitive |

Quand la source ne permet pas de savoir si une décision est devenue définitive, la fiche
le dit en toutes lettres plutôt que de choisir.

## Contrôles et évaluations

Les décisions, avec leur date, par candidat.

<!-- faits:CONTROLE:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

<!-- faits:CONTROLE:fin -->

## Situation

L'état d'avancement des procédures qui ne sont pas closes, à la date de la source.

<!-- faits:SITUATION:debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->

<!-- faits:SITUATION:fin -->

## Les sources, et ce que ce dossier ne sait pas faire

**Une décision de justice se lit de préférence dans la décision elle-même, ou dans le
communiqué de la juridiction qui l'a rendue.** C'est le standard de ce dossier. Il n'est
pas atteint aujourd'hui, pour deux raisons constatées le 28 octobre 2026 :

- les bases officielles de décisions (Judilibre, Légifrance) **pseudonymisent** : le nom du
  condamné y est remplacé (« M. [M] ») — on ne peut donc pas y retrouver nommément un
  candidat, ni citer la phrase qui le nomme ;
- les sites des juridictions étaient inaccessibles à l'automate de chargement : le
  portail Justice.fr, qui héberge les cours d'appel et les tribunaux, répondait par une
  page de maintenance ; Légifrance répondait 403 et le site de la Cour de cassation ne
  rendait aucun contenu.

**Les trois affaires de la première version reposent donc sur la presse** qui les rapporte
(qualité « presse » : citée, non archivée, jamais présentée comme officielle). Elles sont à
remplacer par le communiqué ou l'arrêt de la juridiction dès qu'il est lisible, et la
qualité de la fiche passera alors de « presse » à « officiel ».

Ce que ce dossier ne sait pas faire :

- **être exhaustif.** L'absence d'une ligne pour un candidat ne dit pas qu'il n'a jamais
  été condamné : elle dit que cette décision n'est pas, ou pas encore, recensée ici. La
  liste s'allonge par ajout, chaque ajout est une décision éditoriale relue comme telle ;
- **suivre une procédure en temps réel.** Chaque fiche porte la date de sa source. L'état
  d'une procédure au jour où l'on lit peut avoir changé : le pourvoi d'une affaire jugée
  en appel peut avoir été rejeté, ou ne pas avoir été formé ;
- **dire pourquoi.** Il rapporte ce qu'une juridiction a décidé, pas ce qu'il faut en
  penser.

Une erreur, une omission, une décision à ajouter ou un stade à mettre à jour : toute
correction est publiée avec sa date, et l'ancienne rédaction reste dans l'historique du
dépôt.

## Historique du dossier

- **Version 1** (28 octobre 2026) : première version, trois candidats (Marine Le Pen,
  Jean-Luc Mélenchon, Éric Zemmour), quatre décisions et un état de procédure, toutes
  rapportées par la presse faute d'une source officielle lisible.
