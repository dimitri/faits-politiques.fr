// figure-bulletin régénère, dans docs/cotisations-et-droits.md, la figure « D'une
// fiche de paie aux caisses » à partir des vues derived.bulletin_* (migration
// 0085). La figure est écrite entre deux marqueurs ; le reste du document n'est
// pas touché. Le site affiche ce document tel quel (HTML autorisé).
//
//	go run ./cmd/figure-bulletin
package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	doc   = "docs/cotisations-et-droits.md"
	debut = "<!-- figure-bulletin:debut — généré par cmd/figure-bulletin, ne pas modifier à la main -->"
	fin   = "<!-- figure-bulletin:fin -->"
	cas   = "technicienne-2500"
)

var budgets = map[string]string{
	"ETAT": "État", "SECU_LFSS": "Sécurité sociale · LFSS", "SECU_PARITAIRE": "Sécurité sociale paritaire · hors LFSS",
	"OPERATEUR_ETAT": "Opérateur de l'État", "FONDS_ETAT": "Fonds de l'État", "COLLECTIVITE": "Collectivités",
	"PRIVE": "Organisme privé", "AUTRE": "Affectation choisie par l'employeur",
}

var ordreBudget = []string{"ETAT", "SECU_LFSS", "SECU_PARITAIRE", "OPERATEUR_ETAT", "FONDS_ETAT", "PRIVE", "AUTRE"}

var natures = []struct{ code, nom string }{
	{"SALAIRE", "Salaire versé"},
	{"IMPOT", "Impôts et taxes, sans droit individuel"},
	{"DIFFERE", "Salaire différé : droits proportionnels à ce qui est versé"},
	{"MIXTE", "Mixte"},
	{"SOLIDARITE", "Solidarité : droits sans lien avec ce salaire"},
}

var courts = map[string]string{
	"SALARIE": "Salaire net versé", "DGFIP": "Impôt sur le revenu", "CSG": "CSG", "CADES": "CRDS",
	"FRANCE_COMPETENCES": "Formation, apprentissage", "SOLDE_TA": "Solde de la taxe d'apprentissage", "AGFPN": "Dialogue social",
	"CNAV": "Retraite de base", "AGIRC_ARRCO": "Retraite complémentaire", "UNEDIC": "Assurance chômage",
	"ATMP": "Accidents du travail", "AGS": "Garantie des salaires", "CNAM": "Assurance maladie",
	"CNAF": "Famille", "CNSA": "Autonomie", "FNAL": "Aide au logement",
}

// Ce que ce mois ouvre pour la salariée. Le texte de la retraite complémentaire
// est complété par le calcul des points.
var ouvre = map[string]string{
	"DGFIP":              "Aucun droit individuel : recette du budget de l'État.",
	"CSG":                "Aucun droit individuel : impôt réparti par la loi entre les caisses de sécurité sociale.",
	"CADES":              "Aucun : rembourse la dette sociale accumulée.",
	"FRANCE_COMPETENCES": "Finance l'apprentissage et la formation ; le compte personnel de formation est crédité selon le temps travaillé, pas selon le montant versé.",
	"SOLDE_TA":           "Aucun droit individuel.",
	"AGFPN":              "Aucun droit individuel : finance les organisations syndicales et patronales.",
	"CNAV":               "Valide un trimestre (150 heures au Smic, 1 803 € en 2026, quatre au plus par an) ; le salaire, jusqu'au plafond, entre dans le calcul des 25 meilleures années.",
	"AGIRC_ARRCO":        "%s points, soit %s € de pension par an pour ce seul mois. Seule la cotisation au taux de calcul (6,20 %%) achète des points ; le taux d'appel et la CEG n'en achètent aucun.",
	"UNEDIC":             "Le mois compte pour ouvrir un droit (6 mois sur 24) et pour sa durée ; l'allocation est proportionnelle au salaire.",
	"ATMP":               "Indemnités et rente proportionnelles au salaire en cas d'accident du travail ou de maladie professionnelle.",
	"AGS":                "Garantit le paiement des salaires si l'entreprise fait l'objet d'une procédure collective.",
	"CNAM":               "Les soins sont remboursés à tout résident, cotisant ou non ; les indemnités journalières (50 % du salaire, dans la limite de 1,4 Smic) dépendent de l'activité.",
	"CNAF":               "Aucun lien avec ce salaire : les prestations dépendent des enfants et des ressources du foyer.",
	"CNSA":               "Aucun lien : allocation personnalisée d'autonomie et prestation de compensation du handicap selon la situation.",
	"FNAL":               "Aucun lien : aides personnelles au logement selon les ressources.",
}

