package international

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// Représentativité des dirigeants (docs/international-donnees.md § 7) : ce
// connecteur charge la PARTICIPATION électorale (International IDEA), pas
// la part du vainqueur ni le mode de scrutin — les deux resteraient à
// compiler à la main, pays par pays et élection par élection, hors de
// portée d'un chargement automatisé. Non commercial, avec attribution
// (conditions IDEA, idea.int/terms-and-conditions) — compatible avec ce
// site, qui n'a pas de vocation commerciale.
var SourceIDEA = archive.Source{
	Slug: "idea-participation-electorale", Label: "International IDEA — participation électorale",
	Publisher:   "International Institute for Democracy and Electoral Assistance (International IDEA)",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Usage non commercial avec attribution (conditions IDEA) — ce site n'a pas de vocation commerciale",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : International IDEA, Voter Turnout Database",
	Cadence:     "mise à jour continue, au fil des élections",
	Notes: "Export statique du site (pas une API documentée). Chine et Arabie saoudite absentes " +
		"par construction : IDEA ne recense que les élections législatives et présidentielles au " +
		"suffrage direct, qu'aucun des deux pays ne tient dans ce sens.",
}

const ideaURL = "https://www.idea.int/data-tools/export?type=region_only&themeId=293&world=all&loc=home"

var paysIDEA = map[string]string{
	"FRA": "FR", "USA": "US", "JPN": "JP", "DEU": "DE", "GBR": "GB",
	"ITA": "IT", "CAN": "CA", "RUS": "RU",
}

func IngestIDEA(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceIDEA)
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

	f, err := arch.Fetch(ctx, srcID, runID, ideaURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur IDEA illisible : %w", err))
	}
	defer wb.Close()

	lignes, err := wb.GetRows("All")
	if err != nil {
		return fail(fmt.Errorf("feuille 'All' : %w", err))
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("feuille 'All' vide — format changé"))
	}
	entete := lignes[0]
	idx := map[string]int{}
	for i, h := range entete {
		idx[strings.TrimSpace(h)] = i
	}
	for _, col := range []string{"Country", "ISO3", "Election Type", "Year", "Voter Turnout",
		"Registration", "VAP Turnout", "Voting age population", "Population", "Compulsory voting"} {
		if _, ok := idx[col]; !ok {
			return fail(fmt.Errorf("colonne %q absente de l'export IDEA — format changé", col))
		}
	}

	versPct := func(s string) (*float64, error) {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	versEntier := func(s string) (*int64, error) {
		s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE core.participation_electorale`); err != nil {
		return fail(err)
	}

	trouve := map[string]bool{}
	vus := map[[3]string]bool{}
	var rows [][]any
	for _, l := range lignes[1:] {
		if len(l) <= idx["Compulsory voting"] {
			continue
		}
		iso3 := strings.TrimSpace(l[idx["ISO3"]])
		pays2, ok := paysIDEA[iso3]
		if !ok {
			continue
		}
		trouve[iso3] = true
		date, err := time.Parse("2006-01-02", strings.TrimSpace(l[idx["Year"]]))
		if err != nil {
			return fail(fmt.Errorf("%s : date d'élection %q illisible : %w", iso3, l[idx["Year"]], err))
		}
		typeElection := strings.TrimSpace(l[idx["Election Type"]])
		// Les élections à deux tours publient une ligne par tour, sur la même
		// date pour un même pays et type : la contrainte d'unicité de la
		// table suppose une ligne par (pays, type, date), vérifié avant
		// d'écrire ce connecteur plutôt que découvert par une violation de
		// clé.
		cle := [3]string{iso3, typeElection, date.Format("2006-01-02")}
		if vus[cle] {
			continue
		}
		vus[cle] = true

		tauxInscrits, err := versPct(l[idx["Voter Turnout"]])
		if err != nil {
			return fail(fmt.Errorf("%s %s : Voter Turnout %q : %w", iso3, date.Format("2006-01-02"), l[idx["Voter Turnout"]], err))
		}
		tauxVAP, err := versPct(l[idx["VAP Turnout"]])
		if err != nil {
			return fail(fmt.Errorf("%s %s : VAP Turnout %q : %w", iso3, date.Format("2006-01-02"), l[idx["VAP Turnout"]], err))
		}
		popVAP, err := versEntier(l[idx["Voting age population"]])
		if err != nil {
			return fail(fmt.Errorf("%s %s : Voting age population %q : %w", iso3, date.Format("2006-01-02"), l[idx["Voting age population"]], err))
		}
		popTotale, err := versEntier(l[idx["Population"]])
		if err != nil {
			return fail(fmt.Errorf("%s %s : Population %q : %w", iso3, date.Format("2006-01-02"), l[idx["Population"]], err))
		}
		obligatoire := strings.EqualFold(strings.TrimSpace(l[idx["Compulsory voting"]]), "Yes")

		rows = append(rows, []any{iso3, libellesPays[pays2], typeElection, date,
			tauxInscrits, tauxVAP, popVAP, popTotale, obligatoire, srcID})
	}
	for iso3 := range paysIDEA {
		if !trouve[iso3] {
			return fail(fmt.Errorf("pays de comparaison introuvable dans l'export IDEA : %s — vérifier si une élection existe pour ce pays avant de traiter comme un bug", iso3))
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne lue"))
	}

	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "participation_electorale"},
		[]string{"pays_iso3", "pays_label", "type_election", "date_election",
			"taux_participation_inscrits", "taux_participation_vap", "population_vap", "population_totale",
			"vote_obligatoire", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("participation_electorale : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  Participation électorale (International IDEA) : %d lignes, %d pays\n", n, len(paysIDEA))
	return nil
}
