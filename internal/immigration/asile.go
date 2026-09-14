package immigration

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les demandes d'asile : un flux administratif distinct de l'immigration au
// sens du recensement et des titres de séjour. Voir
// docs/immigration-donnees.md.
var SourceAsileOFPRA = archive.Source{
	Slug: "ofpra-demandes-asile", Label: "Ofpra — demandes d'asile et de statut d'apatride",
	Publisher: "Office français de protection des réfugiés et apatrides", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Ofpra",
	Cadence:     "annuelle",
	Notes: "Les demandes comptées par l'Ofpra ne correspondent PAS au total des demandes " +
		"d'asile publié par la DGEF (qui compte notamment les demandeurs sous procédure " +
		"Dublin, non transmis à l'Ofpra). « premiere_demande » ne compte que les nouvelles " +
		"demandes ; reexamen et reouverture sont des demandes antérieures rouvertes.",
}

// L'identifiant de ressource data.gouv.fr change à chaque publication
// annuelle, comme pour les millésimes du COG (internal/communes/cog.go) : les
// URL sont donc écrites en clair, une par année. 2020 est absent : la
// ressource correspondante renvoie une 404 au moment de l'écriture.
var anneesOFPRA = []struct {
	Annee int
	URL   string
}{
	{2021, "https://static.data.gouv.fr/resources/demandes-dasile-et-de-statut-dapatride-deposees-devant-lofpra/20230720-083712/ofpra-demandes-nat-2021.csv"},
	{2022, "https://static.data.gouv.fr/resources/demandes-dasile-et-de-statut-dapatride-deposees-devant-lofpra/20230720-084246/ofpra-demandes-nat-2022.csv"},
	{2023, "https://static.data.gouv.fr/resources/demandes-dasile-et-de-statut-dapatride-deposees-devant-lofpra/20240726-080915/ofpra-demandes-nat-2023.csv"},
	{2024, "https://static.data.gouv.fr/resources/demandes-dasile-et-de-statut-dapatride-deposees-devant-lofpra/20250718-071533/ofpra-demandes-nat-2024.csv"},
	{2025, "https://static.data.gouv.fr/resources/demandes-dasile-et-de-statut-dapatride-deposees-devant-lofpra/20260812-083012/ofpra-demandes-nat-2025.csv"},
}

func IngestAsile(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAsileOFPRA)
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
	if _, err := tx.Exec(ctx, `DELETE FROM core.demande_asile_ofpra`); err != nil {
		return fail(err)
	}

	var total int
	for _, a := range anneesOFPRA {
		n, err := chargerAsileAnnee(ctx, arch, tx, srcID, runID, a.Annee, a.URL)
		if err != nil {
			return fail(fmt.Errorf("%d : %w", a.Annee, err))
		}
		total += n
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": total, "annees": len(anneesOFPRA)}, "")
	fmt.Printf("  demandes d'asile (Ofpra) : %d lignes, %d millésimes\n", total, len(anneesOFPRA))
	return nil
}

func chargerAsileAnnee(ctx context.Context, arch *archive.Archive, tx pgx.Tx, srcID, runID int64, annee int, url string) (int, error) {
	f, err := arch.Fetch(ctx, srcID, runID, url, ".csv")
	if err != nil {
		return 0, err
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		return 0, err
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return 0, err
	}
	if len(recs) < 2 {
		return 0, fmt.Errorf("fichier vide")
	}
	head := recs[0]
	// Le premier caractère du premier en-tête porte parfois un BOM UTF-8.
	if len(head) > 0 {
		head[0] = trimBOM(head[0])
	}
	idx := map[string]int{}
	for i, h := range head {
		idx[h] = i
	}
	col := func(rec []string, nom string) *int {
		i, ok := idx[nom]
		if !ok || i >= len(rec) || rec[i] == "" {
			return nil
		}
		v, err := strconv.Atoi(rec[i])
		if err != nil {
			return nil
		}
		return &v
	}
	texte := func(rec []string, nom string) any {
		i, ok := idx[nom]
		if !ok || i >= len(rec) || rec[i] == "" {
			return nil
		}
		return rec[i]
	}

	var rows [][]any
	for _, rec := range recs[1:] {
		if len(rec) == 0 {
			continue
		}
		niveauLib, ok := idx["niveau"]
		if !ok || niveauLib >= len(rec) {
			continue
		}
		var niveau string
		switch rec[niveauLib] {
		case "Total":
			niveau = "TOTAL"
		case "Continent":
			niveau = "CONTINENT"
		case "Nationalité":
			niveau = "NATIONALITE"
		default:
			continue
		}
		rows = append(rows, []any{
			annee, niveau, texte(rec, "continent"), texte(rec, "norme_iso_3166"), texte(rec, "nationalite"),
			col(rec, "premiere_demande"), col(rec, "reexamen"), col(rec, "reouverture"),
			col(rec, "maj_premiere_demande"), col(rec, "maj_reexamen"), col(rec, "maj_reouverture"),
			col(rec, "f_premiere_demande"), col(rec, "f_reexamen"), col(rec, "f_reouverture"),
			srcID,
		})
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("aucune ligne reconnue sur %d", len(recs)-1)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "demande_asile_ofpra"},
		[]string{"annee", "niveau", "continent", "code_pays", "nationalite",
			"premiere_demande", "reexamen", "reouverture",
			"mineurs_accompagnants_premiere_demande", "mineurs_accompagnants_reexamen", "mineurs_accompagnants_reouverture",
			"femmes_premiere_demande", "femmes_reexamen", "femmes_reouverture", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func trimBOM(s string) string {
	const bom = "\ufeff"
	if len(s) >= len(bom) && s[:len(bom)] == bom {
		return s[len(bom):]
	}
	return s
}
