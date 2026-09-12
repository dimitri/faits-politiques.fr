package main

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
	Slug, Titre, Fichier string
	Corps                template.HTML
	Sections             []DocSection
	Mots                 int
}

type DocSection struct{ ID, Titre string }

var (
	reTitre   = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	reH2      = regexp.MustCompile(`<h2 id="([^"]+)">(.*?)</h2>`)
	reBalises = regexp.MustCompile(`<[^>]+>`)
)

// ordreDocs : du plus utile au lecteur au plus interne. Un répertoire trié par
// nom de fichier mettrait « architecture » avant « périmètre », ce qui est
// l'inverse de ce qu'on vient chercher.
var ordreDocs = map[string]int{
	"perimetre":                  1,
	"charte-graphique":           2,
	"decisions":                  3,
	"architecture":               4,
	"contributions-utilisateurs": 5,
	"themes-conception":          6,
	"mairies-conception":         7,
	"pre-enregistrement-001":     8,
	"pre-enregistrement-002":     9,
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
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := md.Convert(src, &buf); err != nil {
			return nil, err
		}
		corps := buf.String()

		// Les tableaux larges défilent dans leur propre conteneur : le corps de
		// page ne défile jamais latéralement.
		corps = strings.ReplaceAll(corps, "<table>", `<div class="scroll"><table>`)
		corps = strings.ReplaceAll(corps, "</table>", `</table></div>`)

		d := &Doc{
			Slug:    strings.TrimSuffix(e.Name(), ".md"),
			Fichier: e.Name(),
			Corps:   template.HTML(corps),
			Mots:    len(strings.Fields(reBalises.ReplaceAllString(corps, " "))),
		}
		if m := reTitre.FindSubmatch(src); m != nil {
			d.Titre = string(m[1])
		} else {
			d.Titre = d.Slug
		}
		for _, m := range reH2.FindAllStringSubmatch(corps, -1) {
			d.Sections = append(d.Sections, DocSection{
				ID: m[1], Titre: strings.TrimSpace(reBalises.ReplaceAllString(m[2], "")),
			})
		}
		docs = append(docs, d)
	}

	sort.Slice(docs, func(i, j int) bool {
		a, oka := ordreDocs[docs[i].Slug]
		b, okb := ordreDocs[docs[j].Slug]
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
