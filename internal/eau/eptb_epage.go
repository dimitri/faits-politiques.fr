package eau

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

const ConnectorVersionEPTBEPAGE = "eau-eptb-epage-v1"

// SourceBanaticEPTBEPAGE : BANATIC (base nationale sur l'intercommunalité,
// DGCL) est un registre déclaratif des groupements de collectivités, pas une
// base cartographique — mais deux de ses colonnes (EPAGE, EPTB) identifient
// les structures voulues, et sa liste de membres (communes ou EPCI) permet
// de reconstruire un contour par union de géométries déjà chargées dans ce
// dépôt (geo.contour_cog), le seul chemin trouvé après vérification qu'aucun
// périmètre géographique national des EPTB/EPAGE n'existe en open data
// (chaque DREAL publie, ou non, sa propre couche régionale, dans des formats
// disparates et très incomplets à l'inspection).
var SourceBanaticEPTBEPAGE = archive.Source{
	Slug: "banatic-eptb-epage", Label: "BANATIC — établissements publics territoriaux de bassin (EPTB) et d'aménagement et de gestion des eaux (EPAGE)",
	Publisher: "Direction générale des collectivités locales (DGCL)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : BANATIC (DGCL)",
	Cadence:     "continue",
	Notes: "Registre déclaratif : le nombre de structures reconnues ici (colonnes EPAGE/EPTB) peut " +
		"différer légèrement du décompte de l'ANEB (association professionnelle des établissements " +
		"de bassin), qui suit les arrêtés préfectoraux directement. Le contour de chaque structure " +
		"est reconstruit par union de ses communes et EPCI membres déjà chargés dans ce dépôt, pas " +
		"téléchargé comme tel : un membre qui n'est ni une commune ni un EPCI (un autre syndicat " +
		"mixte, par exemple) reste hors du contour reconstruit.",
}

const (
	urlBanaticExport         = "https://www.banatic.interieur.gouv.fr/consultation/api/export/pregenere/telecharger/France"
	urlBanaticCorrespondance = "https://static.data.gouv.fr/resources/base-nationale-sur-les-intercommunalites/20250312-083024/correspondance-code-commune-siren.csv"

	colGroupementSiren    = 3
	colGroupementNom      = 4
	colNatureJuridique    = 5
	colEPAGE              = 14
	colEPTB               = 15
	colPopulationTotale   = 39
	colNbMembres          = 44
	colMembreSiren        = 46
	colMembreNom          = 47
	colMembreCategorie    = 48
	colMembrePopulation   = 49
	colBanaticColonneMini = colMembrePopulation + 1
)

type groupementEPTBEPAGE struct {
	Nom              string
	Type             string
	NatureJuridique  *string
	CodeDepartement  *string
	PopulationTotale *int
	NbMembres        *int
}

type membreEPTBEPAGE struct {
	EPTBSiren        string
	MembreSiren      string
	MembreNom        *string
	Categorie        *string
	CommuneCode      *string
	PopulationMembre *int
}

// chargerCorrespondanceCommuneSIREN lit la table de passage BANATIC entre
// SIREN de commune et code INSEE, nécessaire car « Siren membre » désigne la
// commune par son SIREN de collectivité, pas par son code INSEE.
func chargerCorrespondanceCommuneSIREN(chemin string) (map[string]string, error) {
	brut, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	cr := csv.NewReader(strings.NewReader(latin1VersUTF8(brut)))
	cr.Comma = ';'
	entete, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("en-tête illisible : %w", err)
	}
	idx := map[string]int{}
	for i, c := range entete {
		idx[strings.TrimSpace(c)] = i
	}
	for _, c := range []string{"Code INSEE de la commune", "Siren"} {
		if _, ok := idx[c]; !ok {
			return nil, fmt.Errorf("colonne %q absente de la correspondance commune/SIREN", c)
		}
	}
	m := map[string]string{}
	for {
		r, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		siren := strings.TrimSpace(r[idx["Siren"]])
		insee := strings.TrimSpace(r[idx["Code INSEE de la commune"]])
		if siren != "" && insee != "" {
			m[siren] = insee
		}
	}
	if len(m) < 30000 {
		return nil, fmt.Errorf("correspondance commune/SIREN : seulement %d entrées, attendu environ 34 800", len(m))
	}
	return m, nil
}

