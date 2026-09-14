package fiscalite

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// La sélection nommée. Le rattachement de ces sociétés à leur groupe est
// public (dénomination, communication du groupe) mais aucun identifiant ne le
// relie : GLEIF n'en connaît qu'une partie. Chaque entrée écrit son fondement.
// pays : celui de la société mère ultime, pas celui de la holding
// intermédiaire (une filiale d'Amazon détenue via le Luxembourg est « US »).
var selectionFiliales = []struct {
	groupe, pays string
	sirens       []string
	fondement    string
}{
	{"Alphabet Inc.", "US", []string{"443061841", "881721583", "799769161"}, "Google France et Google Cloud France, filiales du groupe Alphabet ; Google Ireland Limited, société irlandaise immatriculée en France, cocontractante des annonceurs français (convention judiciaire de 2019)"},
	{"Apple Inc.", "US", []string{"322120916", "483209383"}, "Apple France et Apple Retail France, filiales du groupe Apple"},
	{"Meta Platforms Inc.", "US", []string{"530085802"}, "Facebook France, filiale du groupe Meta"},
	{"Amazon.com Inc.", "US", []string{"428785042", "790933329", "823244371", "809869043", "824031090", "907946404", "831001334"}, "sociétés françaises du groupe Amazon (logistique, vente en ligne, transport, services, centres de données, numérique) ; Amazon Web Services EMEA SARL, société luxembourgeoise immatriculée en France"},
	{"Microsoft Corporation", "US", []string{"327733184", "429600158", "519059281", "431920594", "352568091", "419423728"}, "Microsoft France, Microsoft Engineering Center Paris, Microsoft Research & Development France, Microsoft EMEA et LinkedIn France (LinkedIn appartient à Microsoft depuis 2016) ; Microsoft Ireland Operations Limited, société irlandaise immatriculée en France, cocontractante du ministère de la Défense (proposition de résolution du Sénat n° 27, 2017)"},
	{"Netflix Inc.", "US", []string{"843655549"}, "Netflix Services France, filiale du groupe Netflix"},
	{"The Walt Disney Company", "US", []string{"397471822", "388457004", "383850278"}, "Euro Disney Associés (exploitant de Disneyland Paris), Euro Disneyland Imagineering et Euro Disney Vacances, contrôlées à 100 % par Walt Disney depuis le retrait de la cote en 2017"},
	{"Uber Technologies Inc.", "US", []string{"539454942"}, "Uber France, filiale du groupe Uber"},
	{"Airbnb Inc.", "US", []string{"750885410"}, "Airbnb France, filiale du groupe Airbnb"},
	{"Booking Holdings Inc.", "US", []string{"449620848"}, "Booking.com (France), filiale du groupe Booking Holdings"},
	{"McDonald's Corporation", "US", []string{"722003936"}, "McDonald's France, filiale du groupe McDonald's"},
	{"IBM", "US", []string{"552118465"}, "Compagnie IBM France, filiale du groupe IBM"},
	{"McKinsey & Company", "US", []string{"539261404", "775758337", "799893227"}, "McKinsey & Company SAS et McKinsey & Company Inc. France (succursale), les deux entités françaises du cabinet identifiées par la commission d'enquête du Sénat (rapport n° 578, 2022) ; McKinsey Recovery & Transformation Services France"},
	{"Palantir Technologies Inc.", "US", []string{"810492124"}, "Palantir Technologies France, filiale du groupe Palantir"},
	{"Accenture plc", "IE", []string{"732075312", "445088057"}, "Accenture (SAS) et Accenture Technology Solutions, sociétés françaises du groupe Accenture, dont la société de tête est irlandaise"},
	{"Oracle Corporation", "US", []string{"335092318"}, "Oracle France, filiale du groupe Oracle"},
	{"Cisco Systems Inc.", "US", []string{"349166561"}, "Cisco Systems France, filiale du groupe Cisco"},
	{"Salesforce Inc.", "US", []string{"483993226"}, "Salesforce.com France, filiale du groupe Salesforce"},
	{"Intel Corporation", "US", []string{"302456199"}, "Intel Corporation SAS, filiale française du groupe Intel"},
	{"Pfizer Inc.", "US", []string{"433623550"}, "Pfizer (SAS française), filiale du groupe Pfizer"},
	{"Merck & Co.", "US", []string{"417890589"}, "MSD France, filiale du groupe Merck & Co. (MSD hors États-Unis)"},
	{"Eli Lilly and Company", "US", []string{"609849153"}, "Lilly France, filiale du groupe Eli Lilly"},
	{"Procter & Gamble", "US", []string{"391543576"}, "Procter & Gamble France, filiale du groupe Procter & Gamble"},
	{"PepsiCo Inc.", "US", []string{"381511039"}, "PepsiCo France, filiale du groupe PepsiCo"},
	{"Mondelez International", "US", []string{"808234801"}, "Mondelez France SAS, filiale du groupe Mondelez"},
	{"Philip Morris International", "US", []string{"712054014"}, "Philip Morris France, filiale du groupe Philip Morris International"},
	{"ByteDance Ltd.", "KY", []string{"882530603"}, "TikTok (société française), filiale du groupe ByteDance, société de tête immatriculée aux îles Caïmans"},
	{"Nestlé S.A.", "CH", []string{"542014428", "382597821"}, "Nestlé France et Nespresso France, filiales du groupe Nestlé"},
	{"Unilever PLC", "GB", []string{"552119216"}, "Unilever France, filiale du groupe Unilever"},
	{"Coca-Cola Europacific Partners PLC", "GB", []string{"343688016"}, "Coca-Cola Europacific Partners France, filiale de l'embouteilleur CCEP (et non de The Coca-Cola Company)"},
	{"Samsung Electronics", "KR", []string{"334367497"}, "Samsung Electronics France, filiale du groupe Samsung Electronics"},
	{"Huawei Investment & Holding", "CN", []string{"451063739"}, "Huawei Technologies France, filiale du groupe Huawei"},
	{"Toyota Motor Corporation", "JP", []string{"420559056"}, "Toyota Motor Manufacturing France (usine d'Onnaing), filiale du groupe Toyota"},
	{"Robert Bosch GmbH", "DE", []string{"572067684"}, "Robert Bosch France, filiale du groupe Bosch"},
	{"Schwarz Gruppe", "DE", []string{"343262622"}, "Lidl (SNC française), filiale du groupe Schwarz"},
	{"Inditex S.A.", "ES", []string{"348991555"}, "Zara France, filiale du groupe Inditex"},
	{"ArcelorMittal S.A.", "LU", []string{"562094425"}, "ArcelorMittal France, filiale du groupe ArcelorMittal"},
	{"Koninklijke Philips N.V.", "NL", []string{"811847243"}, "Philips France, filiale du groupe Philips"},
}

