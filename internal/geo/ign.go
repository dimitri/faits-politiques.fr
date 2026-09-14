// Package geo charge les contours administratifs datés par millésime du COG.
//
// Il complète geo.contour (régions et départements, OpenStreetMap) sans le
// modifier : les communes ont besoin d'un millésime, et l'IGN est la seule source
// qui publie des contours communaux alignés sur chaque millésime du COG.
package geo

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "geo-ign-v1"

var SourceIGN = archive.Source{
	Slug: "ign-admin-express-cog-carto", Label: "IGN — Admin Express COG CARTO, petite échelle",
	Publisher: "Institut national de l'information géographique et forestière", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : IGN, Admin Express COG CARTO",
	Cadence:     "annuelle, un millésime par COG",
	Notes: "Version petite échelle (PE) : géométrie généralisée pour la cartographie " +
		"nationale, qui ne se prête à aucune mesure. La superficie publiée est " +
		"cadastrale et fournie comme attribut ; elle n'est jamais recalculée depuis " +
		"la géométrie. Les SIREN d'intercommunalités sont une seconde source " +
		"d'appartenance, indépendante de BANATIC.",
}

const wfs = "https://data.geopf.fr/wfs/ows?SERVICE=WFS&VERSION=2.0.0&REQUEST=GetFeature" +
	"&OUTPUTFORMAT=application/json&SORTBY=code_insee"

// Taille de page. Le tri explicite sur code_insee n'est pas un détail : sans lui,
// une pagination WFS peut renvoyer deux fois la même entité et en omettre une
// autre d'une page à la suivante, sans erreur. La sonde de complétude le verrait,
// mais autant ne pas le provoquer.
const pageWFS = 5000

// Millésimes chargés. Ils suivent ceux de ref.commune (communes.MillesimesCOG).
var Millesimes = []int{2025, 2026}

