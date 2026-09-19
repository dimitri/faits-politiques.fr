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

var SourceEffectifsEtudiants = archive.Source{
	Slug: "sies-atlas-effectifs-etudiants", Label: "Atlas régional des effectifs d'étudiants, détail par établissement",
	Publisher: "SIES (ministère de l'Enseignement supérieur et de la Recherche)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : SIES, data.enseignementsup-recherche.gouv.fr",
	Cadence:     "annuelle",
	Notes: "Agrégé ici par commune et par rentrée à partir du détail établissement × " +
		"composante × filière de la source. Un même étudiant en double inscription peut " +
		"être compté deux fois, comme dans la source elle-même.",
}

const urlEffectifsEtudiants = "https://data.enseignementsup-recherche.gouv.fr/api/explore/v2.1/catalog/datasets/fr-esr-atlas_regional-effectifs-d-etudiants-inscrits-detail_etablissements/exports/csv"

// IngestEffectifsEtudiants charge les effectifs étudiants, agrégés par
// commune et par rentrée. Voir docs/recherche-enseignement-superieur-donnees.md.
func IngestEffectifsEtudiants(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEffectifsEtudiants)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "effectifs-etudiants-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlEffectifsEtudiants, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.Comma = ';'
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête illisible : %w", err))
	}
	// La première colonne porte parfois un BOM UTF-8 résiduel.
	header[0] = strings.TrimPrefix(header[0], "\xEF\xBB\xBF")
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, must := range []string{"rentree", "com_code", "com_nom", "geo", "effectifhdccpge"} {
		if _, ok := col[must]; !ok {
			return fail(fmt.Errorf("colonne %q absente (en-tête : %v)", must, header))
		}
	}

	type cle struct {
		codeInsee string
		rentree   int
	}
	agrege := map[cle]int64{}
	noms := map[string]string{}
	geoms := map[string]string{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("ligne illisible : %w", err))
		}
		code := strings.TrimSpace(rec[col["com_code"]])
		if code == "" {
			continue
		}
		annee, err := strconv.Atoi(strings.TrimSpace(rec[col["rentree"]]))
		if err != nil {
			continue
		}
		eff, err := strconv.ParseInt(strings.TrimSpace(rec[col["effectifhdccpge"]]), 10, 64)
		if err != nil {
			continue
		}
		agrege[cle{code, annee}] += eff
		if _, ok := noms[code]; !ok {
			noms[code] = strings.TrimSpace(rec[col["com_nom"]])
		}
		if _, ok := geoms[code]; !ok {
			if g := strings.TrimSpace(rec[col["geo"]]); g != "" {
				geoms[code] = g
			}
		}
	}
	if len(agrege) < 1000 {
		return fail(fmt.Errorf("seulement %d couples commune/rentrée lus, attendu au moins 1000", len(agrege)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.effectifs_etudiants_commune`); err != nil {
		return fail(err)
	}
	for k, eff := range agrege {
		var geomExpr any
		if g, ok := geoms[k.codeInsee]; ok {
			parts := strings.Split(g, ",")
			if len(parts) == 2 {
				lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
				lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				if errLat == nil && errLon == nil {
					geomExpr = fmt.Sprintf("SRID=4326;POINT(%f %f)", lon, lat)
				}
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.effectifs_etudiants_commune (code_insee, commune, rentree, effectif, geom, source_id)
			VALUES ($1,$2,$3,$4,ST_GeomFromEWKT($5),$6)
			ON CONFLICT (code_insee, rentree) DO NOTHING`,
			k.codeInsee, noms[k.codeInsee], k.rentree, eff, geomExpr, srcID); err != nil {
			return fail(fmt.Errorf("%s %d : insertion : %w", k.codeInsee, k.rentree, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(agrege)}, "")
	fmt.Printf("  Effectifs étudiants par commune (SIES) : %d couples commune/rentrée\n", len(agrege))
	return nil
}
