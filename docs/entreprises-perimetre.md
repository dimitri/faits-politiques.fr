# Quelles entreprises, au juste ? Trois populations à ne pas confondre

> Note de méthode. Version 1 — 13 septembre 2026.
> Elle nomme correctement la liste de `core.entreprise`, et documente le piège
> qui consiste à l'appeler « le CAC 40 ».

---

## 1. Le piège, en une phrase

**Trois objets différents circulent sous le mot « les grandes entreprises »**, et aucun
n'est inclus dans les autres :

| | contenu | taille |
|---|---|---|
| **`core.entreprise`** | groupes déposant des **comptes consolidés en France**, avec un SIREN | 43 |
| **Le CAC 40** | les 40 plus fortes capitalisations d'Euronext Paris | 40 |
| **Le secteur S11** de la comptabilité nationale | **toutes** les sociétés non financières **résidentes** | des centaines de milliers |

Les confondre produit des affirmations fausses dans les deux sens : « Stellantis ne
verse pas de dividendes » (elle n'est pas dans la liste), ou « les entreprises ont versé
302 milliards » (c'est S11, pas le CAC 40).

## 2. Ce que `core.entreprise` contient réellement

Le critère de constitution est inscrit dans la colonne `critere` : *compte consolidé
(type_bilan = K) déposé au greffe*. Deux conséquences mécaniques.

**Elle exclut toute société non domiciliée en France.** Vérification par code ISIN, dont
le préfixe donne la domiciliation légale de l'émetteur — **cinq sociétés du CAC 40 sur
quarante** ne sont pas françaises :

| société | ISIN | domiciliation |
|---|---|---|
| Airbus SE | `NL0000235190` | Pays-Bas |
| ArcelorMittal | `LU1598757687` | Luxembourg |
| Euronext | `NL0006294274` | Pays-Bas |
| Stellantis NV | `NL00150001Q9` | Pays-Bas |
| STMicroelectronics | `NL0000226223` | Pays-Bas |

Deux corrections aux idées reçues au passage : **Renault est française**
(`FR0000131906`) — c'est Stellantis qui est aux Pays-Bas ; et **Eurofins est revenue**
à un ISIN français (`FR0014000MR3`) après des années au Luxembourg.

**Elle en rate d'autres pour des raisons de dépôt.** Au total, **17 des 40 sociétés du
CAC 40 sont absentes** de `core.entreprise` : les cinq ci-dessus, plus Hermès, Kering,
Carrefour, Pernod Ricard, Publicis, Legrand, Dassault Systèmes, Accor, Bureau Veritas et
Eurofins.

**Et elle contient des sociétés qui ne sont pas cotées du tout** : CMA CGM, Les
Mousquetaires, Groupe Adeo, Agache, Financière Agache, B.S.A, Dama, Bonneuil Expansion —
ainsi que des entreprises publiques, La Poste et la SNCF.

Une liste dont 43 % des membres du CAC 40 manquent et qui contient une dizaine de
non-cotées **n'est pas le CAC 40**, et ne doit jamais porter ce nom.

## 3. Le nom correct

> **Groupes déposant des comptes consolidés en France.**

Ni « grandes entreprises » — le critère n'est pas la taille — ni « CAC 40 » — le critère
n'est pas la cotation. C'est un critère **de dépôt légal**, et il faut le dire, parce
qu'il détermine exactement qui manque.

Migration à appliquer, écrite mais **non appliquée** : `core.entreprise` est en cours
d'écriture par une autre session, et un `ALTER TABLE ... RENAME` casserait son code en
vol. À déposer sous le prochain numéro libre, en coordination.

```sql
ALTER TABLE core.entreprise RENAME TO groupe_consolide;
ALTER TABLE core.entreprise_compte RENAME TO groupe_consolide_compte;

COMMENT ON TABLE core.groupe_consolide IS
  'Groupes déposant des comptes consolidés en France (type_bilan = K), identifiés par '
  'leur SIREN. CE N''EST PAS LE CAC 40 : 17 des 40 sociétés de l''indice en sont '
  'absentes, dont 5 parce qu''elles ne sont pas domiciliées en France (Airbus, '
  'ArcelorMittal, Euronext, Stellantis, STMicroelectronics). La liste contient par '
  'ailleurs des sociétés non cotées (CMA CGM, Les Mousquetaires, Adeo) et publiques '
  '(La Poste, SNCF). Toute page qui l''affiche doit nommer le critère de dépôt, faute '
  'de quoi un lecteur conclura qu''une société absente ne verse pas de dividendes.';
```

## 4. Pourquoi la comptabilité nationale ne souffre pas du même piège

Les séries `dividendes.verses.snf` et `ebe.snf` portent sur **S11**, et S11 n'est pas
une liste d'entreprises : c'est un **secteur institutionnel**. Il couvre toutes les
sociétés non financières résidentes, cotées ou non, françaises ou filiales de groupes
étrangers.

Ce qui change la nature des réserves à écrire :

- S11 **exclut les sociétés financières** — AXA, BNP Paribas, Crédit Agricole et Société
  Générale sont dans `dividendes.verses.sf` (57,7 Md€ en 2024), pas dans les 302 Md€ ;
- S11 **exclut les activités étrangères des groupes français** ;
- S11 **inclut les filiales françaises de groupes étrangers** : le dividende que la
  filiale française de Stellantis remonte à sa maison mère néerlandaise **est** dans les
  302 Md€, comme un flux vers le reste du monde.

Autrement dit, le piège de domiciliation frappe les agrégats construits à partir d'une
**liste** — le type « les entreprises du CAC 40 ont versé X » — et pas les agrégats
construits à partir d'un **secteur**.

## 5. La réserve à écrire sur toute page affichant les dividendes versés

Elle n'est pas optionnelle, parce que le chiffre est vrai et trompeur dans la même
phrase. Vérifié le 13 septembre 2026 sur Eurostat :

- les sociétés non financières résidentes **versent** 301 932 M€ de dividendes en 2024 ;
- elles en **reçoivent** 253 787 M€ — soit **84 % de ce qu'elles versent est encaissé
  par d'autres sociétés**. La série est non consolidée : elle additionne les remontées
  de filiales, les flux entre holdings, les participations croisées ;
- les **ménages** en reçoivent **68 201 M€**, le **reste du monde** 70 227 M€.

« Les entreprises ont versé 302 milliards de dividendes » est donc exact au sens
comptable et faux au sens que le lecteur lui donnera. La formulation défendable est :
**« flux brut de dividendes versés par les sociétés non financières, dont 84 % encaissés
par d'autres sociétés ; 68 Md€ parviennent aux ménages »**.

## 6. Ce qui reste à faire

1. Appliquer la migration du § 3, en coordination avec la session qui écrit sur cette table.
2. Aucune page ne doit afficher cette liste avant que son critère de dépôt y figure.
3. Si l'objet éditorial est bien « les grandes entreprises », alors `core.entreprise`
   n'est pas la bonne base : il faudrait partir de l'indice lui-même et assumer que cinq
   de ses membres n'ont pas de comptes français à montrer.
