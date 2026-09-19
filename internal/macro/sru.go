package macro

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceSRU = archive.Source{
	Slug: "sru-inventaire-communes", Label: "Inventaire SRU par commune",
	Publisher: "DGALN/DHUP (ministère du Logement)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : DGALN/DHUP, data.gouv.fr",
	Cadence:     "annuelle",
	Notes: "Ne couvre que les communes dans le périmètre de l'article 55 de la loi SRU " +
		"(2 196 communes au 1er janvier 2025), pas l'ensemble des communes françaises. " +
		"Fichier source encodé en Latin-1, converti en UTF-8 à l'ingestion.",
}

const urlSRU = "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20251219-143258/donnees-sru-data-gouv-2025-v2.csv"

func parserPourcentageFr(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func aBool01(s string) bool { return strings.TrimSpace(s) == "1" }

// IngestSRU charge l'inventaire SRU par commune. Voir
// docs/logement-territoires-donnees.md.
func IngestSRU(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSRU)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "sru-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlSRU, ".csv")
	if err != nil {
		return fail(err)
	}
	octets, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	// Fichier source encodé en Latin-1 (voir SourceSRU.Notes) : converti ici
	// avec le même patron que internal/macro/accord_paris.go, plutôt que
	// d'ajouter un nouvel usage de golang.org/x/text pour un cas aussi simple.
	r := csv.NewReader(strings.NewReader(latin1(octets)))
	r.Comma = ';'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête illisible : %w", err))
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	must := []string{"Code_INSEE_commune", "Nom_commune", "Departement", "Population_municipale_01_01_2025",
		"Taux_SRU_au_01_01_2024", "commune_deficitaire", "Commune_carencée", "Commune_exemptée_2023_2025",
		"Taux_cible_commune_2023_2025"}
	for _, m := range must {
		if _, ok := col[m]; !ok {
			return fail(fmt.Errorf("colonne %q absente (en-tête : %v)", m, header))
		}
	}
	colLLS := -1
	for h, i := range col {
		if strings.HasPrefix(h, "Nombre_lls") {
			colLLS = i
		}
	}
	if colLLS == -1 {
		return fail(fmt.Errorf("colonne du nombre de logements sociaux introuvable"))
	}

	type ligne struct {
		codeInsee, commune, departement string
		population, nbLLS               *int
		tauxSRU, tauxCible              *float64
		deficitaire, carencee, exemptee bool
	}
	var lignes []ligne
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("ligne illisible : %w", err))
		}
		l := ligne{
			codeInsee: strings.TrimSpace(rec[col["Code_INSEE_commune"]]), commune: strings.TrimSpace(rec[col["Nom_commune"]]),
			departement: strings.TrimSpace(rec[col["Departement"]]),
			deficitaire: aBool01(rec[col["commune_deficitaire"]]), carencee: aBool01(rec[col["Commune_carencée"]]),
			exemptee: aBool01(rec[col["Commune_exemptée_2023_2025"]]),
		}
		if v, err := strconv.Atoi(strings.TrimSpace(rec[col["Population_municipale_01_01_2025"]])); err == nil {
			l.population = &v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(rec[colLLS])); err == nil {
			l.nbLLS = &v
		}
		if v, ok := parserPourcentageFr(rec[col["Taux_SRU_au_01_01_2024"]]); ok {
			l.tauxSRU = &v
		}
		if v, ok := parserPourcentageFr(rec[col["Taux_cible_commune_2023_2025"]]); ok {
			l.tauxCible = &v
		}
		if l.codeInsee == "" {
			continue
		}
		lignes = append(lignes, l)
	}
	if len(lignes) < 2000 {
		return fail(fmt.Errorf("seulement %d lignes lues, attendu au moins 2000", len(lignes)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.sru_commune`); err != nil {
		return fail(err)
	}
	for _, l := range lignes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.sru_commune
				(code_insee, commune, departement, population, nombre_logements_sociaux,
				 taux_sru_pct, taux_cible_pct, deficitaire, carencee, exemptee, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (code_insee) DO NOTHING`,
			l.codeInsee, l.commune, l.departement, l.population, l.nbLLS,
			l.tauxSRU, l.tauxCible, l.deficitaire, l.carencee, l.exemptee, srcID); err != nil {
			return fail(fmt.Errorf("%s : insertion : %w", l.codeInsee, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(lignes)}, "")
	fmt.Printf("  Inventaire SRU : %d communes\n", len(lignes))
	return nil
}
