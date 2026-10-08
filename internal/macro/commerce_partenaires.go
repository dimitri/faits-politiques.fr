package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceComtradeFrance = archive.Source{
	Slug: "un-comtrade-france-partenaires", Label: "UN Comtrade — importations françaises par partenaire commercial",
	Publisher: "Division statistique des Nations unies (UN Comtrade)", Tier: "PRIMARY_OFFICIAL",
	License: "UN Comtrade — réutilisation libre avec attribution", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : UN Comtrade, reporterCode 251 (France)",
	Cadence:     "ponctuelle",
	Notes: "Miroir onusien des déclarations douanières nationales, PAS les douanes françaises elles-mêmes " +
		"(Le Kiosque, DGDDI, n'a pas d'API et demande un connecteur de formulaire séparé, non fait ici). " +
		"Valeurs en DOLLARS COURANTS (convention Comtrade), jamais à comparer directement à un montant en " +
		"euros. Tous les partenaires disponibles sont chargés pour chaque code HS et chaque année, sans " +
		"sélection préalable de pays : le classement (plus gros partenaires, plus forte progression) se fait " +
		"à l'affichage, sur la donnée complète, pas au chargement.",
}

// tradeSectors : un secteur peut regrouper plusieurs codes HS (le
// textile-habillement additionne bonneterie et habillement classique) —
// chaque code est chargé et stocké séparément, sommé seulement à
// l'affichage (internal/sitegen), pour ne jamais masquer la composition.
var tradeSectors = []struct {
	Sector  string
	HSCodes []string
}{
	{"automobile", []string{"8703"}},
	{"textile-habillement", []string{"61", "62"}},
	{"electronique-tv", []string{"8528"}},
}

// tradeYears : 2013 (avant l'essentiel du mouvement observé vers
// l'Europe de l'Est et le Maghreb) et la dernière année complète disponible
// dans Comtrade au moment du chargement — à vérifier et bumper à la main
// lors d'une prochaine mise à jour, pas recalculé automatiquement.
var tradeYears = []int{2013, 2024}

type comtradeRow struct {
	PartnerCode int     `json:"partnerCode"`
	PartnerDesc string  `json:"partnerDesc"`
	PrimaryVal  float64 `json:"primaryValue"`
}

type comtradeResponse struct {
	Count int           `json:"count"`
	Data  []comtradeRow `json:"data"`
	Error string        `json:"error"`
}

// IngestTradePartners charge, pour chaque secteur et chaque année
// retenue, les importations françaises par partenaire commercial (UN
// Comtrade, HS6/H6, flux M = importations).
func IngestTradePartners(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceComtradeFrance)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "commerce-partenaires-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	var allRows [][]any
	counts := map[string]int{}
	first := true
	for _, sector := range tradeSectors {
		for _, code := range sector.HSCodes {
			for _, year := range tradeYears {
				if !first {
					time.Sleep(3 * time.Second) // service public gratuit, sans clé : ne pas le solliciter trop vite
				}
				first = false

				url := fmt.Sprintf("https://comtradeapi.un.org/public/v1/preview/C/A/HS?"+
					"reporterCode=251&period=%d&flowCode=M&cmdCode=%s&motCode=0&partner2Code=0&includeDesc=true",
					year, code)
				f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
				if err != nil {
					return fail(fmt.Errorf("%s %d : %w", code, year, err))
				}
				raw, err := os.ReadFile(f.Path)
				if err != nil {
					return fail(err)
				}
				var resp comtradeResponse
				if err := json.Unmarshal(raw, &resp); err != nil {
					return fail(fmt.Errorf("%s %d : réponse illisible : %w", code, year, err))
				}
				if resp.Error != "" {
					return fail(fmt.Errorf("%s %d : Comtrade : %s", code, year, resp.Error))
				}
				if resp.Count == 0 {
					return fail(fmt.Errorf("%s %d : aucune ligne — code HS ou année invalide", code, year))
				}
				for _, row := range resp.Data {
					if row.PartnerDesc == "" {
						return fail(fmt.Errorf("%s %d : partenaire %d sans nom", code, year, row.PartnerCode))
					}
					allRows = append(allRows, []any{sector.Sector, code, year, row.PartnerCode, row.PartnerDesc, row.PrimaryVal, srcID})
				}
				counts[fmt.Sprintf("%s-%d", code, year)] = resp.Count
			}
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité des secteurs et années à chaque rechargement,
	// changement ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_commerce_partenaire_secteur (
			secteur text, code_hs text, annee int, code_partenaire int,
			nom_partenaire text, valeur_usd numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_commerce_partenaire_secteur"},
		[]string{"secteur", "code_hs", "annee", "code_partenaire", "nom_partenaire", "valeur_usd", "source_id"},
		pgx.CopyFromRows(allRows)); err != nil {
		return fail(fmt.Errorf("core.commerce_partenaire_secteur : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.commerce_partenaire_secteur AS tgt
		USING tmp_commerce_partenaire_secteur AS src
		ON tgt.code_hs = src.code_hs AND tgt.annee = src.annee AND tgt.code_partenaire = src.code_partenaire
		WHEN MATCHED AND (tgt.secteur, tgt.nom_partenaire, tgt.valeur_usd, tgt.source_id)
		                  IS DISTINCT FROM (src.secteur, src.nom_partenaire, src.valeur_usd, src.source_id) THEN
		    UPDATE SET secteur = src.secteur, nom_partenaire = src.nom_partenaire,
		               valeur_usd = src.valeur_usd, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (secteur, code_hs, annee, code_partenaire, nom_partenaire, valeur_usd, source_id)
		    VALUES (src.secteur, src.code_hs, src.annee, src.code_partenaire, src.nom_partenaire,
		            src.valeur_usd, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion : %w", err))
	}
	touchees := ct.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"lignes": len(allRows), "requetes": counts, "touchees": touchees}, "")
	fmt.Printf("  Commerce par partenaire (UN Comtrade) : %d lignes (%d touchées par la fusion)\n",
		len(allRows), touchees)
	return nil
}
