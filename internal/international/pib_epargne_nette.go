package international

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

// Le PIB compte l'extraction de ressources naturelles comme une production,
// sans jamais retrancher l'épuisement du stock : deux pays qui vendent le
// même pétrole, l'un en le remplaçant par une industrie, l'autre en épuisant
// un gisement fini, affichent la même croissance. L'épargne nette ajustée de
// la Banque mondiale corrige précisément cet angle mort — voir
// docs/international-donnees.md § 4.
var SourcePIBEpargneNette = archive.Source{
	Slug: "banque-mondiale-pib-epargne-nette", Label: "Banque mondiale — PIB, commerce extérieur et structure sectorielle des économies",
	Publisher: "Banque mondiale", Tier: "PRIMARY_OFFICIAL",
	Licence: "Creative Commons Attribution 4.0 (CC BY 4.0)", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Banque mondiale, World Development Indicators",
	Cadence:     "annuelle",
	Notes: "Dix pays de comparaison (G8 historique, Chine, Arabie saoudite) plus l'Union " +
		"européenne (agrégat Banque mondiale, code EU/EUU) pour comparer la zone à monnaie " +
		"unique aux États-Unis et à la Chine. NY.ADJ.SVNG.GN.ZS (épargne nette ajustée) et " +
		"NY.ADJ.DRES.GN.ZS (épuisement des ressources naturelles) sont des pourcentages du " +
		"revenu national brut ; NV.AGR/IND/SRV.TOTL.ZS (valeur ajoutée par secteur primaire, " +
		"secondaire, tertiaire) sont des pourcentages du PIB — aucun n'est un montant, jamais " +
		"à comparer en valeur absolue au PIB (NY.GDP.MKTP.CD, en dollars courants). Les États-Unis " +
		"n'ont pas de valeur récente pour la ventilation sectorielle (dernière année disponible : " +
		"2021, pas 2023 ou 2024 comme les autres) — chaque comparaison cite son année exacte, " +
		"jamais une même année supposée pour tous.",
}

// Le G8 historique (avant l'exclusion de la Russie en 2014), plus la Chine
// (première économie par le PIB en parité de pouvoir d'achat, absente du G8),
// l'Arabie saoudite (économie dont l'épuisement pétrolier illustre le mieux
// ce que le PIB ne mesure pas) et l'Union européenne (la seule zone à
// monnaie unique de cette liste, comparée comme bloc à la Chine et aux
// États-Unis).
var paysComparaisonMondiale = []string{"FR", "US", "JP", "DE", "GB", "IT", "CA", "RU", "CN", "SA", "EU"}

var indicateursMondiaux = []string{
	"NY.GDP.MKTP.CD", "NY.ADJ.SVNG.GN.ZS", "NY.ADJ.DRES.GN.ZS",
	"NE.EXP.GNFS.CD", "NE.IMP.GNFS.CD",
	"NV.AGR.TOTL.ZS", "NV.IND.TOTL.ZS", "NV.SRV.TOTL.ZS",
}

const worldBankBase = "https://api.worldbank.org/v2/country/"

func IngestPIBEpargneNette(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePIBEpargneNette)
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

	pays := ""
	for i, p := range paysComparaisonMondiale {
		if i > 0 {
			pays += ";"
		}
		pays += p
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	// MERGE plutôt que DELETE+COPY, sur une vue scopée aux indicateurs de CE
	// connecteur : core.indicateur_mondial est partagée avec SIPRI, l'OCDE
	// (espérance de vie, dépense de santé) et le commerce extra-UE, chacun sur
	// ses propres codes — la vue garantit que le MERGE (notamment son NOT
	// MATCHED BY SOURCE) ne touche jamais les lignes des trois autres
	// connecteurs, et l'ancien DELETE payait par ailleurs le prix des
	// triggers RI pour tout son périmètre à chaque republication, changement
	// ou non.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_indicateur_mondial_wb (
			pays_code text, pays_label text, indicateur text, annee smallint, valeur numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	// Une vue ne peut pas être paramétrée par un $1 : la liste des indicateurs
	// (une constante Go fixe, pas une entrée utilisateur) est donc inlinée en
	// littéral de tableau SQL.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE TEMPORARY VIEW indicateur_mondial_wb AS
		  SELECT * FROM core.indicateur_mondial WHERE indicateur IN ('%s')
		  WITH LOCAL CHECK OPTION`, strings.Join(indicateursMondiaux, "','"))); err != nil {
		return fail(err)
	}

	var total int64
	for _, ind := range indicateursMondiaux {
		url := worldBankBase + pays + "/indicator/" + ind + "?format=json&per_page=1000&date=2000:2025"
		f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
		if err != nil {
			return fail(err)
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return fail(err)
		}
		var page []json.RawMessage
		if err := json.Unmarshal(b, &page); err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		if len(page) != 2 {
			return fail(fmt.Errorf("%s : réponse Banque mondiale inattendue", ind))
		}
		var meta struct {
			Pages int `json:"pages"`
		}
		if err := json.Unmarshal(page[0], &meta); err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		if meta.Pages > 1 {
			return fail(fmt.Errorf("%s : %d pages — la fenêtre 2000-2024 dépasse per_page=1000, à agrandir plutôt qu'à tronquer en silence", ind, meta.Pages))
		}
		var lignes []ligneWB
		if err := json.Unmarshal(page[1], &lignes); err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		var rows [][]any
		for _, l := range lignes {
			if l.Value == nil {
				continue
			}
			annee, err := strconv.Atoi(l.Date)
			if err != nil {
				return fail(fmt.Errorf("%s : année %q : %w", ind, l.Date, err))
			}
			rows = append(rows, []any{l.Country.ID, l.Country.Value, ind, annee, *l.Value, srcID})
		}
		if len(rows) == 0 {
			return fail(fmt.Errorf("%s : aucune valeur", ind))
		}
		n, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_indicateur_mondial_wb"},
			[]string{"pays_code", "pays_label", "indicateur", "annee", "valeur", "source_id"},
			pgx.CopyFromRows(rows))
		if err != nil {
			return fail(fmt.Errorf("%s : %w", ind, err))
		}
		total += n
	}

	ct, err := tx.Exec(ctx, `
		MERGE INTO indicateur_mondial_wb AS tgt
		USING tmp_indicateur_mondial_wb AS src
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
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": total, "touchees": touchees}, "")
	fmt.Printf("  PIB et épargne nette ajustée (Banque mondiale) : %d lignes touchées, %d pays, %d indicateurs\n",
		touchees, len(paysComparaisonMondiale), len(indicateursMondiaux))
	return nil
}

type ligneWB struct {
	Country struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"country"`
	Date  string   `json:"date"`
	Value *float64 `json:"value"`
}
