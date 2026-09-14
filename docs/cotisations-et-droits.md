# Ce qu'une cotisation achète : retraites, chômage et autres droits sociaux

> Note de méthode. Version 1 — 14 septembre 2026.
> Elle répond à une question simple en apparence : quand je cotise, qu'est-ce que
> j'obtiens, et qu'est-ce qui le garantit ? La réponse tient en deux distinctions
> que le débat public confond presque toujours.
>
> La dernière section sort du constat. Elle chiffre une hypothèse — un revenu
> universel articulé aux droits contributifs — et porte son étiquette.

---

## 1. Deux questions à ne jamais confondre

| question | les deux réponses possibles |
|---|---|
| **Comment le droit est-il financé ?** | **répartition** — les prélèvements de l'année paient les prestations de l'année ; **capitalisation** — les cotisations sont placées et servies plus tard |
| **Le droit dépend-il de ce que j'ai versé ?** | **contributif** — il s'ouvre par la cotisation et croît avec elle ; **non contributif** — il s'ouvre par la situation (résidence, âge, ressources, enfants) |

Les deux axes sont indépendants. Un droit peut être financé en répartition et
contributif (la retraite), en répartition et non contributif (le remboursement des
soins), capitalisé et contributif (le RAFP). Presque tout le débat sur « la
Sécu qui ne garantit rien » porte sur le premier axe, alors que presque toute la
difficulté d'une réforme porte sur le second.

## 2. Le financement : presque tout est en répartition

Les cotisations d'un mois paient les pensions, les indemnités et les allocations du
même mois. Rien n'est mis de côté au nom de celui qui cotise ; ce qu'il acquiert
est **un droit**, pas un capital.

Les exceptions sont rares et instructives :

- **AGIRC-ARRCO** (retraites complémentaires, régime unique depuis 2019) : en
  répartition, mais tenu par ses règles de pilotage de conserver des **réserves**
  d'au moins six mois de prestations. C'est un amortisseur, pas un compte
  individuel.
- **Le Fonds de réserve pour les retraites**, créé en 1999 pour absorber le choc
  démographique, dont les ressources servent **depuis 2011 à rembourser la dette
  sociale** via la CADES. La France a constitué un tampon de capitalisation, puis
  l'a affecté à autre chose.
- **Le RAFP** (retraite additionnelle de la fonction publique, 2005), assis sur les
  primes des fonctionnaires : le seul régime obligatoire **réellement capitalisé**.

## 3. Le droit : contributif ou non, branche par branche

| branche | le droit dépend-il des cotisations ? | ce qui l'ouvre |
|---|---|---|
| **Retraite de base** | **oui** | trimestres validés, salaire de référence |
| **Retraite complémentaire** | **oui** | points acquis ; leur valeur en euros est fixée chaque année par les partenaires sociaux |
| **Chômage** | **oui** | 6 mois travaillés sur 24 ; montant proportionnel au salaire antérieur |
| **Indemnités journalières** (maladie, maternité) | **oui** | durée d'activité, montant proportionnel au salaire |
| **Accidents du travail, invalidité** | **oui** | lien avec l'emploi, rente proportionnelle au salaire |
| **Remboursement des soins** | **non** | résidence stable et régulière — protection universelle maladie depuis 2016 |
| **Famille** | **non** | enfants à charge ; montants modulés selon les revenus depuis 2015 |
| **Autonomie** (APA, PCH) | **non** | perte d'autonomie, handicap |
| **RSA, logement, AAH, minimum vieillesse** | **non** | condition de ressources |

Toute la seconde moitié du tableau fonctionne **déjà** sans lien individuel entre ce
qu'on verse et ce qu'on reçoit.

## 3 bis. D'une fiche de paie aux caisses

Le tableau précédent, appliqué à un bulletin de paie. Le bulletin est **factice mais
calculé selon les règles en vigueur au 1er janvier 2026** : taux, plafond, réduction
générale dégressive unique. Chaque ligne est rangée selon ce qu'elle ouvre pour la
salariée, et chaque destinataire selon **le budget dont il relève**.

Le « salaire différé » désigne la part qui ouvre un droit **à son nom et
proportionnel à ce qui a été versé** : retraites, allocation chômage, indemnités
d'accident du travail, garantie des salaires. Le reste finance des droits qui ne
dépendent pas de ce salaire, ou n'ouvre aucun droit individuel.

### Qui reçoit : l'État, la Sécurité sociale, et d'autres budgets

« L'État » et « la Sécu » ne suffisent pas à décrire les destinataires d'un bulletin.
La loi distingue au moins six budgets :

