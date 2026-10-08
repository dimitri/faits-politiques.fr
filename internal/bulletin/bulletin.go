// Package bulletin régénère, dans docs/cotisations-et-droits.md, la figure
// « D'une fiche de paie aux caisses » à partir des vues derived.bulletin_*
// (migration 0085). La figure est écrite entre deux marqueurs ; le reste du
// document n'est pas touché. Le site affiche ce document tel quel (HTML
// autorisé). Appelé par fpctl (voir cmd/fpctl) : fpctl generate bulletin.
package bulletin

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	doc      = "docs/cotisations-et-droits.md"
	start    = "<!-- figure-bulletin:debut — généré par cmd/figure-bulletin, ne pas modifier à la main -->"
	end      = "<!-- figure-bulletin:fin -->"
	scenario = "technicienne-2500"
)

var budgets = map[string]string{
	"ETAT": "État", "SECU_LFSS": "Sécurité sociale · LFSS", "SECU_PARITAIRE": "Sécurité sociale paritaire · hors LFSS",
	"OPERATEUR_ETAT": "Opérateur de l'État", "FONDS_ETAT": "Fonds de l'État", "COLLECTIVITE": "Collectivités",
	"PRIVE": "Organisme privé", "AUTRE": "Affectation choisie par l'employeur",
}

var budgetOrder = []string{"ETAT", "SECU_LFSS", "SECU_PARITAIRE", "OPERATEUR_ETAT", "FONDS_ETAT", "PRIVE", "AUTRE"}

var natures = []struct{ code, name string }{
	{"SALAIRE", "Salaire versé"},
	{"IMPOT", "Impôts et taxes, sans droit individuel"},
	{"DIFFERE", "Salaire différé : droits proportionnels à ce qui est versé"},
	{"MIXTE", "Mixte"},
	{"SOLIDARITE", "Solidarité : droits sans lien avec ce salaire"},
}

var shortLabels = map[string]string{
	"SALARIE": "Salaire net versé", "DGFIP": "Impôt sur le revenu", "CSG": "CSG", "CADES": "CRDS",
	"FRANCE_COMPETENCES": "Formation, apprentissage", "SOLDE_TA": "Solde de la taxe d'apprentissage", "AGFPN": "Dialogue social",
	"CNAV": "Retraite de base", "AGIRC_ARRCO": "Retraite complémentaire", "UNEDIC": "Assurance chômage",
	"ATMP": "Accidents du travail", "AGS": "Garantie des salaires", "CNAM": "Assurance maladie",
	"CNAF": "Famille", "CNSA": "Autonomie", "FNAL": "Aide au logement",
}

// Ce que ce mois ouvre pour la salariée. Le texte de la retraite complémentaire
// est complété par le calcul des points.
var opens = map[string]string{
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

func rate(x float64) string {
	s := fmt.Sprintf("%.3f", x)
	s = strings.TrimSuffix(s, "0")
	return strings.ReplaceAll(s, ".", ",")
}

type flow struct {
	org, name, budget, subSector, nature string
	employee, employer, reduction, paid  float64
	budgetText                           string
}

type line struct {
	code, part, label, heading string
	order                      int
	base, rate, amount         float64
}

// Run exécute la commande bulletin. Ne prend aucune option ; args n'existe
// que pour l'uniformité avec les autres commandes routées par fpctl. ctx
// est celui de fpctl (cmd.Context()), déjà annulé au premier signal.
func Run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("bulletin ne prend aucune option (%q inattendu)", args[0])
	}
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	frag, err := figure(ctx, pool)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(doc)
	if err != nil {
		return err
	}
	i, j := bytes.Index(src, []byte(start)), bytes.Index(src, []byte(end))
	if i < 0 || j < i {
		return fmt.Errorf("%s : marqueurs de la figure introuvables", doc)
	}
	var out bytes.Buffer
	out.Write(src[:i+len(start)])
	out.WriteString("\n")
	out.WriteString(frag)
	out.Write(src[j:])
	if err := os.WriteFile(doc, out.Bytes(), 0o644); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%s: figure regenerated (%d bytes)", doc, len(frag)))
	return nil
}

