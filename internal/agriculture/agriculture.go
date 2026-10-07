// Package agriculture charge les bilans alimentaires et l'appareil de
// production agricole.
//
// La question à laquelle ces données répondent : un pays arrive-t-il à nourrir
// ses habitants ? Un bilan alimentaire y répond par un rapport — ce qui est
// produit sur ce qui est consommé — et par une décomposition : ce qui va
// directement aux humains, ce qui passe par le bétail, ce qui est exporté.
//
// Ce qu'il NE dit PAS : produire plus de céréales qu'on n'en consomme ne veut
// pas dire qu'on pourrait se nourrir seul. Les systèmes de production dépendent
// d'intrants importés — engrais azotés, tourteaux de soja, carburant — que le
// bilan ne compte pas.
package agriculture

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "agriculture-v1"

var Source = archive.Source{
	Slug: "faostat", Label: "FAOSTAT — bilans alimentaires et usage des terres",
	Publisher:   "Organisation des Nations unies pour l'alimentation et l'agriculture",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "CC BY 4.0",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : FAOSTAT, Organisation des Nations unies pour l'alimentation et l'agriculture",
	Cadence:     "annuelle",
	Notes: "Mêmes conventions pour tous les pays, ce qui rend les comparaisons " +
		"possibles. Les bilans alimentaires courants commencent en 2010 ; l'usage " +
		"des terres remonte à 1961.",
}

const (
	foodBalanceURL = "https://bulks-faostat.fao.org/production/FoodBalanceSheets_E_Europe.zip"
	landURL        = "https://bulks-faostat.fao.org/production/Inputs_LandUse_E_Europe.zip"
	employmentURL  = "https://bulks-faostat.fao.org/production/Employment_Indicators_Agriculture_E_Europe.zip"

	country = "France"
)

// Les éléments du bilan qui portent la réponse. Les autres — variations de
// stock, résidus, apports en protéines et lipides — sont ignorés : les charger
// tous multiplierait le volume sans éclairer la question posée.
var retainedElements = map[string]bool{
	"Production":                          true,
	"Import quantity":                     true,
	"Export quantity":                     true,
	"Domestic supply quantity":            true,
	"Food":                                true,
	"Feed":                                true,
	"Food supply (kcal/capita/day)":       true,
	"Food supply quantity (kg/capita/yr)": true,
	// La population est publiée comme un « produit » du bilan, avec son propre
	// élément. Sans elle, aucune conversion entre le par-habitant et le total.
	"Total Population - Both sexes": true,
}

// Les agrégats de la FAO. Ils contiennent déjà leurs composants : les
// additionner à ceux-ci compterait deux fois.
var aggregates = map[string]bool{
	"Grand Total": true, "Animal Products": true, "Vegetal Products": true,
	"Population": true,
}