// lireZipCSV ouvre le seul fichier CSV d'une archive et appelle f ligne à
// ligne avec l'index des colonnes.
func lireZipCSV(path string, f func(col map[string]int, rec []string) error) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		return fmt.Errorf("%d fichiers dans l'archive, 1 attendu", len(zr.File))
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return parcourirCSV(rc, f)
}

// lireCSVFlux lit un CSV trop gros pour être chargé en mémoire.
func lireCSVFlux(path string, f func(col map[string]int, rec []string) error) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return parcourirCSV(bufio.NewReaderSize(fh, 1<<20), f)
}

func parcourirCSV(r io.Reader, f func(col map[string]int, rec []string) error) error {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	entete, err := cr.Read()
	if err != nil {
		return err
	}
	col := map[string]int{}
	for i, c := range entete {
		col[strings.TrimPrefix(c, "\uFEFF")] = i
	}
	n := len(entete)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(rec) < n {
			return fmt.Errorf("ligne à %d colonnes pour %d en en-tête", len(rec), n)
		}
		if err := f(col, rec); err != nil {
			return err
		}
	}
}

// ressourceDataGouv lit, dans la fiche d'un jeu de données data.gouv.fr, l'URL
// de la ressource qui porte ce titre.
func ressourceDataGouv(path, titre string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var d struct {
		Resources []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return "", err
	}
	for _, r := range d.Resources {
		if r.Title == titre {
			return r.URL, nil
		}
	}
	return "", fmt.Errorf("ressource %q absente du jeu de données", titre)
}

