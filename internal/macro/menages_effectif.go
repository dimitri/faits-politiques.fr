package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le nombre de ménages par grand type, recensement de la population, via
// l'API Melodi de l'Insee — pour pondérer core.menage_type_drees, qui ne porte
// que des montants moyens. Voir docs/revenu-universel-microsimulation.md.
var SourceMenagesEffectif = archive.Source{
	Slug: "insee-rp-menages-familles", Label: "Insee — recensement, ménages et familles détaillés",
	Publisher: "INSEE", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, recensement de la population 2023, diffusion Melodi",
	Cadence:     "annuelle",
	Notes: "Univers du recensement (tous les ménages), pas celui de l'enquête ERFS " +
		"utilisée par core.menage_type_drees (qui exclut les revenus déclarés " +
		"négatifs et les ménages étudiants) : les deux ne coïncident pas exactement.",
}

const (
	melodiFamillesURL = "https://api.insee.fr/melodi/data/DS_RP_TD_FAMILLE_NBENF_COMP" +
		"?GEO=FRANCE-FM&PCS=_T&NATIONALITY_TYPE=_T&CIVIL_STATUS=_T&RP_MEASURE=NBFAM&maxResult=10000"
	melodiMenagesURL = "https://api.insee.fr/melodi/data/DS_RP_TD_MENAGES_TPH_COMP" +
		"?GEO=FRANCE-FM&AGE=_T&RP_MEASURE=DWELLINGS&maxResult=10000"
)

type melodiReponse struct {
	Observations []struct {
		Dimensions map[string]string `json:"dimensions"`
		Measures   struct {
			OBSVALUENIVEAU struct {
				Value float64 `json:"value"`
			} `json:"OBS_VALUE_NIVEAU"`
		} `json:"measures"`
	} `json:"observations"`
}

func IngestMenagesEffectif(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMenagesEffectif)
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

	familles, err := melodiLire(ctx, arch, srcID, runID, melodiFamillesURL)
	if err != nil {
		return fail(fmt.Errorf("familles par nombre d'enfants : %w", err))
	}
	menages, err := melodiLire(ctx, arch, srcID, runID, melodiMenagesURL)
	if err != nil {
		return fail(fmt.Errorf("ménages selon le type : %w", err))
	}

	// TFN (type de famille) x NCH (nombre d'enfants de moins de 25 ans) ->
	// notre type_menage. Un père seul et une mère seule sont regroupés : la
	// simulation ne distingue pas le sexe du parent. Les couples SANS enfant
	// résident (TFN 21) et AVEC enfants (221 : du couple seulement, 222 :
	// mixte) sont trois branches disjointes d'un même total (TFN 2) — voir
	// data/geo-projections... non, voir directement le codes-list Melodi
	// (DS_RP_TD_FAMILLE_NBENF_COMP, dimension TFN).
	nb := map[string]float64{}
	for _, o := range familles.Observations {
		tfn, nch, annee := o.Dimensions["TFN"], o.Dimensions["NCH"], o.Dimensions["TIME_PERIOD"]
		if annee != "2023" {
			continue
		}
		v := o.Measures.OBSVALUENIVEAU.Value
		switch {
		case (tfn == "11" || tfn == "12") && nch == "CH1_Y_LT25":
			nb["monoparentale_1_enfant"] += v
		case (tfn == "11" || tfn == "12") && (nch == "CH2_Y_LT25" || nch == "CH3_Y_LT25" || nch == "CH_GE4_Y_LT25"):
			nb["monoparentale_2p_enfants"] += v
		case tfn == "21" && nch == "CH0_Y_LT25":
			nb["couple_sans_enfant"] += v
		case (tfn == "221" || tfn == "222") && nch == "CH1_Y_LT25":
			nb["couple_1_enfant"] += v
		case (tfn == "221" || tfn == "222") && nch == "CH2_Y_LT25":
			nb["couple_2_enfants"] += v
		case (tfn == "221" || tfn == "222") && nch == "CH3_Y_LT25":
			nb["couple_3_enfants"] += v
		case (tfn == "221" || tfn == "222") && nch == "CH_GE4_Y_LT25":
			nb["couple_4p_enfants"] += v
		}
	}
	for _, o := range menages.Observations {
		tph, annee, ocs := o.Dimensions["TPH"], o.Dimensions["TIME_PERIOD"], o.Dimensions["OCS"]
		if annee != "2023" || ocs != "DW_MAIN" {
			continue
		}
		v := o.Measures.OBSVALUENIVEAU.Value
		switch tph {
		case "11":
			nb["personne_seule"] += v
		case "12":
			nb["complexe_sans_enfant"] += v
		}
	}

	if len(nb) < 9 {
		return fail(fmt.Errorf("effectifs incomplets : %d types sur 9 attendus", len(nb)))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	const annee = 2023
	if _, err := tx.Exec(ctx, `DELETE FROM core.menage_type_effectif WHERE annee = $1`, annee); err != nil {
		return fail(err)
	}
	var rows [][]any
	for typ, v := range nb {
		rows = append(rows, []any{typ, annee, int64(v), srcID})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "menage_type_effectif"},
		[]string{"type_menage", "annee", "nb_menages", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"types_charges": len(rows)}, "")
	fmt.Printf("  effectifs de ménages par type : %d types (Insee RP 2023)\n", len(rows))
	return nil
}

func melodiLire(ctx context.Context, arch *archive.Archive, srcID, runID int64, url string) (*melodiReponse, error) {
	f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	var r melodiReponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("réponse Melodi illisible : %w", err)
	}
	if len(r.Observations) == 0 {
		return nil, fmt.Errorf("réponse Melodi vide (%s)", url)
	}
	return &r, nil
}
