package geo

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les circonscriptions législatives : contour, composition communale et
// indicateurs, trois fichiers de la publication Insee « Portraits des
// circonscriptions législatives ». Voir db/migrations/0110.

const ConnectorVersionCirco = "geo-circonscriptions-v1"

var SourceCirconscriptions = archive.Source{
	Slug: "insee-circonscriptions-legislatives", Label: "Insee — portraits des circonscriptions législatives",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, portraits des circonscriptions législatives",
	Cadence:     "au redécoupage ou à chaque élection législative",
	Notes: "Fond cartographique du 3 mai 2022 (558 circonscriptions de métropole et des DROM, " +
		"géométrie simplifiée, WGS84) ; correspondance communes → circonscriptions en géographie " +
		"au 1er janvier 2021, une commune pouvant relever de plusieurs circonscriptions ; " +
		"indicateurs : population légale 2019 et 2013, inscrits au 11 avril 2022, recensement " +
		"2018, Filosofi 2019, BPE 2020. Aucune donnée pour les onze circonscriptions des " +
		"Français établis hors de France.",
}

const (
	urlFondCirco        = "https://www.insee.fr/fr/statistiques/fichier/6441661/contours_circonscriptions_legislatives_03052022.zip"
	urlCompositionCirco = "https://www.insee.fr/fr/statistiques/fichier/6436476/circo_composition.xlsx"
	urlIndicateursCirco = "https://www.insee.fr/fr/statistiques/fichier/6436476/indic-stat-circonscriptions-legislatives-2022.xlsx"
	millesimeCompoCirco = 2021
	nbCircoAttendues    = 566 // 577 sièges, moins les 11 circonscriptions des Français de l'étranger
	nbContoursAttendus  = 558 // les mêmes, moins les 8 des collectivités d'outre-mer
)

var reCodeCirco = regexp.MustCompile(`^(?:[0-9]{2}|2[AB])[0-9]{3}$`)

// deptDeCirco : « 24001 » → 24, « 2A002 » → 2A, « 97302 » → 973, « 98801 » → 988.
func deptDeCirco(code string) string {
	if strings.HasPrefix(code, "97") || strings.HasPrefix(code, "98") {
		return code[:3]
	}
	return code[:2]
}

type circoIn struct {
	code, nom, dep string
}

