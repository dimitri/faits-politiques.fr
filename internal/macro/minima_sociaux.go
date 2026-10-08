package macro

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les minima sociaux, dispositif par dispositif, depuis 1990 — dont la
// continuité RMI (jusqu'en 2009) → RSA. Voir docs/chomage-donnees.md et
// docs/retraite-donnees.md (pour la ligne ASV/ASPA), et le commentaire de
// db/migrations/0072_minima_sociaux.sql.
var SourceSocialMinima = archive.Source{
	Slug: "drees-minima-sociaux-dispositif", Label: "Drees — minima sociaux par dispositif",
	Publisher: "Direction de la recherche, des études, de l'évaluation et des statistiques",
	Tier:      "PRIMARY_OFFICIAL",
	License:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Drees, à partir de Cnaf, MSA, Cnav, France Travail, FSV, Ofii",
	Cadence:     "annuelle",
	Notes: "Champ France métropolitaine pour les effectifs, France pour les dépenses " +
		"(la Drees ne publie pas les dépenses séparément par champ). Plusieurs ruptures " +
		"de série documentées par la source elle-même (voir le fichier source) : la plus " +
		"visible est la bascule RMI → RSA au 1er juin 2009.",
}

const (
	minimaEffectifURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
		"336_minima-sociaux-rsa-et-prime-d-activite/attachments/minima_sociaux_donnees_nationales_par_dispositif_xlsx"
	minimaDepenseURL = "https://data.drees.solidarites-sante.gouv.fr/api/explore/v2.1/catalog/datasets/" +
		"336_minima-sociaux-rsa-et-prime-d-activite/attachments/minima_sociaux_donnees_de_depenses_par_dispositif_xlsx"
)

// schemePrefix : l'ordre compte — il conditionne le premier préfixe
// reconnu, et le classeur des dépenses accole le numéro de sa note de bas de
// page directement au libellé ("RSA1,2", "ASS1") sans espace, d'où des
// correspondances par PRÉFIXE plutôt que par égalité stricte.
var schemePrefix = []struct{ prefix, code, label string }{
	{"Revenu de solidarité active (RSA)", "RSA", "Revenu de solidarité active"},
	{"RSA", "RSA", "Revenu de solidarité active"},
	{"Revenu minimum d'insertion (RMI)", "RMI", "Revenu minimum d'insertion"},
	{"Allocation de parent isolé (API)", "API", "Allocation de parent isolé"},
	{"Allocation aux adultes handicapés (AAH)", "AAH", "Allocation aux adultes handicapés"},
	{"AAH", "AAH", "Allocation aux adultes handicapés"},
	{"Allocation supplémentaire d'invalidité (ASI)", "ASI", "Allocation supplémentaire d'invalidité"},
	{"ASI", "ASI", "Allocation supplémentaire d'invalidité"},
	{"Allocation de solidarité spécifique (ASS)", "ASS", "Allocation de solidarité spécifique"},
	{"ASS", "ASS", "Allocation de solidarité spécifique"},
	{"Allocation pour demandeur d'asile (ADA)", "ADA", "Allocation pour demandeur d'asile"},
	{"ADA", "ADA", "Allocation pour demandeur d'asile"},
	{"Allocation d'insertion (AI) ou Allocation temporaire d'attente (ATA)", "AI_ATA",
		"Allocation d'insertion ou allocation temporaire d'attente"},
	{"ATA", "ATA", "Allocation temporaire d'attente"},
	{"Allocation supplémentaire vieillesse (ASV) et allocation de solidarité aux personnes âgées (ASPA)",
		"ASV_ASPA", "Minimum vieillesse (ASV et ASPA)"},
	{"Minimum vieillesse (ASV et ASPA)", "ASV_ASPA", "Minimum vieillesse (ASV et ASPA)"},
	{"Allocation veuvage (AV)", "AV", "Allocation veuvage"},
	{"AV", "AV", "Allocation veuvage"},
	{"Allocation équivalent retraite - remplacement (AER-R)", "AER_ATS",
		"Allocation équivalent retraite ou allocation transitoire de solidarité, remplacement"},
	{"AER-R/ATS", "AER_ATS", "Allocation équivalent retraite ou allocation transitoire de solidarité, remplacement"},
	{"Allocation des travailleurs indépendants (ATI)", "ATI", "Allocation des travailleurs indépendants"},
	{"ATI", "ATI", "Allocation des travailleurs indépendants"},
	{"RSO", "RSO", "Revenu de solidarité (outre-mer)"},
	{"Ensemble", "ENSEMBLE", "Ensemble des minima sociaux"},
}