func e(s string) string { return html.EscapeString(s) }

func eur(x float64) string {
	s := fmt.Sprintf("%.2f", math.Abs(x))
	ent, dec, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range ent {
		if i > 0 && (len(ent)-i)%3 == 0 {
			b.WriteString(" ")
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + dec
	if x < 0 {
		out = "−" + out
	}
	return out
}

func eur0(x float64) string { v := eur(math.Round(x)); return strings.TrimSuffix(v, ",00") }

func taux(x float64) string {
	s := fmt.Sprintf("%.3f", x)
	s = strings.TrimSuffix(s, "0")
	return strings.ReplaceAll(s, ".", ",")
}

type flux struct {
	org, nom, budget, sousSecteur, nature string
	salarie, employeur, reduction, verse  float64
	texteBudget                           string
}

type ligne struct {
	code, part, libelle, rubrique string
	ordre                         int
	base, taux, montant           float64
}

func main() {
	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	frag, err := figure(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	src, err := os.ReadFile(doc)
	if err != nil {
		log.Fatal(err)
	}
	i, j := bytes.Index(src, []byte(debut)), bytes.Index(src, []byte(fin))
	if i < 0 || j < i {
		log.Fatalf("%s : marqueurs de la figure introuvables", doc)
	}
	var out bytes.Buffer
	out.Write(src[:i+len(debut)])
	out.WriteString("\n")
	out.WriteString(frag)
	out.Write(src[j:])
	if err := os.WriteFile(doc, out.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s : figure régénérée (%d octets)\n", doc, len(frag))
}

func figure(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var brut, cotSal, cotEmpDu, coef, red, cotEmp, netAvant, netImp, pas, net, cout float64
	var descr string
	var effectif int
	var tauxPas, pmss float64
	if err := pool.QueryRow(ctx, `SELECT s.brut, s.cotisations_salarie, s.cotisations_employeur_dues, s.coefficient,
		s.reduction_generale, s.cotisations_employeur, s.net_avant_impot, s.net_imposable, s.impot_source, s.net_paye,
		s.cout_employeur, b.description, b.effectif, b.taux_pas,
		(SELECT valeur FROM ref.parametre_social p WHERE p.millesime = b.millesime AND p.code = 'PMSS')
		FROM derived.bulletin_synthese s JOIN ref.bulletin_cas b USING (cas) WHERE cas = $1`, cas).
		Scan(&brut, &cotSal, &cotEmpDu, &coef, &red, &cotEmp, &netAvant, &netImp, &pas, &net, &cout, &descr, &effectif, &tauxPas, &pmss); err != nil {
		return "", fmt.Errorf("synthèse : %w", err)
	}
	var calcul, prixPoint, valeurPoint float64
	if err := pool.QueryRow(ctx, `SELECT
		max(valeur) FILTER (WHERE code = 'AGIRC_ARRCO_TAUX_CALCUL_T1'),
		max(valeur) FILTER (WHERE code = 'AGIRC_ARRCO_PRIX_ACHAT'),
		max(valeur) FILTER (WHERE code = 'AGIRC_ARRCO_VALEUR_SERVICE')
		FROM ref.parametre_social p JOIN ref.bulletin_cas b ON b.millesime = p.millesime WHERE b.cas = $1`, cas).
		Scan(&calcul, &prixPoint, &valeurPoint); err != nil {
		return "", err
	}
	points := math.Min(brut, pmss) * calcul / 100 / prixPoint

	rows, err := pool.Query(ctx, `SELECT code, part, libelle, rubrique, ordre, base, taux, montant
		FROM derived.bulletin_ligne WHERE cas = $1 ORDER BY ordre, part DESC`, cas)
	if err != nil {
		return "", err
	}
	var lignes []ligne
	for rows.Next() {
		var l ligne
		if err := rows.Scan(&l.code, &l.part, &l.libelle, &l.rubrique, &l.ordre, &l.base, &l.taux, &l.montant); err != nil {
			return "", err
		}
		lignes = append(lignes, l)
	}
	rows.Close()

	rows, err = pool.Query(ctx, `SELECT f.organisme, f.nom, coalesce(f.budget, ''), coalesce(f.sous_secteur, ''), f.nature_droit,
		f.salarie, f.employeur_du, f.reduction, f.verse, coalesce(o.texte_budget, '')
		FROM derived.bulletin_flux f LEFT JOIN ref.organisme_social o ON o.code = f.organisme WHERE f.cas = $1`, cas)
	if err != nil {
		return "", err
	}
	var fl []flux
	for rows.Next() {
		var f flux
		if err := rows.Scan(&f.org, &f.nom, &f.budget, &f.sousSecteur, &f.nature, &f.salarie, &f.employeur, &f.reduction, &f.verse, &f.texteBudget); err != nil {
			return "", err
		}
		fl = append(fl, f)
	}
	rows.Close()
	rang := map[string]int{}
	for i, n := range natures {
		rang[n.code] = i
	}
	sort.SliceStable(fl, func(a, b int) bool {
		if rang[fl[a].nature] != rang[fl[b].nature] {
			return rang[fl[a].nature] < rang[fl[b].nature]
		}
		return fl[a].verse > fl[b].verse
	})

	var w strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&w, format, a...) }

	p(`<style>
.bp{--bp-differe:var(--encre,#17181D);--bp-solid:#A8A396;--bp-net:var(--filet,#E4E0D6);font-variant-numeric:tabular-nums}
@media(prefers-color-scheme:dark){:root:not([data-theme=light]) .bp{--bp-solid:#6F6B63}}
[data-theme=dark] .bp{--bp-solid:#6F6B63}
.bp-bulletin{border:1px solid var(--filet,#E4E0D6);background:var(--carte,#fff);margin:1rem 0;font-size:.84rem}
.bp-tete{display:grid;grid-template-columns:repeat(auto-fit,minmax(13rem,1fr));border-bottom:1px solid var(--encre,#17181D)}
.bp-tete>div{padding:.6rem .8rem}
.bp-tete p{margin:0}
.bp-lab{font:500 .66rem/1.3 var(--mono,monospace);letter-spacing:.06em;text-transform:uppercase;color:var(--attenue,#5A5850)}
.bp-defil{overflow-x:auto}
.bp table{border-collapse:collapse;width:100%%;min-width:40rem}
.bp th,.bp td{padding:.25rem .55rem;border-bottom:1px solid var(--filet2,#EFEBE2);text-align:left;vertical-align:top}
.bp thead th{font:500 .64rem/1.25 var(--mono,monospace);letter-spacing:.04em;text-transform:uppercase;color:var(--attenue,#5A5850);vertical-align:bottom}
.bp .n{text-align:right;font-family:var(--mono,monospace);white-space:nowrap}
.bp tr.bp-rub th{font-weight:600;padding-top:.55rem;border-bottom:1px solid var(--filet,#E4E0D6)}
.bp tr.bp-tot td{font-weight:600;border-top:1px solid var(--encre,#17181D)}
.bp-pied{display:grid;grid-template-columns:repeat(auto-fit,minmax(16rem,1fr));border-top:1px solid var(--encre,#17181D)}
.bp-pied>div{display:flex;justify-content:space-between;gap:1rem;padding:.35rem .8rem;border-bottom:1px solid var(--filet2,#EFEBE2)}
.bp-pied b{font-family:var(--mono,monospace);font-weight:500;white-space:nowrap}
.bp-pied .bp-fort span,.bp-pied .bp-fort b{font-weight:600}
.bp svg{display:block;width:100%%;height:auto}
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
.bp-leg .l-MIXTE{background:linear-gradient(135deg,var(--bp-differe) 50%%,var(--bp-solid) 50%%)}
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
`)
	p(`<div class="bp">`)
	p(`<p><strong>Hypothèses.</strong> %s</p>`, e(descr))

	// Le bulletin.
	p(`<div class="bp-bulletin" role="region" aria-label="Bulletin de paie factice, janvier 2026">`)
	p(`<div class="bp-tete"><div><p class="bp-lab">Employeur</p><p><strong>Atelier Durand SAS</strong> (factice)<br>%d salariés · métallurgie</p></div>`, effectif)
	p(`<div><p class="bp-lab">Salariée</p><p><strong>Camille Martin</strong> (factice)<br>technicienne d'atelier, non-cadre, CDI, 151,67 h</p></div>`)
	p(`<div><p class="bp-lab">Période</p><p><strong>Janvier 2026</strong><br>plafond mensuel : %s €</p></div></div>`, eur0(pmss))
	p(`<div class="bp-defil"><table><thead><tr><th>Rubrique</th><th class="n">Base</th><th class="n">Taux salarié %%</th><th class="n">Part salarié €</th><th class="n">Taux employeur %%</th><th class="n">Part employeur €</th></tr></thead><tbody>`)
	p(`<tr class="bp-tot"><td>Salaire brut</td><td></td><td></td><td class="n">%s</td><td></td><td></td></tr>`, eur(brut))
	vus := map[string]bool{}
	rub := ""
	for _, l := range lignes {
		if vus[l.code] {
			continue
		}
		vus[l.code] = true
		if l.rubrique != rub {
			rub = l.rubrique
			p(`<tr class="bp-rub"><th colspan="6">%s</th></tr>`, e(rub))
		}
		var ts, ms, te, me string
		for _, m := range lignes {
			if m.code != l.code {
				continue
			}
			if m.part == "SALARIE" {
				ts, ms = taux(m.taux), eur(m.montant)
			} else {
				te, me = taux(m.taux), eur(m.montant)
			}
		}
		p(`<tr><td>%s</td><td class="n">%s</td><td class="n">%s</td><td class="n">%s</td><td class="n">%s</td><td class="n">%s</td></tr>`,
			e(l.libelle), eur(l.base), ts, ms, te, me)
	}
	p(`<tr class="bp-rub"><th colspan="6">Exonérations et allègements de cotisations</th></tr>`)
	p(`<tr><td>Réduction générale dégressive unique (coefficient %s)</td><td class="n">%s</td><td></td><td></td><td></td><td class="n">−%s</td></tr>`,
		strings.ReplaceAll(fmt.Sprintf("%.4f", coef), ".", ","), eur(brut), eur(red))
	p(`<tr class="bp-tot"><td>Total des cotisations et contributions</td><td></td><td></td><td class="n">%s</td><td></td><td class="n">%s</td></tr>`, eur(cotSal), eur(cotEmp))
	p(`</tbody></table></div>`)
	p(`<div class="bp-pied"><div><span>Net à payer avant impôt sur le revenu</span><b>%s €</b></div><div><span>Net imposable</span><b>%s €</b></div>`, eur(netAvant), eur(netImp))
	p(`<div><span>Impôt sur le revenu prélevé à la source (taux %s %%)</span><b>−%s €</b></div><div class="bp-fort"><span>Net payé</span><b>%s €</b></div>`,
		strings.ReplaceAll(fmt.Sprintf("%.1f", tauxPas), ".", ","), eur(pas), eur(net))
	p(`<div><span>Montant net social</span><b>%s €</b></div><div class="bp-fort"><span>Coût total pour l'employeur</span><b>%s €</b></div></div>`, eur(netAvant), eur(cout))
	p(`</div>`)

	// Répartition par budget : une barre à l'échelle, et la légende en liste
	// (les petits budgets ne tiendraient pas dans la barre).
	parBudget := map[string]float64{}
	for _, f := range fl {
		if f.org == "SALARIE" {
			continue
		}
		parBudget[f.budget] += f.verse
	}
	{
		type seg struct {
			lib, cls string
			v        float64
		}
		segs := []seg{{"Salariée (salaire net)", "s-b0", net}}
		for _, b := range ordreBudget {
			v, ok := parBudget[b]
			if !ok {
				continue
			}
			cls := map[string]string{"ETAT": "s-b1", "SECU_LFSS": "s-b2", "SECU_PARITAIRE": "s-b3"}[b]
			if cls == "" {
				cls = "s-b4"
			}
			segs = append(segs, seg{budgets[b], cls, v})
		}
		W, x0, x1 := 1000.0, 0.0, 1000.0
		k := (x1 - x0) / cout
		p(`<figure><p><strong>Les %s € du coût employeur, par budget qui les reçoit</strong></p><svg viewBox="0 0 %.0f 36" role="img" aria-label="Répartition du coût employeur par budget destinataire">`, eur(cout), W)
		x := x0
		for _, sg := range segs {
			p(`<rect class="s-budget %s" x="%.1f" y="0" width="%.1f" height="36"><title>%s : %s €</title></rect>`, sg.cls, x, sg.v*k, e(sg.lib), eur(sg.v))
			x += sg.v * k
		}
		p(`</svg><ul class="bp-budgets">`)
		for _, sg := range segs {
			p(`<li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="%s" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>%s</span><b>%s €</b><em>%s %%</em></li>`,
				sg.cls, e(sg.lib), eur(sg.v), strings.ReplaceAll(fmt.Sprintf("%.1f", 100*sg.v/cout), ".", ","))
		}
		p(`</ul></figure>`)
	}

	// Le schéma des flux.
	{
		W, x0, nw, x1 := 1000.0, 150.0, 16.0, 440.0
		top, gapNat, gap := 34.0, 30.0, 7.0
		k := 760 / cout
		h := func(v float64) float64 { return math.Max(v*k, 1.6) }
		type pos struct{ y, hh float64 }
		ps := map[string]pos{}
		var entetes []struct {
			nat string
			y   float64
		}
		y, prec := top, ""
		for _, f := range fl {
			if f.nature != prec {
				if prec != "" {
					y += gapNat
				}
				entetes = append(entetes, struct {
					nat string
					y   float64
				}{f.nature, y})
				y += 18
				prec = f.nature
			}
			ps[f.org] = pos{y, h(f.verse)}
			y += math.Max(h(f.verse), 18) + gap
		}
		H := y + 16
		hb, hp := h(brut), h(cotEmp)
		yb := top + 18
		yp := yb + hb + 40
		p(`<figure><svg viewBox="0 0 %.0f %.0f" role="img" aria-label="Chemin de chaque euro du coût employeur jusqu'à son destinataire">`, W, H)
		p(`<defs><pattern id="bp-hach" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="6" height="6" class="p-fond"/><line x1="0" y1="0" x2="0" y2="6" class="p-trait"/></pattern>`)
		p(`<pattern id="bp-demi" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="8" height="8" class="n-SOLIDARITE"/><rect width="4" height="8" class="n-DIFFERE"/></pattern></defs>`)
		p(`<rect class="n-g" x="%.0f" y="%.1f" width="%.0f" height="%.1f"/>`, x0-nw, yb, nw, hb)
		p(`<text class="t-lbl t-fort" x="%.0f" y="%.1f" text-anchor="end">Salaire brut</text><text class="t-num" x="%.0f" y="%.1f" text-anchor="end">%s €</text>`, x0-nw-10, yb+hb/2-6, x0-nw-10, yb+hb/2+12, eur(brut))
		p(`<rect class="n-g" x="%.0f" y="%.1f" width="%.0f" height="%.1f"/>`, x0-nw, yp, nw, hp)
		p(`<text class="t-lbl t-fort" x="%.0f" y="%.1f" text-anchor="end">Cotisations</text><text class="t-lbl t-fort" x="%.0f" y="%.1f" text-anchor="end">employeur</text><text class="t-num" x="%.0f" y="%.1f" text-anchor="end">%s €</text>`,
			x0-nw-10, yp+hp/2-8, x0-nw-10, yp+hp/2+8, x0-nw-10, yp+hp/2+26, eur(cotEmp))
		p(`<text class="t-pt t-fort" x="%.0f" y="%.1f">Coût total pour l'employeur : %s €</text>`, x0-nw, yb-12, eur(cout))
		bande := func(ya, yc, hh float64, nat string) {
			m := (x0 + x1) / 2
			p(`<path class="f-%s" d="M%.0f,%.2f C%.1f,%.2f %.1f,%.2f %.0f,%.2f L%.0f,%.2f C%.1f,%.2f %.1f,%.2f %.0f,%.2f Z"/>`,
				nat, x0, ya, m, ya, m, yc, x1, yc, x1, yc+hh, m, yc+hh, m, ya+hh, x0, ya+hh)
		}
		curB, curP := yb, yp
		for _, f := range fl {
			ps0 := ps[f.org]
			rempli := 0.0
			partSal := f.salarie
			if f.org == "SALARIE" || f.org == "DGFIP" {
				partSal = f.verse
			}
			if partSal > 0 {
				hh := partSal * k
				bande(curB, ps0.y+rempli, hh, f.nature)
				curB += hh
				rempli += hh
			}
			if emp := f.employeur - f.reduction; emp > 0 && f.org != "SALARIE" && f.org != "DGFIP" {
				hh := emp * k
				bande(curP, ps0.y+rempli, hh, f.nature)
				curP += hh
			}
		}
		noms := map[string]string{}
		for _, n := range natures {
			noms[n.code] = n.nom
		}
		for _, en := range entetes {
			p(`<text class="t-surt" x="%.0f" y="%.1f">%s</text>`, x1, en.y+11, e(strings.ToUpper(noms[en.nat])))
		}
		for _, f := range fl {
			ps0 := ps[f.org]
			p(`<rect class="n-%s" x="%.0f" y="%.1f" width="%.0f" height="%.1f"/>`, f.nature, x1, ps0.y, nw, ps0.hh)
			yc := ps0.y + math.Max(ps0.hh, 14)/2 + 4
			p(`<text class="t-lbl t-fort" x="%.0f" y="%.1f">%s</text>`, x1+nw+10, yc, e(courts[f.org]))
			p(`<text class="t-num" x="%.0f" y="%.1f" text-anchor="end">%s €</text>`, x1+nw+290, yc, eur(f.verse))
			if lib, ok := budgets[f.budget]; ok {
				wb := float64(len([]rune(lib)))*6.7 + 12
				p(`<rect class="r-badge" x="%.0f" y="%.1f" width="%.0f" height="17" rx="2"/><text class="t-badge" x="%.0f" y="%.1f">%s</text>`,
					x1+nw+302, yc-13, wb, x1+nw+308, yc-1, e(lib))
			}
		}
		p(`</svg>`)
		p(`<div class="bp-leg"><span><i class="l-SALAIRE"></i>salaire versé</span><span><i class="l-DIFFERE"></i>salaire différé</span><span><i class="l-MIXTE"></i>mixte</span><span><i class="l-SOLIDARITE"></i>solidarité</span><span><i class="l-IMPOT"></i>impôts et taxes</span><span>cadre : budget qui reçoit</span></div>`)
		p(`<figcaption>Épaisseurs proportionnelles aux montants versés après réduction générale. La part de la réduction imputée sur la retraite complémentaire suit la règle officielle (réduction × 6,01 %% / coefficient maximal) ; sa répartition entre les autres caisses suit les taux, à titre d'illustration. Source : barème 2026 (ref.taux_cotisation), vues derived.bulletin_*.</figcaption></figure>`)
	}

	// Le tableau des destinataires.
	p(`<div class="bp-defil"><table class="bp-dest"><thead><tr><th>Destinataire</th><th>Budget</th><th class="n">Versé €</th><th class="n">Réduction générale €</th><th>Ce que ce mois ouvre pour la salariée</th></tr></thead><tbody>`)
	prec := ""
	for _, f := range fl {
		if f.org == "SALARIE" {
			continue
		}
		if f.nature != prec {
			prec = f.nature
			for _, n := range natures {
				if n.code == f.nature {
					p(`<tr class="bp-rub"><th colspan="5">%s</th></tr>`, e(n.nom))
				}
			}
		}
		txt := ouvre[f.org]
		if f.org == "AGIRC_ARRCO" {
			txt = fmt.Sprintf(txt, strings.ReplaceAll(fmt.Sprintf("%.2f", points), ".", ","), eur(points*valeurPoint))
		}
		ss := ""
		if f.sousSecteur != "" {
			ss = " · " + f.sousSecteur
		}
		redTxt := "—"
		if f.reduction > 0 {
			redTxt = eur(f.reduction)
		}
		p(`<tr><td><strong>%s</strong><br>%s</td><td><span class="bp-budget">%s%s</span><br><small>%s</small></td><td class="n">%s</td><td class="n">%s</td><td>%s</td></tr>`,
			e(courts[f.org]), e(f.nom), e(budgets[f.budget]), e(ss), e(f.texteBudget), eur(f.verse), redTxt, e(txt))
	}
	p(`</tbody></table></div>`)
	p(`</div>`)

	// goldmark termine un bloc HTML à la première ligne vide : il n'y en a aucune.
	var sortie []string
	for _, l := range strings.Split(w.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			sortie = append(sortie, l)
		}
	}
	return strings.Join(sortie, "\n") + "\n", nil
}