func IngestCirconscriptions(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, projectionsCSV string) error {
	srids, err := lireProjections(projectionsCSV)
	if err != nil {
		return err
	}
	srcID, err := arch.EnsureSource(ctx, SourceCirconscriptions)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionCirco)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	// ── Indicateurs : la liste des circonscriptions fait foi ──────────────
	fInd, err := arch.Fetch(ctx, srcID, runID, urlIndicateursCirco, ".xlsx")
	if err != nil {
		return fail(err)
	}
	xi, err := openXLSX(fInd.Path)
	if err != nil {
		return fail(err)
	}
	defer xi.Close()
	defs, err := xi.rows("Liste des variables")
	if err != nil {
		return fail(err)
	}
	libelles := map[string][2]string{}
	for _, l := range defs {
		if l["A"] != "" && l["B"] != "" && l["C"] != "" {
			libelles[l["A"]] = [2]string{l["B"], l["C"]}
		}
	}
	lignes, err := xi.rows("indicateurs_circonscriptions")
	if err != nil {
		return fail(err)
	}
	var entete map[string]string // colonne → variable
	var circos []circoIn
	var indic, variables [][]any
	varVue := map[string]bool{}
	var nd int
	for _, l := range lignes {
		if l["A"] == "circo" {
			entete = l
			continue
		}
		if entete == nil || !reCodeCirco.MatchString(l["A"]) {
			continue
		}
		code := l["A"]
		if code != "00000" {
			circos = append(circos, circoIn{code: code, nom: strings.Join(strings.Fields(l["B"]), " "), dep: deptDeCirco(code)})
		}
		for col, v := range entete {
			if col == "A" || col == "B" {
				continue
			}
			def, ok := libelles[v]
			if !ok {
				return fail(fmt.Errorf("variable %q sans définition dans la feuille « Liste des variables »", v))
			}
			if !varVue[v] {
				varVue[v] = true
				variables = append(variables, []any{v, def[0], def[1]})
			}
			x, err := strconv.ParseFloat(strings.ReplaceAll(l[col], ",", "."), 64)
			if err != nil {
				nd++ // « nd » : non déterminé, ou cellule vide
				continue
			}
			indic = append(indic, []any{code, v, x, srcID})
		}
	}
	if len(circos) != nbCircoAttendues {
		return fail(fmt.Errorf("%d circonscriptions dans les indicateurs, %d attendues", len(circos), nbCircoAttendues))
	}
	connue := map[string]bool{}
	for _, c := range circos {
		connue[c.code] = true
	}

	// ── Composition communale ──────────────────────────────────────────────
	fCom, err := arch.Fetch(ctx, srcID, runID, urlCompositionCirco, ".xlsx")
	if err != nil {
		return fail(err)
	}
	xc, err := openXLSX(fCom.Path)
	if err != nil {
		return fail(err)
	}
	defer xc.Close()
	var compo [][]any
	vuCompo := map[[2]string]bool{}
	circoAvecCommune := map[string]bool{}
	var sansCirco int
	for _, feuille := range []struct{ nom, com, lib, circo, typ string }{
		{"table", "E", "F", "G", "H"},
		{"table COM", "C", "D", "E", "F"},
	} {
		rows, err := xc.rows(feuille.nom)
		if err != nil {
			return fail(err)
		}
		for i, l := range rows {
			if i == 0 {
				continue // en-tête
			}
			circo, com := l[feuille.circo], l[feuille.com]
			if circo == "" {
				// Les six villages de la Meuse « morts pour la France », sans
				// habitant ni électeur : aucune circonscription, et c'est exact.
				sansCirco++
				continue
			}
			if !connue[circo] {
				return fail(fmt.Errorf("feuille %q ligne %d : circonscription %q inconnue des indicateurs", feuille.nom, i+1, circo))
			}
			k := [2]string{circo, com}
			if vuCompo[k] {
				continue
			}
			vuCompo[k] = true
			circoAvecCommune[circo] = true
			compo = append(compo, []any{circo, com, l[feuille.lib], millesimeCompoCirco,
				strings.EqualFold(l[feuille.typ], "entière"), srcID})
		}
	}
	for _, c := range circos {
		if !circoAvecCommune[c.code] {
			return fail(fmt.Errorf("circonscription %s sans aucune commune dans la table de correspondance", c.code))
		}
	}

	// ── Contours ───────────────────────────────────────────────────────────
	fFond, err := arch.Fetch(ctx, srcID, runID, urlFondCirco, ".zip")
	if err != nil {
		return fail(err)
	}
	formes, err := lireShapefileCirco(fFond.Path)
	if err != nil {
		return fail(err)
	}
	var contours [][]any
	for _, f := range formes {
		// Le fond écrit la métropole sur quatre caractères (« 3803 »), comme le
		// répertoire des élus ; les indicateurs sur cinq (« 38003 »).
		if len(f.code) == 4 {
			f.code = f.code[:2] + "0" + f.code[2:]
		}
		if !connue[f.code] {
			return fail(fmt.Errorf("contour %s absent des indicateurs", f.code))
		}
		contours = append(contours, []any{f.code, f.geojson, sridPour(deptDeCirco(f.code), f.code, srids), srcID})
	}
	if len(contours) != nbContoursAttendus {
		return fail(fmt.Errorf("%d contours, %d attendus", len(contours), nbContoursAttendus))
	}

	// ── Chargement ─────────────────────────────────────────────────────────
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	for _, q := range []string{
		`DELETE FROM core.circonscription_indicateur`,
		`DELETE FROM ref.circonscription_variable`,
		`DELETE FROM ref.circonscription_legislative`, // cascade : communes et contours
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fail(err)
		}
	}
	var rc [][]any
	for _, c := range circos {
		rc = append(rc, []any{c.code, c.nom, c.dep, srcID})
	}
	copies := []struct {
		table []string
		cols  []string
		rows  [][]any
	}{
		{[]string{"ref", "circonscription_legislative"}, []string{"code", "nom", "code_departement", "source_id"}, rc},
		{[]string{"ref", "circonscription_commune"}, []string{"circonscription", "commune_code", "commune_nom", "cog_millesime", "entiere", "source_id"}, compo},
		{[]string{"ref", "circonscription_variable"}, []string{"variable", "libelle", "source"}, variables},
		{[]string{"core", "circonscription_indicateur"}, []string{"circonscription", "variable", "valeur", "source_id"}, indic},
	}
	for _, c := range copies {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier(c.table), c.cols, pgx.CopyFromRows(c.rows)); err != nil {
			return fail(fmt.Errorf("%s : %w", strings.Join(c.table, "."), err))
		}
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE circo_in (code text, gj text, srid int, source_id bigint) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"circo_in"}, []string{"code", "gj", "srid", "source_id"},
		pgx.CopyFromRows(contours)); err != nil {
		return fail(err)
	}
	// Les anneaux arrivent en lignes : ST_BuildArea reconstitue les polygones,
	// trous et îles compris, sans dépendre du sens de parcours des anneaux.
	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.contour_circonscription (code, geom, srid_rendu, source_id)
		SELECT code, ST_Multi(ST_CollectionExtract(ST_MakeValid(
		         ST_BuildArea(ST_SetSRID(ST_GeomFromGeoJSON(gj), 4326))), 3)), srid, source_id
		  FROM circo_in`); err != nil {
		return fail(fmt.Errorf("contours : %w", err))
	}
	var vides int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM geo.contour_circonscription WHERE ST_IsEmpty(geom)`).Scan(&vides); err != nil {
		return fail(err)
	}
	if vides > 0 {
		return fail(fmt.Errorf("%d contours vides après reconstruction", vides))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"circonscriptions":              len(circos),
		"contours":                      len(contours),
		"liens_communes":                len(compo),
		"communes_sans_circonscription": sansCirco,
		"variables":                     len(variables),
		"indicateurs":                   len(indic),
		"valeurs_non_determinees":       nd,
	}, "")
	fmt.Printf("  circonscriptions : %d, %d contours, %d liens communes, %d indicateurs (%d non déterminés)\n",
		len(circos), len(contours), len(compo), len(indic), nd)
	return nil
}

