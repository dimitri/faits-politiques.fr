package eau

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

const ConnectorVersionAides = "eau-aides-v1"

// SourceAidesLoireBretagne : décisions d'aide, un fichier par programme
// d'intervention. Seuls les 11e (2019-2024) et 12e (2025-2030, en cours)
// sont chargés — le 10e programme (2013-2018) a un format de colonnes
// différent à chaque millésime, vérifié à l'inspection, pas encore repris.
// Les URL sont datées (« liste arrêtée au ... ») : l'agence republie un
// nouveau fichier à chaque mise à jour plutôt que de faire vivre une URL
// stable — à réviser périodiquement depuis la page qui les liste.
var SourceAidesLoireBretagne = archive.Source{
	Slug: "aides-agence-eau-loire-bretagne", Label: "Agence de l'eau Loire-Bretagne — décisions d'aide (11e et 12e programmes)",
	Publisher: "Agence de l'eau Loire-Bretagne", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Agence de l'eau Loire-Bretagne",
	Cadence:     "mise à jour irrégulière (liste cumulative arrêtée à une date)",
	Notes: "10e programme (2013-2018) non chargé : chaque année a son propre jeu de colonnes " +
		"(2013-2015 partagent un format, 2016 et 2018 en ont chacun un autre, vérifié à " +
		"l'inspection avant de renoncer plutôt que de deviner un format commun).",
}

// SourceAidesArtoisPicardie : conventions de subvention publiées au format
// standard fixé par le décret n° 2017-779 du 5 mai 2017 (données
// essentielles des conventions de subvention supérieures à 23 000 €).
var SourceAidesArtoisPicardie = archive.Source{
	Slug: "aides-agence-eau-artois-picardie", Label: "Agence de l'eau Artois-Picardie — conventions de subvention",
	Publisher: "Agence de l'eau Artois-Picardie", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Agence de l'eau Artois-Picardie",
	Cadence:     "continue (publication réglementaire, décret n° 2017-779)",
}

const (
	urlAidesLB11P = "https://aides-redevances.eau-loire-bretagne.fr/files/live/sites/aides-redevances/files/Aides-11/Decisions-daides/TAB_decisions_aides_AELB_11P_20241212.xlsx"
	urlAidesLB12P = "https://aides-redevances.eau-loire-bretagne.fr/files/live/sites/aides-redevances/files/Aides-12prog/Decisions-daides/TAB_decisions_aides_AELB_12P_cumul_20260626.xlsx"

	urlAidesAP1011 = "https://static.data.gouv.fr/resources/conventions-de-subvention-signees/20260513-062710/aidesattribueesaeap-10-11-prg.csv"
	urlAidesAP12   = "https://static.data.gouv.fr/resources/conventions-de-subvention-signees/20260513-063121/aidesattribueesaeap-12-prg.csv"
)

type ligneAideEau struct {
	Programme         string
	Annee             int
	DateDecision      *string
	ReferenceDecision *string
	NomBeneficiaire   string
	SiretBeneficiaire *string
	CodeDepartement   *string
	CodeInseeCommune  *string
	Objet             *string
	MontantEUR        float64
	Nature            *string
}

func normaliserEntete(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func parseMontantAide(s string) (float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, fmt.Errorf("montant vide")
	}
	return strconv.ParseFloat(s, 64)
}

