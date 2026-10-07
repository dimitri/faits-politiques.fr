package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les 93 postes de recettes fiscales et sociales publiés par Eurostat pour la
// France, de 1995 à aujourd'hui.
//
// Pourquoi une table dédiée plutôt que 93 entrées dans ref.macro_serie : cette
// dernière est éditoriale — chaque série y porte une définition écrite à la
// main, qui explique ce qu'elle mesure et ce qu'elle ne mesure pas. Quatre-
// vingt-treize postes ne sont pas quatre-vingt-treize décisions éditoriales,
// c'est une NOMENCLATURE. Elle se charge telle que la source la publie, avec
// ses libellés, et le travail éditorial porte sur la façon de la lire.
const fiscalQuery = "gov_10a_taxag?format=JSON&lang=FR&geo=FR&sector=S13&unit=MIO_EUR"

const fiscalSector = "S13"

func IngestTaxRevenue(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEurostat)
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

	f, err := arch.Fetch(ctx, srcID, runID, eurostatBase+fiscalQuery, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}

	var doc struct {
		Value     map[string]float64 `json:"value"`
		Dimension map[string]struct {
			Category struct {
				Index map[string]int    `json:"index"`
				Label map[string]string `json:"label"`
			} `json:"category"`
		} `json:"dimension"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fail(fmt.Errorf("réponse Eurostat illisible : %w", err))
	}
	items := doc.Dimension["na_item"].Category
	timeDim := doc.Dimension["time"].Category
	if len(items.Index) == 0 || len(timeDim.Index) == 0 {
		return fail(fmt.Errorf("dimensions na_item ou time absentes"))
	}

	// La réponse est un tableau aplati : l'indice d'une valeur encode la
	// position dans le produit cartésien des dimensions. Ici deux dimensions
	// varient seulement, poste et année, dans cet ordre.
	itemByIndex := make([]string, len(items.Index))
	for code, i := range items.Index {
		itemByIndex[i] = code
	}
	yearByIndex := make([]string, len(timeDim.Index))
	for y, i := range timeDim.Index {
		yearByIndex[i] = y
	}
	yearCount := len(yearByIndex)

	refRows := make([][]any, 0, len(items.Index))
	for code := range items.Index {
		label := items.Label[code]
		if label == "" {
			label = code
		}
		// Un underscore signale un code composite : D2_D5_D91 n'est pas un
		// impôt mais une addition déjà faite. Le drapeau évite qu'on l'ajoute
		// une seconde fois à ses propres composantes.
		refRows = append(refRows, []any{code, label, strings.Contains(code, "_"), itemDepth(code)})
	}

	var valRows [][]any
	var rejectedKey, rejectedItem int
	for k, v := range doc.Value {
		i, err := strconv.Atoi(k)
		if err != nil || i < 0 || i >= len(itemByIndex)*yearCount {
			rejectedKey++
			continue
		}
		code := itemByIndex[i/yearCount]
		year, err := strconv.Atoi(yearByIndex[i%yearCount])
		if code == "" || err != nil {
			rejectedItem++
			continue
		}
		valRows = append(valRows, []any{code, fiscalSector, year, v, srcID})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// core.recette_fiscale est TRUNCATÉE (pas de DELETE+COPY, donc hors du
	// motif visé ici) : le TRUNCATE ne déclenche pas les triggers RI ligne
	// par ligne comme le ferait un DELETE, il n'y a donc rien à convertir.
	// Il doit précéder la fusion de ref.poste_fiscal ci-dessous, pour que la
	// FK recette_fiscale.poste -> poste_fiscal.code ne bloque jamais un
	// WHEN NOT MATCHED BY SOURCE THEN DELETE sur un poste disparu.
	if _, err := tx.Exec(ctx, `TRUNCATE core.recette_fiscale`); err != nil {
		return fail(err)
	}

	// MERGE plutôt que DELETE+COPY sur ref.poste_fiscal : l'ancien DELETE
	// (table entière, ce connecteur en est l'unique propriétaire) payait le
	// prix des triggers RI pour l'intégralité de la nomenclature à chaque
	// republication d'Eurostat, changement ou non — une nomenclature qui ne
	// bouge quasiment jamais.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_poste_fiscal (
			code text, libelle text, agregat boolean, profondeur smallint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_poste_fiscal"},
		[]string{"code", "libelle", "agregat", "profondeur"},
		pgx.CopyFromRows(refRows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO ref.poste_fiscal AS tgt
		USING tmp_poste_fiscal AS src
		ON tgt.code = src.code
		WHEN MATCHED AND (tgt.libelle, tgt.agregat, tgt.profondeur)
		                  IS DISTINCT FROM (src.libelle, src.agregat, src.profondeur) THEN
		    UPDATE SET libelle = src.libelle, agregat = src.agregat, profondeur = src.profondeur
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (code, libelle, agregat, profondeur)
		    VALUES (src.code, src.libelle, src.agregat, src.profondeur)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("fusion ref.poste_fiscal : %w", err))
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "recette_fiscale"},
		[]string{"poste", "secteur", "annee", "montant_meur", "source_id"},
		pgx.CopyFromRows(valRows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes_recues":       len(doc.Value),
		"lignes_chargees":     len(valRows),
		"rejet_cle_illisible": rejectedKey,
		"rejet_poste_inconnu": rejectedItem,
		"postes":              len(refRows),
	}, "")
	fmt.Printf("  recettes fiscales : %d postes, %d valeurs\n", len(refRows), len(valRows))
	return nil
}

// itemDepth : combien de niveaux le code descend dans la nomenclature.
// D2 vaut 1, D21 vaut 2, D211 vaut 3. Heuristique assumée — la nomenclature SEC
// a des exceptions — qui sert à choisir un niveau de lecture cohérent, jamais à
// reconstituer l'arbre.
func itemDepth(code string) int16 {
	if i := strings.IndexByte(code, '_'); i >= 0 {
		code = code[:i]
	}
	n := 0
	for _, r := range code {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return int16(n)
}
