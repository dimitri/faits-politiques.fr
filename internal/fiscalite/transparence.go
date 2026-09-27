package fiscalite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ce que les multinationales paient en France : les paramètres pour calculer un
// impôt « théorique », les déclarations pays par pays que les groupes publient
// eux-mêmes, les impôts de groupe déposés à la SEC, et le statut fiscal établi
// de chaque groupe. Voir D-064.

var SourceParametresIS = archive.Source{
	Slug: "parametres-impot-societes", Label: "Impôt sur les sociétés : taux normal, contribution sociale, contribution exceptionnelle 2025",
	Publisher: "Direction générale des finances publiques (BOFiP)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Contenu public de l'administration fiscale, cité avec lien", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : BOFiP-Impôts ; code général des impôts, art. 219 et 235 ter ZC ; loi de finances pour 2025, art. 48",
	Cadence:     "à chaque loi de finances",
	Notes: "Taux du droit commun, sans régimes particuliers (PME, plus-values à long terme, régime " +
		"mère-fille). La contribution exceptionnelle s'applique au premier exercice clos à compter du " +
		"31 décembre 2025, pour un chiffre d'affaires rattaché aux bénéfices imposables en France d'au " +
		"moins 1 Md€ ; le seuil s'apprécie au niveau du groupe fiscalement intégré.",
}

var SourceCbCRPublics = archive.Source{
	Slug: "cbcr-publics-groupes", Label: "Déclarations pays par pays publiées par les groupes (directive (UE) 2021/2101)",
	Publisher: "Les groupes concernés", Tier: "DECLARATIVE",
	Licence: "Publication légale obligatoire, reprise avec attribution", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : rapports publics sur les informations relatives à l'impôt sur les revenus des sociétés, publiés par chaque groupe",
	Cadence:     "annuelle, dans les 12 mois suivant la clôture",
	Notes: "Premiers rapports pour les exercices ouverts à compter du 22 juin 2024 : publiés à partir de " +
		"mi-2026 (fin 2026 pour les exercices calendaires). Toutes les entités d'une juridiction " +
		"agrégées ; chiffre d'affaires intragroupe compris ; l'impôt payé peut être négatif " +
		"(remboursement). Transcrits du document scellé, contrôlés contre le bénéfice du groupe déposé " +
		"à la SEC.",
}

var SourceSEC = archive.Source{
	Slug: "sec-xbrl-companyfacts", Label: "SEC EDGAR — données XBRL des rapports annuels (10-K) des groupes cotés aux États-Unis",
	Publisher: "U.S. Securities and Exchange Commission", Tier: "PRIMARY_OFFICIAL",
	Licence: "Domaine public (données du gouvernement fédéral américain)", ReuseClass: "OPEN",
	Attribution: "Source : SEC EDGAR, API XBRL « companyfacts »",
	Cadence:     "à chaque dépôt",
	Notes: "Les rapports annuels américains ne ventilent pas l'impôt par pays : ils séparent le bénéfice " +
		"avant impôt « domestique » (États-Unis) et « étranger », et l'impôt courant étranger. Accès " +
		"limité à dix requêtes par seconde.",
}

type rapportCbCR struct {
	groupe, url, debut, fin, devise string
	lignes                          [][7]string // juridiction, CA, bénéfice, impôt payé, impôt dû, bénéfices non distribués, salariés
}