// ── Shapefile ─────────────────────────────────────────────────────────────

type formeCirco struct {
	code    string
	geojson string // MultiLineString des anneaux
}

// lireShapefileCirco lit le .shp et le .dbf du fond Insee : polygones simples
// (type 5), attributs en UTF-8. Juste ce que ce fichier contient, rien de plus.
func lireShapefileCirco(cheminZip string) ([]formeCirco, error) {
	zr, err := zip.OpenReader(cheminZip)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	lire := func(suffixe string) ([]byte, error) {
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), suffixe) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("aucun fichier %s dans %s", suffixe, cheminZip)
	}
	shp, err := lire(".shp")
	if err != nil {
		return nil, err
	}
	dbf, err := lire(".dbf")
	if err != nil {
		return nil, err
	}
	if prj, err := lire(".prj"); err != nil || !bytes.Contains(prj, []byte("WGS_1984")) {
		return nil, fmt.Errorf("projection du fond inattendue (WGS84 attendu)")
	}

	codes, err := lireDBFColonne(dbf, "id_circo")
	if err != nil {
		return nil, err
	}
	var out []formeCirco
	pos := 100
	for i := 0; pos+8 <= len(shp); i++ {
		longueur := int(binary.BigEndian.Uint32(shp[pos+4:])) * 2
		c := shp[pos+8 : pos+8+longueur]
		pos += 8 + longueur
		if i >= len(codes) {
			return nil, fmt.Errorf("plus de formes que d'enregistrements dans le .dbf")
		}
		switch t := binary.LittleEndian.Uint32(c); t {
		case 0:
			continue // forme nulle
		case 5:
		default:
			return nil, fmt.Errorf("forme %d de type %d, seul le polygone (5) est prévu", i, t)
		}
		nParts := int(binary.LittleEndian.Uint32(c[36:]))
		nPoints := int(binary.LittleEndian.Uint32(c[40:]))
		parts := make([]int, nParts+1)
		for p := 0; p < nParts; p++ {
			parts[p] = int(binary.LittleEndian.Uint32(c[44+4*p:]))
		}
		parts[nParts] = nPoints
		base := 44 + 4*nParts
		anneaux := make([][][2]float64, 0, nParts)
		for p := 0; p < nParts; p++ {
			var a [][2]float64
			for k := parts[p]; k < parts[p+1]; k++ {
				x := math.Float64frombits(binary.LittleEndian.Uint64(c[base+16*k:]))
				y := math.Float64frombits(binary.LittleEndian.Uint64(c[base+16*k+8:]))
				a = append(a, [2]float64{x, y})
			}
			anneaux = append(anneaux, a)
		}
		gj, err := json.Marshal(map[string]any{"type": "MultiLineString", "coordinates": anneaux})
		if err != nil {
			return nil, err
		}
		out = append(out, formeCirco{code: codes[i], geojson: string(gj)})
	}
	return out, nil
}

// lireDBFColonne : les valeurs d'une colonne texte d'un fichier dBase III.
func lireDBFColonne(dbf []byte, nom string) ([]string, error) {
	if len(dbf) < 32 {
		return nil, fmt.Errorf(".dbf tronqué")
	}
	n := int(binary.LittleEndian.Uint32(dbf[4:]))
	lgEntete := int(binary.LittleEndian.Uint16(dbf[8:]))
	lgEnreg := int(binary.LittleEndian.Uint16(dbf[10:]))
	debut, largeur := -1, 0
	decalage := 1 // l'octet d'effacement
	for off := 32; off+32 <= lgEntete && dbf[off] != 0x0D; off += 32 {
		champ := string(bytes.TrimRight(dbf[off:off+11], "\x00"))
		lg := int(dbf[off+16])
		if strings.EqualFold(champ, nom) {
			debut, largeur = decalage, lg
		}
		decalage += lg
	}
	if debut < 0 {
		return nil, fmt.Errorf("colonne %q absente du .dbf", nom)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		e := dbf[lgEntete+i*lgEnreg:]
		out = append(out, strings.TrimSpace(string(e[debut:debut+largeur])))
	}
	return out, nil
}
