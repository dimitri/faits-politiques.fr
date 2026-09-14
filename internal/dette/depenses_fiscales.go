package dette

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les dépenses fiscales — les « niches » : un impôt qu'on ne prélève pas, au
// lieu d'une dépense qu'on verserait. Deux sources ouvertes, et deux seulement,
// les chiffrent mesure par mesure :
//
//   - l'annexe Voies et moyens, tome II, des PLF 2020 à 2023, publiée en
//     classeurs sur data.economie.gouv.fr (pièces jointes des jeux de la
//     Direction du budget) : la seule qui déclare aussi la NATURE du
//     bénéficiaire (entreprises, ménages) ;
//   - le « budget vert » des PLF 2024, 2025 et 2026, qui reprend chaque
//     dépense fiscale avec son chiffrage pour trois années.
//
// Les classeurs des PLF 2024 à 2026 ne sont publiés que sur une page de
// budget.gouv.fr derrière une protection anti-robot (Incapsula), qu'on ne
// contourne pas.
var SourceVoiesEtMoyens = archive.Source{
	Slug: "voies-et-moyens-t2", Label: "PLF 2020 à 2023 — Évaluation des voies et moyens, tome II (dépenses fiscales)",
	Publisher: "Direction du budget", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : annexes aux PLF 2020 à 2023, Évaluation des voies et moyens, tome II",
	Cadence:     "annuelle (octobre) ; quatre millésimes en données ouvertes",
	Notes: "Chiffrages en millions d'euros : exécution N-2, prévisions N-1 et N. « ε » = moins " +
		"de 0,5 M€, « nc » = non chiffré, « - » = sans objet : jamais des zéros. Des dizaines de " +
		"mesures non chiffrées chaque année : les totaux sont des minorants. Nature du bénéficiaire " +
		"déclarée par l'administration, pas vérifiée. Le classeur du PLF 2020 n'a pas de ligne " +
		"d'années : elles sont déduites du millésime.",
}

var SourceBudgetVert = archive.Source{
	Slug: "budget-vert-depenses-fiscales", Label: "Budget vert (PLF 2024 à 2026) — chiffrage des dépenses fiscales",
	Publisher: "Direction du budget", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : rapport sur l'impact environnemental du budget de l'État (budget vert), PLF 2024, 2025 et 2026",
	Cadence:     "annuelle (octobre)",
	Notes: "Une dépense fiscale peut occuper plusieurs lignes, une par cotation environnementale, " +
		"avec une QUOTE-PART du montant sur chacune : on additionne les lignes, on ne dédoublonne " +
		"pas (2024 : 483 lignes, 457 mesures, 89,4 Md€). Montant absent sans mention : ligne non " +
		"chargée, faute de pouvoir distinguer un « ε » d'un « nc ».",
}

const odsEconomie = "https://data.economie.gouv.fr/api/explore/v2.1/catalog/datasets/"

// Les classeurs, un par millésime : des pièces jointes aux jeux de la Direction
// du budget, dont les noms ne suivent aucune règle.
var classeursVMT = []struct {
	millesime int
	url       string
}{
	{2020, odsEconomie + "projet-de-loi-de-finances-pour-2020-plf-2020-donnees-du-plf-et-des-annexes-proje/attachments/plf_2020_liste_des_depenses_fiscales_xlsx"},
	{2021, odsEconomie + "projet-de-loi-de-finances-pour-2021-plf-2021-donnees-du-plf-et-des-annexes-proje/attachments/plf_2021_liste_des_depenses_fiscales_xlsx"},
	{2022, odsEconomie + "projet-de-loi-de-finances-pour-2022-plf-2022-donnees-du-plf-et-des-annexes-proje/attachments/plf_2022_liste_depenses_fiscales_xlsx"},
	{2023, "https://data.economie.gouv.fr/api/v2/catalog/datasets/plf2023_voies_et_moyens_t2_liste_des_depenses_fiscales/attachments/plf2023_voies_et_moyens_t2_liste_des_depenses_fiscales_xlsx"},
}

