package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La page « Les candidats et la justice » : une fiche par candidat, avec son
// portrait, son organisation, le stade de chaque décision et la peine la plus
// lourde prononcée — plutôt qu'une frise de décisions où il faut lire chaque
// paragraphe pour savoir de qui l'on parle.
//
// Les deux repères sont DESCRIPTIFS et fixés d'avance :
//
//   - le stade dit où en est la procédure (première instance, appel, cassation,
//     définitive) ; il vient de la source citée, jamais d'une déduction ;
//   - la peine prononcée est rangée dans l'ordre des peines du code pénal
//     (article 131-3 : l'emprisonnement avant l'amende) — amende, emprisonnement
//     avec sursis, emprisonnement ferme ; l'inéligibilité s'affiche à part, elle
//     n'est pas comparable à une peine de prison.
//
// Aucun des deux ne mesure la gravité des faits ni ne juge la personne ; la page
// le dit. Les fiches sont rangées par ordre alphabétique, jamais par « gravité » :
// un classement de personnes par peine serait un jugement que le dossier ne porte pas.

const dossierJudicial = "candidats-condamnations"

// Stades de la procédure, du premier au dernier.
var stadesJudicial = []string{"Première instance", "Appel", "Cassation", "Définitive"}

// Peines, de la moins lourde à la plus lourde au sens du code pénal.
var peinesJudicial = []string{"Amende", "Emprisonnement avec sursis", "Emprisonnement ferme"}

// judicialMeta : les deux repères de chaque décision, tels que la source citée
// par le fait les rapporte. Une décision sans entrée s'affiche sans repères.
type judicialMeta struct {
	stade      int    // index dans stadesJudicial
	peine      int    // index dans peinesJudicial, -1 si la source n'en donne pas
	inelig     string // inéligibilité rapportée, vide si aucune
	detail     string // la peine, en clair
	infraction string // la qualification, en clair
}

var judicialMetas = map[string]judicialMeta{
	"justice-le-pen-appel-2026": {
		stade: 1, peine: 2,
		inelig:     "45 mois dont 30 avec sursis",
		detail:     "3 ans d'emprisonnement dont 2 avec sursis (1 an ferme, sous bracelet électronique)",
		infraction: "Détournement de fonds publics",
	},
	"justice-melenchon-bobigny-2019": {
		stade: 0, peine: 1,
		detail:     "3 mois d'emprisonnement avec sursis et 8 000 € d'amende",
		infraction: "Rébellion et provocation",
	},
	"justice-zemmour-convention-droite": {
		stade: 3, peine: 0,
		detail:     "15 000 € d'amende",
		infraction: "Provocation à la haine raciale et injures racistes",
	},
	"justice-zemmour-mineurs-isoles": {
		stade: 3, peine: 0,
		detail:     "100 jours-amende à 100 € (10 000 €)",
		infraction: "Complicité d'injure publique et de provocation à la haine",
	},
}

// judicialCard : un candidat et ses décisions.
type judicialCard struct {
	slug, name, firstName, org, orgSlug string
	portrait                            *Media
	decisions                           []factTopic
}

func loadJudicialCards(ctx context.Context, pool *pgxpool.Pool, dataDir string, facts []factTopic) ([]judicialCard, error) {
	cands, err := loadCandidates(filepath.Join(dataDir, "candidats.csv"), nil)
	if err != nil {
		return nil, err
	}
	bySlug := map[string]*Candidate{}
	for _, c := range cands {
		bySlug[c.Slug] = c
	}
	portraits, _, err := loadMedias(ctx, pool)
	if err != nil {
		return nil, err
	}
	index := map[string]*judicialCard{}
	var order []string
	for _, f := range facts {
		if f.slug == "" || f.section != "CONTROLE" {
			continue
		}
		c := index[f.slug]
		if c == nil {
			c = &judicialCard{slug: f.slug, name: f.name, portrait: portraits[f.slug]}
			if cand := bySlug[f.slug]; cand != nil {
				c.name, c.firstName, c.org, c.orgSlug = cand.Name, cand.FirstName, cand.Organization, cand.OrganizationSlug
			}
			index[f.slug] = c
			order = append(order, f.slug)
		}
		c.decisions = append(c.decisions, f)
	}
	var out []judicialCard
	for _, slug := range order {
		if c := index[slug]; len(c.decisions) > 0 {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return KeySort(out[i].name+" "+out[i].firstName) < KeySort(out[j].name+" "+out[j].firstName)
	})
	return out, nil
}

// judicialHeaviest : la peine la plus lourde parmi les décisions d'un candidat,
// et l'inéligibilité la plus longue rapportée si l'une d'elles en mentionne une.
func judicialHeaviest(c judicialCard) (peine int, inelig string) {
	peine = -1
	for _, f := range c.decisions {
		m, ok := judicialMetas[f.id]
		if !ok {
			continue
		}
		if m.peine > peine {
			peine = m.peine
		}
		if m.inelig != "" && inelig == "" {
			inelig = m.inelig
		}
	}
	return
}

func judicialStepper(current int) string {
	var b strings.Builder
	b.WriteString(`<ol class="jug-stade" aria-label="Stade de la procédure">`)
	for i, label := range stadesJudicial {
		cls := ""
		switch {
		case i == current:
			cls = ` class="now" aria-current="step"`
		case i < current:
			cls = ` class="passe"`
		}
		fmt.Fprintf(&b, `<li%s>%s</li>`, cls, label)
	}
	b.WriteString(`</ol>`)
	return b.String()
}