type entite struct {
	Properties struct {
		CodeInsee  string   `json:"code_insee"`
		Nom        string   `json:"nom_officiel"`
		Dep        string   `json:"code_insee_du_departement"`
		Reg        string   `json:"code_insee_de_la_region"`
		Superficie *float64 `json:"superficie_cadastrale"`
		Population *float64 `json:"population"`
		SirenEPCI  string   `json:"codes_siren_des_epci"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// courant est le millésime de référence de la base (communes.COGMillesime) : pour
// lui, l'appartenance aux intercommunalités vient de BANATIC ; pour les millésimes
// antérieurs, de l'IGN, seule source datée disponible.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, projectionsCSV string, courant int) error {
	srids, err := lireProjections(projectionsCSV)
	if err != nil {
		return err
	}
	for _, an := range Millesimes {
		if err := chargerMillesime(ctx, pool, arch, an, srids, courant); err != nil {
			return fmt.Errorf("contours %d : %w", an, err)
		}
	}
	return nil
}

func chargerMillesime(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, an int, srids map[string]int, courant int) error {
	srcID, err := arch.EnsureSource(ctx, SourceIGN)
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
	couche := fmt.Sprintf("ADMINEXPRESS-COG-CARTO-PE.%d:commune", an)

	var rows [][]any
	var recues, rejetSansCode, rejetSansGeom, rejetDoublon int
	vus := map[string]bool{}
	for debut := 0; ; debut += pageWFS {
		url := fmt.Sprintf("%s&TYPENAMES=%s&COUNT=%d&STARTINDEX=%d", wfs, couche, pageWFS, debut)
		f, err := arch.Fetch(ctx, srcID, runID, url, ".geojson")
		if err != nil {
			return fail(err)
		}
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return fail(err)
		}
		var page struct {
			Features []entite `json:"features"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return fail(fmt.Errorf("page %d illisible : %w", debut, err))
		}
		for _, e := range page.Features {
			recues++
			p := e.Properties
			if strings.TrimSpace(p.CodeInsee) == "" {
				rejetSansCode++
				continue
			}
			if len(e.Geometry) == 0 || string(e.Geometry) == "null" {
				rejetSansGeom++
				continue
			}
			if vus[p.CodeInsee] {
				rejetDoublon++
				continue
			}
			vus[p.CodeInsee] = true
			// « 200054781/200057974 » pour une commune du Grand Paris (Métropole
			// et EPT), « NR » pour les quatre îles dispensées d'intercommunalité.
			// Seul un SIREN de neuf chiffres est un SIREN : « NR » pris pour un
			// code aurait fabriqué une intercommunalité fictive réunissant
			// Ouessant et l'Île-d'Yeu.
			var sirens []string
			for _, s := range strings.Split(p.SirenEPCI, "/") {
				if s = strings.TrimSpace(s); estSiren(s) {
					sirens = append(sirens, s)
				}
			}
			rows = append(rows, []any{
				p.CodeInsee, an, p.Nom, string(e.Geometry), sridPour(p.Dep, p.CodeInsee, srids),
				nul(p.Dep), nul(p.Reg), p.Superficie, entierOuNil(p.Population), sirens, srcID,
			})
		}
		if len(page.Features) < pageWFS {
			break
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_cog WHERE cog_millesime = $1`, an); err != nil {
		return fail(err)
	}
	// La géométrie arrive en GeoJSON : c'est PostGIS qui la lit, pas le Go. Une
	// table temporaire évite 35 000 allers-retours.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE contour_in (code text, an int, nom text, gj text, srid int,
		  dep text, reg text, sup numeric, pop int, sirens text[], source_id bigint)
		ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"contour_in"},
		[]string{"code", "an", "nom", "gj", "srid", "dep", "reg", "sup", "pop", "sirens", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	// ST_Multi : quelques communes sont d'un seul tenant et arrivent en Polygon ;
	// la colonne est typée MultiPolygon.
	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.contour_cog (niveau, code, cog_millesime, nom, geom, srid_rendu,
		  code_departement, code_region, superficie_cadastrale_ha, population,
		  codes_siren_epci, source_id)
		SELECT 'COMMUNE', code, an, nom,
		       ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(gj), 4326)), srid,
		       dep, reg, sup, pop, sirens, source_id
		  FROM contour_in`); err != nil {
		return fail(err)
	}

	// Les intercommunalités : union des communes membres SELON L'IGN, pour le même
	// millésime. Construire le contour depuis BANATIC mêlerait deux sources et
	// deux dates ; ici le contour correspond exactement à la composition publiée
	// par le producteur de la géométrie. La concordance avec BANATIC est vérifiée
	// à part, dans cmd/verify.
	//
	// Seules les intercommunalités à fiscalité propre sont dessinées : l'IGN ne
	// publie que celles-là, et c'est heureux — les 8 000 syndicats de BANATIC se
	// superposent et ne forment pas une partition du territoire.
	// Les intercommunalités, dessinées comme l'union de leurs communes membres.
	//
	// L'appartenance et la nature viennent d'UNE seule source par millésime :
	//   - millésime courant : BANATIC, qui fait foi dans toute la base et connaît
	//     les fusions du 1er janvier. La couche IGN du même millésime peut retarder
	//     d'un an : en 2026 elle porte encore les quatre intercommunalités
	//     fusionnées en Lévézou et en Thionville Fensch Agglomération ;
	//   - millésime antérieur : l'IGN, commune par commune et couche « epci »,
	//     puisque BANATIC n'est chargé qu'à la date courante.
	// La géométrie, elle, vient toujours des contours communaux IGN du millésime.
	var membres string
	if an == courant {
		membres = `
		  SELECT m.epci_siren AS siren, m.commune_code AS code,
		         e.nom, CASE WHEN e.nature_juridique = 'EPT' THEN 'EPT' ELSE 'EPCI' END AS niveau
		    FROM core.epci_membre m
		    JOIN core.epci e ON e.siren = m.epci_siren
		   WHERE m.cog_millesime = $1
		     AND e.nature_juridique IN ('CC','CA','CU','METRO','MET69','EPT')`
	} else {
		ref, err := referentielEPCI(ctx, arch, srcID, runID, an)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE epci_ign (siren text, nom text, nature text) ON COMMIT DROP`); err != nil {
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"epci_ign"}, []string{"siren", "nom", "nature"},
			pgx.CopyFromRows(ref)); err != nil {
			return fail(err)
		}
		membres = `
		  SELECT s.siren, c.code, i.nom,
		         CASE WHEN i.nature ILIKE 'Etablissement public territorial' THEN 'EPT' ELSE 'EPCI' END AS niveau
		    FROM geo.contour_cog c
		    CROSS JOIN LATERAL unnest(c.codes_siren_epci) AS s(siren)
		    JOIN epci_ign i ON i.siren = s.siren
		   WHERE c.niveau = 'COMMUNE' AND c.cog_millesime = $1`
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.contour_cog (niveau, code, cog_millesime, nom, geom, srid_rendu,
		  code_departement, code_region, source_id)
		SELECT mb.niveau, mb.siren, $1, mb.nom,
		       ST_Multi(ST_Union(c.geom)),
		       mode() WITHIN GROUP (ORDER BY c.srid_rendu),
		       mode() WITHIN GROUP (ORDER BY c.code_departement),
		       mode() WITHIN GROUP (ORDER BY c.code_region),
		       min(c.source_id)
		  FROM (`+membres+`) mb
		  JOIN geo.contour_cog c ON c.niveau = 'COMMUNE' AND c.cog_millesime = $1 AND c.code = mb.code
		 GROUP BY mb.niveau, mb.siren, mb.nom`, an); err != nil {
		return fail(fmt.Errorf("union des intercommunalités : %w", err))
	}

	// Divergence entre l'IGN et BANATIC pour le millésime courant : combien de
	// communes n'ont pas le même EPCI à fiscalité propre selon les deux sources.
	// Non nulle quand l'IGN retarde sur une fusion ; consignée, pas masquée.
	var divergence int
	if an == courant {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM geo.contour_cog c
			 WHERE c.niveau = 'COMMUNE' AND c.cog_millesime = $1
			   AND coalesce((SELECT array_agg(m.epci_siren ORDER BY m.epci_siren)
			                   FROM core.epci_membre m JOIN core.epci e ON e.siren = m.epci_siren
			                  WHERE m.commune_code = c.code AND m.cog_millesime = $1
			                    AND e.nature_juridique IN ('CC','CA','CU','METRO','MET69')), '{}')
			       <> coalesce((SELECT array_agg(s ORDER BY s) FROM unnest(c.codes_siren_epci) s
			                     WHERE s NOT IN (SELECT siren FROM core.epci WHERE nature_juridique = 'EPT')), '{}')`,
			an).Scan(&divergence); err != nil {
			return fail(err)
		}
	}

	var nEPCI, nEPT int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE niveau = 'EPCI'), count(*) FILTER (WHERE niveau = 'EPT')
		  FROM geo.contour_cog WHERE niveau IN ('EPCI','EPT') AND cog_millesime = $1`, an).Scan(&nEPCI, &nEPT); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes_recues":                   recues,
		"lignes_chargees":                 len(rows),
		"rejet_sans_code":                 rejetSansCode,
		"rejet_sans_geometrie":            rejetSansGeom,
		"rejet_doublon":                   rejetDoublon,
		"millesime":                       an,
		"epci_construits":                 nEPCI,
		"ept_construits":                  nEPT,
		"divergence_ign_banatic_communes": divergence,
	}, "")
	fmt.Printf("  contours %d : %d communes, %d intercommunalités, %d EPT\n", an, len(rows), nEPCI, nEPT)
	if divergence > 0 {
		fmt.Printf("  %d communes n'ont pas le même EPCI selon l'IGN et BANATIC (retard de l'IGN sur les fusions)\n", divergence)
	}
	return nil
}