| budget | voté ou arrêté par | sur ce bulletin |
|---|---|---|
| **État** | loi de finances | l'impôt sur le revenu prélevé à la source — **et rien d'autre** |
| **Sécurité sociale, dans le champ de la loi de financement (LFSS)** | loi de financement de la sécurité sociale | maladie, vieillesse de base, famille, accidents du travail, autonomie ; la CSG ; la CRDS, qui va à la Caisse d'amortissement de la dette sociale |
| **Régimes paritaires, hors LFSS** | conseils d'administration paritaires, accords interprofessionnels ; comptés en « administrations de sécurité sociale » par l'INSEE | assurance chômage (Unédic), retraite complémentaire (Agirc-Arrco) |
| **Opérateur de l'État** | son conseil d'administration, sous tutelle de l'État | formation professionnelle et apprentissage (France compétences) |
| **Fonds de l'État** | conseil de gestion sous l'autorité d'un ministre, abondé par le budget de l'État | aide au logement (Fnal) |
| **Organismes privés** | leur propre instance | garantie des salaires (AGS, association d'employeurs), dialogue social (fonds paritaire national) |

S'y ajoutent, hors de ce bulletin, **les collectivités** : le versement mobilité, dû à
partir de onze salariés là où une autorité organisatrice de la mobilité l'a institué,
va à son budget. Et le **solde de la taxe d'apprentissage** n'a pas de budget unique :
l'employeur choisit les établissements qui le reçoivent.

Le classement de chaque destinataire, son fondement et le texte qui arrête son budget
sont dans `ref.organisme_social` ; les taux, leur assiette et leur fondement dans
`ref.taux_cotisation` et `ref.parametre_social`. Le bulletin n'est stocké nulle part :
les vues `derived.bulletin_ligne`, `derived.bulletin_reduction`,
`derived.bulletin_synthese` et `derived.bulletin_flux` le recalculent, et
`cmd/verify` contrôle ses totaux au centime. La figure ci-dessous est régénérée par
`go run ./cmd/figure-bulletin`.

<!-- figure-bulletin:debut — généré par cmd/figure-bulletin, ne pas modifier à la main -->
<style>
.bp{--bp-differe:var(--encre,#17181D);--bp-solid:#A8A396;--bp-net:var(--filet,#E4E0D6);font-variant-numeric:tabular-nums}
@media(prefers-color-scheme:dark){:root:not([data-theme=light]) .bp{--bp-solid:#6F6B63}}
[data-theme=dark] .bp{--bp-solid:#6F6B63}
.bp-bulletin{border:1px solid var(--filet,#E4E0D6);background:var(--carte,#fff);margin:1rem 0;font-size:.84rem}
.bp-tete{display:grid;grid-template-columns:repeat(auto-fit,minmax(13rem,1fr));border-bottom:1px solid var(--encre,#17181D)}
.bp-tete>div{padding:.6rem .8rem}
.bp-tete p{margin:0}
.bp-lab{font:500 .66rem/1.3 var(--mono,monospace);letter-spacing:.06em;text-transform:uppercase;color:var(--attenue,#5A5850)}
.bp-defil{overflow-x:auto}
.bp table{border-collapse:collapse;width:100%;min-width:40rem}
.bp th,.bp td{padding:.25rem .55rem;border-bottom:1px solid var(--filet2,#EFEBE2);text-align:left;vertical-align:top}
.bp thead th{font:500 .64rem/1.25 var(--mono,monospace);letter-spacing:.04em;text-transform:uppercase;color:var(--attenue,#5A5850);vertical-align:bottom}
.bp .n{text-align:right;font-family:var(--mono,monospace);white-space:nowrap}
.bp tr.bp-rub th{font-weight:600;padding-top:.55rem;border-bottom:1px solid var(--filet,#E4E0D6)}
.bp tr.bp-tot td{font-weight:600;border-top:1px solid var(--encre,#17181D)}
.bp-pied{display:grid;grid-template-columns:repeat(auto-fit,minmax(16rem,1fr));border-top:1px solid var(--encre,#17181D)}
.bp-pied>div{display:flex;justify-content:space-between;gap:1rem;padding:.35rem .8rem;border-bottom:1px solid var(--filet2,#EFEBE2)}
.bp-pied b{font-family:var(--mono,monospace);font-weight:500;white-space:nowrap}
.bp-pied .bp-fort span,.bp-pied .bp-fort b{font-weight:600}
.bp svg{display:block;width:100%;height:auto}
.bp svg text{font-family:var(--sans,sans-serif);fill:var(--encre,#17181D)}
.bp .t-lbl{font-size:14px}.bp .t-fort{font-weight:600}.bp .t-num{font:500 13px var(--mono,monospace)}
.bp .t-pt{font-size:12.5px;fill:var(--attenue,#5A5850)}.bp .t-surt{font:500 10.5px var(--mono,monospace);letter-spacing:.06em;fill:var(--attenue,#5A5850)}
.bp .t-badge{font:500 11px var(--mono,monospace);fill:var(--encre,#17181D)}
.bp .r-badge{fill:none;stroke:var(--attenue,#5A5850);stroke-width:.8}
.bp .n-g{fill:var(--encre,#17181D)}.bp .n-SALAIRE{fill:var(--bp-net);stroke:var(--attenue,#5A5850);stroke-width:.6}
.bp .n-DIFFERE{fill:var(--bp-differe)}.bp .n-SOLIDARITE{fill:var(--bp-solid)}.bp .n-MIXTE{fill:url(#bp-demi)}
.bp .n-IMPOT{fill:url(#bp-hach);stroke:var(--attenue,#5A5850);stroke-width:.8}
.bp .p-fond{fill:var(--carte,#fff)}.bp .p-trait{stroke:var(--attenue,#5A5850);stroke-width:2}
.bp .f-SALAIRE{fill:var(--bp-net);opacity:.6}.bp .f-DIFFERE{fill:var(--bp-differe);opacity:.28}
.bp .f-SOLIDARITE,.bp .f-MIXTE{fill:var(--bp-solid);opacity:.45}.bp .f-IMPOT{fill:var(--attenue,#5A5850);opacity:.2}
.bp .s-budget{stroke:var(--papier,#FAF8F3);stroke-width:2}
.bp .s-b0{fill:var(--bp-net)}.bp .s-b1{fill:var(--encre,#17181D)}.bp .s-b2{fill:var(--attenue,#5A5850)}.bp .s-b3{fill:var(--bp-solid)}.bp .s-b4{fill:var(--filet,#E4E0D6)}
.bp-leg{display:flex;flex-wrap:wrap;gap:.3rem 1.2rem;font-size:.82rem;color:var(--attenue,#5A5850);margin:.4rem 0}
.bp-leg i{display:inline-block;width:.85rem;height:.85rem;vertical-align:-.1rem;margin-right:.35rem;border:1px solid var(--attenue,#5A5850)}
.bp-leg .l-DIFFERE{background:var(--bp-differe)}.bp-leg .l-SOLIDARITE{background:var(--bp-solid)}
.bp-leg .l-MIXTE{background:linear-gradient(135deg,var(--bp-differe) 50%,var(--bp-solid) 50%)}
.bp-leg .l-IMPOT{background:repeating-linear-gradient(45deg,var(--carte,#fff) 0 3px,var(--attenue,#5A5850) 3px 5px)}.bp-leg .l-SALAIRE{background:var(--bp-net)}
.bp .bp-budget{font:500 .7rem/1.2 var(--mono,monospace);white-space:nowrap;border:1px solid var(--attenue,#5A5850);padding:.05rem .35rem;display:inline-block}
.bp figcaption{font-size:.8rem;color:var(--attenue,#5A5850);margin-top:.4rem}
.bp ul.bp-budgets{list-style:none;padding:0;margin:.5rem 0 0;display:grid;grid-template-columns:repeat(auto-fit,minmax(19rem,1fr));gap:.15rem 1.5rem;font-size:.84rem}
.bp ul.bp-budgets li{display:grid;grid-template-columns:auto 1fr auto 3.6rem;gap:.5rem;align-items:center;border-bottom:1px solid var(--filet2,#EFEBE2);padding:.2rem 0}
.bp ul.bp-budgets svg{display:inline;width:12px}
.bp ul.bp-budgets b{font:500 .82rem var(--mono,monospace)}.bp ul.bp-budgets em{font-style:normal;text-align:right;color:var(--attenue,#5A5850);font-family:var(--mono,monospace)}
.bp table.bp-dest{min-width:58rem}
.bp table.bp-dest td:first-child{width:12rem}.bp table.bp-dest td:nth-child(2){width:15rem}.bp table.bp-dest td:last-child{min-width:20rem}
</style>
<div class="bp"><p><strong>Hypothèses.</strong> Technicienne d&#39;atelier non-cadre, CDI temps plein (151,67 h), 2 500 € brut, entreprise de 20 salariés de métropole hors Alsace-Moselle ; dispensée de la mutuelle d&#39;entreprise (couverte par celle de son conjoint) ; commune sans versement mobilité ; taux AT-MP notifié 1,50 % et taux d&#39;impôt personnalisé 2,6 % : hypothèses.</p><div class="bp-bulletin" role="region" aria-label="Bulletin de paie factice, janvier 2026"><div class="bp-tete"><div><p class="bp-lab">Employeur</p><p><strong>Atelier Durand SAS</strong> (factice)<br>20 salariés · métallurgie</p></div><div><p class="bp-lab">Salariée</p><p><strong>Camille Martin</strong> (factice)<br>technicienne d'atelier, non-cadre, CDI, 151,67 h</p></div><div><p class="bp-lab">Période</p><p><strong>Janvier 2026</strong><br>plafond mensuel : 4 005 €</p></div></div><div class="bp-defil"><table><thead><tr><th>Rubrique</th><th class="n">Base</th><th class="n">Taux salarié %</th><th class="n">Part salarié €</th><th class="n">Taux employeur %</th><th class="n">Part employeur €</th></tr></thead><tbody><tr class="bp-tot"><td>Salaire brut</td><td></td><td></td><td class="n">2 500,00</td><td></td><td></td></tr><tr class="bp-rub"><th colspan="6">Santé</th></tr><tr><td>Sécurité sociale – maladie, maternité, invalidité, décès</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">13,00</td><td class="n">325,00</td></tr><tr class="bp-rub"><th colspan="6">Accidents du travail – maladies professionnelles</th></tr><tr><td>Accidents du travail – maladies professionnelles</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">1,50</td><td class="n">37,50</td></tr><tr class="bp-rub"><th colspan="6">Retraite</th></tr><tr><td>Sécurité sociale – vieillesse plafonnée</td><td class="n">2 500,00</td><td class="n">6,90</td><td class="n">172,50</td><td class="n">8,55</td><td class="n">213,75</td></tr><tr><td>Sécurité sociale – vieillesse déplafonnée</td><td class="n">2 500,00</td><td class="n">0,40</td><td class="n">10,00</td><td class="n">2,11</td><td class="n">52,75</td></tr><tr><td>Retraite complémentaire Agirc-Arrco – tranche 1</td><td class="n">2 500,00</td><td class="n">3,15</td><td class="n">78,75</td><td class="n">4,72</td><td class="n">118,00</td></tr><tr><td>Contribution d&#39;équilibre général – tranche 1</td><td class="n">2 500,00</td><td class="n">0,86</td><td class="n">21,50</td><td class="n">1,29</td><td class="n">32,25</td></tr><tr class="bp-rub"><th colspan="6">Famille</th></tr><tr><td>Allocations familiales</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">5,25</td><td class="n">131,25</td></tr><tr class="bp-rub"><th colspan="6">Assurance chômage</th></tr><tr><td>Assurance chômage</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">4,00</td><td class="n">100,00</td></tr><tr><td>Garantie des salaires (AGS)</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">0,25</td><td class="n">6,25</td></tr><tr class="bp-rub"><th colspan="6">Autres contributions dues par l&#39;employeur</th></tr><tr><td>Contribution solidarité autonomie</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">0,30</td><td class="n">7,50</td></tr><tr><td>Fonds national d&#39;aide au logement</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">0,10</td><td class="n">2,50</td></tr><tr><td>Contribution à la formation professionnelle</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">1,00</td><td class="n">25,00</td></tr><tr><td>Taxe d&#39;apprentissage – part principale</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">0,59</td><td class="n">14,75</td></tr><tr><td>Taxe d&#39;apprentissage – solde</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">0,09</td><td class="n">2,25</td></tr><tr><td>Contribution au dialogue social</td><td class="n">2 500,00</td><td class="n"></td><td class="n"></td><td class="n">0,016</td><td class="n">0,40</td></tr><tr class="bp-rub"><th colspan="6">CSG et CRDS</th></tr><tr><td>CSG déductible de l&#39;impôt sur le revenu</td><td class="n">2 456,25</td><td class="n">6,80</td><td class="n">167,03</td><td class="n"></td><td class="n"></td></tr><tr><td>CSG non déductible de l&#39;impôt sur le revenu</td><td class="n">2 456,25</td><td class="n">2,40</td><td class="n">58,95</td><td class="n"></td><td class="n"></td></tr><tr><td>CRDS non déductible de l&#39;impôt sur le revenu</td><td class="n">2 456,25</td><td class="n">0,50</td><td class="n">12,28</td><td class="n"></td><td class="n"></td></tr><tr class="bp-rub"><th colspan="6">Exonérations et allègements de cotisations</th></tr><tr><td>Réduction générale dégressive unique (coefficient 0,1719)</td><td class="n">2 500,00</td><td></td><td></td><td></td><td class="n">−429,75</td></tr><tr class="bp-tot"><td>Total des cotisations et contributions</td><td></td><td></td><td class="n">521,01</td><td></td><td class="n">639,40</td></tr></tbody></table></div><div class="bp-pied"><div><span>Net à payer avant impôt sur le revenu</span><b>1 978,99 €</b></div><div><span>Net imposable</span><b>2 050,22 €</b></div><div><span>Impôt sur le revenu prélevé à la source (taux 2,6 %)</span><b>−53,31 €</b></div><div class="bp-fort"><span>Net payé</span><b>1 925,68 €</b></div><div><span>Montant net social</span><b>1 978,99 €</b></div><div class="bp-fort"><span>Coût total pour l'employeur</span><b>3 139,40 €</b></div></div></div><figure><p><strong>Les 3 139,40 € du coût employeur, par budget qui les reçoit</strong></p><svg viewBox="0 0 1000 36" role="img" aria-label="Répartition du coût employeur par budget destinataire"><rect class="s-budget s-b0" x="0.0" y="0" width="613.4" height="36"><title>Salariée (salaire net) : 1 925,68 €</title></rect><rect class="s-budget s-b1" x="613.4" y="0" width="17.0" height="36"><title>État : 53,31 €</title></rect><rect class="s-budget s-b2" x="630.4" y="0" width="276.5" height="36"><title>Sécurité sociale · LFSS : 867,90 €</title></rect><rect class="s-budget s-b3" x="906.8" y="0" width="77.2" height="36"><title>Sécurité sociale paritaire · hors LFSS : 242,44 €</title></rect><rect class="s-budget s-b4" x="984.1" y="0" width="12.7" height="36"><title>Opérateur de l&#39;État : 39,75 €</title></rect><rect class="s-budget s-b4" x="996.7" y="0" width="0.5" height="36"><title>Fonds de l&#39;État : 1,42 €</title></rect><rect class="s-budget s-b4" x="997.2" y="0" width="2.1" height="36"><title>Organisme privé : 6,65 €</title></rect><rect class="s-budget s-b4" x="999.3" y="0" width="0.7" height="36"><title>Affectation choisie par l&#39;employeur : 2,25 €</title></rect></svg><ul class="bp-budgets"><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b0" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Salariée (salaire net)</span><b>1 925,68 €</b><em>61,3 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b1" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>État</span><b>53,31 €</b><em>1,7 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b2" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Sécurité sociale · LFSS</span><b>867,90 €</b><em>27,6 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b3" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Sécurité sociale paritaire · hors LFSS</span><b>242,44 €</b><em>7,7 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b4" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Opérateur de l&#39;État</span><b>39,75 €</b><em>1,3 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b4" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Fonds de l&#39;État</span><b>1,42 €</b><em>0,0 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b4" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Organisme privé</span><b>6,65 €</b><em>0,2 %</em></li><li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="s-b4" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>Affectation choisie par l&#39;employeur</span><b>2,25 €</b><em>0,1 %</em></li></ul></figure><figure><svg viewBox="0 0 1000 1261" role="img" aria-label="Chemin de chaque euro du coût employeur jusqu'à son destinataire"><defs><pattern id="bp-hach" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="6" height="6" class="p-fond"/><line x1="0" y1="0" x2="0" y2="6" class="p-trait"/></pattern><pattern id="bp-demi" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="8" height="8" class="n-SOLIDARITE"/><rect width="4" height="8" class="n-DIFFERE"/></pattern></defs><rect class="n-g" x="134" y="52.0" width="16" height="605.2"/><text class="t-lbl t-fort" x="124" y="348.6" text-anchor="end">Salaire brut</text><text class="t-num" x="124" y="366.6" text-anchor="end">2 500,00 €</text><rect class="n-g" x="134" y="697.2" width="16" height="154.8"/><text class="t-lbl t-fort" x="124" y="766.6" text-anchor="end">Cotisations</text><text class="t-lbl t-fort" x="124" y="782.6" text-anchor="end">employeur</text><text class="t-num" x="124" y="800.6" text-anchor="end">639,40 €</text><text class="t-pt t-fort" x="134" y="40.0">Coût total pour l'employeur : 3 139,40 €</text><path class="f-SALAIRE" d="M150,52.00 C295.0,52.00 295.0,52.00 440,52.00 L440,518.18 C295.0,518.18 295.0,518.18 150,518.18 Z"/><path class="f-IMPOT" d="M150,518.18 C295.0,518.18 295.0,573.18 440,573.18 L440,627.88 C295.0,627.88 295.0,572.88 150,572.88 Z"/><path class="f-IMPOT" d="M150,572.88 C295.0,572.88 295.0,634.88 440,634.88 L440,647.79 C295.0,647.79 295.0,585.79 150,585.79 Z"/><path class="f-IMPOT" d="M150,697.21 C295.0,697.21 295.0,659.88 440,659.88 L440,669.51 C295.0,669.51 295.0,706.83 150,706.83 Z"/><path class="f-IMPOT" d="M150,585.79 C295.0,585.79 295.0,684.88 440,684.88 L440,687.86 C295.0,687.86 295.0,588.76 150,588.76 Z"/><path class="f-IMPOT" d="M150,706.83 C295.0,706.83 295.0,709.88 440,709.88 L440,710.43 C295.0,710.43 295.0,707.38 150,707.38 Z"/><path class="f-IMPOT" d="M150,707.38 C295.0,707.38 295.0,734.88 440,734.88 L440,734.98 C295.0,734.98 295.0,707.48 150,707.48 Z"/><path class="f-DIFFERE" d="M150,588.76 C295.0,588.76 295.0,807.88 440,807.88 L440,852.06 C295.0,852.06 295.0,632.94 150,632.94 Z"/><path class="f-DIFFERE" d="M150,707.48 C295.0,707.48 295.0,852.06 440,852.06 L440,888.72 C295.0,888.72 295.0,744.13 150,744.13 Z"/><path class="f-DIFFERE" d="M150,632.94 C295.0,632.94 295.0,895.72 440,895.72 L440,919.99 C295.0,919.99 295.0,657.21 150,657.21 Z"/><path class="f-DIFFERE" d="M150,744.13 C295.0,744.13 295.0,919.99 440,919.99 L440,940.66 C295.0,940.66 295.0,764.80 150,764.80 Z"/><path class="f-DIFFERE" d="M150,764.80 C295.0,764.80 295.0,947.66 440,947.66 L440,961.41 C295.0,961.41 295.0,778.55 150,778.55 Z"/><path class="f-DIFFERE" d="M150,778.55 C295.0,778.55 295.0,972.66 440,972.66 L440,980.45 C295.0,980.45 295.0,786.35 150,786.35 Z"/><path class="f-DIFFERE" d="M150,786.35 C295.0,786.35 295.0,997.66 440,997.66 L440,999.17 C295.0,999.17 295.0,787.86 150,787.86 Z"/><path class="f-MIXTE" d="M150,787.86 C295.0,787.86 295.0,1070.66 440,1070.66 L440,1115.36 C295.0,1115.36 295.0,832.57 150,832.57 Z"/><path class="f-SOLIDARITE" d="M150,832.57 C295.0,832.57 295.0,1170.36 440,1170.36 L440,1188.42 C295.0,1188.42 295.0,850.62 150,850.62 Z"/><path class="f-SOLIDARITE" d="M150,850.62 C295.0,850.62 295.0,1195.42 440,1195.42 L440,1196.45 C295.0,1196.45 295.0,851.66 150,851.66 Z"/><path class="f-SOLIDARITE" d="M150,851.66 C295.0,851.66 295.0,1220.42 440,1220.42 L440,1220.76 C295.0,1220.76 295.0,852.00 150,852.00 Z"/><text class="t-surt" x="440" y="45.0">SALAIRE VERSÉ</text><text class="t-surt" x="440" y="566.2">IMPÔTS ET TAXES, SANS DROIT INDIVIDUEL</text><text class="t-surt" x="440" y="800.9">SALAIRE DIFFÉRÉ : DROITS PROPORTIONNELS À CE QUI EST VERSÉ</text><text class="t-surt" x="440" y="1063.7">MIXTE</text><text class="t-surt" x="440" y="1163.4">SOLIDARITÉ : DROITS SANS LIEN AVEC CE SALAIRE</text><rect class="n-SALAIRE" x="440" y="52.0" width="16" height="466.2"/><text class="t-lbl t-fort" x="466" y="289.1">Salaire net versé</text><text class="t-num" x="746" y="289.1" text-anchor="end">1 925,68 €</text><rect class="n-IMPOT" x="440" y="573.2" width="16" height="54.7"/><text class="t-lbl t-fort" x="466" y="604.5">CSG</text><text class="t-num" x="746" y="604.5" text-anchor="end">225,98 €</text><rect class="r-badge" x="758" y="591.5" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="603.5">Sécurité sociale · LFSS</text><rect class="n-IMPOT" x="440" y="634.9" width="16" height="12.9"/><text class="t-lbl t-fort" x="466" y="645.9">Impôt sur le revenu</text><text class="t-num" x="746" y="645.9" text-anchor="end">53,31 €</text><rect class="r-badge" x="758" y="632.9" width="39" height="17" rx="2"/><text class="t-badge" x="764" y="644.9">État</text><rect class="n-IMPOT" x="440" y="659.9" width="16" height="9.6"/><text class="t-lbl t-fort" x="466" y="670.9">Formation, apprentissage</text><text class="t-num" x="746" y="670.9" text-anchor="end">39,75 €</text><rect class="r-badge" x="758" y="657.9" width="139" height="17" rx="2"/><text class="t-badge" x="764" y="669.9">Opérateur de l&#39;État</text><rect class="n-IMPOT" x="440" y="684.9" width="16" height="3.0"/><text class="t-lbl t-fort" x="466" y="695.9">CRDS</text><text class="t-num" x="746" y="695.9" text-anchor="end">12,28 €</text><rect class="r-badge" x="758" y="682.9" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="694.9">Sécurité sociale · LFSS</text><rect class="n-IMPOT" x="440" y="709.9" width="16" height="1.6"/><text class="t-lbl t-fort" x="466" y="720.9">Solde de la taxe d&#39;apprentissage</text><text class="t-num" x="746" y="720.9" text-anchor="end">2,25 €</text><rect class="r-badge" x="758" y="707.9" width="246" height="17" rx="2"/><text class="t-badge" x="764" y="719.9">Affectation choisie par l&#39;employeur</text><rect class="n-IMPOT" x="440" y="734.9" width="16" height="1.6"/><text class="t-lbl t-fort" x="466" y="745.9">Dialogue social</text><text class="t-num" x="746" y="745.9" text-anchor="end">0,40 €</text><rect class="r-badge" x="758" y="732.9" width="112" height="17" rx="2"/><text class="t-badge" x="764" y="744.9">Organisme privé</text><rect class="n-DIFFERE" x="440" y="807.9" width="16" height="80.8"/><text class="t-lbl t-fort" x="466" y="852.3">Retraite de base</text><text class="t-num" x="746" y="852.3" text-anchor="end">333,92 €</text><rect class="r-badge" x="758" y="839.3" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="851.3">Sécurité sociale · LFSS</text><rect class="n-DIFFERE" x="440" y="895.7" width="16" height="44.9"/><text class="t-lbl t-fort" x="466" y="922.2">Retraite complémentaire</text><text class="t-num" x="746" y="922.2" text-anchor="end">185,62 €</text><rect class="r-badge" x="758" y="909.2" width="267" height="17" rx="2"/><text class="t-badge" x="764" y="921.2">Sécurité sociale paritaire · hors LFSS</text><rect class="n-DIFFERE" x="440" y="947.7" width="16" height="13.8"/><text class="t-lbl t-fort" x="466" y="958.7">Assurance chômage</text><text class="t-num" x="746" y="958.7" text-anchor="end">56,82 €</text><rect class="r-badge" x="758" y="945.7" width="267" height="17" rx="2"/><text class="t-badge" x="764" y="957.7">Sécurité sociale paritaire · hors LFSS</text><rect class="n-DIFFERE" x="440" y="972.7" width="16" height="7.8"/><text class="t-lbl t-fort" x="466" y="983.7">Accidents du travail</text><text class="t-num" x="746" y="983.7" text-anchor="end">32,21 €</text><rect class="r-badge" x="758" y="970.7" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="982.7">Sécurité sociale · LFSS</text><rect class="n-DIFFERE" x="440" y="997.7" width="16" height="1.6"/><text class="t-lbl t-fort" x="466" y="1008.7">Garantie des salaires</text><text class="t-num" x="746" y="1008.7" text-anchor="end">6,25 €</text><rect class="r-badge" x="758" y="995.7" width="112" height="17" rx="2"/><text class="t-badge" x="764" y="1007.7">Organisme privé</text><rect class="n-MIXTE" x="440" y="1070.7" width="16" height="44.7"/><text class="t-lbl t-fort" x="466" y="1097.0">Assurance maladie</text><text class="t-num" x="746" y="1097.0" text-anchor="end">184,67 €</text><rect class="r-badge" x="758" y="1084.0" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="1096.0">Sécurité sociale · LFSS</text><rect class="n-SOLIDARITE" x="440" y="1170.4" width="16" height="18.1"/><text class="t-lbl t-fort" x="466" y="1183.4">Famille</text><text class="t-num" x="746" y="1183.4" text-anchor="end">74,58 €</text><rect class="r-badge" x="758" y="1170.4" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="1182.4">Sécurité sociale · LFSS</text><rect class="n-SOLIDARITE" x="440" y="1195.4" width="16" height="1.6"/><text class="t-lbl t-fort" x="466" y="1206.4">Autonomie</text><text class="t-num" x="746" y="1206.4" text-anchor="end">4,26 €</text><rect class="r-badge" x="758" y="1193.4" width="166" height="17" rx="2"/><text class="t-badge" x="764" y="1205.4">Sécurité sociale · LFSS</text><rect class="n-SOLIDARITE" x="440" y="1220.4" width="16" height="1.6"/><text class="t-lbl t-fort" x="466" y="1231.4">Aide au logement</text><text class="t-num" x="746" y="1231.4" text-anchor="end">1,42 €</text><rect class="r-badge" x="758" y="1218.4" width="112" height="17" rx="2"/><text class="t-badge" x="764" y="1230.4">Fonds de l&#39;État</text></svg><div class="bp-leg"><span><i class="l-SALAIRE"></i>salaire versé</span><span><i class="l-DIFFERE"></i>salaire différé</span><span><i class="l-MIXTE"></i>mixte</span><span><i class="l-SOLIDARITE"></i>solidarité</span><span><i class="l-IMPOT"></i>impôts et taxes</span><span>cadre : budget qui reçoit</span></div><figcaption>Épaisseurs proportionnelles aux montants versés après réduction générale. La part de la réduction imputée sur la retraite complémentaire suit la règle officielle (réduction × 6,01 % / coefficient maximal) ; sa répartition entre les autres caisses suit les taux, à titre d'illustration. Source : barème 2026 (ref.taux_cotisation), vues derived.bulletin_*.</figcaption></figure><div class="bp-defil"><table class="bp-dest"><thead><tr><th>Destinataire</th><th>Budget</th><th class="n">Versé €</th><th class="n">Réduction générale €</th><th>Ce que ce mois ouvre pour la salariée</th></tr></thead><tbody><tr class="bp-rub"><th colspan="5">Impôts et taxes, sans droit individuel</th></tr><tr><td><strong>CSG</strong><br>Caisses de sécurité sociale bénéficiaires de la CSG</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>loi de financement de la sécurité sociale</small></td><td class="n">225,98</td><td class="n">—</td><td>Aucun droit individuel : impôt réparti par la loi entre les caisses de sécurité sociale.</td></tr><tr><td><strong>Impôt sur le revenu</strong><br>Direction générale des finances publiques</td><td><span class="bp-budget">État · S1311</span><br><small>loi de finances (budget général de l&#39;État)</small></td><td class="n">53,31</td><td class="n">—</td><td>Aucun droit individuel : recette du budget de l&#39;État.</td></tr><tr><td><strong>Formation, apprentissage</strong><br>France compétences</td><td><span class="bp-budget">Opérateur de l&#39;État · S1311</span><br><small>budget propre voté par son conseil d&#39;administration, sous tutelle de l&#39;État</small></td><td class="n">39,75</td><td class="n">—</td><td>Finance l&#39;apprentissage et la formation ; le compte personnel de formation est crédité selon le temps travaillé, pas selon le montant versé.</td></tr><tr><td><strong>CRDS</strong><br>Caisse d&#39;amortissement de la dette sociale</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>budget propre ; son objectif d&#39;amortissement est voté en loi de financement de la sécurité sociale</small></td><td class="n">12,28</td><td class="n">—</td><td>Aucun : rembourse la dette sociale accumulée.</td></tr><tr><td><strong>Solde de la taxe d&#39;apprentissage</strong><br>Établissements de formation choisis par l&#39;employeur</td><td><span class="bp-budget">Affectation choisie par l&#39;employeur</span><br><small>aucun budget unique : lycées, universités, écoles, organismes habilités</small></td><td class="n">2,25</td><td class="n">—</td><td>Aucun droit individuel.</td></tr><tr><td><strong>Dialogue social</strong><br>Association de gestion du fonds paritaire national</td><td><span class="bp-budget">Organisme privé</span><br><small>budget de l&#39;association paritaire</small></td><td class="n">0,40</td><td class="n">—</td><td>Aucun droit individuel : finance les organisations syndicales et patronales.</td></tr><tr class="bp-rub"><th colspan="5">Salaire différé : droits proportionnels à ce qui est versé</th></tr><tr><td><strong>Retraite de base</strong><br>Caisse nationale d&#39;assurance vieillesse</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>loi de financement de la sécurité sociale</small></td><td class="n">333,92</td><td class="n">115,08</td><td>Valide un trimestre (150 heures au Smic, 1 803 € en 2026, quatre au plus par an) ; le salaire, jusqu&#39;au plafond, entre dans le calcul des 25 meilleures années.</td></tr><tr><td><strong>Retraite complémentaire</strong><br>Agirc-Arrco (retraite complémentaire)</td><td><span class="bp-budget">Sécurité sociale paritaire · hors LFSS · S1314</span><br><small>accords nationaux interprofessionnels et conseil d&#39;administration paritaire, hors loi de financement</small></td><td class="n">185,62</td><td class="n">64,88</td><td>7,68 points, soit 11,05 € de pension par an pour ce seul mois. Seule la cotisation au taux de calcul (6,20 %) achète des points ; le taux d&#39;appel et la CEG n&#39;en achètent aucun.</td></tr><tr><td><strong>Assurance chômage</strong><br>Unédic (assurance chômage)</td><td><span class="bp-budget">Sécurité sociale paritaire · hors LFSS · S1314</span><br><small>budget voté par son conseil d&#39;administration paritaire, hors loi de financement ; règles d&#39;indemnisation fixées par décret</small></td><td class="n">56,82</td><td class="n">43,18</td><td>Le mois compte pour ouvrir un droit (6 mois sur 24) et pour sa durée ; l&#39;allocation est proportionnelle au salaire.</td></tr><tr><td><strong>Accidents du travail</strong><br>Branche accidents du travail et maladies professionnelles</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>loi de financement de la sécurité sociale</small></td><td class="n">32,21</td><td class="n">5,29</td><td>Indemnités et rente proportionnelles au salaire en cas d&#39;accident du travail ou de maladie professionnelle.</td></tr><tr><td><strong>Garantie des salaires</strong><br>AGS (garantie des salaires)</td><td><span class="bp-budget">Organisme privé</span><br><small>budget de l&#39;association, taux fixé par son conseil d&#39;administration</small></td><td class="n">6,25</td><td class="n">—</td><td>Garantit le paiement des salaires si l&#39;entreprise fait l&#39;objet d&#39;une procédure collective.</td></tr><tr class="bp-rub"><th colspan="5">Mixte</th></tr><tr><td><strong>Assurance maladie</strong><br>Caisse nationale de l&#39;assurance maladie</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>loi de financement de la sécurité sociale</small></td><td class="n">184,67</td><td class="n">140,33</td><td>Les soins sont remboursés à tout résident, cotisant ou non ; les indemnités journalières (50 % du salaire, dans la limite de 1,4 Smic) dépendent de l&#39;activité.</td></tr><tr class="bp-rub"><th colspan="5">Solidarité : droits sans lien avec ce salaire</th></tr><tr><td><strong>Famille</strong><br>Caisse nationale des allocations familiales</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>loi de financement de la sécurité sociale</small></td><td class="n">74,58</td><td class="n">56,67</td><td>Aucun lien avec ce salaire : les prestations dépendent des enfants et des ressources du foyer.</td></tr><tr><td><strong>Autonomie</strong><br>Caisse nationale de solidarité pour l&#39;autonomie</td><td><span class="bp-budget">Sécurité sociale · LFSS · S1314</span><br><small>loi de financement de la sécurité sociale</small></td><td class="n">4,26</td><td class="n">3,24</td><td>Aucun lien : allocation personnalisée d&#39;autonomie et prestation de compensation du handicap selon la situation.</td></tr><tr><td><strong>Aide au logement</strong><br>Fonds national d&#39;aide au logement</td><td><span class="bp-budget">Fonds de l&#39;État</span><br><small>conseil de gestion placé sous l&#39;autorité du ministre chargé du logement ; gestion financière par la Caisse des dépôts ; abondé par le programme 109 du budget de l&#39;État</small></td><td class="n">1,42</td><td class="n">1,08</td><td>Aucun lien : aides personnelles au logement selon les ressources.</td></tr></tbody></table></div></div>
<!-- figure-bulletin:fin -->

### Ce qu'il faut en retenir

- **Un seul flux va à l'État** : l'impôt sur le revenu. La CSG est un impôt, mais
  affecté à la Sécurité sociale ; les contributions formation vont à un opérateur ;
  l'aide au logement passe par un fonds.
- **Même la retraite complémentaire n'est pas entièrement du salaire différé.**
  Seule la cotisation au taux de calcul (6,20 % de la tranche 1) achète des points ;
  le taux d'appel (127 %) et la contribution d'équilibre général financent
  l'équilibre du régime sans ouvrir de droit supplémentaire.
- **La réduction générale ne retire aucun droit.** Les cotisations non versées par
  l'employeur sont compensées aux caisses ; le trimestre est validé et les points
  sont calculés sur le salaire, pas sur la cotisation réellement payée.
- **L'assurance maladie est mixte** : les soins sont remboursés à tout résident, les
  indemnités journalières dépendent du salaire. Aucune source ne sépare la cotisation
  entre les deux ; elle n'est donc pas découpée.

### Sources et limites

- Réduction générale dégressive unique : [service-public.fr, fiche F24542](https://entreprendre.service-public.gouv.fr/vosdroits/F24542)
  (formule, Tmin, Tdelta, P, Smic annuel, imputation sur la retraite complémentaire,
  limite AT-MP) ; décret n° 2025-1446 du 31 décembre 2025.
- Plafond de la sécurité sociale 2026 : [service-public.fr, A15386](https://entreprendre.service-public.gouv.fr/actualites/A15386) ;
  cotisation AGS : [A17906](https://entreprendre.service-public.gouv.fr/actualites/A17906).
- Périmètre des administrations de sécurité sociale (régimes complémentaires,
  indemnisation du chômage, CADES) : [Insee, Administrations publiques en 2025](https://www.insee.fr/fr/statistiques/8988833).
- France compétences, organisme divers d'administration centrale : arrêté du
  29 août 2023 fixant la liste des ODAC, lu dans le corpus du Journal officiel.
- Le barème de l'URSSAF et Légifrance refusent l'accès automatisé : ils sont cités
  (fondement de chaque taux), pas archivés.
- Limites : personnes, entreprise et identifiants factices ; taux accidents du travail
  et taux d'impôt propres à l'employeur et au foyer, pris comme hypothèses ;
  salariée dispensée de mutuelle, ce qui retire la complémentaire santé et le forfait
  social ; la répartition de la réduction générale entre caisses de l'URSSAF suit les
  taux, faute de clé publiée ; la répartition de la CSG entre caisses n'est pas
  détaillée.

## 4. Ce que pèse chaque nature de droit

Classement des prestations de protection sociale 2024 (DREES, comptes de la
protection sociale), à partir de la nomenclature de niveau 3. Les 37 postes
partitionnent exactement le total de **932,5 Md€**.

| risque | contributif | non contributif | mixte ou non classé | total |
|---|---:|---:|---:|---:|
| Vieillesse-survie | **402,9** | 16,2 | 7,5 | 426,7 |
| Santé | 41,7 | **297,2** | — | 338,9 |
| Famille | 4,5 | **61,3** | — | 65,8 |
| Emploi | **37,5** | 0,6 | 13,0 | 51,1 |
| Pauvreté-exclusion | — | **34,0** | — | 34,0 |
| Logement | — | **16,1** | — | 16,1 |
| **Total** | **486,7** | **425,4** | **20,5** | **932,5** |

**La protection sociale française est contributive pour un peu plus de moitié
(52 %), et non contributive pour un peu moins (46 %).** La part contributive est
presque entièrement la vieillesse ; la part non contributive, presque entièrement
les soins.

Postes classés contributifs : pensions de droit direct et dérivé, indemnités de
départ, allocation chômage, remplacement de revenu temporaire (indemnités
journalières), pensions et rentes d'invalidité, remplacement de revenu définitif
(accidents du travail), prestations liées à la maternité. Postes classés « mixte
ou non classé » : autres prestations chômage (où l'allocation de solidarité
spécifique, non contributive, voisine d'autres dispositifs), formation et insertion
professionnelles, autres prestations vieillesse et survie. Tout le reste est non
contributif. **Ce classement est une décision de lecture** : il suit la nature du
droit, pas l'organisme payeur, et il se discute poste par poste.

## 5. « Cela ouvre des droits, mais leur financement n'est pas garanti »

C'est exact, avec une distinction juridique qui change tout :

- **une pension déjà liquidée** est très solidement protégée ;
- **un droit en cours d'acquisition** ne l'est pas contre la loi. Le législateur peut
  modifier l'âge, la durée d'assurance, l'indexation, la valeur du point — et il l'a
  fait en 1993, 2003, 2010, 2014 et 2023, avant de suspendre en 2026 le calendrier de
  la dernière réforme.

Cotiser garantit donc un droit **selon les règles en vigueur le jour du départ**,
pas selon celles du jour où l'on a cotisé.

Pour être complet sans prendre parti : **la capitalisation ne garantit pas
davantage.** Elle échange un risque politique et démographique — la loi change, les
cotisants se raréfient — contre un risque financier : les marchés, l'inflation, la
défaillance du gestionnaire. Aucun des deux modèles ne met une cotisation à l'abri
du temps.

## 6. Un lien déjà desserré : la cotisation chômage

Depuis le **1er octobre 2018**, les salariés ne paient plus de cotisation
d'assurance chômage. Elle a été remplacée par une hausse de **1,7 point de CSG** au
1er janvier 2018 — un impôt payé par tous, retraités compris.

Les salariés conservent pourtant un droit **proportionnel à leur salaire**. Le
principe « je cotise, donc j'ai droit » n'est donc plus littéralement vrai pour
l'assurance chômage : le droit reste contributif, son financement ne l'est plus
côté salarié. C'est un fait à connaître avant tout débat sur la « fin de la logique
assurantielle » : elle a déjà été entamée, sans que le mot soit prononcé.

---

## 7. Pour aller plus loin — hors constat : un socle universel articulé aux droits contributifs

> **Cette section chiffre une hypothèse de politique publique.** Elle n'est pas un
> constat et ne relève pas du périmètre factuel du projet. Elle est rédigée pour
> répondre à une question précise et laisse le jugement au lecteur.

### 7.1 Le problème à résoudre

Un revenu universel **au niveau du seuil de pauvreté** — **1 288 € par mois** pour
une personne seule en 2023 selon l'INSEE — versé aux **56,3 millions** de résidents
de 16 ans ou plus coûterait **870 Md€ bruts**, soit **66 % de l'ensemble des impôts
et cotisations** prélevés en 2024 (1 319 Md€).

Le financer uniquement par les prestations **non contributives** ne suffit pas, et
de loin. La question devient : peut-on y faire contribuer les droits
**contributifs** sans rompre la promesse individuelle qu'ils portent ?

### 7.2 Trois architectures

Soit **S** le socle universel et **P** le droit contributif calculé comme
aujourd'hui (pension, allocation chômage).

| architecture | ce que reçoit la personne | avantage | défaut |
|---|---|---|---|
| **Différentielle** | **max(S, P)** | personne ne perd ; le socle absorbe la première tranche de chaque droit, ce qui le finance en partie | **les premières cotisations deviennent inutiles** : celui dont le droit est inférieur à S reçoit exactement ce que reçoit celui qui n'a jamais cotisé |
| **Additive** | **S + P** | tout euro cotisé compte, toujours | la plus coûteuse : le socle ne se finance sur aucun droit existant |
| **Dégressive** | **S + α·P**, avec 0 < α < 1 | tout euro cotisé compte encore ; le socle est financé par la part (1 − α) de chaque droit | les droits élevés baissent ; le choix de α est entièrement politique |

**Le défaut de l'architecture différentielle est central** et mérite d'être nommé :
c'est un **effet de seuil sur la contributivité**. Il frappe précisément les
carrières courtes, hachées, à bas salaire — majoritairement féminines : la pension
moyenne des femmes est de **1 306 € bruts**, sous le socle envisagé. Une réforme
qui rendrait leurs cotisations sans effet sur leur pension serait perçue, à juste
titre, comme une spoliation.

**L'architecture dégressive est celle qui répond à la question posée** — un socle
au-dessus du seuil de pauvreté **et** une part individuelle liée aux cotisations.
Les régimes de retraite qui combinent une pension de base forfaitaire de résidence
et un étage professionnel proportionnel en sont des variantes.

### 7.3 Ordre de grandeur, hypothèse la plus favorable

> **Ce chiffrage est dépassé par une version plus rigoureuse.** Il utilise un montant forfaitaire **par
> personne**, alors que le seuil de pauvreté se calcule **par unité de
> consommation d'un ménage** — un forfait par personne surpaie systématiquement
> les ménages de plusieurs adultes et laisse les moins de 16 ans sans socle
> propre. [docs/revenu-universel-microsimulation.md](revenu-universel-microsimulation.md)
> refait ce calcul par unité de consommation (≈ 698 Md€ bruts, contre 870 ici)
> et le complète d'une micro-simulation par type de ménage, financée par une
> reprise fiscale sur le socle plutôt que par les bornes « au plus » ci-dessous
> (coût net obtenu : ≈ 25 Md€/an, pas 382). Le tableau qui suit reste ici comme
> trace du premier chiffrage, pas comme référence.

| poste | Md€ |
|---|---:|
| Socle à 1 288 €, 56,3 M de personnes | **870** |
| − prestations monétaires non contributives devenues redondantes | − 96 |
| − dépenses fiscales rendues redondantes (PLF 2026) | − 88 |
| − première tranche des pensions de droit direct, **au plus** | − 266 |
| − première tranche de l'allocation chômage, **au plus** | − 38 |
| **Reste à financer, au mieux** | **≈ 382** |

Soit **29 % des prélèvements obligatoires actuels**, dans l'hypothèse la plus
favorable. Lecture des trois lignes qui portent le calcul :

- **96 Md€ de prestations substituables** : logement, RSA, prime d'activité, autres
  prestations pauvreté, allocations familiales et assimilées, AAH, minimum
  vieillesse. **Ne sont pas comptés** les services en nature (soins, aide sociale à
  l'enfance, hébergement, accueil des jeunes enfants), qui ne disparaissent pas
  parce qu'un revenu est versé.
- **266 Md€ d'absorption des pensions** est un **plafond** : il suppose que chacun des
  17,2 millions de retraités de droit direct touche au moins 1 288 €. La pension
  moyenne est de 1 666 € bruts, mais une part importante des retraités — dont une
  majorité de femmes — est en dessous. **Le chiffre réel exige la distribution des
  pensions par décile**, publiée par la DREES, qui n'est pas chargée.
- **38 Md€ pour le chômage** est l'enveloppe entière de l'allocation, donc aussi un
  plafond.

### 7.4 Ce que ce tableau ne dit pas, et qui compte plus que lui

**Le coût brut n'est pas le coût net.** Un socle versé à un salarié qui gagne
correctement sa vie est récupéré par l'impôt sur le revenu : c'est l'équivalence
connue entre un revenu universel assorti d'un impôt proportionnel et un crédit
d'impôt dégressif. Le « reste à financer » ci-dessus est donc **le montant à
récupérer**, pas nécessairement un montant de recettes nouvelles. Tout chiffrage
sérieux passe par une microsimulation qui applique à la fois le socle et le barème
fiscal réformé aux revenus réels des ménages — ce que ce document ne fait pas.

**Le seuil de pauvreté est calculé par unité de consommation.** Un ménage de deux
adultes n'a pas besoin de deux fois le revenu d'une personne seule pour atteindre
le même niveau de vie. Un socle strictement individuel au niveau du seuil place
donc les couples **au-dessus** de ce seuil : c'est un choix de conception, pas un
détail d'arrondi.

**Pour chiffrer précisément**, il manque trois séries : la distribution des
pensions par décile (DREES), celle des allocations chômage (Unédic), et un modèle
de microsimulation du revenu disponible des ménages.
