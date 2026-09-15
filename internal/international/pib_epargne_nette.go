package international

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le PIB compte l'extraction de ressources naturelles comme une production,
// sans jamais retrancher l'épuisement du stock : deux pays qui vendent le
// même pétrole, l'un en le remplaçant par une industrie, l'autre en épuisant
// un gisement fini, affichent la même croissance. L'épargne nette ajustée de
// la Banque mondiale corrige précisément cet angle mort — voir
// docs/international-donnees.md § 4.
var SourcePIBEpargneNette = archive.Source{
	Slug: "banque-mondiale-pib-epargne-nette", Label: "Banque mondiale — PIB, épargne nette ajustée, épuisement des ressources",
	Publisher: "Banque mondiale", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Banque mondiale, World Development Indicators",
	Cadence:     "annuelle",
	Notes: "Dix pays de comparaison (G8 historique, Chine, Arabie saoudite). NY.ADJ.SVNG.GN.ZS " +
		"(épargne nette ajustée) et NY.ADJ.DRES.GN.ZS (épuisement des ressources naturelles) " +
		"sont des pourcentages du revenu national brut, jamais des montants — ne pas les " +
		"comparer en valeur absolue au PIB (NY.GDP.MKTP.CD, en dollars courants).",
}

// Le G8 historique (avant l'exclusion de la Russie en 2014), plus la Chine
// (première économie par le PIB en parité de pouvoir d'achat, absente du G8)
// et l'Arabie saoudite (économie dont l'épuisement pétrolier illustre le
// mieux ce que le PIB ne mesure pas).
var paysComparaisonMondiale = []string{"FR", "US", "JP", "DE", "GB", "IT", "CA", "RU", "CN", "SA"}

var indicateursMondiaux = []string{"NY.GDP.MKTP.CD", "NY.ADJ.SVNG.GN.ZS", "NY.ADJ.DRES.GN.ZS"}

const worldBankBase = "https://api.worldbank.org/v2/country/"

func IngestPIBEpargneNette(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePIBEpargneNette)
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

	pays := ""
	for i, p := range paysComparaisonMondiale {
		if i > 0 {
			pays += ";"
		}
		pays += p
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.indicateur_mondial`); err != nil {
		return fail(err)
	}

	var total int64
	for _, ind := range indicateursMondiaux {
		url := worldBankBase + pays + "/indicator/" + ind + "?format=json&per_page=1000&date=2000:2024"
		f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
		if err != nil {
			return fail(err)
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return fail(err)
		}
		var page []json.RawMessage
		if err := json.Unmarshal(b, &page); err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		if len(page) != 2 {
			return fail(fmt.Errorf("%s : réponse Banque mondiale inattendue", ind))
		}
		var meta struct {
			Pages int `json:"pages"`
		}
		if err := json.Unmarshal(page[0], &meta); err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		if meta.Pages > 1 {
			return fail(fmt.Errorf("%s : %d pages — la fenêtre 2000-2024 dépasse per_page=1000, à agrandir plutôt qu'à tronquer en silence", ind, meta.Pages))
		}
		var lignes []ligneWB
		if err := json.Unmarshal(page[1], &lignes); err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		var rows [][]any
		for _, l := range lignes {
			if l.Value == nil {
				continue
			}
			annee, err := strconv.Atoi(l.Date)
			if err != nil {
				return fail(fmt.Errorf("%s : année %q : %w", ind, l.Date, err))
			}
			rows = append(rows, []any{l.Country.ID, l.Country.Value, ind, annee, *l.Value, srcID})
		}
		if len(rows) == 0 {
			return fail(fmt.Errorf("%s : aucune valeur", ind))
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "indicateur_mondial"},
			[]string{"pays_code", "pays_label", "indicateur", "annee", "valeur", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		total += n
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total}, "")
	fmt.Printf("  PIB et épargne nette ajustée (Banque mondiale) : %d lignes, %d pays, %d indicateurs\n",
		total, len(paysComparaisonMondiale), len(indicateursMondiaux))
	return nil
}

type ligneWB struct {
	Country struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"country"`
	Date  string   `json:"date"`
	Value *float64 `json:"value"`
}