func figure(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var gross, employeeContrib, employerContribDue, coef, reduction, employerContrib, netBeforeTax, netTaxable, withholding, net, cost float64
	var description string
	var headcount int
	var withholdingRate, pmss float64
	if err := pool.QueryRow(ctx, `SELECT s.brut, s.cotisations_salarie, s.cotisations_employeur_dues, s.coefficient,
		s.reduction_generale, s.cotisations_employeur, s.net_avant_impot, s.net_imposable, s.impot_source, s.net_paye,
		s.cout_employeur, b.description, b.effectif, b.taux_pas,
		(SELECT valeur FROM ref.parametre_social p WHERE p.millesime = b.millesime AND p.code = 'PMSS')
		FROM derived.bulletin_synthese s JOIN ref.bulletin_cas b USING (cas) WHERE cas = $1`, scenario).
		Scan(&gross, &employeeContrib, &employerContribDue, &coef, &reduction, &employerContrib, &netBeforeTax, &netTaxable, &withholding, &net, &cost, &description, &headcount, &withholdingRate, &pmss); err != nil {
		return "", fmt.Errorf("synthèse : %w", err)
	}
	var calcRate, pointPrice, pointValue float64
	if err := pool.QueryRow(ctx, `SELECT
		max(valeur) FILTER (WHERE code = 'AGIRC_ARRCO_TAUX_CALCUL_T1'),
		max(valeur) FILTER (WHERE code = 'AGIRC_ARRCO_PRIX_ACHAT'),
		max(valeur) FILTER (WHERE code = 'AGIRC_ARRCO_VALEUR_SERVICE')
		FROM ref.parametre_social p JOIN ref.bulletin_cas b ON b.millesime = p.millesime WHERE b.cas = $1`, scenario).
		Scan(&calcRate, &pointPrice, &pointValue); err != nil {
		return "", err
	}
	points := math.Min(gross, pmss) * calcRate / 100 / pointPrice

	rows, err := pool.Query(ctx, `SELECT code, part, libelle, rubrique, ordre, base, taux, montant
		FROM derived.bulletin_ligne WHERE cas = $1 ORDER BY ordre, part DESC`, scenario)
	if err != nil {
		return "", err
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.code, &l.part, &l.label, &l.heading, &l.order, &l.base, &l.rate, &l.amount); err != nil {
			return "", err
		}
		lines = append(lines, l)
	}
	rows.Close()

	rows, err = pool.Query(ctx, `SELECT f.organisme, f.nom, coalesce(f.budget, ''), coalesce(f.sous_secteur, ''), f.nature_droit,
		f.salarie, f.employeur_du, f.reduction, f.verse, coalesce(o.texte_budget, '')
		FROM derived.bulletin_flux f LEFT JOIN ref.organisme_social o ON o.code = f.organisme WHERE f.cas = $1`, scenario)
	if err != nil {
		return "", err
	}
	var flows []flow
	for rows.Next() {
		var f flow
		if err := rows.Scan(&f.org, &f.name, &f.budget, &f.subSector, &f.nature, &f.employee, &f.employer, &f.reduction, &f.paid, &f.budgetText); err != nil {
			return "", err
		}
		flows = append(flows, f)
	}
	rows.Close()
	rank := map[string]int{}
	for i, n := range natures {
		rank[n.code] = i
	}
	sort.SliceStable(flows, func(a, b int) bool {
		if rank[flows[a].nature] != rank[flows[b].nature] {
			return rank[flows[a].nature] < rank[flows[b].nature]
		}
		return flows[a].paid > flows[b].paid
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
	p(`<p><strong>Hypothèses.</strong> %s</p>`, e(description))

	// Le bulletin.
	p(`<div class="bp-bulletin" role="region" aria-label="Bulletin de paie factice, janvier 2026">`)
	p(`<div class="bp-tete"><div><p class="bp-lab">Employeur</p><p><strong>Atelier Durand SAS</strong> (factice)<br>%d salariés · métallurgie</p></div>`, headcount)
	p(`<div><p class="bp-lab">Salariée</p><p><strong>Camille Martin</strong> (factice)<br>technicienne d'atelier, non-cadre, CDI, 151,67 h</p></div>`)
	p(`<div><p class="bp-lab">Période</p><p><strong>Janvier 2026</strong><br>plafond mensuel : %s €</p></div></div>`, eur0(pmss))
	p(`<div class="bp-defil"><table><thead><tr><th>Rubrique</th><th class="n">Base</th><th class="n">Taux salarié %%</th><th class="n">Part salarié €</th><th class="n">Taux employeur %%</th><th class="n">Part employeur €</th></tr></thead><tbody>`)
	p(`<tr class="bp-tot"><td>Salaire brut</td><td></td><td></td><td class="n">%s</td><td></td><td></td></tr>`, eur(gross))
	seen := map[string]bool{}
	currentHeading := ""
	for _, l := range lines {
		if seen[l.code] {
			continue
		}
		seen[l.code] = true
		if l.heading != currentHeading {
			currentHeading = l.heading
			p(`<tr class="bp-rub"><th colspan="6">%s</th></tr>`, e(currentHeading))
		}
		var ts, ms, te, me string
		for _, m := range lines {
			if m.code != l.code {
				continue
			}
			if m.part == "SALARIE" {
				ts, ms = rate(m.rate), eur(m.amount)
			} else {
				te, me = rate(m.rate), eur(m.amount)
			}
		}
		p(`<tr><td>%s</td><td class="n">%s</td><td class="n">%s</td><td class="n">%s</td><td class="n">%s</td><td class="n">%s</td></tr>`,
			e(l.label), eur(l.base), ts, ms, te, me)
	}
	p(`<tr class="bp-rub"><th colspan="6">Exonérations et allègements de cotisations</th></tr>`)
	p(`<tr><td>Réduction générale dégressive unique (coefficient %s)</td><td class="n">%s</td><td></td><td></td><td></td><td class="n">−%s</td></tr>`,
		strings.ReplaceAll(fmt.Sprintf("%.4f", coef), ".", ","), eur(gross), eur(reduction))
	p(`<tr class="bp-tot"><td>Total des cotisations et contributions</td><td></td><td></td><td class="n">%s</td><td></td><td class="n">%s</td></tr>`, eur(employeeContrib), eur(employerContrib))
	p(`</tbody></table></div>`)
	p(`<div class="bp-pied"><div><span>Net à payer avant impôt sur le revenu</span><b>%s €</b></div><div><span>Net imposable</span><b>%s €</b></div>`, eur(netBeforeTax), eur(netTaxable))
	p(`<div><span>Impôt sur le revenu prélevé à la source (taux %s %%)</span><b>−%s €</b></div><div class="bp-fort"><span>Net payé</span><b>%s €</b></div>`,
		strings.ReplaceAll(fmt.Sprintf("%.1f", withholdingRate), ".", ","), eur(withholding), eur(net))
	p(`<div><span>Montant net social</span><b>%s €</b></div><div class="bp-fort"><span>Coût total pour l'employeur</span><b>%s €</b></div></div>`, eur(netBeforeTax), eur(cost))
	p(`</div>`)

	// Répartition par budget : une barre à l'échelle, et la légende en liste
	// (les petits budgets ne tiendraient pas dans la barre).
	byBudget := map[string]float64{}
	for _, f := range flows {
		if f.org == "SALARIE" {
			continue
		}
		byBudget[f.budget] += f.paid
	}
	{
		type segment struct {
			label, cls string
			v          float64
		}
		segments := []segment{{"Salariée (salaire net)", "s-b0", net}}
		for _, b := range budgetOrder {
			v, ok := byBudget[b]
			if !ok {
				continue
			}
			cls := map[string]string{"ETAT": "s-b1", "SECU_LFSS": "s-b2", "SECU_PARITAIRE": "s-b3"}[b]
			if cls == "" {
				cls = "s-b4"
			}
			segments = append(segments, segment{budgets[b], cls, v})
		}
		W, x0, x1 := 1000.0, 0.0, 1000.0
		k := (x1 - x0) / cost
		p(`<figure><p><strong>Les %s € du coût employeur, par budget qui les reçoit</strong></p><svg viewBox="0 0 %.0f 36" role="img" aria-label="Répartition du coût employeur par budget destinataire">`, eur(cost), W)
		x := x0
		for _, sg := range segments {
			p(`<rect class="s-budget %s" x="%.1f" y="0" width="%.1f" height="36"><title>%s : %s €</title></rect>`, sg.cls, x, sg.v*k, e(sg.label), eur(sg.v))
			x += sg.v * k
		}
		p(`</svg><ul class="bp-budgets">`)
		for _, sg := range segments {
			p(`<li><svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true"><rect class="%s" width="12" height="12" stroke="currentColor" stroke-width=".8"/></svg><span>%s</span><b>%s €</b><em>%s %%</em></li>`,
				sg.cls, e(sg.label), eur(sg.v), strings.ReplaceAll(fmt.Sprintf("%.1f", 100*sg.v/cost), ".", ","))
		}
		p(`</ul></figure>`)
	}

	// Le schéma des flux.
	{
		W, x0, nw, x1 := 1000.0, 150.0, 16.0, 440.0
		top, natureGap, gap := 34.0, 30.0, 7.0
		k := 760 / cost
		h := func(v float64) float64 { return math.Max(v*k, 1.6) }
		type pos struct{ y, height float64 }
		ps := map[string]pos{}
		var headers []struct {
			nat string
			y   float64
		}
		y, prev := top, ""
		for _, f := range flows {
			if f.nature != prev {
				if prev != "" {
					y += natureGap
				}
				headers = append(headers, struct {
					nat string
					y   float64
				}{f.nature, y})
				y += 18
				prev = f.nature
			}
			ps[f.org] = pos{y, h(f.paid)}
			y += math.Max(h(f.paid), 18) + gap
		}
		H := y + 16
		hb, hp := h(gross), h(employerContrib)
		yb := top + 18
		yp := yb + hb + 40
		p(`<figure><svg viewBox="0 0 %.0f %.0f" role="img" aria-label="Chemin de chaque euro du coût employeur jusqu'à son destinataire">`, W, H)
		p(`<defs><pattern id="bp-hach" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="6" height="6" class="p-fond"/><line x1="0" y1="0" x2="0" y2="6" class="p-trait"/></pattern>`)
		p(`<pattern id="bp-demi" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="8" height="8" class="n-SOLIDARITE"/><rect width="4" height="8" class="n-DIFFERE"/></pattern></defs>`)
		p(`<rect class="n-g" x="%.0f" y="%.1f" width="%.0f" height="%.1f"/>`, x0-nw, yb, nw, hb)
		p(`<text class="t-lbl t-fort" x="%.0f" y="%.1f" text-anchor="end">Salaire brut</text><text class="t-num" x="%.0f" y="%.1f" text-anchor="end">%s €</text>`, x0-nw-10, yb+hb/2-6, x0-nw-10, yb+hb/2+12, eur(gross))
		p(`<rect class="n-g" x="%.0f" y="%.1f" width="%.0f" height="%.1f"/>`, x0-nw, yp, nw, hp)
		p(`<text class="t-lbl t-fort" x="%.0f" y="%.1f" text-anchor="end">Cotisations</text><text class="t-lbl t-fort" x="%.0f" y="%.1f" text-anchor="end">employeur</text><text class="t-num" x="%.0f" y="%.1f" text-anchor="end">%s €</text>`,
			x0-nw-10, yp+hp/2-8, x0-nw-10, yp+hp/2+8, x0-nw-10, yp+hp/2+26, eur(employerContrib))
		p(`<text class="t-pt t-fort" x="%.0f" y="%.1f">Coût total pour l'employeur : %s €</text>`, x0-nw, yb-12, eur(cost))
		band := func(ya, yc, height float64, nat string) {
			m := (x0 + x1) / 2
			p(`<path class="f-%s" d="M%.0f,%.2f C%.1f,%.2f %.1f,%.2f %.0f,%.2f L%.0f,%.2f C%.1f,%.2f %.1f,%.2f %.0f,%.2f Z"/>`,
				nat, x0, ya, m, ya, m, yc, x1, yc, x1, yc+height, m, yc+height, m, ya+height, x0, ya+height)
		}
		curGross, curEmployer := yb, yp
		for _, f := range flows {
			pos0 := ps[f.org]
			filled := 0.0
			employeeShare := f.employee
			if f.org == "SALARIE" || f.org == "DGFIP" {
				employeeShare = f.paid
			}
			if employeeShare > 0 {
				height := employeeShare * k
				band(curGross, pos0.y+filled, height, f.nature)
				curGross += height
				filled += height
			}
			if emp := f.employer - f.reduction; emp > 0 && f.org != "SALARIE" && f.org != "DGFIP" {
				height := emp * k
				band(curEmployer, pos0.y+filled, height, f.nature)
				curEmployer += height
			}
		}
		names := map[string]string{}
		for _, n := range natures {
			names[n.code] = n.name
		}
		for _, en := range headers {
			p(`<text class="t-surt" x="%.0f" y="%.1f">%s</text>`, x1, en.y+11, e(strings.ToUpper(names[en.nat])))
		}
		for _, f := range flows {
			pos0 := ps[f.org]
			p(`<rect class="n-%s" x="%.0f" y="%.1f" width="%.0f" height="%.1f"/>`, f.nature, x1, pos0.y, nw, pos0.height)
			yc := pos0.y + math.Max(pos0.height, 14)/2 + 4
			p(`<text class="t-lbl t-fort" x="%.0f" y="%.1f">%s</text>`, x1+nw+10, yc, e(shortLabels[f.org]))
			p(`<text class="t-num" x="%.0f" y="%.1f" text-anchor="end">%s €</text>`, x1+nw+290, yc, eur(f.paid))
			if label, ok := budgets[f.budget]; ok {
				wb := float64(len([]rune(label)))*6.7 + 12
				p(`<rect class="r-badge" x="%.0f" y="%.1f" width="%.0f" height="17" rx="2"/><text class="t-badge" x="%.0f" y="%.1f">%s</text>`,
					x1+nw+302, yc-13, wb, x1+nw+308, yc-1, e(label))
			}
		}
		p(`</svg>`)
		p(`<div class="bp-leg"><span><i class="l-SALAIRE"></i>salaire versé</span><span><i class="l-DIFFERE"></i>salaire différé</span><span><i class="l-MIXTE"></i>mixte</span><span><i class="l-SOLIDARITE"></i>solidarité</span><span><i class="l-IMPOT"></i>impôts et taxes</span><span>cadre : budget qui reçoit</span></div>`)
		p(`<figcaption>Épaisseurs proportionnelles aux montants versés après réduction générale. La part de la réduction imputée sur la retraite complémentaire suit la règle officielle (réduction × 6,01 %% / coefficient maximal) ; sa répartition entre les autres caisses suit les taux, à titre d'illustration. Source : barème 2026 (ref.taux_cotisation), vues derived.bulletin_*.</figcaption></figure>`)
	}

	// Le tableau des destinataires.
	p(`<div class="bp-defil"><table class="bp-dest"><thead><tr><th>Destinataire</th><th>Budget</th><th class="n">Versé €</th><th class="n">Réduction générale €</th><th>Ce que ce mois ouvre pour la salariée</th></tr></thead><tbody>`)
	prev := ""
	for _, f := range flows {
		if f.org == "SALARIE" {
			continue
		}
		if f.nature != prev {
			prev = f.nature
			for _, n := range natures {
				if n.code == f.nature {
					p(`<tr class="bp-rub"><th colspan="5">%s</th></tr>`, e(n.name))
				}
			}
		}
		text := opens[f.org]
		if f.org == "AGIRC_ARRCO" {
			text = fmt.Sprintf(text, strings.ReplaceAll(fmt.Sprintf("%.2f", points), ".", ","), eur(points*pointValue))
		}
		subText := ""
		if f.subSector != "" {
			subText = " · " + f.subSector
		}
		reductionText := "—"
		if f.reduction > 0 {
			reductionText = eur(f.reduction)
		}
		p(`<tr><td><strong>%s</strong><br>%s</td><td><span class="bp-budget">%s%s</span><br><small>%s</small></td><td class="n">%s</td><td class="n">%s</td><td>%s</td></tr>`,
			e(shortLabels[f.org]), e(f.name), e(budgets[f.budget]), e(subText), e(f.budgetText), eur(f.paid), reductionText, e(text))
	}
	p(`</tbody></table></div>`)
	p(`</div>`)

	// goldmark termine un bloc HTML à la première ligne vide : il n'y en a aucune.
	var output []string
	for _, l := range strings.Split(w.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			output = append(output, l)
		}
	}
	return strings.Join(output, "\n") + "\n", nil
}