// Les postes d'usage des terres et d'emploi qui décrivent l'appareil de
// production, avec le code sous lequel ils entrent en base.
var indicators = map[string]string{
	"Agricultural land":              "terres.agricoles",
	"Cropland":                       "terres.cultivees",
	"Arable land":                    "terres.arables",
	"Permanent meadows and pastures": "terres.prairies",
	// Les fichiers d'emploi nomment leur colonne « Indicator » et non « Item »,
	// et emploient les libellés de l'OIT.
	"Employment in agriculture - ILO modelled estimates": "emploi.agricole",
	"Total employment in agrifood systems (AFS)":         "emploi.agroalimentaire",
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
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

	fFoodBalance, err := arch.Fetch(ctx, srcID, runID, foodBalanceURL, ".zip")
	if err != nil {
		return fail(err)
	}
	fLand, err := arch.Fetch(ctx, srcID, runID, landURL, ".zip")
	if err != nil {
		return fail(err)
	}
	fEmployment, err := arch.Fetch(ctx, srcID, runID, employmentURL, ".zip")
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// ref.produit_alimentaire n'est jamais vidée : les codes présents dans le
	// chargement courant sont ré-upsertés ci-dessous, et core.bilan_alimentaire
	// référence cette table par clé étrangère — la vider avant que les anciennes
	// lignes de bilan (pas encore fusionnées) ne soient traitées casserait la
	// contrainte.

	// 1. Les bilans alimentaires.
	products := map[string]string{}
	var rows [][]any
	err = walkFAOSTATCSV(fFoodBalance.Path, func(r map[string]string, years map[int]string) error {
		if r["Area"] != country {
			return nil
		}
		el := r["Element"]
		if !retainedElements[el] {
			return nil
		}
		item := r["Item"]
		code := productCode(item)
		products[code] = item
		for year, raw := range years {
			v, ok := parseNumber(raw)
			if !ok {
				continue
			}
			rows = append(rows, []any{code, el, year, v, r["Unit"], srcID})
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucun bilan pour %s : le format de la FAO a changé", country))
	}

	for code, label := range products {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ref.produit_alimentaire (code, libelle, agregat)
			VALUES ($1,$2,$3) ON CONFLICT (code) DO UPDATE SET libelle = EXCLUDED.libelle`,
			code, label, aggregates[label]); err != nil {
			return fail(err)
		}
	}
	// DISTINCT ON en amont : la FAO publie parfois deux lignes pour un même
	// triplet quand une série est révisée. On garde la première rencontrée
	// plutôt que de laisser la clé primaire faire échouer le chargement.
	seen := map[string]bool{}
	var deduped [][]any
	for _, l := range rows {
		k := fmt.Sprintf("%v|%v|%v", l[0], l[1], l[2])
		if seen[k] {
			continue
		}
		seen[k] = true
		deduped = append(deduped, l)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_bilan_alimentaire (
			produit_code text NOT NULL,
			element text NOT NULL,
			annee integer NOT NULL,
			valeur numeric NOT NULL,
			unite text NOT NULL,
			source_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_bilan_alimentaire"},
		[]string{"produit_code", "element", "annee", "valeur", "unite", "source_id"},
		pgx.CopyFromRows(deduped)); err != nil {
		return fail(fmt.Errorf("copie des bilans : %w", err))
	}
	ctBalance, err := tx.Exec(ctx, `
		MERGE INTO core.bilan_alimentaire AS tgt
		USING tmp_bilan_alimentaire AS src
		ON tgt.produit_code = src.produit_code AND tgt.element = src.element AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.valeur, tgt.unite, tgt.source_id)
			IS DISTINCT FROM (src.valeur, src.unite, src.source_id) THEN
			UPDATE SET valeur = src.valeur, unite = src.unite, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (produit_code, element, annee, valeur, unite, source_id)
			VALUES (src.produit_code, src.element, src.annee, src.valeur, src.unite, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion des bilans : %w", err))
	}
	nBalance := ctBalance.RowsAffected()

	// 2. L'appareil de production : terres et emploi.
	var indicatorRows [][]any
	for _, path := range []string{fLand.Path, fEmployment.Path} {
		seen := map[string]bool{}
		err = walkFAOSTATCSV(path, func(r map[string]string, years map[int]string) error {
			if r["Area"] != country {
				return nil
			}
			// « Item » dans les fichiers de production, « Indicator » dans ceux
			// d'emploi : la FAO ne nomme pas sa colonne de la même façon d'un
			// jeu à l'autre.
			label := r["Item"]
			if label == "" {
				label = r["Indicator"]
			}
			code, ok := indicators[label]
			if !ok {
				return nil
			}
			for year, raw := range years {
				v, ok := parseNumber(raw)
				if !ok {
					continue
				}
				k := fmt.Sprintf("%s|%d", code, year)
				if seen[k] {
					continue
				}
				seen[k] = true
				indicatorRows = append(indicatorRows, []any{code, year, v, r["Unit"], srcID})
			}
			return nil
		})
		if err != nil {
			return fail(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_agriculture_indicateur (
			code text NOT NULL,
			annee integer NOT NULL,
			valeur numeric NOT NULL,
			unite text NOT NULL,
			source_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_agriculture_indicateur"},
		[]string{"code", "annee", "valeur", "unite", "source_id"},
		pgx.CopyFromRows(indicatorRows)); err != nil {
		return fail(fmt.Errorf("copie des indicateurs : %w", err))
	}
	ctIndicator, err := tx.Exec(ctx, `
		MERGE INTO core.agriculture_indicateur AS tgt
		USING tmp_agriculture_indicateur AS src
		ON tgt.code = src.code AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.valeur, tgt.unite, tgt.source_id)
			IS DISTINCT FROM (src.valeur, src.unite, src.source_id) THEN
			UPDATE SET valeur = src.valeur, unite = src.unite, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (code, annee, valeur, unite, source_id)
			VALUES (src.code, src.annee, src.valeur, src.unite, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion des indicateurs : %w", err))
	}
	nIndicator := ctIndicator.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"bilan": nBalance, "indicateurs": nIndicator, "produits": len(products)}, "")
	fmt.Printf("  FAOSTAT : %d valeurs de bilan touchées sur %d produits, %d indicateurs de production touchés\n",
		nBalance, len(products), nIndicator)
	return nil
}

// walkFAOSTATCSV lit le CSV « NOFLAG » d'une archive FAOSTAT et appelle fn par
// ligne, avec les colonnes descriptives d'un côté et les colonnes annuelles de
// l'autre. Le fichier est en ISO-8859-1 et ses colonnes d'années s'appellent
// « Y1961 », « Y1962 », etc.
func walkFAOSTATCSV(path string, fn func(r map[string]string, years map[int]string) error) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()

	var f *zip.File
	for _, x := range zr.File {
		if strings.HasSuffix(x.Name, "NOFLAG.csv") {
			f = x
			break
		}
	}
	if f == nil {
		return fmt.Errorf("%s : aucun fichier NOFLAG", path)
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	b, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	r := csv.NewReader(strings.NewReader(latin1(b)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	head, err := r.Read()
	if err != nil {
		return err
	}
	desc := map[string]int{}
	yearCols := map[int]int{}
	for i, h := range head {
		h = strings.TrimSpace(h)
		if len(h) == 5 && h[0] == 'Y' {
			if n, err := strconv.Atoi(h[1:]); err == nil {
				yearCols[n] = i
				continue
			}
		}
		desc[h] = i
	}

	for {
		rec, err := r.Read()
		if err != nil {
			return nil
		}
		m := make(map[string]string, len(desc))
		for k, i := range desc {
			if i < len(rec) {
				m[k] = strings.TrimSpace(rec[i])
			}
		}
		years := make(map[int]string, len(yearCols))
		for year, i := range yearCols {
			if i < len(rec) && strings.TrimSpace(rec[i]) != "" {
				years[year] = rec[i]
			}
		}
		if err := fn(m, years); err != nil {
			return err
		}
	}
}

// productCode fabrique un code stable à partir du libellé de la FAO, qui ne
// publie pas de code court lisible. Le libellé d'origine est conservé à côté.
func productCode(item string) string {
	s := strings.ToLower(item)
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
