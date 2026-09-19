package eau

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersionBudgetAnnexe = "eau-budget-annexe-v1"

// SourceBudgetAnnexeEau : l'eau et l'assainissement sont un budget ANNEXE
// (nomenclature M49/M49A), que core.commune_indicator (internal/communes/
// ofgl.go, budget PRINCIPAL uniquement) ne couvre pas — un manque documenté
// comme structurel au § 9 du dossier eau, jusqu'à vérification directe de
// l'API OFGL qui publie en réalité les budgets annexes séparément, par
// nomenclature.
var SourceBudgetAnnexeEau = archive.Source{
	Slug: "ofgl-budget-annexe-eau", Label: "OFGL — budgets annexes M49 (eau et assainissement), communes et EPCI",
	Publisher: "Observatoire des finances et de la gestion publique locales",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : OFGL (Observatoire des finances et de la gestion publique locales), " +
		"d'après les comptes de gestion de la DGFiP",
	Cadence: "annuelle",
	Notes: "Seuls deux agrégats sont chargés (dépenses totales, recettes totales) sur les " +
		"41 que l'API publie. M49 (présentation normale) et M49A (présentation abrégée, " +
		"petites collectivités) sont la même nomenclature comptable, chargées ensemble. " +
		"Le libellé du budget (nom_budget, ex. « EAU-AUTRECHE », « SPANC CCRAPC ») laisse " +
		"deviner eau ou assainissement pour beaucoup de lignes, mais c'est un texte libre " +
		"de la collectivité, jamais recatégorisé en un champ structuré ici. Une commune " +
		"ou un EPCI qui délègue entièrement le service à un opérateur privé peut ne pas " +
		"avoir de budget annexe M49, ou en avoir un réduit à la seule part publique : " +
		"cette table ne capture pas le chiffre d'affaires de l'opérateur. 334 lignes sur " +
		"195 556 (0,17 %) ont un montant négatif — vérifié à l'inspection : des corrections " +
		"comptables réelles (reprise, remboursement), jamais une anomalie de lecture aussi " +
		"massive que le format 2024 SISPEA, donc laissées telles quelles plutôt qu'exclues.",
}

const ofglBudgetAnnexeDataset = "https://data.ofgl.fr/api/explore/v2.1/catalog/datasets/"

func urlBudgetAnnexe(dataset, selectCols string) string {
	where := `type_de_budget="Budget annexe" AND (nomen="M49" OR nomen="M49A") ` +
		`AND (agregat="Dépenses totales" OR agregat="Recettes totales")`
	return ofglBudgetAnnexeDataset + dataset + "/exports/csv?delimiter=%3B&timezone=UTC&limit=-1&select=" +
		selectCols + "&where=" + url.QueryEscape(where)
}

var urlBudgetAnnexeCommunes = urlBudgetAnnexe("ofgl-base-communes", "com_code,com_name,dep_code,nomen,agregat,exer,montant,lbudg")
var urlBudgetAnnexeGFP = urlBudgetAnnexe("ofgl-base-gfp", "epci_code,epci_name,nomen,agregat,exer,montant,lbudg")

func agregatBudgetAnnexe(s string) (string, bool) {
	switch s {
	case "Dépenses totales":
		return "DEPENSES_TOTALES", true
	case "Recettes totales":
		return "RECETTES_TOTALES", true
	}
	return "", false
}

func lireCSVBudgetAnnexe(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("%s : fichier vide", path)
	}
	head := make([]string, len(recs[0]))
	for i, h := range recs[0] {
		head[i] = strings.TrimSpace(strings.TrimPrefix(h, string(rune(0xFEFF))))
	}
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

type ligneBudgetAnnexe struct {
	TypeCollectivite string
	Code             string
	NomCollectivite  string
	NomBudget        string
	Nomenclature     string
	Annee            int
	Agregat          string
	MontantEUR       float64
}

func chargerBudgetAnnexe(path, typeCollectivite, colCode, colNom string) ([]ligneBudgetAnnexe, error) {
	recs, err := lireCSVBudgetAnnexe(path)
	if err != nil {
		return nil, err
	}
	var lignes []ligneBudgetAnnexe
	for n, r := range recs {
		code := r[colCode]
		nomenclature := r["nomen"]
		if code == "" || (nomenclature != "M49" && nomenclature != "M49A") {
			continue
		}
		agregat, ok := agregatBudgetAnnexe(r["agregat"])
		if !ok {
			continue
		}
		annee, err := strconv.Atoi(r["exer"])
		if err != nil {
			return nil, fmt.Errorf("ligne %d, année illisible %q : %w", n+2, r["exer"], err)
		}
		montant, err := strconv.ParseFloat(r["montant"], 64)
		if err != nil {
			return nil, fmt.Errorf("ligne %d, montant illisible %q : %w", n+2, r["montant"], err)
		}
		lignes = append(lignes, ligneBudgetAnnexe{
			TypeCollectivite: typeCollectivite,
			Code:             code,
			NomCollectivite:  r[colNom],
			NomBudget:        r["lbudg"],
			Nomenclature:     nomenclature,
			Annee:            annee,
			Agregat:          agregat,
			MontantEUR:       montant,
		})
	}
	if lignes == nil {
		return nil, fmt.Errorf("aucune ligne lue")
	}
	return lignes, nil
}

// IngestBudgetAnnexeEau charge les budgets annexes M49/M49A (eau et
// assainissement), communes et EPCI, 2018-2025 — voir
// docs/bassins-versants-donnees.md § 4bis.
func IngestBudgetAnnexeEau(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceBudgetAnnexeEau)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionBudgetAnnexe)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	fCom, err := arch.Fetch(ctx, srcID, runID, urlBudgetAnnexeCommunes, ".csv")
	if err != nil {
		return fail(fmt.Errorf("communes : %w", err))
	}
	lignesCom, err := chargerBudgetAnnexe(fCom.Path, "COMMUNE", "com_code", "com_name")
	if err != nil {
		return fail(fmt.Errorf("communes : %w", err))
	}

	fGFP, err := arch.Fetch(ctx, srcID, runID, urlBudgetAnnexeGFP, ".csv")
	if err != nil {
		return fail(fmt.Errorf("EPCI : %w", err))
	}
	lignesGFP, err := chargerBudgetAnnexe(fGFP.Path, "EPCI", "epci_code", "epci_name")
	if err != nil {
		return fail(fmt.Errorf("EPCI : %w", err))
	}

	toutes := append(lignesCom, lignesGFP...)
	rows := make([][]any, 0, len(toutes))
	for _, l := range toutes {
		rows = append(rows, []any{
			l.TypeCollectivite, l.Code, l.NomCollectivite, l.NomBudget,
			l.Nomenclature, l.Annee, l.Agregat, l.MontantEUR, srcID,
		})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.budget_annexe_eau`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "budget_annexe_eau"},
		[]string{"type_collectivite", "code", "nom_collectivite", "nom_budget",
			"nomenclature", "annee", "agregat", "montant_eur", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("budget_annexe_eau : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"communes": len(lignesCom), "epci": len(lignesGFP)}, "")
	fmt.Printf("  budget annexe eau (M49/M49A) : %d lignes communes, %d lignes EPCI\n", len(lignesCom), len(lignesGFP))
	return nil
}