type ligneDF struct {
	millesime, annee int
	numero, libelle  string
	impot, stade     string
	montant          *float64
	mention          string
	document         int64
}

// Les trois budgets verts : les noms de colonnes changent d'un millésime à
// l'autre ; chacun est décrit explicitement.
var budgetsVerts = []struct {
	millesime int
	jeu       string
	colonnes  [3]string // exécution N-2, prévision N-1, prévision N
	mentionN  string    // colonne des mentions, pour la seule année N
}{
	{2024, "budgetvert_plf2024_opendata_vf", [3]string{"execution_2022_cp", "lfi_2023_cp_ou_prevision_2023_si_depense_fiscale", "plf_2024_cp_ou_prevision_2024_si_depense_fiscale"}, ""},
	{2025, "plf25-budget-vert", [3]string{"execution_2023_cp", "lfi_2024_cp_ou_prevision_2024_si_depense_fiscale", "plf_2025_cp_ou_prevision_2025_si_depense_fiscale"}, ""},
	{2026, "plf-2026-budget-vert", [3]string{"execution_2024_cp", "lfi_2025_cp_ou_prevision_2025_si_depense_fiscale", "plf_2026_cp_ou_prevision_2026_si_depense_fiscale"},
		"depenses_fiscales_e_montant_inferieur_a_0_5_meur_nc_montant_non_calcule_en_prevision_2026"},
}

func IngestDepensesFiscales(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := chargerDF(ctx, pool, arch, SourceVoiesEtMoyens, lireVoiesEtMoyens); err != nil {
		return err
	}
	return chargerDF(ctx, pool, arch, SourceBudgetVert, lireBudgetsVerts)
}

type lecteurDF func(ctx context.Context, arch *archive.Archive, srcID, runID int64) ([]ligneDF, [][]any, error)

// chargerDF remplace toutes les lignes d'une source. Les bénéficiaires, quand
// la source en publie, sont remplacés dans la même transaction.
func chargerDF(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, src archive.Source, lire lecteurDF) error {
	srcID, err := arch.EnsureSource(ctx, src)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		err = fmt.Errorf("%s : %w", src.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	lignes, beneficiaires, err := lire(ctx, arch, srcID, runID)
	if err != nil {
		return fail(err)
	}
	if len(lignes) == 0 {
		return fail(fmt.Errorf("aucune dépense fiscale lue"))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.depense_fiscale WHERE source_id = $1`, srcID); err != nil {
		return fail(err)
	}
	rows := make([][]any, 0, len(lignes))
	for _, l := range lignes {
		var m any
		if l.montant != nil {
			m = *l.montant
		}
		rows = append(rows, []any{l.millesime, l.numero, l.libelle, nul(l.impot), l.annee, l.stade, m, nul(l.mention), srcID, l.document})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "depense_fiscale"},
		[]string{"millesime", "numero", "libelle", "impot", "annee", "stade", "montant_eur", "mention", "source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if beneficiaires != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM ref.depense_fiscale_beneficiaire WHERE source_id = $1`, srcID); err != nil {
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"ref", "depense_fiscale_beneficiaire"},
			[]string{"numero", "nature", "nombre", "millesime", "source_id", "document_id"},
			pgx.CopyFromRows(beneficiaires)); err != nil {
			return fail(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"chiffrages": len(rows), "beneficiaires": len(beneficiaires)}, "")
	fmt.Printf("  %-28s %6d chiffrages  %4d bénéficiaires\n", src.Slug, len(rows), len(beneficiaires))
	return nil
}

// numeroDF normalise un numéro de dépense fiscale : les cellules numériques
// d'un classeur arrivent parfois en « 40101.0 ».
func numeroDF(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".0")
	return s
}

