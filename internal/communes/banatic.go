package communes

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BANATIC — base nationale sur les intercommunalités, tenue par la Direction
// générale des collectivités locales. C'est la seule source qui dise, commune
// par commune, QUELLES COMPÉTENCES ont été transférées à un groupement.
//
// Sans elle, un budget communal ne se lit pas : deux communes voisines peuvent
// afficher des dépenses de fonctionnement du simple au double parce que l'une a
// transféré la collecte des déchets et l'autre non.
var SourceBANATIC = archive.Source{
	Slug: "banatic", Label: "BANATIC — intercommunalités et compétences",
	Publisher: "Direction générale des collectivités locales", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "OPEN",
	Attribution: "Source : BANATIC, Direction générale des collectivités locales",
	Cadence:     "en continu, arrêtés préfectoraux",
	Notes: "L'export national est un tableau dénormalisé : une ligne par couple " +
		"(groupement, membre), 125 colonnes de compétences en OUI/NON.",
}

const (
	banaticExportURL     = "https://www.banatic.interieur.gouv.fr/consultation/api/export/pregenere/telecharger/France"
	banaticCompetenceURL = "https://www.banatic.interieur.gouv.fr/consultation/api/referentiel/competence/all"
)

// Colonnes du tableau national, en numérotation 1. Elles sont repérées par
// position et non par titre : le titre sert à vérifier qu'on lit la bonne.
const (
	colDepartement  = 1
	colSiren        = 4
	colNom          = 5
	colNature       = 6
	colDateCreation = 10
	colPopulation   = 40
	colPresCivilite = 42
	colPresNom      = 43
	colPresPrenom   = 44
	colNbMembres    = 45
	colNbDelegues   = 46
	colSirenMembre  = 47
	colNomMembre    = 48
	colCategorie    = 49
	colPremiereComp = 53
	colDerniereComp = 177
)

type competence struct {
	Code      string `json:"code"`
	Libelle   string `json:"libelle"`
	Ordre     int    `json:"ordre"`
	Categorie struct {
		Code    string `json:"code"`
		Libelle string `json:"libelle"`
	} `json:"categorie"`
}

