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

var SourceActeursNumeriques = archive.Source{
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

// Acteur : une société ou un organisme nommé par un dossier, et la preuve de
// son rattachement. Verif : "SIRENE" (Motif sur la dénomination de l'unité
// légale active), "SECNUMCLOUD" (au moins un service qualifié au dernier
// catalogue), "PAGE" (phrases attendues sur URL).
type Acteur struct {
	Siren, Nom, Categorie, Groupe string
	Fondement, Qualite, URL       string
	Verif, Motif                  string
	Attendus                      []string
}

var acteurs []Acteur

func IngestActeurs(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceActeursNumeriques, func(srcID, runID int64) (map[string]any, error) {
		pages := map[string]string{}
		par := map[string]int{}
		for _, a := range acteurs {
			var denom, etat string
			if err := pool.QueryRow(ctx, `SELECT coalesce(denomination,''), etat_administratif FROM ref.unite_legale WHERE siren = $1`,
				a.Siren).Scan(&denom, &etat); err != nil {
				return nil, fmt.Errorf("%s (%s) : unité légale absente de Sirene (charger -only=sirene) : %w", a.Nom, a.Siren, err)
			}
			if etat != "A" {
				return nil, fmt.Errorf("%s (%s) : unité légale cessée", a.Nom, a.Siren)
			}
			switch a.Verif {
			case "SIRENE":
				if !regexp.MustCompile("(?i)" + a.Motif).MatchString(denom) {
					return nil, fmt.Errorf("%s (%s) : dénomination Sirene « %s » ne correspond pas", a.Nom, a.Siren, denom)
				}
			case "SECNUMCLOUD":
				var n int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.qualification_secnumcloud WHERE siren = $1
					AND catalogue_du = (SELECT max(catalogue_du) FROM core.qualification_secnumcloud)`, a.Siren).Scan(&n); err != nil {
					return nil, err
				}
				if n == 0 {
					return nil, fmt.Errorf("%s (%s) : aucun service qualifié au dernier catalogue (charger -only=numerique-anssi)", a.Nom, a.Siren)
				}
			case "PAGE":
				t, ok := pages[a.URL]
				if !ok {
					f, err := arch.Fetch(ctx, srcID, runID, a.URL, ".html")
					if err != nil {
						return nil, fmt.Errorf("%s : %w", a.Nom, err)
					}
					b, err := os.ReadFile(f.Path)
					if err != nil {
						return nil, err
					}
					t = texteHTML(string(b))
					pages[a.URL] = t
				}
				for _, at := range a.Attendus {
					if !strings.Contains(t, normaliser(at)) {
						return nil, fmt.Errorf("%s : « %s » absent de %s", a.Nom, at, a.URL)
					}
				}
			default:
				return nil, fmt.Errorf("%s : vérification %q inconnue", a.Nom, a.Verif)
			}
			par[a.Categorie]++
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM ref.acteur_numerique`); err != nil {
			return nil, err
		}
		for _, a := range acteurs {
			if _, err := tx.Exec(ctx, `INSERT INTO ref.acteur_numerique (siren, nom, categorie, groupe, fondement, qualite, source_url)
				VALUES ($1,$2,$3,$4,$5,$6,$7)`, a.Siren, a.Nom, a.Categorie, nul(a.Groupe), a.Fondement, a.Qualite, a.URL); err != nil {
				return nil, fmt.Errorf("%s : %w", a.Nom, err)
			}
		}
		stats := map[string]any{"acteurs": len(acteurs), "pages": len(pages)}
		for c, n := range par {
			stats[c] = n
		}
		return stats, tx.Commit(ctx)
	})
}