func lireVoiesEtMoyens(ctx context.Context, arch *archive.Archive, srcID, runID int64) ([]ligneDF, [][]any, error) {
	var lignes []ligneDF
	var bens [][]any
	for _, c := range classeursVMT {
		l, b, err := lireClasseurVMT(ctx, arch, srcID, runID, c.millesime, c.url)
		if err != nil {
			return nil, nil, fmt.Errorf("PLF %d : %w", c.millesime, err)
		}
		lignes = append(lignes, l...)
		bens = append(bens, b...)
	}
	return lignes, bens, nil
}

// mentionVMT normalise une cellule de chiffrage : « ε », « nc », « - » sont
// des mentions ; une cellule vide ou réduite à une espace n'est rien.
func mentionVMT(v string) string {
	return strings.TrimSpace(strings.ReplaceAll(v, "\u00a0", " "))
}

func lireClasseurVMT(ctx context.Context, arch *archive.Archive, srcID, runID int64, millesime int, u string) ([]ligneDF, [][]any, error) {
	f, err := arch.Fetch(ctx, srcID, runID, u, ".xlsx")
	if err != nil {
		return nil, nil, err
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return nil, nil, err
	}
	defer x.Close()

	chiffrages, err := x.rows("Chiffrages")
	if err != nil {
		return nil, nil, err
	}
	if len(chiffrages) == 0 || !strings.Contains(chiffrages[0]["A"], "millions") {
		return nil, nil, fmt.Errorf("unité des chiffrages introuvable")
	}
	// L'en-tête porte le stade (« Réalisation », « Prévision ») ; la ligne
	// suivante porte les années, sauf dans le classeur du PLF 2020 où elles
	// manquent : ce sont alors N-2, N-1 et N, par construction de l'annexe.
	iStade := -1
	for i, r := range chiffrages {
		if r["F"] == "Réalisation" {
			iStade = i
			break
		}
	}
	if iStade < 0 || iStade+1 >= len(chiffrages) {
		return nil, nil, fmt.Errorf("en-tête des chiffrages introuvable")
	}
	type col struct {
		lettre, stade string
		annee         int
	}
	var cols []col
	debut := iStade + 1
	for i, c := range []string{"F", "G", "H"} {
		a, err := strconv.Atoi(chiffrages[iStade+1][c])
		if err != nil {
			a = millesime - 2 + i
		} else {
			debut = iStade + 2
		}
		st := "PREVISION"
		if chiffrages[iStade][c] == "Réalisation" {
			st = "EXECUTION"
		}
		cols = append(cols, col{c, st, a})
	}
	if cols[0].annee != millesime-2 || cols[2].annee != millesime {
		return nil, nil, fmt.Errorf("années %d-%d inattendues pour le PLF %d", cols[0].annee, cols[2].annee, millesime)
	}
	var lignes []ligneDF
	vus := map[string]bool{}
	for _, r := range chiffrages[debut:] {
		num := numeroDF(r["D"])
		if !reNumeroDF.MatchString(num) {
			continue
		}
		if vus[num] {
			return nil, nil, fmt.Errorf("dépense fiscale %s en double dans les chiffrages", num)
		}
		vus[num] = true
		for _, c := range cols {
			l := ligneDF{millesime: millesime, annee: c.annee, numero: num, libelle: r["E"], impot: r["A"],
				stade: c.stade, document: f.DocumentID}
			switch v := mentionVMT(r[c.lettre]); v {
			case "":
				continue // cellule vide : rien de déclaré
			case "ε", "nc", "-":
				l.mention = v
			default:
				m, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
				if err != nil {
					return nil, nil, fmt.Errorf("%s/%d : montant illisible %q", num, c.annee, v)
				}
				m *= 1e6
				l.montant = &m
			}
			lignes = append(lignes, l)
		}
	}

	benef, err := x.rows("Bénéficiaires")
	if err != nil {
		return nil, nil, err
	}
	// Les classeurs écrivent « Ménages » ou « Menages » selon l'année.
	natures := map[string]string{
		"entreprises": "ENTREPRISES", "menages": "MENAGES", "entreprises et menages": "ENTREPRISES_ET_MENAGES",
		"locaux": "LOCAUX", "parcelles": "PARCELLES",
	}
	var bens [][]any
	vusB := map[string]bool{}
	for _, r := range benef {
		num := numeroDF(r["D"])
		if !vus[num] || vusB[num] {
			continue // en-têtes, mesure absente des chiffrages, ou doublon
		}
		brut := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(r["F"], "\u00a0", " ")), " "))
		brut = strings.ReplaceAll(brut, "é", "e")
		if brut == "" {
			continue
		}
		nat, ok := natures[brut]
		if !ok {
			return nil, nil, fmt.Errorf("%s : nature de bénéficiaire inattendue %q", num, r["F"])
		}
		vusB[num] = true
		var nombre any
		if n, err := strconv.ParseFloat(r["G"], 64); err == nil {
			nombre = int64(n)
		}
		bens = append(bens, []any{num, nat, nombre, millesime, srcID, f.DocumentID})
	}
	return lignes, bens, nil
}

