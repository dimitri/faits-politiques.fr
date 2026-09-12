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
var licencesLibres = []string{"cc0", "cc-by", "pd", "publicdomain", "attribution"}

func libre(code string) bool {
	c := strings.ToLower(strings.TrimSpace(code))
	if c == "" || strings.Contains(c, "fair") || strings.Contains(c, "nonfree") {
		return false
	}
	for _, p := range licencesLibres {
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
	http    *http.Client
	dernier time.Time
}

const intervalle = 1200 * time.Millisecond

func (c *client) getJSON(ctx context.Context, endpoint string, params url.Values, out any) error {
	u := endpoint + "?" + params.Encode()
	var last error
	for essai := 1; essai <= 4; essai++ {
		if d := intervalle - time.Since(c.dernier); d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
		c.dernier = time.Now()

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
			case <-time.After(time.Duration(essai*essai) * 2 * time.Second):
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

// Fichier décrit un média retenu.
type Fichier struct {
	Nom, SourceURL, Licence, LicenceCode, Auteur string
	URL                                          string
	Largeur, Hauteur                             int
}

// Resolve retrouve l'image principale d'une page Wikipédia et n'en retourne les
// métadonnées que si sa licence est libre.
func (c *client) Resolve(ctx context.Context, pageFR string, taille int) (*Fichier, string, error) {
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
	var fichier string
	for _, p := range wp.Query.Pages {
		fichier = p.PageImage
	}
	if fichier == "" {
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
		"iiprop": {"extmetadata|url"}, "iiurlwidth": {fmt.Sprint(taille)},
		"titles": {"File:" + fichier}, "format": {"json"}, "formatversion": {"1"},
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
		if !libre(code) {
			return nil, "licence non libre : " + firstNonEmpty(meta("LicenseShortName"), code, "inconnue"), nil
		}
		return &Fichier{
			Nom:         fichier,
			SourceURL:   ii.DescriptionURL,
			Licence:     firstNonEmpty(meta("LicenseShortName"), code),
			LicenceCode: code,
			Auteur:      stripHTML(meta("Artist")),
			URL:         ii.ThumbURL,
			Largeur:     ii.ThumbWidth,
			Hauteur:     ii.ThumbHeight,
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

// Cible désigne le sujet d'un média.
type Cible struct {
	PersonID, OrganizationID *int64
	Kind, PageFR, Libelle    string
}

// Ingest récupère les médias des cibles fournies et les dépose dans mediaDir.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive,
	mediaDir string, cibles []Cible) error {

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
	c := &client{http: &http.Client{Timeout: 60 * time.Second}}

	retenus, refuses := 0, 0
	for _, cible := range cibles {
		if cible.PageFR == "" {
			continue
		}
		taille := 400
		if cible.Kind == "LOGO" {
			taille = 300
		}
		f, raison, err := c.Resolve(ctx, cible.PageFR, taille)
		if err != nil {
			fmt.Printf("    %-34s erreur : %v\n", cible.Libelle, err)
			continue
		}
		if f == nil {
			fmt.Printf("    %-34s écarté — %s\n", cible.Libelle, raison)
			refuses++
			continue
		}
		ext := filepath.Ext(f.Nom)
		if strings.EqualFold(ext, ".svg") {
			ext = ".png" // la vignette Commons d'un SVG est rendue en PNG
		}
		local := slug(cible.Libelle) + "-" + strings.ToLower(cible.Kind) + ext
		if err := download(ctx, c.http, f.URL, filepath.Join(mediaDir, local)); err != nil {
			fmt.Printf("    %-34s erreur de téléchargement : %v\n", cible.Libelle, err)
			continue
		}
		// Reconstruire plutôt que compléter, comme partout ailleurs.
		if _, err := pool.Exec(ctx, `
			DELETE FROM core.media
			 WHERE kind = $3
			   AND (person_id IS NOT DISTINCT FROM $1)
			   AND (organization_id IS NOT DISTINCT FROM $2)`,
			cible.PersonID, cible.OrganizationID, cible.Kind); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.media
			  (person_id, organization_id, kind, fichier, source_url,
			   licence, licence_code, auteur, largeur, hauteur)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10)`,
			cible.PersonID, cible.OrganizationID, cible.Kind, local, f.SourceURL,
			f.Licence, f.LicenceCode, f.Auteur, f.Largeur, f.Hauteur); err != nil {
			return err
		}
		retenus++
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"retenus": retenus, "ecartes": refuses}, "")
	fmt.Printf("  médias        %d retenus, %d écartés faute de licence libre\n", retenus, refuses)
	return nil
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