// Transcrits de la section 2 de chaque rapport. Contrôle : la somme des
// bénéfices des juridictions retrouve, à 0,5 % près, le bénéfice avant impôt du
// groupe déposé à la SEC (cmd/verify).
var rapportsCbCR = []rapportCbCR{
	{"Microsoft Corporation",
		"https://cdn-dynmedia-1.microsoft.com/is/content/microsoftcorp/microsoft/msc/documents/presentations/CSR/FY25-Microsoft-EU-Directive-2021-2101-Report.pdf",
		"2024-07-01", "2025-06-30", "USD",
		[][7]string{
			{"AT", "1346011350", "78316085", "13638404", "16774722", "143861390", "564"},
			{"BE", "1885956075", "98146946", "29055149", "26035889", "194345020", "520"},
			{"BG", "9723687", "1266239", "96882", "125856", "6329371", "36"},
			{"HR", "17543699", "5563107", "779491", "1078803", "18968034", "61"},
			{"CY", "6921966", "3200115", "358033", "407522", "0", "18"},
			{"CZ", "258027878", "28508068", "8851532", "7400327", "70267730", "1451"},
			{"DK", "2319067269", "145143293", "8386999", "14918203", "-589398631", "851"},
			{"EE", "78761584", "17373499", "0", "0", "65842547", "438"},
			{"FI", "1228933971", "103365893", "13616990", "24725697", "-8170105345", "405"},
			{"FR", "6674978672", "486559132", "-96410747", "131253094", "623115452", "2568"},
			{"DE", "11686268643", "661226924", "174229664", "220983083", "739288209", "3471"},
			{"GR", "87048386", "20871164", "3771948", "4811118", "31081149", "267"},
			{"HU", "43658590", "17142352", "1670586", "1038634", "47523628", "189"},
			{"IE", "196030411149", "47083027808", "5584756934", "6645835074", "3899095180", "6654"},
			{"IT", "2890388098", "167641813", "57411875", "58407383", "182221453", "1200"},
			{"LV", "5402580", "740693", "35863", "93695", "2895881", "25"},
			{"LT", "2804909", "311246", "56759", "28705", "999673", "15"},
			{"LU", "198900997", "283358513", "7140467", "9248608", "728198822", "34"},
			{"MT", "2712822986", "-765780100", "9169539", "9231778", "-703694581", "10"},
			{"NL", "6187045829", "311765491", "105488074", "104972926", "-8405453926", "1929"},
			{"PA", "4753714", "76974", "234935", "153226", "1733599", "13"},
			{"PL", "502261087", "84080179", "15807439", "14927284", "166985943", "541"},
			{"PT", "226476049", "21604580", "4120609", "6099621", "12352118", "1490"},
			{"RO", "181960674", "21855533", "4574292", "3390308", "77351640", "1769"},
			{"RU", "4763893", "2381450", "392558", "825799", "13856755", "2"},
			{"SK", "21581519", "9456185", "1165016", "1941779", "-12950279", "45"},
			{"SI", "16332056", "3648380", "718737", "850501", "11756551", "49"},
			{"ES", "2580540106", "150636540", "36157874", "38749910", "43896949", "2294"},
			{"SE", "5208896703", "-81371959", "288842216", "143702224", "-983848732", "2049"},
			{"TT", "2114773", "233300", "34191", "77346", "1979646", "10"},
			{"TR", "114653446", "31405379", "5373586", "9831598", "59896961", "272"},
			{"VN", "27402781", "3213061", "713830", "666817", "10540050", "112"},
			{"AUTRES", "279169811347", "74617512749", "22364575254", "26244319361", "247949211426", "198843"},
		}},
}

// Les groupes cotés aux États-Unis dont les filiales françaises sont suivies.
var groupesSEC = []struct{ groupe, cik string }{
	{"Microsoft Corporation", "789019"}, {"Alphabet Inc.", "1652044"}, {"Amazon.com Inc.", "1018724"},
	{"Apple Inc.", "320193"}, {"Meta Platforms Inc.", "1326801"}, {"Oracle Corporation", "1341439"},
	{"IBM", "51143"}, {"Palantir Technologies Inc.", "1321655"}, {"Accenture plc", "1467373"},
	{"Netflix Inc.", "1065280"}, {"McDonald's Corporation", "63908"}, {"The Walt Disney Company", "1744489"},
	{"Salesforce Inc.", "1108524"}, {"Cisco Systems Inc.", "858877"}, {"Uber Technologies Inc.", "1543151"},
	{"Airbnb Inc.", "1559720"}, {"Booking Holdings Inc.", "1075531"}, {"Intel Corporation", "50863"},
	{"Pfizer Inc.", "78003"}, {"Merck & Co.", "310158"}, {"Eli Lilly and Company", "59478"},
	{"Procter & Gamble", "80424"}, {"PepsiCo Inc.", "77476"}, {"Mondelez International", "1103982"},
	{"Philip Morris International", "1413329"},
}

