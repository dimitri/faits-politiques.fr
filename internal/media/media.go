// Package media récupère portraits et logos sous licence libre.
//
// Ce site ne republie QUE des fichiers librement réutilisables, et affiche
// toujours leur licence et leur auteur. Les photographies officielles des sites
// de campagne et la plupart des logos de partis sont protégés : les reproduire
// serait une contrefaçon, et contredirait la discipline de licences appliquée à
// toutes les autres données du projet. Quand aucun fichier libre n'existe,
// l'absence est affichée comme telle.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Source = archive.Source{
	Slug: "wikimedia-commons", Label: "Wikimedia Commons — portraits et logos libres",
	Publisher: "Wikimedia Commons", Tier: "DECLARATIVE",
	Licence:     "variable, par fichier ; seules les licences libres sont retenues",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Portraits et logos : Wikimedia Commons, licence et auteur indiqués sur chaque image",
	Cadence:     "à la demande",
	Notes: "Un fichier hébergé localement sur Wikipédia plutôt que sur Commons est " +
		"presque toujours non libre : il est alors ignoré.",
}

// Préfixes de licences acceptées. Tout le reste est refusé — y compris les
// « fair use » et les logos sous droit d'auteur tolérés sur Wikipédia.
var freeLicenses = []string{"cc0", "cc-by", "pd", "publicdomain", "attribution"}