var reYear = regexp.MustCompile(`^(\d{4})`)

func schemeCode(label string) (code, properLabel string, ok bool) {
	for _, d := range schemePrefix {
		if strings.HasPrefix(label, d.prefix) {
			return d.code, d.label, true
		}
	}
	return "", "", false
}

// IngestSocialMinima charge les effectifs (1990-2024, France métropolitaine)
// et les dépenses (2009-2024, France, euros constants 2024) par dispositif.
func IngestSocialMinima(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSocialMinima)
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

	nHeadcount, err := loadMinimaHeadcount(ctx, pool, arch, srcID, runID)
	if err != nil {
		return fail(fmt.Errorf("effectifs : %w", err))
	}
	nSpending, err := loadMinimaSpending(ctx, pool, arch, srcID, runID)
	if err != nil {
		return fail(fmt.Errorf("dépenses : %w", err))
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"touchees_effectif": nHeadcount, "touchees_depense": nSpending}, "")
	fmt.Printf("  minima sociaux : %d lignes d'effectifs touchées, %d lignes de dépenses touchées\n", nHeadcount, nSpending)
	return nil
}

func loadMinimaHeadcount(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, srcID, runID int64) (int, error) {
	f, err := arch.Fetch(ctx, srcID, runID, minimaEffectifURL, ".xlsx")
	if err != nil {
		return 0, err
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return 0, err
	}
	defer x.Close()
	sheetRows, err := x.rows("Tableau 1")
	if err != nil {
		return 0, err
	}

	// La ligne d'en-tête porte un LIBELLÉ d'année par colonne, parfois affublé
	// d'une note de bas de page ("2009 (7)") ou dupliqué (une bascule
	// méthodologique publie l'ancienne ET la nouvelle série sous la même
	// année). On garde la DERNIÈRE colonne rencontrée pour une année donnée :
	// c'est celle qui suit la colonne d'origine dans la feuille, donc la
	// version révisée par convention Drees.
	yearCols, err := yearColumns(sheetRows)
	if err != nil {
		return 0, err
	}

	// clé (code, année) -> effectif : une map, pas une slice, pour que la
	// colonne la plus à droite (donc la dernière traitée, `yearCols` étant
	// trié dans l'ordre du tableur) écrase silencieusement une colonne
	// dupliquée plus ancienne pour la même année.
	values := map[[2]any]int{}
	var labelOf = map[string]string{}
	for _, row := range sheetRows {
		label, ok := row["B"]
		if !ok {
			continue
		}
		code, properLabel, ok := schemeCode(label)
		if !ok {
			continue
		}
		labelOf[code] = properLabel
		for _, yc := range yearCols {
			v, ok := row[yc.col]
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			// "52 000 (9)" : un espace insécable sépare parfois les milliers
			// dans les valeurs annotées d'une note ; on ne garde que les chiffres.
			v = strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, strings.SplitN(v, "(", 2)[0])
			if v == "" {
				continue
			}
			count, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			values[[2]any{code, yc.year}] = count
		}
		if code == "ENSEMBLE" {
			break // tout ce qui suit est note de bas de page, pas donnée.
		}
	}
	if len(values) == 0 {
		return 0, fmt.Errorf("aucune ligne reconnue")
	}
	var rows [][]any
	for k, v := range values {
		code := k[0].(string)
		rows = append(rows, []any{code, labelOf[code], k[1], v, srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_minima_sociaux_effectif (
			dispositif_code text, dispositif_libelle text, annee int, effectif int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_minima_sociaux_effectif"},
		[]string{"dispositif_code", "dispositif_libelle", "annee", "effectif", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return 0, err
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité des dispositifs et millésimes à chaque republication.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.minima_sociaux_effectif AS tgt
		USING tmp_minima_sociaux_effectif AS src
		ON tgt.dispositif_code = src.dispositif_code AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.dispositif_libelle, tgt.effectif, tgt.source_id)
		                  IS DISTINCT FROM (src.dispositif_libelle, src.effectif, src.source_id) THEN
		    UPDATE SET dispositif_libelle = src.dispositif_libelle, effectif = src.effectif,
		               source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (dispositif_code, dispositif_libelle, annee, effectif, source_id)
		    VALUES (src.dispositif_code, src.dispositif_libelle, src.annee, src.effectif, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return 0, err
	}
	return int(ct.RowsAffected()), tx.Commit(ctx)
}

func loadMinimaSpending(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, srcID, runID int64) (int, error) {
	f, err := arch.Fetch(ctx, srcID, runID, minimaDepenseURL, ".xlsx")
	if err != nil {
		return 0, err
	}
	x, err := openXLSX(f.Path)
	if err != nil {
		return 0, err
	}
	defer x.Close()
	sheetRows, err := x.rows("Tableau 1")
	if err != nil {
		return 0, err
	}
	yearCols, err := yearColumns(sheetRows)
	if err != nil {
		return 0, err
	}

	values := map[[2]any]float64{}
	var labelOf = map[string]string{}
	for _, row := range sheetRows {
		label, ok := row["B"]
		if !ok {
			continue
		}
		code, properLabel, ok := schemeCode(label)
		if !ok {
			continue
		}
		labelOf[code] = properLabel
		for _, yc := range yearCols {
			v, ok := row[yc.col]
			if !ok {
				continue
			}
			amount, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				continue
			}
			values[[2]any{code, yc.year}] = amount
		}
		if code == "ENSEMBLE" {
			break
		}
	}
	if len(values) == 0 {
		return 0, fmt.Errorf("aucune ligne reconnue")
	}
	var rows [][]any
	for k, v := range values {
		code := k[0].(string)
		rows = append(rows, []any{code, labelOf[code], k[1], v, srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_minima_sociaux_depense (
			dispositif_code text, dispositif_libelle text, annee int,
			depense_meur_reel2024 numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_minima_sociaux_depense"},
		[]string{"dispositif_code", "dispositif_libelle", "annee", "depense_meur_reel2024", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return 0, err
	}
	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table, et l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité des dispositifs et millésimes à chaque republication.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.minima_sociaux_depense AS tgt
		USING tmp_minima_sociaux_depense AS src
		ON tgt.dispositif_code = src.dispositif_code AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.dispositif_libelle, tgt.depense_meur_reel2024, tgt.source_id)
		                  IS DISTINCT FROM (src.dispositif_libelle, src.depense_meur_reel2024, src.source_id) THEN
		    UPDATE SET dispositif_libelle = src.dispositif_libelle,
		               depense_meur_reel2024 = src.depense_meur_reel2024, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (dispositif_code, dispositif_libelle, annee, depense_meur_reel2024, source_id)
		    VALUES (src.dispositif_code, src.dispositif_libelle, src.annee, src.depense_meur_reel2024, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return 0, err
	}
	return int(ct.RowsAffected()), tx.Commit(ctx)
}

// yearCol : une colonne de tableur et l'année que porte son en-tête.
type yearCol struct {
	col  string
	year int
}

// yearColumns repère la ligne d'en-tête (celle où au moins dix cellules
// commencent par quatre chiffres) et renvoie les colonnes triées dans l'ORDRE
// RÉEL DU TABLEUR (A, B, ... Z, AA, AB, ...) — indispensable ici : quand une
// année est dupliquée (bascule méthodologique, ex. « 2009 (7) » après
// « 2009 »), la colonne qui doit l'emporter est celle de DROITE, la plus
// récente selon la convention Drees. Un simple parcours de map Go ne le
// garantirait pas, l'ordre d'itération n'étant pas spécifié par le langage.
func yearColumns(rows []map[string]string) ([]yearCol, error) {
	for _, row := range rows {
		var candidates []yearCol
		for col, v := range row {
			m := reYear.FindStringSubmatch(strings.TrimSpace(v))
			if m == nil {
				continue
			}
			year, _ := strconv.Atoi(m[1])
			if year < 1980 || year > 2100 {
				continue
			}
			candidates = append(candidates, yearCol{col, year})
		}
		if len(candidates) >= 10 {
			sort.Slice(candidates, func(i, j int) bool {
				ci, cj := candidates[i].col, candidates[j].col
				if len(ci) != len(cj) {
					return len(ci) < len(cj)
				}
				return ci < cj
			})
			return candidates, nil
		}
	}
	return nil, fmt.Errorf("ligne d'en-tête des années introuvable")
}