// Concept normalisé → éléments us-gaap acceptés, par ordre de préférence.
var conceptsSEC = []struct {
	code     string
	elements []string
}{
	{"IMPOT", []string{"IncomeTaxExpenseBenefit"}},
	{"BENEFICE_AVANT_IMPOT", []string{"IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest",
		"IncomeLossFromContinuingOperationsBeforeIncomeTaxesMinorityInterestAndIncomeLossFromEquityMethodInvestments"}},
	{"BENEFICE_AVANT_IMPOT_DOMESTIQUE", []string{"IncomeLossFromContinuingOperationsBeforeIncomeTaxesDomestic"}},
	{"BENEFICE_AVANT_IMPOT_ETRANGER", []string{"IncomeLossFromContinuingOperationsBeforeIncomeTaxesForeign"}},
	{"IMPOT_COURANT_ETRANGER", []string{"CurrentForeignTaxExpenseBenefit"}},
	{"CHIFFRE_AFFAIRES", []string{"Revenues", "RevenueFromContractWithCustomerExcludingAssessedTax"}},
}

// Le statut établi de chaque groupe. Sans fait officiel, AUCUN_CONSTAT_PUBLIC :
// avoir des marchés publics ou une filiale peu rentable n'est pas un constat.
var statutsFiscaux = []struct {
	groupe, statut, resume string
	faits                  []string
}{
	{"Alphabet Inc.", "FRAUDE_TRANSIGEE",
		"Convention judiciaire d'intérêt public de 2019 : 500 M€ d'amende et 465 M€ d'impôts ; la justice considérait que Google Ireland exerçait en France une activité imposable.",
		[]string{"google-cjip-2019-amende", "google-cjip-2019-impot"}},
	{"McDonald's Corporation", "FRAUDE_TRANSIGEE",
		"Convention judiciaire d'intérêt public de 2022 : 508 M€ d'amende et 737 M€ d'impôt, pour une redevance à la société mère luxembourgeoise jugée artificiellement gonflée.",
		[]string{"mcdonalds-2022-amende", "mcdonalds-2022-impot"}},
	{"McKinsey & Company", "IMPOT_NUL_CONSTATE",
		"Impôt sur les sociétés nul de 2011 à 2020 pour 329 M€ de chiffre d'affaires en 2020, par des prix de transfert versés à la maison mère du Delaware (commission d'enquête du Sénat).",
		[]string{"mckinsey-is-2022"}},
	{"Microsoft Corporation", "FACTURATION_ETRANGER",
		"Le contrat du ministère de la Défense a été conclu avec la société irlandaise du groupe (Sénat, 2017). Le groupe publie pour la France un impôt dû égal à 27 % du bénéfice qu'il y déclare ; l'Irlande concentre 196 Md$ de chiffre d'affaires et 47 Md$ de bénéfice avec 6 654 salariés.",
		[]string{"microsoft-defense-ppr-2017"}},
	{"Capgemini SE (groupe français)", "GROUPE_FRANCAIS",
		"Société de tête à Paris, imposée en France ; suivie pour ses marchés publics et son rôle dans les offres « cloud de confiance ».", nil},
}

