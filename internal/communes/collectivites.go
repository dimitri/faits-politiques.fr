package communes

import (
	"context"
	"fmt"
	"net/url"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les comptes des régions, départements et groupements à fiscalité propre.
//
// Les communes ne sont qu'un étage. Les autres niveaux décident de masses
// comparables ou supérieures, et sur des compétences qui expliquent une part de
// ce que les communes ne font plus : collèges et action sociale au département,
// lycées et transports à la région, eau et déchets à l'intercommunalité.
//
// Même source que les comptes communaux, donc mêmes conventions et mêmes
// agrégats — à condition de ne jamais additionner les niveaux : une dépense
// portée par un groupement l'est POUR ses communes membres, et la sommer avec
// la leur compterait deux fois le même euro.
var niveaux = []struct {
	niveau, dataset, colCode, colNom string
	// La base départementale est déjà CONSOLIDÉE par l'Observatoire : elle ne
	// porte pas de colonne type_de_budget, puisque la consolidation a déjà
	// réuni budget principal et budgets annexes. Filtrer dessus renvoyait une
	// erreur 400 que rien n'expliquait.
	filtreBudget bool
}{
	{"REGION", "ofgl-base-regions", "reg_code", "reg_name", true},
	{"DEPARTEMENT", "ofgl-base-departements-consolidee", "dep_code", "dep_name", false},
	{"GROUPEMENT", "ofgl-base-gfp", "epci_code", "epci_name", true},
}

func IngestCollectivites(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceOFGL)
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.collectivite_budget`); err != nil {
		return fail(err)
	}

	noms := make([]string, 0, len(ofglAgregats))
	for a := range ofglAgregats {
		noms = append(noms, `"`+a+`"`)
	}

	var total int
	for _, n := range niveaux {
		var lignes [][]any
		for ex := ofglPremierExercice; ex <= ofglDernierExercice; ex++ {
			where := fmt.Sprintf(`exer=date'%d' AND agregat IN (%s)`, ex, joindre(noms))
			if n.filtreBudget {
				where = fmt.Sprintf(
					`exer=date'%d' AND type_de_budget="Budget principal" AND agregat IN (%s)`,
					ex, joindre(noms))
			}
			u := "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/" + n.dataset +
				"/exports/csv?delimiter=%3B&select=" +
				url.QueryEscape(n.colCode+","+n.colNom+",agregat,montant,euros_par_habitant,ptot") +
				"&where=" + url.QueryEscape(where)

			f, err := arch.Fetch(ctx, srcID, runID, u, ".csv")
			if err != nil {
				return fail(fmt.Errorf("%s %d : %w", n.niveau, ex, err))
			}
			recs, err := lireCSV(f.Path, ';')
			if err != nil {
				return fail(err)
			}
			// Une même collectivité dépose plusieurs budgets ; l'agrégat est
			// déjà consolidé par l'OFGL, mais les lignes se répètent. On garde
			// la première rencontrée plutôt que de laisser la clé primaire
			// faire échouer tout le chargement.
			vus := map[string]bool{}
			for _, r := range recs {
				code := r[n.colCode]
				ind, ok := ofglAgregats[r["agregat"]]
				if code == "" || !ok {
					continue
				}
				k := code + "|" + ind
				if vus[k] {
					continue
				}
				vus[k] = true
				lignes = append(lignes, []any{
					n.niveau, code, r[n.colNom], ex, ind,
					decimalNul(r["montant"]), decimalNul(r["euros_par_habitant"]),
					entierNul(r["ptot"]), srcID,
				})
			}
		}
		c, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "collectivite_budget"},
			[]string{"niveau", "code", "nom", "exercice", "indicator_code",
				"montant", "euros_par_hab", "population", "source_id"},
			pgx.CopyFromRows(lignes))
		if err != nil {
			return fail(fmt.Errorf("copie %s : %w", n.niveau, err))
		}
		total += int(c)
		fmt.Printf("    %-12s %6d valeurs\n", n.niveau, c)
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"valeurs": total}, "")
	fmt.Printf("  Collectivités : %d valeurs sur %d-%d\n", total, ofglPremierExercice, ofglDernierExercice)
	return nil
}

func joindre(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}
