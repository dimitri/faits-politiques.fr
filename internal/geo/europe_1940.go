package geo

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceEuropeGuerre1940 = archive.Source{
	Slug: "cshapes-europe-1940", Label: "CShapes 2.0 — frontières de l'Europe au 1ᵉʳ septembre 1940",
	Publisher: "ETH Zürich (International Conflict Research)", Tier: "PRIMARY_OFFICIAL",
	Licence: "CC BY-NC-SA 4.0", ReuseClass: "OPEN",
	Attribution: "Source : CShapes 2.0, icr.ethz.ch/data/cshapes",
	Cadence:     "ponctuelle",
	Notes: "Une seule coupe temporelle du panel CShapes (voir aussi " +
		"internal/geo/empire_colonial.go, même fichier), au 1ᵉʳ septembre 1940 : après " +
		"l'armistice française (25 juin) et l'annexion soviétique des pays baltes et de la " +
		"Bessarabie/Bucovine du Nord (28 juin-3 août), avant l'entrée en guerre de l'Italie " +
		"contre la France n'a pas d'effet ici (10 juin, déjà passée) et bien avant l'opération " +
		"Barbarossa (22 juin 1941). CShapes code la continuité des ÉTATS reconnus par le " +
		"système international, pas le détail de l'administration militaire d'occupation : la " +
		"Tchécoslovaquie et l'Autriche restent des entités géographiques séparées dans ce jeu " +
		"malgré leur annexion de fait par l'Allemagne, et la Pologne, la Tchécoslovaquie et la " +
		"Yougoslavie ne sont pas subdivisées entre occupants — des simplifications du jeu de " +
		"données source, pas de ce chargement.",
}

// dateReferenceEurope1940 : voir SourceEuropeGuerre1940.Notes pour le choix
// de cette date précise.
const dateReferenceEurope1940 = "1940-09-01"

// paysEurope1940 : nom d'affichage (français, vérifié un par un) associé au
// nom CShapes (cntry_name, anglais) — la clé de recherche dans le CSV.
// L'ex-Tchécoslovaquie et l'ex-Yougoslavie sont incluses telles quelles
// (CShapes ne les subdivise pas pour cette période, voir Notes ci-dessus).
var paysEurope1940 = map[string]string{
	"Germany (Prussia)":     "Allemagne",
	"Poland":                "Pologne",
	"Italy/Sardinia":        "Italie",
	"United Kingdom":        "Royaume-Uni",
	"Ireland":               "Irlande",
	"Switzerland":           "Suisse",
	"Spain":                 "Espagne",
	"Portugal":              "Portugal",
	"Netherlands":           "Pays-Bas",
	"Belgium":               "Belgique",
	"Luxembourg":            "Luxembourg",
	"Denmark":               "Danemark",
	"Norway":                "Norvège",
	"Sweden":                "Suède",
	"Finland":               "Finlande",
	"Austria":               "Autriche",
	"Czechoslovakia":        "Tchécoslovaquie",
	"Hungary":               "Hongrie",
	"Rumania":               "Roumanie",
	"Bulgaria":              "Bulgarie",
	"Yugoslavia":            "Yougoslavie",
	"Greece":                "Grèce",
	"Albania":               "Albanie",
	"Russia (Soviet Union)": "URSS",
}

// IngestEuropeGuerre1940 charge, depuis le même CSV CShapes 2.0 que l'empire
// colonial (urlCShapes, empire_colonial.go), la géométrie de chaque pays de
// paysEurope1940 valide à dateReferenceEurope1940 — une ligne par pays dont
// la période [gwsdate, gwedate] couvre cette date. Voir
// docs/seconde-guerre-mondiale-donnees.md.
func IngestEuropeGuerre1940(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEuropeGuerre1940)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "europe-1940-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlCShapes, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	ref, err := time.Parse("2006-01-02", dateReferenceEurope1940)
	if err != nil {
		return fail(err)
	}

	r := csv.NewReader(file)
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête CShapes illisible : %w", err))
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, must := range []string{"cntry_name", "gwsyear", "gwsmonth", "gwsday",
		"gweyear", "gwemonth", "gweday", "the_geom"} {
		if _, ok := col[must]; !ok {
			return fail(fmt.Errorf("colonne %q absente du CSV CShapes", must))
		}
	}
	date := func(rec []string, yCol, mCol, dCol string) (time.Time, error) {
		return time.Parse("2006-1-2", fmt.Sprintf("%s-%s-%s", rec[col[yCol]], rec[col[mCol]], rec[col[dCol]]))
	}

	geomParPays := map[string]string{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("ligne CShapes illisible : %w", err))
		}
		nom := rec[col["cntry_name"]]
		if _, voulu := paysEurope1940[nom]; !voulu {
			continue
		}
		debut, err := date(rec, "gwsyear", "gwsmonth", "gwsday")
		if err != nil {
			continue
		}
		fin, err := date(rec, "gweyear", "gwemonth", "gweday")
		if err != nil {
			continue
		}
		if (debut.Before(ref) || debut.Equal(ref)) && (fin.After(ref) || fin.Equal(ref)) {
			geomParPays[nom] = rec[col["the_geom"]]
		}
	}

	var manquants []string
	for nomEn := range paysEurope1940 {
		if _, ok := geomParPays[nomEn]; !ok {
			manquants = append(manquants, nomEn)
		}
	}
	if len(manquants) > 0 {
		return fail(fmt.Errorf("aucune géométrie CShapes au %s pour : %v", dateReferenceEurope1940, manquants))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_europe_1940`); err != nil {
		return fail(err)
	}
	for nomEn, nomFr := range paysEurope1940 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.contour_europe_1940 (nom, nom_en, geom, source_id)
			VALUES ($1, $2, st_multi(st_geomfromewkt($3)), $4)`,
			nomFr, nomEn, geomParPays[nomEn], srcID); err != nil {
			return fail(fmt.Errorf("%s : %w", nomFr, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	fmt.Printf("  Europe au %s (CShapes 2.0) : %d pays\n", dateReferenceEurope1940, len(paysEurope1940))
	arch.EndRun(ctx, runID, "OK", map[string]any{"pays": len(paysEurope1940)}, "")
	return nil
}
