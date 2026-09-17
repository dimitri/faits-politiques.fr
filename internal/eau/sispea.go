// Package eau charge les données de l'observatoire SISPEA (services publics
// d'eau et d'assainissement) — voir docs/bassins-versants-donnees.md.
package eau

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

const ConnectorVersion = "eau-v1"

// SourceSISPEA : l'observatoire national des services publics d'eau et
// d'assainissement (OFB/eaufrance.fr). L'API Hub'Eau qui aurait pu servir
// cette donnée a été décommissionnée le 10 septembre 2026 ; le remplacement
// est un export en masse par compétence et par année, livré sous une
// extension .xls trompeuse — le fichier récupéré ici est en réalité une
// archive 7-zip (mêmes symptômes que la SAE, internal/sante/sae.go : ce
// dépôt sort du Go pur pour la décompression, jamais pour la lecture du
// tableur lui-même une fois extrait).
var SourceSISPEA = archive.Source{
	Slug: "sispea-eau-potable", Label: "SISPEA — services publics d'eau potable",
	Publisher: "Observatoire des services publics d'eau et d'assainissement (OFB, eaufrance.fr)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : SISPEA, Office français de la biodiversité (eaufrance.fr)",
	Cadence:     "annuelle",
	Notes: "Millésime 2023 seulement : l'export 2024 disponible au moment du chargement est un " +
		"binaire .xls hérité (OLE2/CFBF) que la bibliothèque Excel de ce dépôt ne sait pas lire — " +
		"à revoir séparément (conversion ou bibliothèque dédiée), pas une raison de deviner les " +
		"chiffres pour cette année. Un service peut couvrir plusieurs communes ; ce chargement " +
		"reste au niveau du service (pas de carte communale pour l'instant, faute d'avoir chargé " +
		"la « composition communale des services », un second export distinct). Piège de colonne " +
		"réel, trouvé avant chargement plutôt que deviné : la colonne « p101_1 » n'est PAS le prix " +
		"de l'eau (c'est le taux de conformité microbiologique, 0 à 100) — le prix réel est " +
		"« d102_0 », vérifié contre le Panorama Sispea 2020 (annexe 1, définitions des " +
		"indicateurs) et contre des valeurs plausibles en €/m³ (1 à 4 environ), pas contre le seul " +
		"nom de colonne.",
}

const sispeaURLPotable = "https://www.data.gouv.fr/api/1/datasets/r/180d7556-7243-44dd-90dc-5490363cd792"

func nettoyer(s string) *string {
	if s == "" || s == "." {
		return nil
	}
	return &s
}

func modeGestionCode(brut string) (*string, error) {
	switch brut {
	case "", ".", "Inconnu":
		return nil, nil
	case "Regie":
		v := "REGIE"
		return &v, nil
	case "Delegation":
		v := "DELEGATION"
		return &v, nil
	default:
		return nil, fmt.Errorf("mode_gestion inattendu : %q — le format SISPEA a peut-être changé", brut)
	}
}

func entierNullable(s string) (*int, error) {
	if s == "" || s == "." {
		return nil, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil, fmt.Errorf("valeur entière illisible %q : %w", s, err)
	}
	return &v, nil
}

func flottantNullable(s string) (*float64, error) {
	if s == "" || s == "." {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("valeur décimale illisible %q : %w", s, err)
	}
	return &v, nil
}

// IngestSISPEAEauPotable charge le millésime 2023 des services d'eau potable
// (compétence AEP) : un service par ligne, avec son mode de gestion, son
// opérateur, la population desservie et le prix réel du service.
func IngestSISPEAEauPotable(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSISPEA)
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

	if _, err := exec.LookPath("7z"); err != nil {
		return fail(fmt.Errorf("binaire 7z introuvable sur le PATH : %w", err))
	}

	f, err := arch.Fetch(ctx, srcID, runID, sispeaURLPotable, ".xls")
	if err != nil {
		return fail(err)
	}

	tmp, err := os.MkdirTemp("", "sispea-aep-*")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(tmp)

	cmd := exec.CommandContext(ctx, "7z", "e", f.Path, "-o"+tmp, "-y", "-r", "*.xlsx")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fail(fmt.Errorf("extraction 7z : %w : %s", err, out))
	}
	matches, err := filepath.Glob(filepath.Join(tmp, "*.xlsx"))
	if err != nil {
		return fail(err)
	}
	if len(matches) != 1 {
		return fail(fmt.Errorf("attendu un seul .xlsx extrait, trouvé %d — le format de l'archive a peut-être changé", len(matches)))
	}

	wb, err := excelize.OpenFile(matches[0])
	if err != nil {
		return fail(fmt.Errorf("classeur SISPEA illisible : %w", err))
	}
	defer wb.Close()

	sheets := wb.GetSheetList()
	if len(sheets) == 0 {
		return fail(fmt.Errorf("classeur SISPEA sans feuille"))
	}
	rows, err := wb.GetRows(sheets[0])
	if err != nil {
		return fail(err)
	}
	if len(rows) < 2 {
		return fail(fmt.Errorf("classeur SISPEA : moins de 2 lignes"))
	}
	header := rows[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	requis := []string{"id_sispea_serv", "Nom_serv", "dpt", "mode_gestion", "statut_operateur",
		"nom_operateur", "d101_0", "d102_0", "agence_de_leau", "Bassin_concernE"}
	for _, col := range requis {
		if _, ok := idx[col]; !ok {
			return fail(fmt.Errorf("colonne %q absente du classeur SISPEA — le format a peut-être changé", col))
		}
	}
	get := func(r []string, col string) string {
		i := idx[col]
		if i < len(r) {
			return r[i]
		}
		return ""
	}

	var lignes [][]any
	for n, r := range rows[1:] {
		modeGestion, err := modeGestionCode(get(r, "mode_gestion"))
		if err != nil {
			return fail(fmt.Errorf("ligne %d : %w", n+2, err))
		}
		pop, err := entierNullable(get(r, "d101_0"))
		if err != nil {
			return fail(fmt.Errorf("ligne %d, population desservie : %w", n+2, err))
		}
		prix, err := flottantNullable(get(r, "d102_0"))
		if err != nil {
			return fail(fmt.Errorf("ligne %d, prix : %w", n+2, err))
		}
		lignes = append(lignes, []any{
			2023, get(r, "id_sispea_serv"), get(r, "Nom_serv"),
			nettoyer(get(r, "dpt")), modeGestion,
			nettoyer(get(r, "statut_operateur")), nettoyer(get(r, "nom_operateur")),
			pop, prix,
			nettoyer(get(r, "agence_de_leau")), nettoyer(get(r, "Bassin_concernE")),
			srcID,
		})
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("SISPEA eau potable : aucune ligne lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.service_eau_potable WHERE annee = 2023`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "service_eau_potable"},
		[]string{"annee", "id_sispea", "nom_service", "code_departement", "mode_gestion",
			"statut_operateur", "nom_operateur", "population_desservie", "prix_eur_m3",
			"agence_de_leau", "bassin_code", "source_id"},
		pgx.CopyFromRows(lignes)); err != nil {
		return fail(fmt.Errorf("service_eau_potable : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"services": len(lignes)}, "")
	fmt.Printf("  SISPEA eau potable 2023 : %d services\n", len(lignes))
	return nil
}