var reNumeroDF = regexp.MustCompile(`^[0-9]{5,6}$`)

func lireBudgetsVerts(ctx context.Context, arch *archive.Archive, srcID, runID int64) ([]ligneDF, [][]any, error) {
	var lignes []ligneDF
	for _, bv := range budgetsVerts {
		q := url.Values{"where": {`type_depense="Dépenses fiscales"`}, "order_by": {"code_depense"}}
		u := odsEconomie + bv.jeu + "/exports/json?" + q.Encode()
		f, err := arch.Fetch(ctx, srcID, runID, u, ".json")
		if err != nil {
			return nil, nil, err
		}
		var recs []map[string]any
		if err := lireJSON(f.Path, &recs); err != nil {
			return nil, nil, fmt.Errorf("%s : %w", bv.jeu, err)
		}
		if len(recs) == 0 {
			return nil, nil, fmt.Errorf("%s : aucune dépense fiscale", bv.jeu)
		}
		type cumul struct {
			libelle, impot string
			somme          [3]float64
			renseigne      [3]bool
			mentionN       string
		}
		parNumero := map[string]*cumul{}
		var ordre []string
		for _, r := range recs {
			num := numeroDF(fmt.Sprint(r["code_depense"]))
			c, ok := parNumero[num]
			if !ok {
				lib, _ := r["libelle"].(string)
				imp, _ := r["impot_si_depense_fiscale"].(string)
				c = &cumul{libelle: lib, impot: imp}
				parNumero[num] = c
				ordre = append(ordre, num)
			}
			for i, colonne := range bv.colonnes {
				if _, existe := r[colonne]; !existe {
					return nil, nil, fmt.Errorf("%s : colonne %s absente", bv.jeu, colonne)
				}
				if v, ok := r[colonne].(float64); ok {
					c.somme[i] += v
					c.renseigne[i] = true
				}
			}
			if bv.mentionN != "" {
				if m, ok := r[bv.mentionN].(string); ok && m != "" {
					c.mentionN = strings.TrimSpace(m)
				}
			}
		}
		for _, num := range ordre {
			c := parNumero[num]
			for i := 0; i < 3; i++ {
				l := ligneDF{millesime: bv.millesime, annee: bv.millesime - 2 + i, numero: num,
					libelle: c.libelle, impot: c.impot, stade: "PREVISION", document: f.DocumentID}
				if i == 0 {
					l.stade = "EXECUTION"
				}
				switch {
				case c.renseigne[i]:
					m := c.somme[i]
					l.montant = &m
				case i == 2 && (c.mentionN == "ε" || c.mentionN == "nc"):
					l.mention = c.mentionN
				default:
					continue
				}
				lignes = append(lignes, l)
			}
		}
	}
	return lignes, nil, nil
}