func judicialSquares(level int) string {
	var b strings.Builder
	b.WriteString(`<span class="jug-echelle" aria-hidden="true">`)
	for i := range peinesJudicial {
		if i <= level {
			b.WriteString(`<i class="on"></i>`)
		} else {
			b.WriteString(`<i></i>`)
		}
	}
	b.WriteString(`</span>`)
	return b.String()
}

func judicialCards(cards []judicialCard, root string) string {
	e := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<div class="jug-lecture"><p><strong>Deux repères, rien de plus.</strong> ` +
		`Le <em>stade</em> dit où en est la procédure, d'après la source citée. La <em>peine</em> ` +
		`est rangée dans l'ordre du code pénal (amende, emprisonnement avec sursis, emprisonnement ferme) ; ` +
		`l'inéligibilité est indiquée à part, elle ne se compare pas à une peine de prison. ` +
		`Ces repères ne mesurent ni la gravité des faits ni la personne : ils décrivent ce qu'une juridiction a décidé. ` +
		`Les candidats sont rangés par ordre alphabétique.</p></div>`)
	b.WriteString(`<div class="jug-liste">`)
	for _, c := range cards {
		fmt.Fprintf(&b, `<article class="jug" id="jug-%s"><header>`, e(c.slug))
		if c.portrait != nil {
			fmt.Fprintf(&b, `<img src="%s/media/%s" alt="" width="72" height="72" loading="lazy" class="portrait">`,
				root, e(c.portrait.File))
		} else {
			b.WriteString(`<div class="portrait vide" style="width:72px;height:72px"></div>`)
		}
		fmt.Fprintf(&b, `<div class="jug-id"><h3><a href="%s/candidat/%s/">%s %s</a></h3>`,
			root, e(c.slug), e(c.firstName), e(c.name))
		if c.org != "" {
			org := e(c.org)
			if c.orgSlug != "" {
				org = `<a href="` + root + `/organisation/` + e(c.orgSlug) + `/">` + org + `</a>`
			}
			fmt.Fprintf(&b, `<p class="jug-org">%s</p>`, org)
		}
		b.WriteString(`</div>`)
		if peine, inelig := judicialHeaviest(c); peine >= 0 {
			fmt.Fprintf(&b, `<div class="jug-peine"><span class="jug-lbl">Peine la plus lourde prononcée</span>%s<span class="jug-val">%s</span>`,
				judicialSquares(peine), peinesJudicial[peine])
			if inelig != "" {
				fmt.Fprintf(&b, `<span class="jug-inelig">+ inéligibilité de %s</span>`, e(inelig))
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</header><ol class="faits-frise jug-decisions">`)
		for _, f := range c.decisions {
			q := libQuality[f.quality]
			m, ok := judicialMetas[f.id]
			fmt.Fprintf(&b, `<li><span class="f-date">%s</span><div><p class="f-t">%s</p>`, e(f.year), e(f.caption))
			if ok {
				b.WriteString(judicialStepper(m.stade))
				fmt.Fprintf(&b, `<p class="jug-faits"><span class="jug-lbl">Faits</span> %s <span class="jug-lbl">Peine</span> %s</p>`,
					e(m.infraction), e(m.detail))
			}
			fmt.Fprintf(&b, `<p class="f-c">%s</p><p class="f-src"><span class="qual %s">%s</span> %s · <a href="%s">source</a></p></div></li>`,
				e(f.finding), q[1], q[0], e(f.author), e(f.url))
		}
		b.WriteString(`</ol></article>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// judicialStades : les stades distincts des décisions d'un candidat, du premier
// au dernier, pour le résumé en tête de page.
func judicialStades(c judicialCard) []string {
	seen := map[int]bool{}
	for _, f := range c.decisions {
		if m, ok := judicialMetas[f.id]; ok {
			seen[m.stade] = true
		}
	}
	var out []string
	for i, label := range stadesJudicial {
		if seen[i] {
			out = append(out, label)
		}
	}
	return out
}

func pluriel(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// judicialSummary : le qui avant le quoi — un portrait, un nom, une organisation,
// le stade et la peine la plus lourde, avec un lien vers la fiche détaillée plus bas.
func judicialSummary(cards []judicialCard, root string) string {
	e := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<ul class="jug-resume">`)
	for _, c := range cards {
		b.WriteString(`<li><a href="#jug-` + e(c.slug) + `">`)
		if c.portrait != nil {
			fmt.Fprintf(&b, `<img src="%s/media/%s" alt="" width="64" height="64" loading="lazy" class="portrait">`,
				root, e(c.portrait.File))
		} else {
			b.WriteString(`<span class="portrait vide"></span>`)
		}
		fmt.Fprintf(&b, `<span class="jug-id"><strong>%s %s</strong>`, e(c.firstName), e(c.name))
		if c.org != "" {
			fmt.Fprintf(&b, `<span class="jug-org">%s</span>`, e(c.org))
		}
		if stades := judicialStades(c); len(stades) > 0 {
			fmt.Fprintf(&b, `<span class="jug-ligne"><span class="jug-lbl">Stade</span> %s</span>`, e(strings.Join(stades, " · ")))
		}
		if peine, inelig := judicialHeaviest(c); peine >= 0 {
			fmt.Fprintf(&b, `<span class="jug-ligne"><span class="jug-lbl">Peine</span> %s%s</span>`,
				judicialSquares(peine), e(peinesJudicial[peine]))
			if inelig != "" {
				fmt.Fprintf(&b, `<span class="jug-ligne"><span class="jug-lbl"></span> + inéligibilité de %s</span>`, e(inelig))
			}
		}
		n := len(c.decisions)
		fmt.Fprintf(&b, `<span class="jug-ligne jug-n">%d décision%s rapportée%s</span></span></a></li>`, n, pluriel(n), pluriel(n))
	}
	b.WriteString(`</ul>`)
	return b.String()
}
