package sitegen

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Le plan du site : une entrée par page HTML effectivement écrite, jamais une
// liste recalculée séparément qui pourrait diverger de ce qui est publié. La
// source de vérité est le système de fichiers de out/ une fois la
// construction terminée — pas une deuxième comptabilité tenue au fil des
// centaines d'appels à writeAlways() dans ce paquet.
//
// À 78 000 pages, un fichier unique dépasserait la limite de 50 000 URL par
// plan que Google impose ; ce site n'a d'ailleurs aucune section qui, seule,
// l'approche. Un plan par section (scrutin, collectivites, depute…), référencé
// par un index — sitemap.xml lui-même — est donc à la fois nécessaire et
// naturel : chaque section reste sous la limite sans découpage arbitraire.

type urlEntry struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod,omitempty"`
}
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	Xmlns   string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}
type sitemapRef struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod,omitempty"`
}
type sitemapIndex struct {
	XMLName  xml.Name     `xml:"sitemapindex"`
	Xmlns    string       `xml:"xmlns,attr"`
	Sitemaps []sitemapRef `xml:"sitemap"`
}

const nsSitemap = "http://www.sitemaps.org/schemas/sitemap/0.9"

// writeSitemap parcourt out/ après que toutes les pages ont été écrites, et
// regroupe chaque index.html trouvé par sa première section d'URL. Renvoie le
// nombre total d'URL référencées.
func writeSitemap(out, canonicalBase string) (int, error) {
	sections := map[string][]urlEntry{}
	total := 0
	err := filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Base(p) != "index.html" {
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimSuffix(p, "index.html"), out)
		rel = "/" + strings.Trim(rel, "/")
		if rel != "/" {
			rel += "/"
		}
		section := "accueil"
		if seg := strings.SplitN(strings.TrimPrefix(rel, "/"), "/", 2); seg[0] != "" {
			section = seg[0]
		}
		sections[section] = append(sections[section], urlEntry{
			Loc:     canonicalBase + rel,
			Lastmod: info.ModTime().Format("2006-01-02"),
		})
		total++
		return nil
	})
	if err != nil {
		return 0, err
	}

	var noms []string
	for s := range sections {
		noms = append(noms, s)
	}
	sort.Strings(noms)

	var idx sitemapIndex
	idx.Xmlns = nsSitemap
	for _, s := range noms {
		urls := sections[s]
		sort.Slice(urls, func(i, j int) bool { return urls[i].Loc < urls[j].Loc })
		if err := writeXML(filepath.Join(out, "sitemap-"+s+".xml"),
			urlSet{Xmlns: nsSitemap, URLs: urls}); err != nil {
			return 0, err
		}
		idx.Sitemaps = append(idx.Sitemaps, sitemapRef{
			Loc: canonicalBase + "/sitemap-" + s + ".xml",
		})
	}
	if err := writeXML(filepath.Join(out, "sitemap.xml"), idx); err != nil {
		return 0, err
	}
	return total, nil
}

func writeXML(path string, v any) error {
	b, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	body := append([]byte(xml.Header), b...)
	return os.WriteFile(path, body, 0o644)
}

// placeholderCanonical : voir le commentaire de ce jeton dans
// web/templates/base.gohtml — jamais inséré par un gabarit avec l'URL déjà
// résolue, parce qu'aucun gabarit ne connaît son propre chemin de
// publication.
const placeholderCanonical = "__FP_CANONICAL__"

// rewriteCanonical reprend le même parcours que writeSitemap (chaque
// index.html de out/, après que tout a été écrit) pour remplacer le jeton
// posé par base.gohtml par l'URL canonique réellement déduite du chemin de
// fichier — jamais une deuxième fonction qui recalculerait cette URL
// autrement et pourrait un jour diverger de celle du plan du site.
//
// 404.html n'a pas d'URL publique à s'attribuer (ce n'est la page d'aucune
// adresse précise) : filepath.Base(p) != "index.html" l'écarte déjà du même
// coup que writeSitemap, donc son jeton reste en l'état — ÔTÉ explicitement
// plus bas plutôt que publié tel quel, qui romprait le HTML.
func rewriteCanonical(out, canonicalBase string) (int, error) {
	n := 0
	err := filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(p)
		if base != "index.html" && base != "404.html" {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), placeholderCanonical) {
			return nil
		}
		if base == "404.html" {
			// Pas de balise canonique du tout plutôt qu'une URL inventée :
			// supprime la ligne entière, elle est seule sur la sienne.
			lines := strings.Split(string(body), "\n")
			kept := lines[:0]
			for _, line := range lines {
				if !strings.Contains(line, placeholderCanonical) {
					kept = append(kept, line)
				}
			}
			return os.WriteFile(p, []byte(strings.Join(kept, "\n")), 0o644)
		}
		rel := strings.TrimPrefix(strings.TrimSuffix(p, "index.html"), out)
		rel = "/" + strings.Trim(rel, "/")
		if rel != "/" {
			rel += "/"
		}
		rewritten := strings.ReplaceAll(string(body), placeholderCanonical, canonicalBase+rel)
		if err := os.WriteFile(p, []byte(rewritten), 0o644); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

// writeRobots écrit une politique délibérément permissive : ce site n'a ni
// recherche à facettes, ni panier, ni espace privé — rien dont l'exploration
// gaspillerait le budget de Google. Seule exception : les images de partage
// social (internal/sitegen/social.go), générées pour Slack/X/iMessage, jamais pour
// la recherche d'images — les y exposer n'apporterait rien et ressemblerait à
// une série d'images quasi identiques.
func writeRobots(out, canonicalBase string) error {
	body := fmt.Sprintf(`User-agent: *
Allow: /
Disallow: /media/og/

Sitemap: %s/sitemap.xml
`, canonicalBase)
	return os.WriteFile(filepath.Join(out, "robots.txt"), []byte(body), 0o644)
}
