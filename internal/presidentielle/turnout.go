package presidentielle

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// International IDEA compile les chiffres de participation publiés par les
// commissions électorales nationales. C'est un tiers, pas une autorité : d'où
// le niveau 2. Et la base ne publie AUCUNE licence explicite — or une absence
// de licence n'est pas une autorisation (voir raw.source.reuse_class), d'où
// RESTRICTED : consultable ici, jamais reversable dans un export.
var SourceTurnout = archive.Source{
	Slug: "idea-voter-turnout", Label: "International IDEA — Voter Turnout Database",
	Publisher: "International Institute for Democracy and Electoral Assistance",
	Tier:      "SECONDARY_PRESS",
	Licence:   "non publiée",
	// Les données IDEA ne portant pas de licence, elles restent hors export.
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : International IDEA, Voter Turnout Database",
	Cadence:     "continue",
	Notes: "Inscrits, votants et population en âge de voter par élection nationale " +
		"depuis 1945, un tour par ligne. AUCUN résultat par candidat : le score du " +
		"vainqueur rapporté aux inscrits n'est pas calculable depuis cette source. " +
		"L'export est régénéré à chaque appel : ses octets changent même quand les " +
		"données sont identiques, si bien que chaque exécution scelle un document " +
		"de plus. C'est le prix d'une source sans fichier stable, et c'est visible " +
		"dans raw.document plutôt que dissimulé.",
}

const TurnoutURL = "https://www.idea.int/data-tools/export?type=region_only&themeId=293&world=all&loc=home"

// Les feuilles retenues. « All » est un doublon des trois autres et n'est pas
// chargée : elle ferait entrer chaque élection deux fois.
var feuillesTurnout = []string{"Presidential", "Parliamentary", "EU Parliament"}

func IngestTurnout(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTurnout)
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

	f, err := arch.Fetch(ctx, srcID, runID, TurnoutURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return fail(err)
	}
	defer x.Close()

	var rows [][]any
	vu := map[string]bool{}
	ignorees := 0
	for _, sheet := range feuillesTurnout {
		lignes, err := x.rows(sheet)
		if err != nil {
			return fail(err)
		}
		for i, l := range lignes {
			if i == 0 {
				// En-tête : on vérifie qu'il n'a pas bougé plutôt que de
				// supposer que les colonnes sont restées à leur place.
				if l["A"] != "Country" || l["C"] != "ISO3" || l["E"] != "Year" ||
					l["G"] != "Total vote" || l["H"] != "Registration" {
					return fail(fmt.Errorf("feuille %s : en-tête inattendu %v", sheet, l))
				}
				continue
			}
			iso3, pays, date := l["C"], l["A"], l["E"]
			if len(iso3) != 3 || pays == "" || date == "" {
				ignorees++
				continue
			}
			d, err := time.Parse("2006-01-02", date)
			if err != nil {
				ignorees++
				continue
			}
			typ := l["D"]
			if typ == "" {
				typ = sheet
			}
			// Une même élection ne doit apparaître qu'une fois : la source
			// duplique certaines lignes entre feuilles.
			k := iso3 + "|" + typ + "|" + date
			if vu[k] {
				continue
			}
			vu[k] = true
			rows = append(rows, []any{
				iso3, pays, typ, d,
				nombreOuNil(l["G"]), nombreOuNil(l["H"]), nombreOuNil(l["J"]),
				pourcentOuNil(l["L"]), ouiNonOuNil(l["M"]), srcID,
			})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune ligne exploitable dans %s", TurnoutURL))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE core.turnout_election`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "turnout_election"},
		[]string{"pays_iso3", "pays_nom", "type_scrutin", "date_scrutin",
			"votants", "inscrits", "vap", "votes_invalides_pct", "vote_obligatoire", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"elections": len(rows), "lignes_ignorees": ignorees}, "")
	fmt.Printf("  participation comparée : %d élections, %d lignes ignorées\n", len(rows), ignorees)
	return nil
}

// Les nombres sont écrits « 1,824,401 ». Une cellule vide, un tiret ou « N/A »
// veulent dire « la source ne sait pas » : c'est NULL, jamais zéro.
func nombreOuNil(s string) any {
	s = strings.Map(func(r rune) rune {
		if r == ',' || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" || s == "-" || strings.EqualFold(s, "N/A") {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return n
}

func pourcentOuNil(s string) any {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" || s == "-" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return v
}

func ouiNonOuNil(s string) any {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes":
		return true
	case "no":
		return false
	}
	return nil
}
