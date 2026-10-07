package dossiers

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceActorsNumeriques = archive.Source{
	Slug: "acteurs-numeriques-francais", Label: "Acteurs français du numérique nommés par le dossier souveraineté",
	Publisher:   "Répertoire Sirene (Insee), catalogue de l'ANSSI, sites des organismes cités",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Sirene : licence ouverte ; pages d'organismes citées sans reproduction",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Fondement cité acteur par acteur (colonne source_url)",
	Cadence:     "au fil du dossier",
	Notes: "Chaque rattachement est vérifié au chargement : dénomination de l'unité légale Sirene, qualification " +
		"SecNumCloud au catalogue chargé, ou phrase attendue sur la page de l'organisme (qualité DECLARATIF : " +
		"l'organisme se présente ainsi).",
}

// Actor : une société ou un organisme nommé par un dossier, et la preuve de
// son rattachement. Check : "SIRENE" (Pattern sur la dénomination de l'unité
// légale active), "SECNUMCLOUD" (au moins un service qualifié au dernier
// catalogue), "PAGE" (phrases attendues sur URL).
type Actor struct {
	Siren, Name, Category, Group string
	Basis, Quality, URL          string
	Check, Pattern               string
	Expected                     []string
}

var actors []Actor

func IngestActors(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return run(ctx, arch, SourceActorsNumeriques, func(srcID, runID int64) (map[string]any, error) {
		pages := map[string]string{}
		byCategory := map[string]int{}
		for _, a := range actors {
			var denom, status string
			if err := pool.QueryRow(ctx, `SELECT coalesce(denomination,''), etat_administratif FROM ref.unite_legale WHERE siren = $1`,
				a.Siren).Scan(&denom, &status); err != nil {
				return nil, fmt.Errorf("%s (%s) : unité légale absente de Sirene (charger -only=sirene) : %w", a.Name, a.Siren, err)
			}
			if status != "A" {
				return nil, fmt.Errorf("%s (%s) : unité légale cessée", a.Name, a.Siren)
			}
			switch a.Check {
			case "SIRENE":
				if !regexp.MustCompile("(?i)" + a.Pattern).MatchString(denom) {
					return nil, fmt.Errorf("%s (%s) : dénomination Sirene « %s » ne correspond pas", a.Name, a.Siren, denom)
				}
			case "SECNUMCLOUD":
				var n int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.qualification_secnumcloud WHERE siren = $1
					AND catalogue_du = (SELECT max(catalogue_du) FROM core.qualification_secnumcloud)`, a.Siren).Scan(&n); err != nil {
					return nil, err
				}
				if n == 0 {
					return nil, fmt.Errorf("%s (%s) : aucun service qualifié au dernier catalogue (charger -only=numerique-anssi)", a.Name, a.Siren)
				}
			case "PAGE":
				t, ok := pages[a.URL]
				if !ok {
					f, err := arch.Fetch(ctx, srcID, runID, a.URL, ".html")
					if err != nil {
						return nil, fmt.Errorf("%s : %w", a.Name, err)
					}
					b, err := os.ReadFile(f.Path)
					if err != nil {
						return nil, err
					}
					t = textFromHTML(string(b))
					pages[a.URL] = t
				}
				for _, at := range a.Expected {
					if !strings.Contains(t, normalize(at)) {
						return nil, fmt.Errorf("%s : « %s » absent de %s", a.Name, at, a.URL)
					}
				}
			default:
				return nil, fmt.Errorf("%s : vérification %q inconnue", a.Name, a.Check)
			}
			byCategory[a.Category]++
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM ref.acteur_numerique`); err != nil {
			return nil, err
		}
		for _, a := range actors {
			if _, err := tx.Exec(ctx, `INSERT INTO ref.acteur_numerique (siren, nom, categorie, groupe, fondement, qualite, source_url)
				VALUES ($1,$2,$3,$4,$5,$6,$7)`, a.Siren, a.Name, a.Category, nullable(a.Group), a.Basis, a.Quality, a.URL); err != nil {
				return nil, fmt.Errorf("%s : %w", a.Name, err)
			}
		}
		stats := map[string]any{"actors": len(actors), "pages": len(pages)}
		for c, n := range byCategory {
			stats[c] = n
		}
		return stats, tx.Commit(ctx)
	})
}
