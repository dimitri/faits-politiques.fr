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

var SourceMuseesFrance = archive.Source{
	Slug: "museofile", Label: "Répertoire des Musées de France (Muséofile)",
	Publisher: "Ministère de la Culture", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : ministère de la Culture, data.culture.gouv.fr",
	Cadence:     "hebdomadaire",
}

const urlMuseofile = "https://ministere-culture.s3.sbg.io.cloud.ovh.net/POP/museofile.csv"

// IngestMuseesFrance charge le répertoire des musées labellisés « Musée de
// France » (Muséofile). Voir docs/culture-donnees.md.
func IngestMuseesFrance(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMuseesFrance)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "museofile-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlMuseofile, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.Comma = '|'
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête illisible : %w", err))
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, must := range []string{"Identifiant", "Nom_officiel", "Ville", "Code_postal", "Departement", "Region", "Domaine_thematique", "Coordonnees"} {
		if _, ok := col[must]; !ok {
			return fail(fmt.Errorf("colonne %q absente", must))
		}
	}

	type ligne struct {
		identifiant, nom, ville, codePostal, departement, region, domaine string
		lon, lat                                                          float64
		aCoord                                                            bool
	}
	var lignes []ligne
	var sansCoord int
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("ligne illisible : %w", err))
		}
		l := ligne{
			identifiant: rec[col["Identifiant"]], nom: rec[col["Nom_officiel"]], ville: rec[col["Ville"]],
			codePostal: rec[col["Code_postal"]], departement: rec[col["Departement"]], region: rec[col["Region"]],
			domaine: rec[col["Domaine_thematique"]],
		}
		coord := strings.TrimSpace(rec[col["Coordonnees"]])
		if coord != "" {
			parts := strings.Split(coord, ",")
			if len(parts) == 2 {
				lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
				lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				if errLat == nil && errLon == nil {
					l.lat, l.lon, l.aCoord = lat, lon, true
				}
			}
		}
		if !l.aCoord {
			sansCoord++
		}
		lignes = append(lignes, l)
	}
	if len(lignes) < 1000 {
		return fail(fmt.Errorf("seulement %d lignes lues, attendu au moins 1000", len(lignes)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.musee_france`); err != nil {
		return fail(err)
	}
	for _, l := range lignes {
		var geomExpr any
		if l.aCoord {
			geomExpr = fmt.Sprintf("SRID=4326;POINT(%f %f)", l.lon, l.lat)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.musee_france (identifiant, nom, ville, code_postal, departement, region, domaine_thematique, geom, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,ST_GeomFromEWKT($8),$9)`,
			l.identifiant, l.nom, l.ville, nullifEmpty(l.codePostal), l.departement, l.region,
			nullifEmpty(l.domaine), geomExpr, srcID); err != nil {
			return fail(fmt.Errorf("%s : insertion : %w", l.identifiant, err))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(lignes), "sans_coordonnees": sansCoord}, "")
	fmt.Printf("  Musées de France (Muséofile) : %d musées, %d sans coordonnées\n", len(lignes), sansCoord)
	return nil
}