func IngestTransparence(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := executer(ctx, arch, SourceParametresIS, func(srcID, runID int64) (map[string]any, error) {
		const bofip = "https://bofip.impots.gouv.fr/bofip/14609-PGP.html/ACTU-2025-00035"
		d, err := arch.Fetch(ctx, srcID, runID, bofip, ".html")
		if err != nil {
			return nil, err
		}
		t, err := texteHTML(d.Path)
		if err != nil {
			return nil, err
		}
		for _, a := range []string{"20,6 %", "41,2 %", "1 milliard", "31 décembre 2025"} {
			if !strings.Contains(t, a) {
				return nil, fmt.Errorf("BOFiP : « %s » absent", a)
			}
		}
		params := [][]any{
			{"IS_TAUX_NORMAL", "Taux normal de l'impôt sur les sociétés", "25", "PCT", "Code général des impôts, art. 219, I (depuis les exercices ouverts en 2022)", nil},
			{"CSB_TAUX", "Contribution sociale sur l'impôt sur les sociétés", "3.3", "PCT", "Code général des impôts, art. 235 ter ZC", nil},
			{"CSB_ABATTEMENT", "Abattement de la contribution sociale, en impôt sur les sociétés", "763000", "EUR", "Code général des impôts, art. 235 ter ZC", nil},
			{"CEBGE_TAUX_1_3_MD", "Contribution exceptionnelle, chiffre d'affaires de 1 à 3 Md€, en % de l'IS", "20.6", "PCT", "Loi n° 2025-127 du 14 février 2025, art. 48 ; BOFiP", d.DocumentID},
			{"CEBGE_TAUX_3_MD", "Contribution exceptionnelle, chiffre d'affaires d'au moins 3 Md€, en % de l'IS", "41.2", "PCT", "Loi n° 2025-127 du 14 février 2025, art. 48 ; BOFiP", d.DocumentID},
			{"CEBGE_SEUIL_BAS", "Contribution exceptionnelle : seuil de chiffre d'affaires", "1000000000", "EUR", "Loi n° 2025-127 du 14 février 2025, art. 48 ; BOFiP", d.DocumentID},
			{"CEBGE_SEUIL_HAUT", "Contribution exceptionnelle : seuil du taux majoré", "3000000000", "EUR", "Loi n° 2025-127 du 14 février 2025, art. 48 ; BOFiP", d.DocumentID},
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM ref.parametre_is`); err != nil {
			return nil, err
		}
		for _, p := range params {
			if _, err := tx.Exec(ctx, `INSERT INTO ref.parametre_is (code, libelle, valeur, unite, fondement, source_id, document_id)
				VALUES ($1,$2,$3::numeric,$4,$5,$6,$7)`, p[0], p[1], p[2], p[3], p[4], srcID, p[5]); err != nil {
				return nil, err
			}
		}
		return map[string]any{"parametres": len(params)}, tx.Commit(ctx)
	}); err != nil {
		return err
	}

	if err := executer(ctx, arch, SourceCbCRPublics, func(srcID, runID int64) (map[string]any, error) {
		var lignes [][]any
		for _, r := range rapportsCbCR {
			d, err := arch.Fetch(ctx, srcID, runID, r.url, ".pdf")
			if err != nil {
				return nil, err
			}
			debut, _ := time.Parse("2006-01-02", r.debut)
			fin, _ := time.Parse("2006-01-02", r.fin)
			vus := map[string]bool{}
			for _, l := range r.lignes {
				if vus[l[0]] {
					return nil, fmt.Errorf("%s : juridiction %s en double", r.groupe, l[0])
				}
				vus[l[0]] = true
				lignes = append(lignes, []any{r.groupe, debut, fin, r.devise, l[0], l[1], l[2], l[3], l[4], l[5], l[6], srcID, d.DocumentID})
			}
			if !vus["FR"] {
				return nil, fmt.Errorf("%s : pas de ligne France", r.groupe)
			}
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_cbcr_public (
				groupe text, exercice_debut date, exercice_fin date, devise text, juridiction text,
				chiffre_affaires numeric, benefice_avant_impot numeric, impot_paye numeric,
				impot_du numeric, benefices_non_distribues numeric, salaries numeric,
				source_id bigint, document_id bigint
			) ON COMMIT DROP`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_cbcr_public"},
			[]string{"groupe", "exercice_debut", "exercice_fin", "devise", "juridiction", "chiffre_affaires", "benefice_avant_impot",
				"impot_paye", "impot_du", "benefices_non_distribues", "salaries", "source_id", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}

		// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire
		// de la table ; l'ancien DELETE payait le prix des triggers RI pour
		// l'intégralité de la table à chaque republication, changement ou non.
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.cbcr_public AS tgt
			USING tmp_cbcr_public AS src
			ON tgt.groupe = src.groupe AND tgt.exercice_fin = src.exercice_fin AND tgt.juridiction = src.juridiction
			WHEN MATCHED AND (tgt.exercice_debut, tgt.devise, tgt.chiffre_affaires, tgt.benefice_avant_impot,
			                   tgt.impot_paye, tgt.impot_du, tgt.benefices_non_distribues, tgt.salaries,
			                   tgt.source_id, tgt.document_id)
			                  IS DISTINCT FROM
			                  (src.exercice_debut, src.devise, src.chiffre_affaires, src.benefice_avant_impot,
			                   src.impot_paye, src.impot_du, src.benefices_non_distribues, src.salaries,
			                   src.source_id, src.document_id) THEN
			    UPDATE SET exercice_debut = src.exercice_debut, devise = src.devise,
			               chiffre_affaires = src.chiffre_affaires, benefice_avant_impot = src.benefice_avant_impot,
			               impot_paye = src.impot_paye, impot_du = src.impot_du,
			               benefices_non_distribues = src.benefices_non_distribues, salaries = src.salaries,
			               source_id = src.source_id, document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (groupe, exercice_debut, exercice_fin, devise, juridiction, chiffre_affaires,
			            benefice_avant_impot, impot_paye, impot_du, benefices_non_distribues, salaries,
			            source_id, document_id)
			    VALUES (src.groupe, src.exercice_debut, src.exercice_fin, src.devise, src.juridiction,
			            src.chiffre_affaires, src.benefice_avant_impot, src.impot_paye, src.impot_du,
			            src.benefices_non_distribues, src.salaries, src.source_id, src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return nil, fmt.Errorf("fusion cbcr_public : %w", err)
		}
		touchees := ct.RowsAffected()
		return map[string]any{"rapports": len(rapportsCbCR), "lignes": len(lignes), "touchees": touchees}, tx.Commit(ctx)
	}); err != nil {
		return err
	}

	if err := executer(ctx, arch, SourceSEC, func(srcID, runID int64) (map[string]any, error) {
		var lignes [][]any
		for _, g := range groupesSEC {
			url := "https://data.sec.gov/api/xbrl/companyfacts/CIK" + strings.Repeat("0", 10-len(g.cik)) + g.cik + ".json"
			d, err := arch.Fetch(ctx, srcID, runID, url, ".json")
			if err != nil {
				return nil, fmt.Errorf("%s : %w", g.groupe, err)
			}
			l, err := lireCompanyFacts(d.Path, g.groupe, g.cik, d.DocumentID)
			if err != nil {
				return nil, fmt.Errorf("%s : %w", g.groupe, err)
			}
			lignes = append(lignes, l...)
			time.Sleep(200 * time.Millisecond) // politique d'accès de la SEC : 10 requêtes par seconde au plus
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_groupe_resultat_sec (
				groupe text, cik text, exercice_fin date, concept text, element_xbrl text,
				valeur numeric, unite text, formulaire text, depot text, document_id bigint
			) ON COMMIT DROP`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_groupe_resultat_sec"},
			[]string{"groupe", "cik", "exercice_fin", "concept", "element_xbrl", "valeur", "unite", "formulaire", "depot", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}

		// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire
		// de la table ; l'ancien DELETE payait le prix des triggers RI pour
		// l'intégralité de la table à chaque republication, changement ou non.
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.groupe_resultat_sec AS tgt
			USING tmp_groupe_resultat_sec AS src
			ON tgt.groupe = src.groupe AND tgt.exercice_fin = src.exercice_fin AND tgt.concept = src.concept
			WHEN MATCHED AND (tgt.cik, tgt.element_xbrl, tgt.valeur, tgt.unite, tgt.formulaire, tgt.depot,
			                   tgt.document_id)
			                  IS DISTINCT FROM
			                  (src.cik, src.element_xbrl, src.valeur, src.unite, src.formulaire, src.depot,
			                   src.document_id) THEN
			    UPDATE SET cik = src.cik, element_xbrl = src.element_xbrl, valeur = src.valeur,
			               unite = src.unite, formulaire = src.formulaire, depot = src.depot,
			               document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (groupe, cik, exercice_fin, concept, element_xbrl, valeur, unite, formulaire, depot, document_id)
			    VALUES (src.groupe, src.cik, src.exercice_fin, src.concept, src.element_xbrl, src.valeur,
			            src.unite, src.formulaire, src.depot, src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return nil, fmt.Errorf("fusion groupe_resultat_sec : %w", err)
		}
		touchees := ct.RowsAffected()
		return map[string]any{"groupes": len(groupesSEC), "valeurs": len(lignes), "touchees": touchees}, tx.Commit(ctx)
	}); err != nil {
		return err
	}

	// Statuts : les groupes suivis sans fait officiel reçoivent AUCUN_CONSTAT_PUBLIC.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM ref.groupe_statut_fiscal`); err != nil {
		return err
	}
	for _, s := range statutsFiscaux {
		faits := s.faits
		if faits == nil {
			faits = []string{}
		}
		for _, f := range faits {
			var qualite string
			if err := tx.QueryRow(ctx, `SELECT qualite FROM ref.fait_multinationale WHERE id = $1`, f).Scan(&qualite); err != nil {
				return fmt.Errorf("statut %s : fait %s introuvable (charger -only=fiscalite-faits) : %w", s.groupe, f, err)
			}
			if qualite != "OFFICIEL" {
				return fmt.Errorf("statut %s : le fait %s n'est pas officiel", s.groupe, f)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ref.groupe_statut_fiscal (groupe, statut, resume, faits) VALUES ($1,$2,$3,$4)`,
			s.groupe, s.statut, s.resume, faits); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO ref.groupe_statut_fiscal (groupe, statut, resume)
		SELECT DISTINCT groupe, 'AUCUN_CONSTAT_PUBLIC',
		       'Aucune source officielle ne documente de fraude, de facturation depuis l''étranger ni d''impôt nul pour ce groupe en France.'
		FROM core.filiale_groupe_etranger
		WHERE origine = 'SELECTION' ON CONFLICT (groupe) DO NOTHING`)
	if err != nil {
		return err
	}
	fmt.Printf("  %-34s %v\n", "statuts fiscaux", map[string]any{"etablis": len(statutsFiscaux), "sans_constat": tag.RowsAffected()})
	return tx.Commit(ctx)
}

// lireCompanyFacts retient, pour chaque concept, les valeurs annuelles des
// rapports 10-K (ou 20-F) : durée d'environ un an, dernier dépôt pour chaque
// date de clôture, exercices clos depuis 2020.
func lireCompanyFacts(path, groupe, cik string, doc int64) ([][]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf struct {
		Facts map[string]map[string]struct {
			Units map[string][]struct {
				Start, End, Accn, Form, Filed string
				Val                           float64
			} `json:"units"`
		} `json:"facts"`
	}
	if err := json.Unmarshal(b, &cf); err != nil {
		return nil, err
	}
	gaap := cf.Facts["us-gaap"]
	var out [][]any
	for _, c := range conceptsSEC {
		type val struct {
			element, accn, form, filed string
			v                          float64
		}
		parFin := map[string]val{}
		for _, el := range c.elements {
			f, ok := gaap[el]
			if !ok {
				continue
			}
			for _, x := range f.Units["USD"] {
				if x.Form != "10-K" && x.Form != "10-K/A" && x.Form != "20-F" {
					continue
				}
				s, e1 := time.Parse("2006-01-02", x.Start)
				e, e2 := time.Parse("2006-01-02", x.End)
				if e1 != nil || e2 != nil || e.Year() < 2020 {
					continue
				}
				if jours := e.Sub(s).Hours() / 24; jours < 350 || jours > 380 {
					continue
				}
				if cur, ok := parFin[x.End]; ok && (cur.element != el || cur.filed >= x.Filed) {
					continue // un élément préféré déjà retenu, ou un dépôt plus récent
				}
				parFin[x.End] = val{el, x.Accn, x.Form, x.Filed, x.Val}
			}
		}
		fins := make([]string, 0, len(parFin))
		for f := range parFin {
			fins = append(fins, f)
		}
		sort.Strings(fins)
		for _, f := range fins {
			v := parFin[f]
			fin, _ := time.Parse("2006-01-02", f)
			out = append(out, []any{groupe, cik, fin, c.code, v.element, v.v, "USD", v.form, v.accn, doc})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("aucune valeur annuelle lue")
	}
	return out, nil
}