func isFreeLicense(code string) bool {
	c := strings.ToLower(strings.TrimSpace(code))
	if c == "" || strings.Contains(c, "fair") || strings.Contains(c, "nonfree") {
		return false
	}
	for _, p := range freeLicenses {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

// Les API Wikimédia limitent le débit et répondent 429 au-delà. On espace les
// requêtes et on réessaie : marteler un service public gratuit serait à la fois
// inefficace et malpoli.
type client struct {
	http *http.Client
	last time.Time
}

const interval = 1200 * time.Millisecond

func (c *client) getJSON(ctx context.Context, endpoint string, params url.Values, out any) error {
	u := endpoint + "?" + params.Encode()
	var last error
	for attempt := 1; attempt <= 4; attempt++ {
		if d := interval - time.Since(c.last); d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
		c.last = time.Now()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "faits-politiques.fr (ingestion média, contact via le dépôt)")
		resp, err := c.http.Do(req)
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			last = fmt.Errorf("%s : HTTP %d", endpoint, resp.StatusCode)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 2 * time.Second):
			}
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("%s : HTTP %d", endpoint, resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return last
}

// File décrit un média retenu.
type File struct {
	Name, SourceURL, Licence, LicenceCode, Author string
	URL                                           string
	Width, Height                                 int
}

// Resolve retrouve l'image principale d'une page Wikipédia et n'en retourne les
// métadonnées que si sa licence est libre.
func (c *client) Resolve(ctx context.Context, pageFR string, size int) (*File, string, error) {
	var wp struct {
		Query struct {
			Pages map[string]struct {
				PageImage string `json:"pageimage"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, "https://fr.wikipedia.org/w/api.php", url.Values{
		"action": {"query"}, "prop": {"pageimages"}, "titles": {pageFR},
		"format": {"json"}, "formatversion": {"1"},
	}, &wp); err != nil {
		return nil, "", err
	}
	var fileName string
	for _, p := range wp.Query.Pages {
		fileName = p.PageImage
	}
	if fileName == "" {
		return nil, "aucune image sur la page Wikipédia", nil
	}

	var cm struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					ThumbURL       string `json:"thumburl"`
					ThumbWidth     int    `json:"thumbwidth"`
					ThumbHeight    int    `json:"thumbheight"`
					DescriptionURL string `json:"descriptionurl"`
					ExtMetadata    map[string]struct {
						Value any `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, "https://commons.wikimedia.org/w/api.php", url.Values{
		"action": {"query"}, "prop": {"imageinfo"},
		"iiprop": {"extmetadata|url"}, "iiurlwidth": {fmt.Sprint(size)},
		"titles": {"File:" + fileName}, "format": {"json"}, "formatversion": {"1"},
	}, &cm); err != nil {
		return nil, "", err
	}
	for _, p := range cm.Query.Pages {
		if len(p.ImageInfo) == 0 {
			// Fichier hébergé localement sur Wikipédia, pas sur Commons :
			// presque toujours un logo sous droit d'auteur.
			return nil, "fichier absent de Commons, présumé non libre", nil
		}
		ii := p.ImageInfo[0]
		meta := func(k string) string {
			if v, ok := ii.ExtMetadata[k]; ok {
				return strings.TrimSpace(fmt.Sprint(v.Value))
			}
			return ""
		}
		code := meta("License")
		if !isFreeLicense(code) {
			return nil, "licence non libre : " + firstNonEmpty(meta("LicenseShortName"), code, "inconnue"), nil
		}
		return &File{
			Name:        fileName,
			SourceURL:   ii.DescriptionURL,
			Licence:     firstNonEmpty(meta("LicenseShortName"), code),
			LicenceCode: code,
			Author:      stripHTML(meta("Artist")),
			URL:         ii.ThumbURL,
			Width:       ii.ThumbWidth,
			Height:      ii.ThumbHeight,
		}, "", nil
	}
	return nil, "fichier introuvable", nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func stripHTML(s string) string {
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case r == '<':
			skip = true
		case r == '>':
			skip = false
		case !skip:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Target désigne le sujet d'un média.
type Target struct {
	PersonID, OrganizationID *int64
	Kind, PageFR, Label      string
}

// Ingest récupère les médias des cibles fournies et les dépose dans mediaDir.
// cachePath : voir cache.go — résolutions déjà vues (positives ou négatives),
// relues puis réécrites à chaque run, pour ne réinterroger Wikimédia que pour
// une cible nouvelle ou dont le fichier local a disparu.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive,
	mediaDir, cachePath string, targets []Target) error {

	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "media/1")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return err
	}
	cch, err := loadCache(cachePath)
	if err != nil {
		return fmt.Errorf("cache média %s : %w", cachePath, err)
	}
	c := &client{http: &http.Client{Timeout: 60 * time.Second}}

	kept, rejected, fromCache := 0, 0, 0
	for _, target := range targets {
		if target.PageFR == "" {
			continue
		}
		key := cacheKey(target.PageFR, target.Kind)

		if e, ok := cch.Entries[key]; ok {
			if e.Rejected {
				rejected++
				fromCache++
				continue
			}
			if _, err := os.Stat(filepath.Join(mediaDir, e.Local)); err == nil {
				if err := insert(ctx, pool, target, e.Local, e.SourceURL,
					e.Licence, e.LicenceCode, e.Author, e.Width, e.Height); err != nil {
					return err
				}
				kept++
				fromCache++
				continue
			}
			// Le fichier a disparu de mediaDir (nettoyage manuel, clone
			// partiel) : l'entrée ne vaut plus rien, on la résout en direct
			// comme une cible nouvelle.
		}

		size := 400
		if target.Kind == "LOGO" {
			size = 300
		}
		f, reason, err := c.Resolve(ctx, target.PageFR, size)
		if err != nil {
			fmt.Printf("    %-34s erreur : %v\n", target.Label, err)
			continue
		}
		if f == nil {
			fmt.Printf("    %-34s écarté — %s\n", target.Label, reason)
			rejected++
			cch.Entries[key] = cacheEntry{Rejected: true, Reason: reason}
			continue
		}
		ext := filepath.Ext(f.Name)
		if strings.EqualFold(ext, ".svg") {
			ext = ".png" // la vignette Commons d'un SVG est rendue en PNG
		}
		local := slug(target.Label) + "-" + strings.ToLower(target.Kind) + ext
		if err := download(ctx, c.http, f.URL, filepath.Join(mediaDir, local)); err != nil {
			fmt.Printf("    %-34s erreur de téléchargement : %v\n", target.Label, err)
			continue
		}
		if err := insert(ctx, pool, target, local, f.SourceURL,
			f.Licence, f.LicenceCode, f.Author, f.Width, f.Height); err != nil {
			return err
		}
		cch.Entries[key] = cacheEntry{
			Name: f.Name, SourceURL: f.SourceURL, Licence: f.Licence, LicenceCode: f.LicenceCode,
			Author: f.Author, URL: f.URL, Width: f.Width, Height: f.Height, Local: local,
		}
		kept++
	}
	if err := cch.save(cachePath); err != nil {
		return fmt.Errorf("cache média %s : %w", cachePath, err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"retenus": kept, "ecartes": rejected, "depuis_cache": fromCache}, "")
	fmt.Printf("  médias        %d retenus, %d écartés faute de licence libre (%d depuis le cache)\n",
		kept, rejected, fromCache)
	return nil
}

// insert : reconstruit plutôt que compléter, comme partout ailleurs —
// factorisé parce que le chemin cache et le chemin résolution en direct
// écrivent tous deux la même ligne, jamais deux copies divergentes de ce SQL.
func insert(ctx context.Context, pool *pgxpool.Pool, target Target,
	local, sourceURL, licence, licenceCode, author string, width, height int) error {
	if _, err := pool.Exec(ctx, `
		DELETE FROM core.media
		 WHERE kind = $3
		   AND (person_id IS NOT DISTINCT FROM $1)
		   AND (organization_id IS NOT DISTINCT FROM $2)`,
		target.PersonID, target.OrganizationID, target.Kind); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO core.media
		  (person_id, organization_id, kind, fichier, source_url,
		   licence, licence_code, auteur, largeur, hauteur)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10)`,
		target.PersonID, target.OrganizationID, target.Kind, local, sourceURL,
		licence, licenceCode, author, width, height)
	return err
}

func download(ctx context.Context, hc *http.Client, src, dst string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	req.Header.Set("User-Agent", "faits-politiques.fr (ingestion média)")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func slug(s string) string {
	repl := strings.NewReplacer("à", "a", "â", "a", "é", "e", "è", "e", "ê", "e",
		"ë", "e", "î", "i", "ï", "i", "ô", "o", "ö", "o", "ù", "u", "û", "u",
		"ü", "u", "ç", "c", "œ", "oe", "æ", "ae")
	s = repl.Replace(strings.ToLower(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
