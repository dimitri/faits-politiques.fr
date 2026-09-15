// Package numerique charge ce qui documente la souveraineté numérique de
// l'État : les services Cloud qualifiés par l'ANSSI, les sanctions de la
// CNIL, l'ensemble des marchés publics informatiques et les faits établis
// (textes, jurisprudence, constats d'enquête parlementaire).
// Voir docs/souverainete-numerique.md.
package numerique

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "numerique-v1"

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	for _, e := range []func(context.Context, *pgxpool.Pool, *archive.Archive) error{
		IngestQualifications, IngestSanctionsCNIL, IngestSILL, IngestMarches,
	} {
		if err := e(ctx, pool, arch); err != nil {
			return err
		}
	}
	return nil
}

func executer(ctx context.Context, arch *archive.Archive, src archive.Source,
	f func(srcID, runID int64) (map[string]any, error)) error {
	srcID, err := arch.EnsureSource(ctx, src)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	stats, err := f(srcID, runID)
	if err != nil {
		err = fmt.Errorf("%s : %w", src.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	fmt.Printf("  %-34s %v\n", src.Slug, stats)
	return nil
}

var (
	reBalises = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	reBlancs  = regexp.MustCompile(`\s+`)
)

// normaliser ramène espaces insécables et retours à la ligne à une espace, pour
// comparer un texte scellé à la phrase qu'on lui fait dire.
func normaliser(s string) string {
	s = strings.NewReplacer("\u00a0", " ", "\u202f", " ", "\u2009", " ").Replace(s)
	return strings.TrimSpace(reBlancs.ReplaceAllString(s, " "))
}

func texteHTML(fragment string) string {
	return normaliser(html.UnescapeString(reBalises.ReplaceAllString(fragment, " ")))
}

// textePDF passe par pdftotext (poppler-utils) : le projet n'embarque pas de
// lecteur PDF, et les deux documents lus ici (catalogue de l'ANSSI, rapport du
// Sénat) n'existent qu'en PDF. Comme 7z pour la SAE, l'outil est exigé
// explicitement plutôt que contourné.
func textePDF(ctx context.Context, path string, disposition bool) (string, error) {
	args := []string{path, "-"}
	if disposition {
		args = []string{"-layout", path, "-"}
	}
	out, err := exec.CommandContext(ctx, "pdftotext", args...).Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext (paquet poppler-utils) : %w", err)
	}
	return string(out), nil
}

func nul(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Lecture en flux d'un CSV avec en-tête, comme pour les marchés de l'évasion
// fiscale : le fichier consolidé dépasse 2,5 Go.
func lireCSVFlux(path string, f func(col map[string]int, rec []string) error) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	cr := csv.NewReader(bufio.NewReaderSize(fh, 1<<20))
	cr.ReuseRecord = true
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	entete, err := cr.Read()
	if err != nil {
		return err
	}
	col := map[string]int{}
	for i, c := range entete {
		col[strings.TrimPrefix(c, "\uFEFF")] = i
	}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(rec) < len(entete) {
			continue
		}
		if err := f(col, rec); err != nil {
			return err
		}
	}
}

func ressourceDataGouv(path, titre string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var d struct {
		Resources []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return "", err
	}
	for _, r := range d.Resources {
		if r.Title == titre {
			return r.URL, nil
		}
	}
	return "", fmt.Errorf("ressource %q absente du jeu de données", titre)
}