func exigerColonnes(col map[string]int, noms ...string) error {
	for _, n := range noms {
		if _, ok := col[n]; !ok {
			return fmt.Errorf("colonne %q absente", n)
		}
	}
	return nil
}

// URL des derniers fichiers Golden Copy, lue à chaque exécution.
func goldenCopy(ctx context.Context, arch *archive.Archive, srcID, runID int64) (lei2, rr string, err error) {
	f, err := arch.Fetch(ctx, srcID, runID, "https://goldencopy.gleif.org/api/v2/golden-copies/publishes/latest", ".json")
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return "", "", err
	}
	type fichier struct {
		FullFile struct {
			CSV struct {
				URL string `json:"url"`
			} `json:"csv"`
		} `json:"full_file"`
	}
	var d struct {
		Data struct {
			LEI2 fichier `json:"lei2"`
			RR   fichier `json:"rr"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return "", "", err
	}
	lei2, rr = d.Data.LEI2.FullFile.CSV.URL, d.Data.RR.FullFile.CSV.URL
	if lei2 == "" || rr == "" {
		return "", "", fmt.Errorf("URL Golden Copy absente de la réponse")
	}
	return lei2, rr, nil
}

type entiteLEI struct{ nom, pays string }

// Autorités d'enregistrement françaises dont l'identifiant est le SIREN :
// RA000189 (répertoire SIRENE de l'INSEE) et RA000192 (registre du commerce
// et des sociétés). RA000190 (fonds agréés par l'AMF) est écarté.
var raSIREN = map[string]bool{"RA000189": true, "RA000192": true}

// filialesGLEIF : sociétés françaises actives, identifiées par leur SIREN,
// dont la société mère ULTIME déclarée est immatriculée hors de France.
func filialesGLEIF(ctx context.Context, arch *archive.Archive, srcID, runID int64) ([][]any, map[string]any, error) {
	urlLEI2, urlRR, err := goldenCopy(ctx, arch, srcID, runID)
	if err != nil {
		return nil, nil, err
	}
	frr, err := arch.Fetch(ctx, srcID, runID, urlRR, ".zip")
	if err != nil {
		return nil, nil, err
	}
	mere := map[string]string{} // LEI enfant -> LEI mère ultime
	err = lireZipCSV(frr.Path, func(col map[string]int, rec []string) error {
		if len(mere) == 0 {
			if err := exigerColonnes(col, "Relationship.StartNode.NodeID", "Relationship.EndNode.NodeID",
				"Relationship.EndNode.NodeIDType", "Relationship.RelationshipType", "Relationship.RelationshipStatus"); err != nil {
				return err
			}
		}
		if rec[col["Relationship.RelationshipType"]] != "IS_ULTIMATELY_CONSOLIDATED_BY" ||
			rec[col["Relationship.RelationshipStatus"]] != "ACTIVE" ||
			rec[col["Relationship.EndNode.NodeIDType"]] != "LEI" {
			return nil
		}
		mere[rec[col["Relationship.StartNode.NodeID"]]] = rec[col["Relationship.EndNode.NodeID"]]
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("relations GLEIF : %w", err)
	}
	estMere := map[string]bool{}
	for _, m := range mere {
		estMere[m] = true
	}

	flei, err := arch.Fetch(ctx, srcID, runID, urlLEI2, ".zip")
	if err != nil {
		return nil, nil, err
	}
	meres := map[string]entiteLEI{}
	type enfant struct{ lei, siren string }
	var enfants []enfant
	lus := 0
	err = lireZipCSV(flei.Path, func(col map[string]int, rec []string) error {
		if lus == 0 {
			if err := exigerColonnes(col, "LEI", "Entity.LegalName", "Entity.LegalAddress.Country",
				"Entity.RegistrationAuthority.RegistrationAuthorityID",
				"Entity.RegistrationAuthority.RegistrationAuthorityEntityID", "Entity.EntityStatus"); err != nil {
				return err
			}
		}
		lus++
		lei := rec[col["LEI"]]
		if estMere[lei] {
			meres[lei] = entiteLEI{rec[col["Entity.LegalName"]], rec[col["Entity.LegalAddress.Country"]]}
		}
		if rec[col["Entity.LegalAddress.Country"]] != "FR" || rec[col["Entity.EntityStatus"]] != "ACTIVE" ||
			!raSIREN[rec[col["Entity.RegistrationAuthority.RegistrationAuthorityID"]]] {
			return nil
		}
		if _, ok := mere[lei]; !ok {
			return nil
		}
		chiffres := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			if r == ' ' || r == '.' || r == '-' {
				return -1
			}
			return 'x'
		}, rec[col["Entity.RegistrationAuthority.RegistrationAuthorityEntityID"]])
		if strings.Contains(chiffres, "x") || (len(chiffres) != 9 && len(chiffres) != 14) {
			return nil
		}
		enfants = append(enfants, enfant{lei, chiffres[:9]})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("répertoire LEI : %w", err)
	}
	var lignes [][]any
	vus := map[string]bool{}
	for _, e := range enfants {
		m, ok := meres[mere[e.lei]]
		if !ok || m.pays == "FR" || m.pays == "" || vus[e.siren] {
			continue
		}
		vus[e.siren] = true
		lignes = append(lignes, []any{e.siren, "GLEIF", m.nom, m.pays, e.lei, mere[e.lei], nil, srcID, flei.DocumentID})
	}
	return lignes, map[string]any{"lei_lus": lus, "relations_ultimes": len(mere), "filiales_fr_mere_etrangere": len(lignes)}, nil
}

func IngestFiliales(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	var sirenes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ref.unite_legale`).Scan(&sirenes); err != nil || sirenes < 1_000_000 {
		return fmt.Errorf("ref.unite_legale vide ou absent (charger -only=sirene) : %v", err)
	}
	var lignes [][]any
	stats := map[string]any{}
	err := executer(ctx, arch, SourceGLEIF, func(srcID, runID int64) (map[string]any, error) {
		g, st, err := filialesGLEIF(ctx, arch, srcID, runID)
		if err != nil {
			return nil, err
		}
		lignes = append(lignes, g...)
		for _, s := range selectionFiliales {
			for _, siren := range s.sirens {
				lignes = append(lignes, []any{siren, "SELECTION", s.groupe, s.pays, nil, nil, s.fondement, srcID, nil})
			}
		}
		st["selection"] = len(lignes) - len(g)
		return st, nil
	})
	if err != nil {
		return err
	}

	// Écriture : seules les sociétés présentes dans SIRENE sont retenues (un
	// identifiant GLEIF peut être un SIRET mal saisi ou une société radiée).
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE filiale_tmp (LIKE core.filiale_groupe_etranger) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"filiale_tmp"},
		[]string{"siren", "origine", "groupe", "pays_groupe", "lei", "lei_groupe", "justification", "source_id", "document_id"},
		pgx.CopyFromRows(lignes)); err != nil {
		return err
	}
	var absentsSelection []string
	rows, err := tx.Query(ctx, `SELECT t.siren FROM filiale_tmp t LEFT JOIN ref.unite_legale u USING (siren)
		WHERE t.origine = 'SELECTION' AND u.siren IS NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s string
		rows.Scan(&s)
		absentsSelection = append(absentsSelection, s)
	}
	rows.Close()
	if len(absentsSelection) > 0 {
		return fmt.Errorf("sélection : SIREN absents de SIRENE %v", absentsSelection)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.filiale_groupe_etranger`); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO core.filiale_groupe_etranger
		SELECT t.* FROM filiale_tmp t JOIN ref.unite_legale u USING (siren)`)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	stats["filiales_retenues"] = tag.RowsAffected()
	fmt.Printf("  %-34s %v\n", "filiales (écriture)", stats)

	return IngestComptes(ctx, pool, arch)
}

// IngestComptes charge les ratios financiers INPI/BCE des filiales retenues.
func IngestComptes(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	var sirens []string
	rows, err := pool.Query(ctx, `SELECT DISTINCT siren FROM core.filiale_groupe_etranger ORDER BY siren`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		sirens = append(sirens, s)
	}
	rows.Close()
	sort.Strings(sirens)
	return executer(ctx, arch, SourceRatiosINPI, func(srcID, runID int64) (map[string]any, error) {
		const lot = 150
		type cle struct{ siren, date, bilan string }
		vus := map[cle]bool{}
		var lignes [][]any
		for i := 0; i < len(sirens); i += lot {
			fin := min(i+lot, len(sirens))
			liste := `"` + strings.Join(sirens[i:fin], `","`) + `"`
			q := url.Values{}
			q.Set("select", "siren,date_cloture_exercice,type_bilan,chiffre_d_affaires,ebe,resultat_net,resultat_courant_avant_impots_sur_ca,confidentiality")
			q.Set("where", "siren in ("+liste+")")
			u := "https://data.economie.gouv.fr/api/explore/v2.1/catalog/datasets/ratios_inpi_bce/exports/json?" + q.Encode()
			f, err := arch.Fetch(ctx, srcID, runID, u, ".json")
			if err != nil {
				return nil, err
			}
			b, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, err
			}
			var recs []struct {
				Siren      string   `json:"siren"`
				Date       string   `json:"date_cloture_exercice"`
				TypeBilan  string   `json:"type_bilan"`
				CA         *float64 `json:"chiffre_d_affaires"`
				EBE        *float64 `json:"ebe"`
				RN         *float64 `json:"resultat_net"`
				RCAISurCA  *float64 `json:"resultat_courant_avant_impots_sur_ca"`
				Confidence string   `json:"confidentiality"`
			}
			if err := json.Unmarshal(b, &recs); err != nil {
				return nil, fmt.Errorf("lot %d : %w", i/lot, err)
			}
			for _, r := range recs {
				d, err := time.Parse("2006-01-02", r.Date)
				if err != nil {
					return nil, fmt.Errorf("%s : date %q", r.Siren, r.Date)
				}
				k := cle{r.Siren, r.Date, r.TypeBilan}
				if vus[k] {
					continue // dépôt en double dans le jeu : même clé, on garde le premier
				}
				vus[k] = true
				// Le résultat courant avant impôt est publié en % du CA : on le
				// reconstitue, NULL si l'un des deux manque.
				var rcai any
				if r.CA != nil && r.RCAISurCA != nil {
					rcai = *r.CA * *r.RCAISurCA / 100
				}
				lignes = append(lignes, []any{r.Siren, d, r.TypeBilan, r.CA, r.EBE, rcai, r.RN, nul(r.Confidence), f.DocumentID})
			}
			time.Sleep(500 * time.Millisecond)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.entreprise_comptes`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "entreprise_comptes"},
			[]string{"siren", "date_cloture", "type_bilan", "chiffre_affaires", "ebe", "resultat_courant_ai", "resultat_net", "confidentialite", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		return map[string]any{"sirens": len(sirens), "comptes": len(lignes)}, tx.Commit(ctx)
	})
}
