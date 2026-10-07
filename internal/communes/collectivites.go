package communes

import (
	"context"
	"fmt"
	"net/url"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
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
type collectiviteNiveau struct {
	niveau, dataset, colCode, colNom string
	// La base départementale est déjà CONSOLIDÉE par l'Observatoire : elle ne
	// porte pas de colonne type_de_budget, puisque la consolidation a déjà
	// réuni budget principal et budgets annexes. Filtrer dessus renvoyait une
	// erreur 400 que rien n'expliquait.
	filtreBudget bool
}

var niveaux = []collectiviteNiveau{
	{"REGION", "ofgl-base-regions", "reg_code", "reg_name", true},
	{"DEPARTEMENT", "ofgl-base-departements-consolidee", "dep_code", "dep_name", false},
	{"GROUPEMENT", "ofgl-base-gfp", "epci_code", "epci_name", true},
}

// collectivitesURL construit l'URL OFGL d'un niveau et d'un exercice — seule
// source de vérité pour IngestCollectivites et pour DownloadTargets (voir ce
// dernier), pour que les deux ne puissent pas diverger.
func collectivitesURL(n collectiviteNiveau, ex int) string {
	noms := make([]string, 0, len(ofglAggregates))
	for a := range ofglAggregates {
		noms = append(noms, `"`+a+`"`)
	}
	where := fmt.Sprintf(`exer=date'%d' AND agregat IN (%s)`, ex, joindre(noms))
	if n.filtreBudget {
		where = fmt.Sprintf(
			`exer=date'%d' AND type_de_budget="Budget principal" AND agregat IN (%s)`,
			ex, joindre(noms))
	}
	return "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/" + n.dataset +
		"/exports/csv?delimiter=%3B&select=" +
		url.QueryEscape(n.colCode+","+n.colNom+",agregat,montant,euros_par_habitant,ptot") +
		"&where=" + url.QueryEscape(where)
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

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_collectivite_budget (
			niveau text, code text, nom text, exercice int, indicator_code text,
			montant numeric, euros_par_hab numeric, population int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	var total int
	for _, n := range niveaux {
		var lignes [][]any
		for ex := ofglFirstFiscalYear; ex <= ofglLastFiscalYear; ex++ {
			f, err := arch.Fetch(ctx, srcID, runID, collectivitesURL(n, ex), ".csv")
			if err != nil {
				return fail(fmt.Errorf("%s %d : %w", n.niveau, ex, err))
			}
			recs, err := readCSV(f.Path, ';')
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
				ind, ok := ofglAggregates[r["agregat"]]
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
					parseFloatOrNull(r["montant"]), parseFloatOrNull(r["euros_par_habitant"]),
					parseIntOrNull(r["ptot"]), srcID,
				})
			}
		}
		c, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_collectivite_budget"},
			[]string{"niveau", "code", "nom", "exercice", "indicator_code",
				"montant", "euros_par_hab", "population", "source_id"},
			pgx.CopyFromRows(lignes))
		if err != nil {
			return fail(fmt.Errorf("copie %s : %w", n.niveau, err))
		}
		total += int(c)
		fmt.Printf("    %-12s %6d valeurs\n", n.niveau, c)
	}

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers
	// RI pour l'intégralité des trois niveaux et huit exercices à chaque
	// republication de l'OFGL, changement ou non.
	var touchees int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.collectivite_budget", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.collectivite_budget AS tgt
			USING tmp_collectivite_budget AS src
			ON tgt.niveau = src.niveau AND tgt.code = src.code
			   AND tgt.exercice = src.exercice AND tgt.indicator_code = src.indicator_code
			WHEN MATCHED AND (tgt.nom, tgt.montant, tgt.euros_par_hab, tgt.population, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.nom, src.montant, src.euros_par_hab, src.population, src.source_id) THEN
			    UPDATE SET nom = src.nom, montant = src.montant, euros_par_hab = src.euros_par_hab,
			               population = src.population, source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (niveau, code, nom, exercice, indicator_code, montant, euros_par_hab,
			            population, source_id)
			    VALUES (src.niveau, src.code, src.nom, src.exercice, src.indicator_code, src.montant,
			            src.euros_par_hab, src.population, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		touchees = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"valeurs": total, "touchees": touchees}, "")
	fmt.Printf("  Collectivités : %d valeurs sur %d-%d (%d touchées par la fusion)\n",
		total, ofglFirstFiscalYear, ofglLastFiscalYear, touchees)
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
