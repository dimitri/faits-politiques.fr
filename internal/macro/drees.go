package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La DREES publie le suivi mensuel des prestations de solidarité : combien de
// personnes dépendent du RSA, de l'AAH, de la prime d'activité ou de l'ASS, et
// dans quel département. C'est le pendant territorial et mensuel des comptes de
// la protection sociale, qui sont nationaux et annuels.
var SourceDREESPrestations = archive.Source{
	Slug: "drees-prestations-solidarite", Label: "DREES — suivi mensuel des prestations de solidarité",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : DREES, suivi mensuel des prestations de solidarité",
	Cadence:     "mensuelle",
	Notes: "Trois échelles géographiques cohabitent dans le même fichier (France, " +
		"région, département) : sommer sans filtrer compterait chaque allocataire " +
		"trois fois. La colonne maturite distingue les valeurs définitives des " +
		"valeurs provisoires, que la DREES révise ensuite.",
}

// L'API Opendatasoft refuse offset + limit > 10 000 et renvoie un HTTP 400.
// Ce jeu en compte plus de 120 000 : la pagination échouerait à mi-parcours,
// silencieusement si l'on ne vérifiait pas le code de retour. L'export en un
// seul appel est le seul chemin correct.
const dreesPrestationsURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/donnees-mensuelles-sur-les-prestations-de-solidarite/exports/json"

type dreesLigne struct {
	Mois           string   `json:"mois"`
	Serie          string   `json:"serie"`
	NomSerie       string   `json:"nom_serie"`
	NomRegion      string   `json:"nom_region"`
	Region         string   `json:"region"`
	NomDepartement string   `json:"nom_departement"`
	Departement    string   `json:"departement"`
	Valeur         *float64 `json:"valeur"`
	Maturite       string   `json:"maturite"`
	Unite          string   `json:"unite"`
	Commentaire    string   `json:"commentaire"`
}

func IngestPrestationsSolidarite(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDREESPrestations)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, dreesPrestationsURL, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var lignes []dreesLigne
	if err := json.Unmarshal(raw, &lignes); err != nil {
		return fail(fmt.Errorf("export DREES illisible : %w", err))
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("export DREES vide"))
	}

	var rows [][]any
	vu := map[string]bool{}
	var sansValeur, sansDate int
	for _, l := range lignes {
		// Une valeur absente n'est pas un zéro : la DREES ne publie pas toujours
		// le détail départemental d'une série. On l'écarte plutôt que de la
		// transformer en « aucun allocataire ».
		if l.Valeur == nil {
			sansValeur++
			continue
		}
		// La source date au mois, pas au jour : « 2026-06 ». On ancre au
		// premier du mois plutôt que d'inventer une date de publication.
		mois, err := time.Parse("2006-01", l.Mois)
		if err != nil {
			if mois, err = time.Parse("2006-01-02", l.Mois); err != nil {
				sansDate++
				continue
			}
		}
		// Le niveau se déduit des colonnes renseignées : la source ne le donne
		// pas explicitement, mais un département implique une région, et une
		// ligne sans l'un ni l'autre est nationale.
		// Piège : la DREES écrit « NA » — la chaîne, pas une valeur vide —
		// quand le découpage ne descend pas au département. Traiter « NA »
		// comme un code de département écrase toutes les régions sur une
		// seule clé : 6 604 lignes disparaissaient silencieusement avant
		// qu'on le voie. Une valeur manquante déguisée en valeur est le
		// piège le plus coûteux d'un fichier statistique.
		dep, reg := absent(l.Departement), absent(l.Region)
		niveau, code, nom := "NATIONAL", "FR", "France"
		switch {
		case dep != "":
			niveau, code, nom = "DEPARTEMENT", dep, l.NomDepartement
		case reg != "":
			niveau, code, nom = "REGION", reg, l.NomRegion
		}
		if nom == "" {
			nom = code
		}
		k := l.Serie + "|" + l.Mois + "|" + niveau + "|" + code
		if vu[k] {
			continue
		}
		vu[k] = true
		rows = append(rows, []any{
			l.Serie, l.NomSerie, mois, niveau, code, nom, *l.Valeur,
			nilSiVide(l.Unite), nilSiVide(l.Maturite), nilSiVide(l.Commentaire), srcID,
		})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Reconstruction : la DREES révise ses valeurs provisoires, donc compléter
	// laisserait cohabiter deux millésimes du même mois.
	if _, err := tx.Exec(ctx, `TRUNCATE core.prestation_solidarite`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "prestation_solidarite"},
		[]string{"serie", "nom_serie", "mois", "niveau", "code_geo", "nom_geo",
			"valeur", "unite", "maturite", "commentaire", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes": len(rows), "sans_valeur": sansValeur, "sans_date": sansDate}, "")
	fmt.Printf("  prestations de solidarité : %d lignes (%d sans valeur, %d sans date)\n",
		len(rows), sansValeur, sansDate)
	return nil
}

// absent rend la chaîne vide pour tout ce que la source utilise comme marqueur
// de valeur manquante : le vide lui-même, et le « NA » littéral.
func absent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" || s == "na" {
		return ""
	}
	return s
}

func nilSiVide(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
