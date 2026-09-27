package international

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le commerce extérieur de l'UE ne se résume pas à la somme des exportations
// de ses membres (NE.EXP.GNFS.CD de la Banque mondiale, déjà chargé) : cette
// somme compte le commerce ENTRE pays de l'Union (une vente française en
// Allemagne) comme une "exportation de l'UE", ce qu'aucune définition
// usuelle du commerce extérieur d'un bloc ne fait — d'où un ratio
// exportations/PIB de 50 % pour l'UE dans les données déjà chargées, contre
// 11 % pour les États-Unis et 20 % pour la Chine : pas un vrai écart de
// performance commerciale, un artefact de calcul. Eurostat publie
// séparément le commerce EXTRA-UE (avec le reste du monde seulement),
// la seule mesure comparable à l'export total des États-Unis ou de la
// Chine — pour les BIENS uniquement (pas les services, à la différence de
// NE.EXP.GNFS.CD), une différence de champ disclosed dans le dossier.
var SourceCommerceExtraUE = archive.Source{
	Slug: "eurostat-commerce-extra-ue", Label: "Eurostat — commerce extra-UE (biens), exportations et importations",
	Publisher: "Eurostat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Eurostat, ext_lt_maineu",
	Cadence:     "annuelle",
	Notes: "Commerce de BIENS uniquement (SITC, total), UE27 avec le reste du monde (partenaire " +
		"EXT_EU27_2020) — exclut le commerce interne à l'Union et les services, à la différence de " +
		"NE.EXP.GNFS.CD (Banque mondiale, biens et services, déjà chargé pour les autres pays de " +
		"comparaison). Millions d'euros, pas de dollars — jamais à convertir sans le dire.",
}

const urlCommerceExtraUE = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/ext_lt_maineu?" +
	"format=JSON&geo=EU27_2020&partner=EXT_EU27_2020&sitc06=TOTAL"

// IngestCommerceExtraUE charge la série 2002-2025 des exportations et
// importations de biens de l'UE27 avec le reste du monde (hors commerce
// intra-UE).
func IngestCommerceExtraUE(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCommerceExtraUE)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "commerce-extra-ue-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlCommerceExtraUE, ".json")
	if err != nil {
		return fail(err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return fail(err)
	}
	var doc struct {
		Error     []struct{ Label string } `json:"error"`
		Value     map[string]float64       `json:"value"`
		Size      []int                    `json:"size"`
		Dimension struct {
			IndicEt struct {
				Category struct {
					Index map[string]int `json:"index"`
				} `json:"category"`
			} `json:"indic_et"`
			Time struct {
				Category struct {
					Index map[string]int `json:"index"`
				} `json:"category"`
			} `json:"time"`
		} `json:"dimension"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fail(fmt.Errorf("JSON-stat illisible : %w", err))
	}
	if len(doc.Error) > 0 {
		return fail(fmt.Errorf("Eurostat : %s", doc.Error[0].Label))
	}
	indicIdx := doc.Dimension.IndicEt.Category.Index
	timeIdx := doc.Dimension.Time.Category.Index
	nTemps := len(timeIdx)
	iExp, okExp := indicIdx["MIO_EXP_VAL"]
	iImp, okImp := indicIdx["MIO_IMP_VAL"]
	if !okExp || !okImp || nTemps == 0 {
		return fail(fmt.Errorf("dimensions attendues absentes"))
	}

	codes := map[int]string{iExp: "EU_EXTRA_EXPORT_MEUR", iImp: "EU_EXTRA_IMPORT_MEUR"}
	var rows [][]any
	for _, indicI := range []int{iExp, iImp} {
		ind := codes[indicI]
		for anneeStr, ti := range timeIdx {
			v, ok := doc.Value[strconv.Itoa(indicI*nTemps+ti)]
			if !ok {
				continue
			}
			annee, err := strconv.Atoi(anneeStr)
			if err != nil {
				continue
			}
			rows = append(rows, []any{"EU", "Union européenne", ind, annee, v, srcID})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune valeur décodée"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : core.indicateur_mondial est partagée par
	// quatre connecteurs de ce dossier (pib_epargne_nette.go, sante_ocde.go,
	// sipri.go, celui-ci), chacun sur ses propres codes d'indicateur ; une vue
	// scopée sur les deux codes de CE connecteur, plutôt que la table
	// réelle, pour que le MERGE (notamment son NOT MATCHED BY SOURCE) ne
	// touche jamais les lignes des trois autres.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_indicateur_mondial_eu_extra (
			pays_code text, pays_label text, indicateur text, annee smallint, valeur numeric, source_id bigint
		) ON COMMIT DROP;
		CREATE OR REPLACE TEMPORARY VIEW indicateur_mondial_eu_extra AS
		  SELECT * FROM core.indicateur_mondial
		  WHERE indicateur = ANY('{EU_EXTRA_EXPORT_MEUR,EU_EXTRA_IMPORT_MEUR}')
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_indicateur_mondial_eu_extra"},
		[]string{"pays_code", "pays_label", "indicateur", "annee", "valeur", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("core.indicateur_mondial : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO indicateur_mondial_eu_extra AS tgt
		USING tmp_indicateur_mondial_eu_extra AS src
		ON tgt.pays_code = src.pays_code AND tgt.indicateur = src.indicateur AND tgt.annee = src.annee
		WHEN MATCHED AND (tgt.pays_label, tgt.valeur, tgt.source_id)
		                  IS DISTINCT FROM (src.pays_label, src.valeur, src.source_id) THEN
		    UPDATE SET pays_label = src.pays_label, valeur = src.valeur, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (pays_code, pays_label, indicateur, annee, valeur, source_id)
		    VALUES (src.pays_code, src.pays_label, src.indicateur, src.annee, src.valeur, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion core.indicateur_mondial : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows), "touchees": touchees}, "")
	fmt.Printf("  Commerce extra-UE (Eurostat) : %d lignes touchées\n", touchees)
	return nil
}