// sridPour : la projection de rendu d'une commune se lit sur son territoire.
// Les codes d'outre-mer ont trois caractères (971, 974…), ceux de la métropole
// deux (01, 2A). Et les communes des collectivités d'outre-mer n'ont pas de
// département : l'IGN écrit « NR » pour Saint-Pierre et Miquelon-Langlade. Le
// premier chargement les dessinait donc en Lambert-93, dans le repère de
// l'hexagone. On retombe alors sur le préfixe du code commune (975…).
func sridPour(dep, codeCommune string, srids map[string]int) int {
	for _, cle := range []string{dep, prefixe(dep, 3), prefixe(codeCommune, 3)} {
		if cle == "" || cle == "NR" {
			continue
		}
		if s, ok := srids[cle]; ok {
			return s
		}
	}
	return srids["DEFAUT"]
}

func prefixe(s string, n int) string {
	if len(s) < n {
		return ""
	}
	return s[:n]
}

func lireProjections(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range recs[1:] {
		if len(r) < 2 {
			continue
		}
		if s, err := strconv.Atoi(strings.TrimSpace(r[1])); err == nil {
			out[strings.TrimSpace(r[0])] = s
		}
	}
	if _, ok := out["DEFAUT"]; !ok {
		return nil, fmt.Errorf("%s : projection DEFAUT absente", path)
	}
	return out, nil
}

// referentielEPCI lit la couche « epci » de l'IGN pour un millésime : SIREN, nom
// et nature. Une seule page suffit, il y en a environ 1 300.
func referentielEPCI(ctx context.Context, arch *archive.Archive, srcID, runID int64, an int) ([][]any, error) {
	url := fmt.Sprintf("https://data.geopf.fr/wfs/ows?SERVICE=WFS&VERSION=2.0.0&REQUEST=GetFeature"+
		"&OUTPUTFORMAT=application/json&SORTBY=code_siren&PROPERTYNAME=code_siren,nom_officiel,nature"+
		"&COUNT=%d&TYPENAMES=ADMINEXPRESS-COG-CARTO-PE.%d:epci", pageWFS, an)
	f, err := arch.Fetch(ctx, srcID, runID, url, ".geojson")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Features []struct {
			Properties struct {
				Siren  string `json:"code_siren"`
				Nom    string `json:"nom_officiel"`
				Nature string `json:"nature"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("référentiel EPCI %d illisible : %w", an, err)
	}
	if len(doc.Features) == 0 || len(doc.Features) >= pageWFS {
		return nil, fmt.Errorf("référentiel EPCI %d : %d entités, pagination à revoir", an, len(doc.Features))
	}
	out := make([][]any, 0, len(doc.Features))
	for _, e := range doc.Features {
		if estSiren(e.Properties.Siren) {
			out = append(out, []any{e.Properties.Siren, e.Properties.Nom, e.Properties.Nature})
		}
	}
	return out, nil
}

func estSiren(s string) bool {
	if len(s) != 9 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func entierOuNil(f *float64) any {
	if f == nil {
		return nil
	}
	return int(*f)
}