// latin1VersUTF8 convertit de l'ISO-8859-1 vers UTF-8, écrit à la main plutôt
// que par une dépendance : la correspondance commune/SIREN de BANATIC est le
// seul fichier de ce connecteur à ne pas être déjà en UTF-8.
func latin1VersUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func entierOuNil(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func texteOuNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// IngestEPTBEPAGE reconstruit la liste et le contour des EPTB/EPAGE à partir
// de l'export national BANATIC (~140 000 lignes groupement × membre) : une
// passe unique en flux (le fichier est trop volumineux pour être chargé
// entièrement en mémoire par ligne matricielle), puis une union SQL des
// géométries déjà chargées pour chaque structure retenue.
func IngestEPTBEPAGE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceBanaticEPTBEPAGE)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionEPTBEPAGE)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	fCorr, err := arch.Fetch(ctx, srcID, runID, urlBanaticCorrespondance, ".csv")
	if err != nil {
		return fail(err)
	}
	correspondance, err := chargerCorrespondanceCommuneSIREN(fCorr.Path)
	if err != nil {
		return fail(fmt.Errorf("correspondance commune/SIREN : %w", err))
	}

	fExport, err := arch.Fetch(ctx, srcID, runID, urlBanaticExport, ".xlsx")
	if err != nil {
		return fail(err)
	}

	wb, err := excelize.OpenFile(fExport.Path)
	if err != nil {
		return fail(fmt.Errorf("export BANATIC illisible : %w", err))
	}
	defer wb.Close()
	sheets := wb.GetSheetList()
	if len(sheets) == 0 {
		return fail(fmt.Errorf("export BANATIC sans feuille"))
	}
	rows, err := wb.Rows(sheets[0])
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return fail(fmt.Errorf("export BANATIC vide"))
	}
	header, err := rows.Columns()
	if err != nil {
		return fail(err)
	}
	attendues := map[int]string{
		colGroupementSiren: "N° SIREN", colGroupementNom: "Nom du groupement",
		colNatureJuridique: "Nature juridique", colEPAGE: "EPAGE", colEPTB: "EPTB",
		colPopulationTotale: "Population totale", colNbMembres: "Nombre de membres",
		colMembreSiren: "Siren membre", colMembreNom: "Nom membre",
		colMembreCategorie:  "Catégorie des membres du groupement",
		colMembrePopulation: "Population totale du membre du groupement",
	}
	for i, attendu := range attendues {
		if i >= len(header) || strings.TrimSpace(header[i]) != attendu {
			return fail(fmt.Errorf("colonne %d attendue %q, le format BANATIC a peut-être changé", i, attendu))
		}
	}

	groupements := map[string]*groupementEPTBEPAGE{}
	var membres []membreEPTBEPAGE
	n := 0
	for rows.Next() {
		n++
		r, err := rows.Columns()
		if err != nil {
			return fail(fmt.Errorf("ligne %d illisible : %w", n+1, err))
		}
		if len(r) < colBanaticColonneMini {
			continue
		}
		epage := strings.TrimSpace(r[colEPAGE])
		eptb := strings.TrimSpace(r[colEPTB])
		if epage != "OUI" && eptb != "OUI" {
			continue
		}
		siren := strings.TrimSpace(r[colGroupementSiren])
		if siren == "" {
			return fail(fmt.Errorf("ligne %d : SIREN de groupement vide", n+1))
		}
		if _, ok := groupements[siren]; !ok {
			typ := "EPTB"
			switch {
			case eptb == "OUI" && epage == "OUI":
				typ = "EPTB_EPAGE"
			case epage == "OUI":
				typ = "EPAGE"
			}
			var dept *string
			if dep := strings.TrimSpace(r[0]); dep != "" {
				if i := strings.Index(dep, " - "); i > 0 {
					d := dep[:i]
					dept = &d
				}
			}
			groupements[siren] = &groupementEPTBEPAGE{
				Nom: strings.TrimSpace(r[colGroupementNom]), Type: typ,
				NatureJuridique: texteOuNil(r[colNatureJuridique]), CodeDepartement: dept,
				PopulationTotale: entierOuNil(r[colPopulationTotale]), NbMembres: entierOuNil(r[colNbMembres]),
			}
		}
		membreSiren := strings.TrimSpace(r[colMembreSiren])
		if membreSiren == "" {
			continue
		}
		categorie := texteOuNil(r[colMembreCategorie])
		var communeCode *string
		if categorie != nil && *categorie == "commune" {
			if code, ok := correspondance[membreSiren]; ok {
				communeCode = &code
			}
		}
		membres = append(membres, membreEPTBEPAGE{
			EPTBSiren: siren, MembreSiren: membreSiren, MembreNom: texteOuNil(r[colMembreNom]),
			Categorie: categorie, CommuneCode: communeCode, PopulationMembre: entierOuNil(r[colMembrePopulation]),
		})
	}
	if len(groupements) == 0 {
		return fail(fmt.Errorf("aucun EPTB/EPAGE trouvé — le format BANATIC a peut-être changé"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.contour_eptb_epage`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.eptb_epage_membre`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.eptb_epage`); err != nil {
		return fail(err)
	}

	groupementRows := make([][]any, 0, len(groupements))
	for siren, g := range groupements {
		groupementRows = append(groupementRows, []any{
			siren, g.Nom, g.Type, g.NatureJuridique, g.CodeDepartement, g.PopulationTotale, g.NbMembres, srcID,
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "eptb_epage"},
		[]string{"siren", "nom", "type", "nature_juridique", "code_departement", "population_totale", "nb_membres", "source_id"},
		pgx.CopyFromRows(groupementRows)); err != nil {
		return fail(fmt.Errorf("core.eptb_epage : %w", err))
	}

	membreRows := make([][]any, 0, len(membres))
	for _, m := range membres {
		membreRows = append(membreRows, []any{
			m.EPTBSiren, m.MembreSiren, m.MembreNom, m.Categorie, m.CommuneCode, m.PopulationMembre,
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "eptb_epage_membre"},
		[]string{"eptb_siren", "membre_siren", "membre_nom", "categorie", "commune_code", "population_membre"},
		pgx.CopyFromRows(membreRows)); err != nil {
		return fail(fmt.Errorf("core.eptb_epage_membre : %w", err))
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO geo.contour_eptb_epage (siren, geom, nb_membres_resolus, nb_membres_total, source_id)
		SELECT e.siren,
		       ST_Multi(ST_Union(g.geom)),
		       count(g.geom),
		       count(*),
		       $1
		FROM core.eptb_epage e
		JOIN core.eptb_epage_membre m ON m.eptb_siren = e.siren
		LEFT JOIN geo.contour_cog g ON g.cog_millesime = 2026 AND (
		     (m.categorie = 'commune' AND g.niveau = 'COMMUNE' AND g.code = m.commune_code)
		  OR (m.categorie = 'groupement' AND g.niveau = 'EPCI' AND g.code = m.membre_siren)
		)
		GROUP BY e.siren
		HAVING count(g.geom) > 0`, srcID); err != nil {
		return fail(fmt.Errorf("union des contours : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"structures": len(groupements), "membres": len(membres)}, "")
	fmt.Printf("  EPTB/EPAGE (BANATIC) : %d structures, %d membres\n", len(groupements), len(membres))
	return nil
}
