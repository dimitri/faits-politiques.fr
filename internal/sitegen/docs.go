package sitegen

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// Les documents de méthode sont la moitié de ce que ce site promet : « la
// source primaire, et ce qu'elle ne permet pas de conclure ». Les laisser dans
// le dépôt sous forme de chemins de fichiers — « voir docs/perimetre.md » —
// revenait à les réserver à qui sait cloner un dépôt Git. Ils sont désormais
// des pages du site.
type Doc struct {
	Slug, Title, File string
	Body              template.HTML
	Sections          []DocSection
	Words             int
	Family            string
}

// GroupDocs : les documents rangés par famille, dans l'ordre de familles.
// Dix-huit cartes en une grille plate, triées par un ordre que le lecteur ne
// devine pas, c'est un mur — la note sur le budget de l'État s'y perdait au
// dixième rang.
type GroupDocs struct {
	Family, Intro string
	Docs          []*Doc
}

type DocSection struct{ ID, Title string }

var (
	reTitle = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	reH2    = regexp.MustCompile(`<h2 id="([^"]+)">(.*?)</h2>`)
	reTags  = regexp.MustCompile(`<[^>]+>`)
)

// ordreDocs : du plus utile au lecteur au plus interne. Un répertoire trié par
// nom de fichier mettrait « architecture » avant « périmètre », ce qui est
// l'inverse de ce qu'on vient chercher.
var orderDocs = map[string]int{
	"perimetre":                  1,
	"charte-graphique":           2,
	"architecture":               3,
	"contributions-utilisateurs": 4,
	"monnaie-et-inflation":       5,
	"budget-donnees":             6,
	"gouvernement-donnees":       7,
	"candidats-donnees":          8,
	"securite-conception":        9,
	"mairies-conception":         10,
	"themes-conception":          11,
	"scrutins-et-bulletins":      12,
	"enrichissement-conception":  13,
	"pre-enregistrement-001":     14,
	"pre-enregistrement-002":     15,
}

// familleDocs : à quoi sert le document, du point de vue du lecteur — pas de
// celui du dépôt. Un document sans entrée tombe dans « Autres notes », ce qui
// se voit et invite à le ranger.
var familyDocs = map[string]string{
	"perimetre":                  "Les règles du site",
	"charte-graphique":           "Les règles du site",
	"architecture":               "Les règles du site",
	"contributions-utilisateurs": "Les règles du site",
	"monnaie-et-inflation":       "Lire les chiffres d'argent",
	"budget-donnees":             "Lire les chiffres d'argent",
	"gouvernement-donnees":       "Ce que chaque source permet",
	"candidats-donnees":          "Ce que chaque source permet",
	"securite-conception":        "Ce que chaque source permet",
	"mairies-conception":         "Ce que chaque source permet",
	"themes-conception":          "Ce que chaque source permet",
	"scrutins-et-bulletins":      "Ce que chaque source permet",
	"enrichissement-conception":  "Chantiers en cours",
	"pre-enregistrement-001":     "Chantiers en cours",
	"pre-enregistrement-002":     "Chantiers en cours",
}

var orderFamilies = []struct{ name, intro string }{
	{"Les règles du site", "Ce que le site s'interdit, comment il est construit, et à quoi ressemble ce qu'il publie."},
	{"Lire les chiffres d'argent", "Budgets, comptes et montants : les pièges de lecture, avant les chiffres eux-mêmes."},
	{"Ce que chaque source permet", "Source par source : ce qu'elle contient, ce qu'elle ne contient pas, et pourquoi certaines pages s'arrêtent où elles s'arrêtent."},
	{"Chantiers en cours", "Ce qui est décidé mais pas encore fait, et les hypothèses posées avant de regarder les données."},
	{"Autres notes", ""},
}

// GroupDocsByFamily range les documents chargés par famille, familles vides omises.
func GroupDocsByFamily(docs []*Doc) []GroupDocs {
	var out []GroupDocs
	for _, f := range orderFamilies {
		g := GroupDocs{Family: f.name, Intro: f.intro}
		for _, d := range docs {
			if d.Family == f.name {
				g.Docs = append(g.Docs, d)
			}
		}
		if len(g.Docs) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// horsSite : présents dans docs/, volontairement non publiés. Les nommer ici
// plutôt que les renommer ou les déplacer garde le document versionné là où le
// dépôt l'attend, et la raison de son absence à côté de son nom.
var outsideSite = map[string]string{
	"decisions": "journal des arbitrages — retiré du site le temps d'en revoir la forme",
	"README":    "index du répertoire docs/, sans objet hors du dépôt",
}

func loadDocs(dir string) ([]*Doc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)

	var docs []*Doc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if _, outside := outsideSite[strings.TrimSuffix(e.Name(), ".md")]; outside {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := md.Convert(src, &buf); err != nil {
			return nil, err
		}
		body := buf.String()

		// Les tableaux larges défilent dans leur propre conteneur : le corps de
		// page ne défile jamais latéralement.
		body = strings.ReplaceAll(body, "<table>", `<div class="scroll"><table>`)
		body = strings.ReplaceAll(body, "</table>", `</table></div>`)

		d := &Doc{
			Slug:  strings.TrimSuffix(e.Name(), ".md"),
			File:  e.Name(),
			Body:  template.HTML(body),
			Words: len(strings.Fields(reTags.ReplaceAllString(body, " "))),
		}
		if m := reTitle.FindSubmatch(src); m != nil {
			d.Title = string(m[1])
		} else {
			d.Title = d.Slug
		}
		d.Family = familyDocs[d.Slug]
		if d.Family == "" {
			d.Family = "Autres notes"
		}
		for _, m := range reH2.FindAllStringSubmatch(body, -1) {
			d.Sections = append(d.Sections, DocSection{
				ID: m[1], Title: strings.TrimSpace(reTags.ReplaceAllString(m[2], "")),
			})
		}
		docs = append(docs, d)
	}

	sort.Slice(docs, func(i, j int) bool {
		a, oka := orderDocs[docs[i].Slug]
		b, okb := orderDocs[docs[j].Slug]
		if oka != okb {
			return oka
		}
		if a != b {
			return a < b
		}
		return docs[i].Slug < docs[j].Slug
	})
	return docs, nil
}
