# Fontes

Hébergées en propre, et non chargées depuis un tiers : **aucune requête sortante,
aucun traceur, aucun décalage au chargement**. C'est une différence de nature avec
un CDN de fontes, pas de degré.

| Famille | Rôle | Licence |
|---|---|---|
| IBM Plex Sans | texte, données, interface | SIL Open Font License 1.1 |
| IBM Plex Mono | identifiants, empreintes, surtitres | SIL Open Font License 1.1 |
| Newsreader | titres | SIL Open Font License 1.1 |

Sous-jeux `latin` et `latin-ext` seulement — le français n'a besoin de rien d'autre.
Plex Sans et Newsreader sont variables (une seule ressource par sous-jeu couvre
toutes les graisses) ; Plex Mono ne l'est pas, d'où deux fichiers par sous-jeu.

Les trois familles sont sous OFL 1.1, redistribuable — cohérent avec l'AGPL-3.0 du
dépôt. Texte de la licence : <https://openfontlicense.org/>.

Sources amont : <https://github.com/IBM/plex> et
<https://github.com/productiontype/Newsreader>.