// chargerFeuilleLB lit un fichier de décisions d'aide Loire-Bretagne dont la
// ligne 0 est un titre fusionné et la ligne 1 porte les en-têtes réels — un
// format vérifié identique entre le 11e et le 12e programme, malgré des
// colonnes différentes d'un programme à l'autre.
func chargerFeuilleLB(chemin, programme string, colAnnee, colDept, colSiret, colBeneficiaire, colObjet, colMontant, colNature, colDecision, colDate string) ([]ligneAideEau, error) {
	wb, err := excelize.OpenFile(chemin)
	if err != nil {
		return nil, fmt.Errorf("classeur illisible : %w", err)
	}
	defer wb.Close()
	sheets := wb.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("classeur sans feuille")
	}
	rows, err := wb.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) < 3 {
		return nil, fmt.Errorf("moins de 3 lignes (titre + en-tête + données attendues)")
	}
	header := rows[1]
	idx := map[string]int{}
	for i, h := range header {
		idx[normaliserEntete(h)] = i
	}
	requis := []string{colAnnee, colBeneficiaire, colObjet, colMontant, colNature, colDecision, colDate}
	for _, c := range requis {
		if _, ok := idx[c]; !ok {
			return nil, fmt.Errorf("colonne %q absente — le format a peut-être changé", c)
		}
	}
	get := func(r []string, col string) string {
		i, ok := idx[col]
		if !ok || i >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[i])
	}
	nettoyerPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}

	var lignes []ligneAideEau
	for n, r := range rows[2:] {
		anneeStr := get(r, colAnnee)
		annee, err := strconv.Atoi(anneeStr)
		if err != nil {
			return nil, fmt.Errorf("ligne %d, année illisible %q : %w", n+3, anneeStr, err)
		}
		montant, err := parseMontantAide(get(r, colMontant))
		if err != nil {
			return nil, fmt.Errorf("ligne %d, montant illisible : %w", n+3, err)
		}
		beneficiaire := get(r, colBeneficiaire)
		if beneficiaire == "" {
			return nil, fmt.Errorf("ligne %d : bénéficiaire vide", n+3)
		}
		var dept, siret *string
		if colDept != "" {
			dept = nettoyerPtr(get(r, colDept))
		}
		if colSiret != "" {
			siret = nettoyerPtr(get(r, colSiret))
		}
		lignes = append(lignes, ligneAideEau{
			Programme:         programme,
			Annee:             annee,
			DateDecision:      nettoyerPtr(get(r, colDate)),
			ReferenceDecision: nettoyerPtr(get(r, colDecision)),
			NomBeneficiaire:   beneficiaire,
			SiretBeneficiaire: siret,
			CodeDepartement:   dept,
			Objet:             nettoyerPtr(get(r, colObjet)),
			MontantEUR:        montant,
			Nature:            nettoyerPtr(get(r, colNature)),
		})
	}
	return lignes, nil
}

// chargerCSVArtoisPicardie lit un fichier « données essentielles des
// conventions de subvention » (décret n° 2017-779) : colonnes fixées par
// arrêté, séparateur point-virgule, jamais de code département ou commune
// (seul le SIRET du bénéficiaire situe l'entité).
func chargerCSVArtoisPicardie(chemin, programme string) ([]ligneAideEau, error) {
	f, err := os.Open(chemin)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.Comma = ';'
	cr.LazyQuotes = true
	entete, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("en-tête illisible : %w", err)
	}
	idx := map[string]int{}
	for i, c := range entete {
		idx[strings.TrimPrefix(strings.TrimSpace(c), string(rune(0xFEFF)))] = i
	}
	requis := []string{"dateConvention", "referenceDecision", "nomBeneficiaire", "idBeneficiaire", "objet", "montant", "nature"}
	for _, c := range requis {
		if _, ok := idx[c]; !ok {
			return nil, fmt.Errorf("colonne %q absente — le format a peut-être changé", c)
		}
	}
	get := func(r []string, col string) string {
		i := idx[col]
		if i >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[i])
	}
	nettoyerPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}

	var lignes []ligneAideEau
	n := 1
	for {
		r, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ligne %d illisible : %w", n+1, err)
		}
		n++
		date := get(r, "dateConvention")
		if len(date) < 4 {
			return nil, fmt.Errorf("ligne %d : date de convention illisible %q", n, date)
		}
		annee, err := strconv.Atoi(date[:4])
		if err != nil {
			return nil, fmt.Errorf("ligne %d, année illisible depuis %q : %w", n, date, err)
		}
		montant, err := parseMontantAide(get(r, "montant"))
		if err != nil {
			return nil, fmt.Errorf("ligne %d, montant illisible : %w", n, err)
		}
		beneficiaire := get(r, "nomBeneficiaire")
		if beneficiaire == "" {
			return nil, fmt.Errorf("ligne %d : bénéficiaire vide", n)
		}
		lignes = append(lignes, ligneAideEau{
			Programme:         programme,
			Annee:             annee,
			DateDecision:      nettoyerPtr(date),
			ReferenceDecision: nettoyerPtr(get(r, "referenceDecision")),
			NomBeneficiaire:   beneficiaire,
			SiretBeneficiaire: nettoyerPtr(get(r, "idBeneficiaire")),
			Objet:             nettoyerPtr(get(r, "objet")),
			MontantEUR:        montant,
			Nature:            nettoyerPtr(get(r, "nature")),
		})
	}
	if lignes == nil {
		return nil, fmt.Errorf("aucune ligne lue")
	}
	return lignes, nil
}