func IngestBANATIC(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceBANATIC)
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

	// 1. La nomenclature des compétences.
	fComp, err := arch.Fetch(ctx, srcID, runID, banaticCompetenceURL, ".json")
	if err != nil {
		return fail(err)
	}
	nomenclature, comps, err := lireCompetences(fComp.Path)
	if err != nil {
		return fail(err)
	}

	// 2. Le tableau national. 76 Mo compressés, 1,4 Go de XML une fois ouvert :
	//    il est lu en flux, jamais chargé en mémoire.
	fExp, err := arch.Fetch(ctx, srcID, runID, banaticExportURL, ".xlsx")
	if err != nil {
		return fail(err)
	}

	// 3. La correspondance SIREN -> code INSEE. BANATIC désigne ses membres par
	//    leur SIREN ; tout le reste de la base par leur code INSEE. L'OFGL
	//    publie les deux sur la même ligne : c'est un appariement d'identifiants,
	//    pas de noms.
	sirenVersInsee, err := correspondanceSiren(ctx, arch, srcID, runID)
	if err != nil {
		return fail(err)
	}

	connues, err := communesConnues(ctx, pool)
	if err != nil {
		return fail(err)
	}

	type epci struct {
		nom, nature, dep, creation string
		presCiv, presNom, presPre  string
		population, membres        int
		delegues                   int
		comps                      []string
	}
	epcis := map[string]*epci{}
	type membre struct{ siren, commune, categorie string }
	var membres []membre
	sansInsee := map[string]bool{}
	communesSirenInconnu := map[string]bool{}
	communesHorsCOG := map[string]bool{}
	var lignesMembre, rejetCommune, rejetNonCommune int

	var entetes []string
	err = parcourirXLSX(fExp.Path, func(n int, row map[int]string) error {
		if n == 0 {
			entetes = make([]string, colDerniereComp+1)
			for i := 1; i <= colDerniereComp; i++ {
				entetes[i] = row[i]
			}
			// Le tableau est lu par position : si les colonnes bougeaient sans
			// qu'on s'en aperçoive, on chargerait des compétences fausses sous
			// des codes justes. Ce contrôle rend la dérive impossible.
			if entetes[colSiren] != "N° SIREN" || entetes[colSirenMembre] != "Siren membre" {
				return fmt.Errorf("colonnes inattendues : %q en %d, %q en %d",
					entetes[colSiren], colSiren, entetes[colSirenMembre], colSirenMembre)
			}
			return nil
		}
		siren := strings.TrimSpace(row[colSiren])
		if siren == "" {
			return nil
		}
		e, ok := epcis[siren]
		if !ok {
			e = &epci{
				nom:      strings.TrimSpace(row[colNom]),
				nature:   strings.TrimSpace(row[colNature]),
				dep:      codeDepartement(row[colDepartement]),
				creation: strings.TrimSpace(row[colDateCreation]),
			}
			e.population = atoiSouple(row[colPopulation])
			e.membres = atoiSouple(row[colNbMembres])
			e.delegues = atoiSouple(row[colNbDelegues])
			// Le président du groupement. C'est la réponse à « qui décide, si
			// ce n'est le maire » : il arbitre des budgets souvent supérieurs à
			// ceux de ses communes membres, et n'est élu par personne
			// directement — seulement par les conseillers communautaires.
			e.presCiv = strings.TrimSpace(row[colPresCivilite])
			e.presNom = strings.TrimSpace(row[colPresNom])
			e.presPre = strings.TrimSpace(row[colPresPrenom])
			for c := colPremiereComp; c <= colDerniereComp; c++ {
				if strings.EqualFold(strings.TrimSpace(row[c]), "OUI") {
					if code, ok := comps[entetes[c]]; ok {
						e.comps = append(e.comps, code)
					}
				}
			}
			epcis[siren] = e
		}
		ms := strings.TrimSpace(row[colSirenMembre])
		if ms == "" {
			return nil
		}
		lignesMembre++
		insee, ok := sirenVersInsee[ms]
		if !ok || !connues[insee] {
			// Deux situations que l'ancien code confondait. Un membre qui
			// n'est pas une commune — autre groupement, département — est
			// légitimement écarté. Une COMMUNE dont le SIREN est introuvable
			// est une perte : c'est ce qui vidait 97 communes de leur
			// intercommunalité sans lever la moindre erreur. BANATIC dit
			// lui-même quelle nature a le membre.
			if strings.EqualFold(strings.TrimSpace(row[colCategorie]), "commune") {
				if !ok {
					communesSirenInconnu[ms] = true
				} else {
					communesHorsCOG[ms] = true
				}
				rejetCommune++
			} else {
				sansInsee[ms] = true
				rejetNonCommune++
			}
			return nil
		}
		membres = append(membres, membre{siren, insee, strings.TrimSpace(row[colCategorie])})
		return nil
	})
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	for _, c := range nomenclature {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ref.competence (code, libelle, categorie_code, categorie, ordre)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (code) DO UPDATE SET libelle = EXCLUDED.libelle,
			  categorie_code = EXCLUDED.categorie_code, categorie = EXCLUDED.categorie`,
			c.Code, c.Libelle, c.Categorie.Code, c.Categorie.Libelle, c.Ordre); err != nil {
			return fail(err)
		}
	}

	// Reconstruction complète : un groupement dissous doit disparaître.
	for _, q := range []string{
		`DELETE FROM core.epci_competence`, `DELETE FROM core.epci_membre`, `DELETE FROM core.epci`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fail(err)
		}
	}

	var lignesE, lignesC [][]any
	for siren, e := range epcis {
		var creation any
		if t, err := time.Parse("02/01/2006", e.creation); err == nil {
			creation = t
		}
		lignesE = append(lignesE, []any{siren, e.nom, e.nature, nul(e.dep), creation,
			nulZero(e.population), nulZero(e.membres), srcID,
			nul(e.presCiv), nul(e.presNom), nul(e.presPre), nulZero(e.delegues)})
		for _, c := range e.comps {
			lignesC = append(lignesC, []any{siren, c})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "epci"},
		[]string{"siren", "nom", "nature_juridique", "code_departement", "date_creation",
			"population_totale", "nb_membres", "source_id",
			"president_civilite", "president_nom", "president_prenom", "nb_delegues"},
		pgx.CopyFromRows(lignesE)); err != nil {
		return fail(fmt.Errorf("copie des groupements : %w", err))
	}

	vus := map[string]bool{}
	var lignesM [][]any
	var rejetDoublon int
	for _, m := range membres {
		k := m.siren + "|" + m.commune
		if vus[k] {
			rejetDoublon++
			continue
		}
		vus[k] = true
		lignesM = append(lignesM, []any{m.siren, m.commune, COGMillesime, m.siren, nul(m.categorie)})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "epci_membre"},
		[]string{"epci_siren", "commune_code", "cog_millesime", "membre_siren", "categorie"},
		pgx.CopyFromRows(lignesM)); err != nil {
		return fail(fmt.Errorf("copie des membres : %w", err))
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "epci_competence"},
		[]string{"epci_siren", "competence_code"}, pgx.CopyFromRows(lignesC)); err != nil {
		return fail(fmt.Errorf("copie des compétences : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	// Convention de complétude lue par cmd/verify : toute ligne d'adhésion lue
	// est chargée ou rejetée sous un motif nommé.
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes_recues":             lignesMembre,
		"lignes_chargees":           len(lignesM),
		"rejet_membre_non_commune":  rejetNonCommune,
		"rejet_commune_non_resolue": rejetCommune,
		"rejet_doublon":             rejetDoublon,
		"groupements":               len(lignesE),
		"competences_exercees":      len(lignesC),
		"communes_siren_inconnu":    len(communesSirenInconnu),
		"communes_hors_cog":         len(communesHorsCOG),
	}, "")
	fmt.Printf("  BANATIC : %d groupements, %d adhésions de communes, %d compétences exercées\n",
		len(lignesE), len(lignesM), len(lignesC))
	fmt.Printf("  %d membres qui ne sont pas des communes (autres groupements, départements) : ignorés\n",
		len(sansInsee))
	if n := len(communesSirenInconnu) + len(communesHorsCOG); n > 0 {
		fmt.Printf("  ATTENTION : %d communes membres non résolues (%d SIREN inconnus, %d hors COG %d)\n",
			n, len(communesSirenInconnu), len(communesHorsCOG), COGMillesime)
	}
	return nil
}

// correspondanceSiren construit SIREN -> code INSEE à partir de l'OFGL, qui
// publie les deux identifiants sur la même ligne.
func correspondanceSiren(ctx context.Context, arch *archive.Archive, srcID, runID int64) (map[string]string, error) {
	// Tous les exercices, pas le seul dernier. La correspondance lisait
	// autrefois l'exercice 2025 seul : or les comptes d'une année ne sont
	// complets que tard l'année suivante, et en septembre 2026 environ deux
	// cents communes n'avaient pas encore de compte 2025 publié. Elles
	// n'avaient donc pas de SIREN, et disparaissaient en silence de leur
	// intercommunalité — 97 des 101 communes « sans EPCI » venaient de là.
	// L'agrégat ne sert qu'à obtenir une ligne par commune et par exercice.
	url := "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/" + ofglDataset +
		"/exports/csv?delimiter=%3B&select=com_code,siren,exer&where=" +
		"agregat%3D%22Encours%20de%20dette%22"
	f, err := arch.Fetch(ctx, srcID, runID, url, ".csv")
	if err != nil {
		return nil, err
	}
	recs, err := lireCSV(f.Path, ';')
	if err != nil {
		return nil, err
	}
	// Un SIREN peut, rarement, avoir désigné deux codes INSEE au fil des
	// fusions : on retient l'exercice le plus récent. Les dates ISO se comparent
	// comme des chaînes.
	out := make(map[string]string, len(recs))
	annee := make(map[string]string, len(recs))
	for _, r := range recs {
		s, c, e := r["siren"], r["com_code"], r["exer"]
		if s == "" || c == "" {
			continue
		}
		if prev, ok := annee[s]; !ok || e > prev {
			out[s], annee[s] = c, e
		}
	}
	return out, nil
}

// lireCompetences renvoie la nomenclature et l'index libellé -> code. Le
// tableau national ne nomme ses colonnes que par le libellé ; c'est le même
// producteur qui publie les deux, l'égalité est donc exacte et vérifiée (125
// libellés sur 125 s'apparient).
func lireCompetences(path string) ([]competence, map[string]string, error) {
	b, err := readFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Data []competence `json:"data"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Data) == 0 {
		return nil, nil, fmt.Errorf("%s : aucune compétence", path)
	}
	index := make(map[string]string, len(doc.Data))
	for _, c := range doc.Data {
		index[c.Libelle] = c.Code
	}
	return doc.Data, index, nil
}