func chargerEtInserer(ctx context.Context, pool *pgxpool.Pool, agence string, lignes []ligneAideEau, srcID int64) error {
	if len(lignes) == 0 {
		return fmt.Errorf("%s : aucune ligne à charger", agence)
	}
	programmes := map[string]bool{}
	rows := make([][]any, 0, len(lignes))
	for _, l := range lignes {
		programmes[l.Programme] = true
		rows = append(rows, []any{
			agence, l.Programme, l.Annee, l.DateDecision, l.ReferenceDecision,
			l.NomBeneficiaire, l.SiretBeneficiaire, l.CodeDepartement, l.CodeInseeCommune,
			l.Objet, l.MontantEUR, l.Nature, srcID,
		})
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for prog := range programmes {
		if _, err := tx.Exec(ctx, `DELETE FROM core.aide_agence_eau WHERE agence = $1 AND programme = $2`, agence, prog); err != nil {
			return err
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "aide_agence_eau"},
		[]string{"agence", "programme", "annee", "date_decision", "reference_decision",
			"nom_beneficiaire", "siret_beneficiaire", "code_departement", "code_insee_commune",
			"objet", "montant_eur", "nature", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("%s : %w", agence, err)
	}
	return tx.Commit(ctx)
}

// IngestAidesLoireBretagne charge les décisions d'aide des 11e et 12e
// programmes de l'agence de l'eau Loire-Bretagne.
func IngestAidesLoireBretagne(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAidesLoireBretagne)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionAides)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f11, err := arch.Fetch(ctx, srcID, runID, urlAidesLB11P, ".xlsx")
	if err != nil {
		return fail(err)
	}
	lignes11, err := chargerFeuilleLB(f11.Path, "11e",
		"Année d'engagement", "Dépt", "N° SIRET du bénéficiaire", "Raison sociale bénéficiaire",
		"Descriptif du dossier", "Aide en €", "Type de financement", "N° de décision", "Date de décision")
	if err != nil {
		return fail(fmt.Errorf("11e programme : %w", err))
	}

	f12, err := arch.Fetch(ctx, srcID, runID, urlAidesLB12P, ".xlsx")
	if err != nil {
		return fail(err)
	}
	lignes12, err := chargerFeuilleLB(f12.Path, "12e",
		"Année", "Dépt", "SIRET", "Maître d'ouvrage",
		"Libellé Aide", "Montant aide (€)", "Nature aide", "N° décision", "Date de la décision")
	if err != nil {
		return fail(fmt.Errorf("12e programme : %w", err))
	}

	if err := chargerEtInserer(ctx, pool, "LOIRE_BRETAGNE", append(lignes11, lignes12...), srcID); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"aides_11e": len(lignes11), "aides_12e": len(lignes12)}, "")
	fmt.Printf("  Aides Loire-Bretagne : %d (11e programme) + %d (12e programme)\n", len(lignes11), len(lignes12))
	return nil
}

// IngestAidesArtoisPicardie charge les conventions de subvention publiées
// par l'agence de l'eau Artois-Picardie au format décret n° 2017-779.
func IngestAidesArtoisPicardie(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAidesArtoisPicardie)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionAides)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f1011, err := arch.Fetch(ctx, srcID, runID, urlAidesAP1011, ".csv")
	if err != nil {
		return fail(err)
	}
	lignes1011, err := chargerCSVArtoisPicardie(f1011.Path, "10e-11e")
	if err != nil {
		return fail(fmt.Errorf("10e-11e programme : %w", err))
	}

	f12, err := arch.Fetch(ctx, srcID, runID, urlAidesAP12, ".csv")
	if err != nil {
		return fail(err)
	}
	lignes12, err := chargerCSVArtoisPicardie(f12.Path, "12e")
	if err != nil {
		return fail(fmt.Errorf("12e programme : %w", err))
	}

	if err := chargerEtInserer(ctx, pool, "ARTOIS_PICARDIE", append(lignes1011, lignes12...), srcID); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"aides_10e_11e": len(lignes1011), "aides_12e": len(lignes12)}, "")
	fmt.Printf("  Aides Artois-Picardie : %d (10e-11e programme) + %d (12e programme)\n", len(lignes1011), len(lignes12))
	return nil
}