// parcourirXLSX lit la première feuille d'un classeur en flux et appelle fn une
// fois par ligne, avec les cellules indexées en numérotation 1.
//
// Écrit à la main plutôt que par une bibliothèque : le fichier fait 1,4 Go une
// fois décompressé, et les lecteurs XLSX courants construisent le classeur
// entier en mémoire. Un flux de jetons n'en garde qu'une ligne. Le classeur
// n'utilise pas de table de chaînes partagées — toutes les valeurs sont en
// ligne — ce qui rend la lecture d'autant plus directe.
func parcourirXLSX(path string, fn func(n int, row map[int]string) error) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()

	var sheet *zip.File
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			sheet = f
			break
		}
	}
	if sheet == nil {
		return fmt.Errorf("%s : feuille introuvable", path)
	}
	rc, err := sheet.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	var (
		n       int
		row     map[int]string
		col     int
		dansCel bool
		valeur  strings.Builder
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = map[int]string{}
			case "c":
				col = 0
				for _, a := range t.Attr {
					if a.Name.Local == "r" {
						col = colonneDepuisRef(a.Value)
					}
				}
				dansCel = true
				valeur.Reset()
			case "t", "v":
				// le texte est collecté par le cas CharData ci-dessous
			}
		case xml.CharData:
			if dansCel {
				valeur.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "c":
				if row != nil && col > 0 {
					row[col] = valeur.String()
				}
				dansCel = false
			case "row":
				if err := fn(n, row); err != nil {
					return err
				}
				n++
				row = nil
			}
		}
	}
}

// colonneDepuisRef traduit « BC1234 » en 55.
func colonneDepuisRef(ref string) int {
	n := 0
	for _, c := range ref {
		if c < 'A' || c > 'Z' {
			break
		}
		n = n*26 + int(c-'A') + 1
	}
	return n
}

func codeDepartement(s string) string {
	// « 01 - Ain » -> « 01 »
	if i := strings.Index(s, " - "); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func atoiSouple(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	s = strings.ReplaceAll(s, " ", "")
	if i := strings.IndexAny(s, ",."); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}

func nulZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
